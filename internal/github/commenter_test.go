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

func TestFormatReport_CommitSHA(t *testing.T) {
	report := Report{
		TotalGenerated: 1, TotalValidated: 1,
		Model: "m", Duration: time.Second,
		CommitSHA:    "abc1234567890def",
		RepoFullName: "owner/repo",
	}

	md := FormatReportMarkdown(report)

	if !strings.Contains(md, "abc1234") {
		t.Error("should contain short SHA")
	}
	if !strings.Contains(md, "https://github.com/owner/repo/commit/abc1234567890def") {
		t.Error("should contain commit link")
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

func TestPostReport_CreatesNew(t *testing.T) {
	posted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posted = true
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id": 1}`))
			return
		}
		t.Errorf("unexpected method: %s", r.Method)
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

	if !posted {
		t.Error("expected POST request")
	}
}

func TestPostReport_AlwaysCreatesNew(t *testing.T) {
	calls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s (should not search for existing comments)", r.Method)
			return
		}
		calls++
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 1}`))
	}))
	defer server.Close()

	c := &Commenter{
		token: "tok", owner: "o", repo: "r", prNum: 1,
		apiBase: server.URL,
	}

	_ = c.PostReport(Report{Model: "run1", Duration: time.Second})
	_ = c.PostReport(Report{Model: "run2", Duration: time.Second})

	if calls != 2 {
		t.Errorf("expected 2 POST calls, got %d", calls)
	}
}
