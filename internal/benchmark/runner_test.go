package benchmark

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/ablation"
	"github.com/gizatulin/testgen-agent/internal/report"
)

// TestRunAll_SkipClone exercises the benchmark runner end-to-end using a
// mock agent binary (a shell script) and `SkipClone` so we don't touch
// the network. Skipped on Windows because the fake agent uses a POSIX
// shell shebang.
func TestRunAll_SkipClone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agent is a POSIX shell; skipping on Windows")
	}
	out := t.TempDir()
	repoA := t.TempDir()
	repoB := t.TempDir()

	// Seed each repo with a testgen-report-*.json so the ablation
	// runner has something to relocate into OutDir.
	for _, r := range []struct{ dir, name string }{{repoA, "a"}, {repoB, "b"}} {
		run := report.JSONRun{SchemaVersion: "1.0", ProjectName: r.name}
		data, _ := json.MarshalIndent(run, "", "  ")
		if err := os.WriteFile(filepath.Join(r.dir, "testgen-report-seed.json"), data, 0644); err != nil {
			t.Fatalf("seed report: %v", err)
		}
	}

	agent := filepath.Join(t.TempDir(), "agent.sh")
	if err := os.WriteFile(agent, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	ds := &Dataset{
		WorkDir: t.TempDir(),
		Repos: []Repo{
			{Name: "a", URL: repoA, Base: "main", Head: "HEAD"},
			{Name: "b", URL: repoB, Base: "main", Head: "HEAD"},
		},
	}
	if err := ds.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	var stderr bytes.Buffer
	runner := NewRunner(Options{
		AgentBin:  agent,
		Configs:   []ablation.Config{{Name: "full"}, {Name: "no-types", Flags: []string{"--no-types"}}},
		OutDir:    out,
		SkipClone: true,
		Stderr:    &stderr,
	})

	results, err := runner.RunAll(ds)
	if err != nil {
		t.Fatalf("RunAll: %v\nstderr=%s", err, stderr.String())
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 repo results, got %d", len(results))
	}
	for _, rr := range results {
		if rr.Err != "" {
			t.Fatalf("%s: %s", rr.Repo.Name, rr.Err)
		}
		if len(rr.Runs) != 2 {
			t.Fatalf("%s: expected 2 runs, got %d", rr.Repo.Name, len(rr.Runs))
		}
	}

	// Verify produced layout: <out>/<repo>/<config>.json and repo.json.
	for _, name := range []string{"a", "b"} {
		for _, cfg := range []string{"full", "no-types"} {
			p := filepath.Join(out, name, cfg+".json")
			if _, err := os.Stat(p); err != nil {
				t.Errorf("missing %s: %v", p, err)
			}
		}
		if _, err := os.Stat(filepath.Join(out, name, "repo.json")); err != nil {
			t.Errorf("missing repo.json for %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "benchmark-index.json")); err != nil {
		t.Errorf("missing benchmark-index.json: %v", err)
	}
}

func TestRunAll_AgentMissing(t *testing.T) {
	ds := &Dataset{
		Repos: []Repo{{Name: "x", URL: "/tmp/anywhere", Base: "m", Head: "h"}},
	}
	r := NewRunner(Options{AgentBin: "/no/such/binary", OutDir: t.TempDir(), SkipClone: true})
	_, err := r.RunAll(ds)
	if err == nil {
		t.Fatalf("expected error for missing agent binary")
	}
}
