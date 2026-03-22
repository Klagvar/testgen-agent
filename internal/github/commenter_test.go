package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFormatReport(t *testing.T) {
	report := Report{
		Files: []FileReport{
			{
				File:           "calc.go",
				Functions:      []string{"Add", "Multiply", "Divide"},
				TestsTotal:     12,
				TestsPassed:    10,
				TestsPruned:    2,
				DiffCoverage:   85.3,
				MutationScore:  90.0,
				MutationKilled: 9,
				MutationTotal:  10,
				Status:         "success",
			},
			{
				File:         "strutil.go",
				Functions:    []string{"Reverse", "Capitalize"},
				TestsTotal:   8,
				TestsPassed:  6,
				TestsPruned:  2,
				DiffCoverage: 72.1,
				Status:       "partial",
			},
		},
		TotalGenerated: 20,
		TotalValidated: 16,
		TotalCached:    3,
		TotalDiffCov:   78.7,
		CoverageTarget: 80.0,
		Model:          "qwen3-coder:30b",
		Duration:       135 * time.Second,
		BaseBranch:     "main",
	}

	md := FormatReportMarkdown(report)
	t.Logf("Report:\n%s", md)

	checks := []struct {
		name     string
		contains string
	}{
		{"bot marker", botMarker},
		{"title", "Testgen Agent Report"},
		{"status line", "Status:"},
		{"summary section", "Summary"},
		{"model", "qwen3-coder:30b"},
		{"base branch", "main"},
		{"files processed", "2"},
		{"total generated", "20"},
		{"total validated", "16"},
		{"cached", "3"},
		{"diff coverage", "78.7%"},
		{"coverage target", "80%"},
		{"duration", "2m15s"},
		{"file details section", "File Details"},
		{"calc.go row", "calc.go"},
		{"strutil.go row", "strutil.go"},
		{"success emoji", "✅"},
		{"partial emoji", "⚠️"},
		{"mutation section", "Mutation Testing Details"},
		{"mutation score", "90.0%"},
		{"mutation killed", "9"},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(md, check.contains) {
				t.Errorf("report should contain %q", check.contains)
			}
		})
	}
}

func TestFormatReport_NoValidated(t *testing.T) {
	report := Report{
		TotalGenerated: 5,
		TotalValidated: 0,
		Model:          "test-model",
		Duration:       10 * time.Second,
	}

	md := FormatReportMarkdown(report)

	if !strings.Contains(md, "No tests passed validation") {
		t.Error("should contain warning about no validated tests")
	}
	if !strings.Contains(md, "❌ No tests passed validation") {
		t.Error("should contain failure status")
	}
}

func TestFormatReport_NoGenerated(t *testing.T) {
	report := Report{
		TotalGenerated: 0,
		TotalValidated: 0,
		Model:          "test-model",
		Duration:       2 * time.Second,
	}

	md := FormatReportMarkdown(report)

	if !strings.Contains(md, "No tests generated") {
		t.Error("should contain 'No tests generated' status")
	}
}

func TestFormatReport_AllPassed(t *testing.T) {
	report := Report{
		Files: []FileReport{
			{File: "a.go", Functions: []string{"Foo"}, TestsTotal: 3, TestsPassed: 3, Status: "success", DiffCoverage: 95.0},
		},
		TotalGenerated: 3,
		TotalValidated: 3,
		Model:          "gpt-4o",
		Duration:       30 * time.Second,
	}

	md := FormatReportMarkdown(report)

	if !strings.Contains(md, "All tests passed") {
		t.Error("should contain 'All tests passed' status")
	}
}

func TestFormatReport_CollapsibleWhenManyFiles(t *testing.T) {
	files := make([]FileReport, 5)
	for i := range files {
		files[i] = FileReport{
			File: fmt.Sprintf("file%d.go", i), Functions: []string{"F"},
			TestsTotal: 1, TestsPassed: 1, Status: "success",
		}
	}

	report := Report{
		Files: files, TotalGenerated: 5, TotalValidated: 5,
		Model: "m", Duration: time.Second,
	}

	md := FormatReportMarkdown(report)

	if !strings.Contains(md, "<details>") {
		t.Error("should use collapsible details for >3 files")
	}
}

func TestFormatReport_NoCollapsibleFewFiles(t *testing.T) {
	report := Report{
		Files: []FileReport{
			{File: "a.go", Functions: []string{"A"}, TestsTotal: 1, TestsPassed: 1, Status: "success"},
		},
		TotalGenerated: 1, TotalValidated: 1,
		Model: "m", Duration: time.Second,
	}

	md := FormatReportMarkdown(report)

	if strings.Contains(md, "<details>\n<summary>Click to expand") {
		t.Error("should NOT use collapsible details for <=3 files")
	}
}

func TestFormatReport_NoMutationSection(t *testing.T) {
	report := Report{
		Files: []FileReport{
			{File: "a.go", Functions: []string{"A"}, TestsTotal: 1, TestsPassed: 1, Status: "success"},
		},
		TotalGenerated: 1, TotalValidated: 1,
		Model: "m", Duration: time.Second,
	}

	md := FormatReportMarkdown(report)

	if strings.Contains(md, "Mutation Testing Details") {
		t.Error("should not show mutation section when no mutation data")
	}
}

func TestPostComment(t *testing.T) {
	var receivedBody string
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")

		body, _ := io.ReadAll(r.Body)
		var req commentRequest
		json.Unmarshal(body, &req)
		receivedBody = req.Body

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 1}`))
	}))
	defer server.Close()

	c := &Commenter{
		token:   "test-token",
		owner:   "testowner",
		repo:    "testrepo",
		prNum:   42,
		apiBase: server.URL,
	}

	err := c.postComment("Hello from testgen-agent")
	if err != nil {
		t.Fatalf("postComment error: %v", err)
	}

	if receivedBody != "Hello from testgen-agent" {
		t.Errorf("body = %q, want 'Hello from testgen-agent'", receivedBody)
	}

	if receivedAuth != "Bearer test-token" {
		t.Errorf("auth = %q, want 'Bearer test-token'", receivedAuth)
	}
}

func TestPostComment_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer server.Close()

	c := &Commenter{
		token:   "bad-token",
		owner:   "testowner",
		repo:    "testrepo",
		prNum:   42,
		apiBase: server.URL,
	}

	err := c.postComment("test")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}

	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should contain 403, got: %v", err)
	}
}

func TestFindBotComment_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		comments := []commentResponse{
			{ID: 10, Body: "some other comment"},
			{ID: 42, Body: botMarker + "\n## 🤖 Testgen Agent Report\n..."},
			{ID: 99, Body: "another comment"},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(comments)
	}))
	defer server.Close()

	c := &Commenter{
		token: "tok", owner: "o", repo: "r", prNum: 1,
		apiBase: server.URL,
	}

	id, err := c.findBotComment()
	if err != nil {
		t.Fatalf("findBotComment error: %v", err)
	}
	if id != 42 {
		t.Errorf("expected comment ID 42, got %d", id)
	}
}

func TestFindBotComment_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		comments := []commentResponse{
			{ID: 10, Body: "some other comment"},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(comments)
	}))
	defer server.Close()

	c := &Commenter{
		token: "tok", owner: "o", repo: "r", prNum: 1,
		apiBase: server.URL,
	}

	id, err := c.findBotComment()
	if err != nil {
		t.Fatalf("findBotComment error: %v", err)
	}
	if id != 0 {
		t.Errorf("expected 0 (not found), got %d", id)
	}
}

func TestUpdateComment(t *testing.T) {
	var receivedMethod string
	var receivedPath string
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path

		body, _ := io.ReadAll(r.Body)
		var req commentRequest
		json.Unmarshal(body, &req)
		receivedBody = req.Body

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 42}`))
	}))
	defer server.Close()

	c := &Commenter{
		token: "tok", owner: "myowner", repo: "myrepo", prNum: 7,
		apiBase: server.URL,
	}

	err := c.updateComment(42, "updated report content")
	if err != nil {
		t.Fatalf("updateComment error: %v", err)
	}

	if receivedMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", receivedMethod)
	}
	if receivedPath != "/repos/myowner/myrepo/issues/comments/42" {
		t.Errorf("path = %q, want /repos/myowner/myrepo/issues/comments/42", receivedPath)
	}
	if receivedBody != "updated report content" {
		t.Errorf("body = %q, want 'updated report content'", receivedBody)
	}
}

func TestPostReport_CreatesNew(t *testing.T) {
	callOrder := []string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			callOrder = append(callOrder, "GET")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[]`))
			return
		}
		if r.Method == "POST" {
			callOrder = append(callOrder, "POST")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id": 1}`))
			return
		}
	}))
	defer server.Close()

	c := &Commenter{
		token: "tok", owner: "o", repo: "r", prNum: 1,
		apiBase: server.URL,
	}

	err := c.PostReport(Report{Model: "test", Duration: time.Second})
	if err != nil {
		t.Fatalf("PostReport error: %v", err)
	}

	if len(callOrder) != 2 || callOrder[0] != "GET" || callOrder[1] != "POST" {
		t.Errorf("expected [GET, POST], got %v", callOrder)
	}
}

func TestPostReport_UpdatesExisting(t *testing.T) {
	callOrder := []string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			callOrder = append(callOrder, "GET")
			comments := []commentResponse{
				{ID: 99, Body: botMarker + "\nold report"},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(comments)
			return
		}
		if r.Method == "PATCH" {
			callOrder = append(callOrder, "PATCH")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id": 99}`))
			return
		}
		t.Errorf("unexpected method: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	c := &Commenter{
		token: "tok", owner: "o", repo: "r", prNum: 1,
		apiBase: server.URL,
	}

	err := c.PostReport(Report{Model: "test", Duration: time.Second})
	if err != nil {
		t.Fatalf("PostReport error: %v", err)
	}

	if len(callOrder) != 2 || callOrder[0] != "GET" || callOrder[1] != "PATCH" {
		t.Errorf("expected [GET, PATCH], got %v", callOrder)
	}
}
