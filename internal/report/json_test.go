package report

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildTotals_EmptyFiles(t *testing.T) {
	got := BuildTotals(nil)
	if got.FilesProcessed != 0 {
		t.Fatalf("FilesProcessed = %d, want 0", got.FilesProcessed)
	}
	if got.DiffCoveragePct != nil {
		t.Errorf("DiffCoveragePct must be nil for empty input")
	}
	if got.TokenEfficiency != nil {
		t.Errorf("TokenEfficiency must be nil for empty input")
	}
}

func TestBuildTotals_AggregatesAllMetrics(t *testing.T) {
	cov80, cov60 := 80.0, 60.0
	files := []JSONFile{
		{
			File:             "a.go",
			TestsTotal:       5,
			TestsPassed:      4,
			DiffCoverage:     &cov80,
			MutationKilled:   3,
			MutationTotal:    5,
			BranchesCovered:  6,
			BranchesTotal:    10,
			ErrorPathsCov:    1,
			ErrorPathsTotal:  2,
			PromptTokens:     1000,
			CompletionTokens: 500,
		},
		{
			File:             "b.go",
			TestsTotal:       3,
			TestsPassed:      2,
			DiffCoverage:     &cov60,
			MutationKilled:   2,
			MutationTotal:    4,
			BranchesCovered:  2,
			BranchesTotal:    5,
			ErrorPathsCov:    0,
			ErrorPathsTotal:  1,
			PromptTokens:     600,
			CompletionTokens: 200,
		},
	}

	tot := BuildTotals(files)

	if tot.FilesProcessed != 2 || tot.TestsGenerated != 8 || tot.TestsValidated != 6 {
		t.Fatalf("counters wrong: %+v", tot)
	}
	if got := approxf(tot.DiffCoveragePct); math.Abs(got-70) > 0.001 {
		t.Errorf("DiffCoveragePct = %v, want ~70", got)
	}
	if got := approxf(tot.MutationScorePct); math.Abs(got-float64(5)/float64(9)*100) > 0.001 {
		t.Errorf("MutationScorePct = %v", got)
	}
	if tot.BranchesCovered != 8 || tot.BranchesTotal != 15 {
		t.Errorf("branch aggregate wrong: %+v", tot)
	}
	if got := approxf(tot.BranchCoveragePct); math.Abs(got-float64(8)/float64(15)*100) > 0.001 {
		t.Errorf("BranchCoveragePct = %v", got)
	}
	if tot.ErrorPathsCovered != 1 || tot.ErrorPathsTotal != 3 {
		t.Errorf("errpath aggregate wrong: %+v", tot)
	}
	wantEff := float64(1000+500+600+200) / float64(6)
	if got := approxf(tot.TokenEfficiency); math.Abs(got-wantEff) > 0.001 {
		t.Errorf("TokenEfficiency = %v, want %v", got, wantEff)
	}
}

func TestBuildTotals_NoTokensNoEfficiency(t *testing.T) {
	cov50 := 50.0
	files := []JSONFile{{File: "a.go", TestsPassed: 2, DiffCoverage: &cov50}}
	got := BuildTotals(files)
	if got.TokenEfficiency != nil {
		t.Errorf("TokenEfficiency must be nil when no tokens observed")
	}
	if got.MutationScorePct != nil {
		t.Errorf("MutationScorePct must be nil when no mutations observed")
	}
}

// TestBuildTotals_ZeroDiffCovIsValid verifies that an explicit zero
// diff-coverage observation is averaged in (not silently dropped). A
// previous version of BuildTotals filtered values with `> 0`, which
// silently biased the mean upward when generated tests covered no
// changed lines.
func TestBuildTotals_ZeroDiffCovIsValid(t *testing.T) {
	cov0, cov100 := 0.0, 100.0
	files := []JSONFile{
		{File: "a.go", DiffCoverage: &cov0},   // computed: tests covered nothing
		{File: "b.go", DiffCoverage: &cov100}, // computed: full coverage
		{File: "c.go", DiffCoverage: nil},     // not computed (e.g. dry-run)
	}
	got := BuildTotals(files)
	if got.DiffCoveragePct == nil {
		t.Fatalf("DiffCoveragePct must not be nil when at least one file reported a value")
	}
	if math.Abs(*got.DiffCoveragePct-50) > 0.001 {
		t.Errorf("DiffCoveragePct = %v, want 50 (average of 0 and 100, c.go excluded)", *got.DiffCoveragePct)
	}
}

func TestGenerateJSON_WritesValidFile(t *testing.T) {
	dir := t.TempDir()
	run := JSONRun{
		SchemaVersion: "1.0",
		ProjectName:   "demo",
		Branch:        "main",
		Model:         "gpt-4o-mini",
		Timestamp:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		DurationSec:   12.5,
		Files: []JSONFile{{
			File:        "x.go",
			Functions:   []string{"Foo"},
			Status:      "success",
			TestsTotal:  1,
			TestsPassed: 1,
		}},
	}
	run.Totals = BuildTotals(run.Files)

	path, err := GenerateJSON(run, dir)
	if err != nil {
		t.Fatalf("GenerateJSON error: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("path %q not inside %q", path, dir)
	}
	if !strings.HasSuffix(path, ".json") {
		t.Errorf("path must end with .json, got %q", path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var back JSONRun
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.SchemaVersion != "1.0" || back.Model != "gpt-4o-mini" {
		t.Errorf("round-trip mismatch: %+v", back)
	}
	if back.Totals.FilesProcessed != 1 || back.Totals.TestsValidated != 1 {
		t.Errorf("totals not persisted: %+v", back.Totals)
	}
}

func approxf(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}
