package ablation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/report"
)

// TestRunner_HappyPath verifies that Runner.Run forwards flags to the
// agent binary, copies the resulting JSON report into OutDir, and
// normalises its name to "<config>.json".
//
// We substitute the agent binary with a tiny "fake agent" script that
// simply writes a testgen-report-*.json into --repo.
func TestRunner_HappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agent script is a POSIX shell; skipping on Windows")
	}
	repo := t.TempDir()
	out := t.TempDir()

	// Pre-write a fake JSON report into the repo so the runner will
	// discover it as the "latest" and relocate it.
	fakeReport := report.JSONRun{SchemaVersion: "1.0", ProjectName: "demo"}
	data, _ := json.MarshalIndent(fakeReport, "", "  ")
	if err := os.WriteFile(filepath.Join(repo, "testgen-report-fake.json"), data, 0644); err != nil {
		t.Fatalf("write fake report: %v", err)
	}

	// Minimal agent: exits 0 immediately. We don't need it to actually
	// regenerate the report because the runner just looks for the
	// newest testgen-report-*.json.
	agent := filepath.Join(t.TempDir(), "agent.sh")
	if err := os.WriteFile(agent, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	r := Runner{Opts: RunOptions{
		AgentBin:   agent,
		RepoPath:   repo,
		BaseBranch: "main",
		Report:     "json",
		OutDir:     out,
	}}
	cfg := Config{Name: "no-types", Flags: []string{"--no-types"}}
	rec := r.Run(cfg)

	if rec.Err != "" {
		t.Fatalf("unexpected error: %s", rec.Err)
	}
	if rec.ExitCode != 0 {
		t.Fatalf("exit code: %d", rec.ExitCode)
	}
	if rec.ReportPath == "" {
		t.Fatalf("expected report path to be populated")
	}
	if filepath.Base(rec.ReportPath) != "no-types.json" {
		t.Fatalf("expected no-types.json, got %s", filepath.Base(rec.ReportPath))
	}
	if _, err := os.Stat(rec.ReportPath); err != nil {
		t.Fatalf("report not found at %s: %v", rec.ReportPath, err)
	}
}

func TestFindLatestJSONReport_Empty(t *testing.T) {
	dir := t.TempDir()
	if p := findLatestJSONReport(dir); p != "" {
		t.Fatalf("expected empty string for dir without reports, got %q", p)
	}
}
