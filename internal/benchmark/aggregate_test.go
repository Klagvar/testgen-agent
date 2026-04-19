package benchmark

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/report"
)

// writeRun serialises a JSONRun into <dir>/<repo>/<config>.json.
func writeRun(t *testing.T, out, repo, cfg string, diffCov, mut float64) {
	t.Helper()
	dir := filepath.Join(out, repo)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	d := diffCov
	m := mut
	run := report.JSONRun{
		SchemaVersion: "1.0",
		ProjectName:   repo,
		DurationSec:   10,
		Totals: report.JSONTotals{
			FilesProcessed:   1,
			TestsGenerated:   3,
			TestsValidated:   2,
			DiffCoveragePct:  &d,
			MutationScorePct: &m,
		},
		Config: &report.JSONConfig{AblationConfig: cfg},
	}
	data, _ := json.MarshalIndent(run, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, cfg+".json"), data, 0644); err != nil {
		t.Fatalf("write run: %v", err)
	}
}

func TestLoadAll_BasicOrdering(t *testing.T) {
	out := t.TempDir()
	writeRun(t, out, "zeta-repo", "full", 80, 70)
	writeRun(t, out, "zeta-repo", "no-types", 60, 50)
	writeRun(t, out, "alpha-repo", "full", 90, 85)

	repos, err := LoadAll(out)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].Name != "alpha-repo" || repos[1].Name != "zeta-repo" {
		t.Fatalf("repos sorted wrong: %v", []string{repos[0].Name, repos[1].Name})
	}
	if len(repos[0].Rows) != 1 || len(repos[1].Rows) != 2 {
		t.Fatalf("row counts wrong: %d, %d", len(repos[0].Rows), len(repos[1].Rows))
	}
}

func TestAveragePerConfig(t *testing.T) {
	out := t.TempDir()
	writeRun(t, out, "a", "full", 80, 90)
	writeRun(t, out, "b", "full", 100, 70)
	writeRun(t, out, "a", "no-types", 60, 50)
	// `b` intentionally omits no-types; aggregator must still include
	// the row, computing the average over the single observation.

	repos, _ := LoadAll(out)
	rows := AveragePerConfig(repos)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	byCfg := map[string]int{}
	for i, r := range rows {
		byCfg[r.Config] = i
	}
	full := rows[byCfg["full"]]
	if full.DiffCovPct == nil || *full.DiffCovPct != 90 {
		t.Fatalf("full avg diff-cov = %v, want 90", full.DiffCovPct)
	}
	if full.MutationPct == nil || *full.MutationPct != 80 {
		t.Fatalf("full avg mutation = %v, want 80", full.MutationPct)
	}
	nt := rows[byCfg["no-types"]]
	if nt.DiffCovPct == nil || *nt.DiffCovPct != 60 {
		t.Fatalf("no-types avg diff-cov = %v, want 60 (single obs)", nt.DiffCovPct)
	}
	// Count-like fields are summed across repos regardless of config.
	if full.TestsGenerated != 6 {
		t.Fatalf("tests generated sum = %d, want 6 (3 per repo × 2 repos)", full.TestsGenerated)
	}
}

func TestBuildMatrix(t *testing.T) {
	out := t.TempDir()
	writeRun(t, out, "a", "full", 80, 90)
	writeRun(t, out, "a", "no-types", 60, 50)
	writeRun(t, out, "b", "full", 100, 70)

	repos, _ := LoadAll(out)
	mx := BuildMatrix(repos, MetricDiffCov)

	if len(mx.Configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(mx.Configs))
	}
	if mx.Configs[0] != "full" {
		t.Fatalf("baseline 'full' must come first, got %v", mx.Configs)
	}
	// a / full
	if got := mx.Cells[0][0]; got == nil || *got != 80 {
		t.Fatalf("a/full = %v, want 80", got)
	}
	// b / no-types must be missing
	if mx.Cells[1][1] != nil {
		t.Fatalf("b/no-types should be nil, got %v", *mx.Cells[1][1])
	}
}

func TestRenderMatrixMarkdown(t *testing.T) {
	out := t.TempDir()
	writeRun(t, out, "a", "full", 80, 90)
	writeRun(t, out, "b", "full", 100, 70)
	repos, _ := LoadAll(out)
	mx := BuildMatrix(repos, MetricDiffCov)

	var buf bytes.Buffer
	if err := RenderMatrixMarkdown(&buf, mx); err != nil {
		t.Fatalf("render: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, "| Repo |") || !strings.Contains(s, "| a |") || !strings.Contains(s, "| b |") {
		t.Fatalf("unexpected output:\n%s", s)
	}
	if !strings.Contains(s, "80.0%") || !strings.Contains(s, "100.0%") {
		t.Fatalf("values missing:\n%s", s)
	}
}

func TestDumpJSON(t *testing.T) {
	out := t.TempDir()
	writeRun(t, out, "a", "full", 80, 90)
	repos, _ := LoadAll(out)

	var buf bytes.Buffer
	if err := Dump(&buf, repos); err != nil {
		t.Fatalf("dump: %v", err)
	}
	var payload struct {
		Summary  []map[string]any            `json:"summary"`
		Matrices map[string]map[string]any   `json:"matrices"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Summary) != 1 {
		t.Fatalf("summary len=%d", len(payload.Summary))
	}
	if _, ok := payload.Matrices["diff_cov"]; !ok {
		t.Fatalf("diff_cov matrix missing")
	}
}
