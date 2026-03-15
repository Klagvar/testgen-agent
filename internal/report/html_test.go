package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateHTML_Basic(t *testing.T) {
	dir := t.TempDir()

	data := ReportData{
		ProjectName:    "test-project",
		Branch:         "main",
		Model:          "qwen3-coder:30b",
		Timestamp:      time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		Duration:       45 * time.Second,
		TotalGenerated: 12,
		TotalValidated: 10,
		TotalCached:    3,
		TotalDiffCov:   85.5,
		Files: []FileResult{
			{
				File:         "calc.go",
				Functions:    []string{"Add", "Subtract", "Multiply"},
				TestsTotal:   6,
				TestsPassed:  6,
				DiffCoverage: 92.3,
				Status:       "success",
			},
			{
				File:         "strutil.go",
				Functions:    []string{"Reverse", "Capitalize"},
				TestsTotal:   6,
				TestsPassed:  4,
				TestsPruned:  2,
				DiffCoverage: 78.7,
				Status:       "partial",
			},
		},
	}

	path, err := GenerateHTML(data, dir)
	if err != nil {
		t.Fatalf("GenerateHTML error: %v", err)
	}

	// Check file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("report file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("report file is empty")
	}

	// Read and check content
	content, _ := os.ReadFile(path)
	html := string(content)

	checks := []string{
		"test-project",
		"qwen3-coder:30b",
		"calc.go",
		"strutil.go",
		"Add",
		"Reverse",
		"92.3%",
		"78.7%",
		"Testgen Agent",
		"<!DOCTYPE html>",
	}

	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("report should contain %q", check)
		}
	}
}

func TestGenerateHTML_WithMutation(t *testing.T) {
	dir := t.TempDir()

	data := ReportData{
		ProjectName:     "mutation-test",
		Branch:          "feature",
		Model:           "gpt-4o-mini",
		Timestamp:       time.Now(),
		Duration:        2 * time.Minute,
		MutationEnabled: true,
		MutationScore:   75.0,
		MutantsTotal:    20,
		MutantsKilled:   15,
		Files: []FileResult{
			{
				File:          "calc.go",
				Functions:     []string{"Add"},
				TestsPassed:   5,
				DiffCoverage:  90.0,
				MutationScore: 75.0,
				MutantsTotal:  20,
				MutantsKilled: 15,
				Status:        "success",
			},
		},
	}

	path, err := GenerateHTML(data, dir)
	if err != nil {
		t.Fatalf("GenerateHTML error: %v", err)
	}

	content, _ := os.ReadFile(path)
	html := string(content)

	if !strings.Contains(html, "Mutation") {
		t.Error("mutation section should be present")
	}
	if !strings.Contains(html, "75.0%") {
		t.Error("mutation score should be present")
	}
}

func TestGenerateHTML_EmptyFiles(t *testing.T) {
	dir := t.TempDir()

	data := ReportData{
		ProjectName: "empty",
		Branch:      "main",
		Model:       "test",
		Timestamp:   time.Now(),
		Duration:    time.Second,
		Files:       []FileResult{},
	}

	path, err := GenerateHTML(data, dir)
	if err != nil {
		t.Fatalf("GenerateHTML error: %v", err)
	}

	info, _ := os.Stat(path)
	if info.Size() == 0 {
		t.Error("even empty report should have HTML structure")
	}
}

func TestGenerateHTML_DefaultDir(t *testing.T) {
	// Test with empty output dir (should use ".")
	data := ReportData{
		ProjectName: "test",
		Timestamp:   time.Now(),
	}

	path, err := GenerateHTML(data, "")
	if err != nil {
		t.Fatalf("GenerateHTML error: %v", err)
	}

	// Clean up
	os.Remove(path)

	if filepath.Dir(path) != "." {
		t.Errorf("expected dir '.', got %q", filepath.Dir(path))
	}
}

func TestStatusEmoji(t *testing.T) {
	tests := []struct {
		status string
		emoji  string
	}{
		{"success", "✅"},
		{"partial", "⚠️"},
		{"failed", "❌"},
		{"unknown", "❓"},
	}

	for _, tt := range tests {
		got := statusEmoji(tt.status)
		if got != tt.emoji {
			t.Errorf("statusEmoji(%q) = %q, want %q", tt.status, got, tt.emoji)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{5 * time.Second, "5.0s"},
		{90 * time.Second, "1m 30s"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestBarWidth(t *testing.T) {
	tests := []struct {
		val  float64
		want string
	}{
		{50.0, "50.0%"},
		{100.0, "100.0%"},
		{0.0, "0.0%"},
		{150.0, "100.0%"},  // capped
		{-5.0, "0.0%"},     // floored
	}

	for _, tt := range tests {
		got := barWidth(tt.val)
		if got != tt.want {
			t.Errorf("barWidth(%f) = %q, want %q", tt.val, got, tt.want)
		}
	}
}
