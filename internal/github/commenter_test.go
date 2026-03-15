package github

import (
	"encoding/json"
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
				File:         "calc.go",
				Functions:    []string{"Add", "Multiply", "Divide"},
				TestsTotal:   12,
				TestsPassed:  10,
				TestsPruned:  2,
				DiffCoverage: 85.3,
				Status:       "success",
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
		TotalDiffCov:   78.7,
		Model:          "qwen3-coder:30b",
		Duration:       135 * time.Second,
	}

	md := FormatReportMarkdown(report)
	t.Logf("Report:\n%s", md)

	checks := []struct {
		name     string
		contains string
	}{
		{"title", "Testgen Agent Report"},
		{"table header", "| File |"},
		{"calc.go row", "calc.go"},
		{"strutil.go row", "strutil.go"},
		{"success emoji", "✅"},
		{"partial emoji", "⚠️"},
		{"model", "qwen3-coder:30b"},
		{"total generated", "Tests generated:** 20"},
		{"total validated", "Tests passed validation:** 16"},
		{"diff coverage", "78.7%"},
		{"duration", "2m15s"},
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
