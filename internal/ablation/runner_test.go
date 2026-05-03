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

// TestRunner_ResetCallback verifies that runOnce invokes the Reset
// callback before launching the agent and that a Reset failure aborts
// the run with an explanatory error. This guarantees consecutive
// ablation configurations cannot leak state between each other.
func TestRunner_ResetCallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agent script is a POSIX shell; skipping on Windows")
	}
	repo := t.TempDir()
	out := t.TempDir()
	agent := filepath.Join(t.TempDir(), "agent.sh")
	if err := os.WriteFile(agent, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	called := 0
	r := Runner{Opts: RunOptions{
		AgentBin:   agent,
		RepoPath:   repo,
		BaseBranch: "main",
		Report:     "json",
		OutDir:     out,
		Reset:      func() error { called++; return nil },
	}}
	r.Run(Config{Name: "no-types", Flags: []string{"--no-types"}})
	if called != 1 {
		t.Fatalf("Reset must be invoked once per run, got %d", called)
	}

	// RunRepeated with N=3 must invoke Reset exactly N times.
	called = 0
	r.Opts.Runs = 3
	r.Opts.SeedBase = 42
	recs := r.RunRepeated(Config{Name: "full"})
	if called != 3 {
		t.Fatalf("Reset must be invoked once per repeat, got %d (recs=%d)", called, len(recs))
	}
}

func TestRunner_ResetFailureAborts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agent script is a POSIX shell; skipping on Windows")
	}
	repo := t.TempDir()
	out := t.TempDir()
	agent := filepath.Join(t.TempDir(), "agent.sh")
	// Marker that fails the test if the agent runs despite Reset error.
	if err := os.WriteFile(agent, []byte("#!/bin/sh\ntouch \"$1/agent-ran\"\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	r := Runner{Opts: RunOptions{
		AgentBin:   agent,
		RepoPath:   repo,
		BaseBranch: "main",
		Report:     "json",
		OutDir:     out,
		Reset:      func() error { return errSentinel },
	}}
	rec := r.Run(Config{Name: "full"})
	if rec.Err == "" {
		t.Fatal("expected error when Reset fails")
	}
	if _, err := os.Stat(filepath.Join(repo, "agent-ran")); err == nil {
		t.Fatal("agent must not run when Reset fails")
	}
}

var errSentinel = &resetErr{msg: "boom"}

type resetErr struct{ msg string }

func (e *resetErr) Error() string { return e.msg }

func TestFindLatestJSONReport_Empty(t *testing.T) {
	dir := t.TempDir()
	if p := findLatestJSONReport(dir); p != "" {
		t.Fatalf("expected empty string for dir without reports, got %q", p)
	}
}
