package ablation

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/report"
)

// writeReport serialises run as a testgen-report JSON and drops it into
// dir under the given file name. Returns the full path.
func writeReport(t *testing.T, dir, name string, run report.JSONRun) string {
	t.Helper()
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func sampleRun(config string, diffCov float64, mut float64) report.JSONRun {
	diff := diffCov
	mutV := mut
	return report.JSONRun{
		SchemaVersion: "1.0",
		ProjectName:   "demo",
		Branch:        "main",
		Model:         "test-model",
		DurationSec:   12.3,
		Totals: report.JSONTotals{
			FilesProcessed:   1,
			TestsGenerated:   3,
			TestsValidated:   2,
			DiffCoveragePct:  &diff,
			MutationScorePct: &mutV,
		},
		Config: &report.JSONConfig{
			AblationConfig: config,
		},
	}
}

func TestLoadReports_BasicOrdering(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "no-types.json", sampleRun("no-types", 70, 60))
	writeReport(t, dir, "full.json", sampleRun("full", 90, 85))
	// A file without a config label falls back to its filename.
	runNoLabel := sampleRun("", 50, 40)
	runNoLabel.Config.AblationConfig = ""
	writeReport(t, dir, "custom.json", runNoLabel)
	// index.json must be ignored by the loader.
	os.WriteFile(filepath.Join(dir, "index.json"), []byte("{}"), 0644)

	rows, err := LoadReports(dir)
	if err != nil {
		t.Fatalf("LoadReports: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].Config != "full" {
		t.Fatalf("expected 'full' first, got %s", rows[0].Config)
	}
	if rows[1].Config != "no-types" {
		t.Fatalf("expected 'no-types' second, got %s", rows[1].Config)
	}
	if rows[2].Config != "custom" {
		t.Fatalf("expected file-name fallback 'custom', got %s", rows[2].Config)
	}
}

func TestRenderMarkdown_StableShape(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "full.json", sampleRun("full", 90, 85))
	writeReport(t, dir, "no-pruning.json", sampleRun("no-pruning", 60, 40))

	rows, err := LoadReports(dir)
	if err != nil {
		t.Fatalf("LoadReports: %v", err)
	}
	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, rows); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "| full |") || !strings.Contains(out, "| no-pruning |") {
		t.Fatalf("markdown output missing configurations:\n%s", out)
	}
	if !strings.Contains(out, "DiffCov") {
		t.Fatalf("expected DiffCov header, got:\n%s", out)
	}
}

func TestRenderCSV_HeaderAndRows(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "full.json", sampleRun("full", 90, 85))
	rows, _ := LoadReports(dir)
	var buf bytes.Buffer
	if err := RenderCSV(&buf, rows); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "config,") {
		t.Fatalf("CSV must start with header 'config,…', got %q", out[:20])
	}
	if !strings.Contains(out, "full,1,3,2,") {
		t.Fatalf("CSV row missing or malformed:\n%s", out)
	}
}

func TestRenderLaTeX_Envelope(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "full.json", sampleRun("full", 90, 85))
	rows, _ := LoadReports(dir)
	var buf bytes.Buffer
	if err := RenderLaTeX(&buf, rows); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\\begin{tabular}") || !strings.Contains(out, "\\end{tabular}") {
		t.Fatalf("LaTeX output missing envelope:\n%s", out)
	}
}
