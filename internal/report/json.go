// JSON report generation. Produces a machine-readable record of every
// metric the agent collects so that thesis experiments (ablation, metric
// tables, model comparisons) can be aggregated without re-scraping the
// Markdown PR comment or the HTML artifact.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// JSONRun is the top-level schema of the exported experimental record. It
// intentionally mirrors the Markdown/HTML views but keeps every field in its
// raw numeric form (no formatting, no emoji) so that downstream tooling can
// treat it as data.
type JSONRun struct {
	SchemaVersion string    `json:"schema_version"`
	ProjectName   string    `json:"project_name"`
	Branch        string    `json:"branch"`
	BaseBranch    string    `json:"base_branch,omitempty"`
	Model         string    `json:"model"`
	Timestamp     time.Time `json:"timestamp"`
	DurationSec   float64   `json:"duration_seconds"`

	Totals JSONTotals  `json:"totals"`
	Files  []JSONFile  `json:"files"`
	Config *JSONConfig `json:"config,omitempty"`
}

// JSONTotals aggregates run-level counters and quality metrics.
type JSONTotals struct {
	FilesProcessed int `json:"files_processed"`
	TestsGenerated int `json:"tests_generated"`
	TestsValidated int `json:"tests_validated"`
	TestsCached    int `json:"tests_cached"`

	// Coverage / mutation / branch / error-path / token metrics aggregated
	// across files. Fields with no observations are omitted (pointer types)
	// so that absence and "exactly 0" can be distinguished by consumers.
	DiffCoveragePct      *float64 `json:"diff_coverage_pct,omitempty"`
	MutationScorePct     *float64 `json:"mutation_score_pct,omitempty"`
	MutationsKilled      int      `json:"mutations_killed"`
	MutationsTotal       int      `json:"mutations_total"`
	BranchCoveragePct    *float64 `json:"branch_coverage_pct,omitempty"`
	BranchesCovered      int      `json:"branches_covered"`
	BranchesTotal        int      `json:"branches_total"`
	ErrorPathCoveragePct *float64 `json:"error_path_coverage_pct,omitempty"`
	ErrorPathsCovered    int      `json:"error_paths_covered"`
	ErrorPathsTotal      int      `json:"error_paths_total"`
	PromptTokens         int      `json:"prompt_tokens"`
	CompletionTokens     int      `json:"completion_tokens"`
	// TokenEfficiency is tokens / passing test. Reported only when both
	// tokens and validated tests are present.
	TokenEfficiency *float64 `json:"token_efficiency_tokens_per_test,omitempty"`

	// Naturalness aggregates per-file JSONNaturalness as straight means
	// over files with at least one top-level test. Nil when naturalness
	// was not computed for any file in the run.
	Naturalness *JSONNaturalness `json:"naturalness,omitempty"`
}

// JSONFile mirrors github.FileReport but trims presentation-only fields.
type JSONFile struct {
	File             string   `json:"file"`
	Functions        []string `json:"functions"`
	Status           string   `json:"status"`
	TestsTotal       int      `json:"tests_total"`
	TestsPassed      int      `json:"tests_passed"`
	TestsPruned      int      `json:"tests_pruned,omitempty"`
	DiffCoverage     float64  `json:"diff_coverage_pct"`
	BranchCoverage   float64  `json:"branch_coverage_pct,omitempty"`
	BranchesTotal    int      `json:"branches_total,omitempty"`
	BranchesCovered  int      `json:"branches_covered,omitempty"`
	ErrorPathCov     float64  `json:"error_path_coverage_pct,omitempty"`
	ErrorPathsTotal  int      `json:"error_paths_total,omitempty"`
	ErrorPathsCov    int      `json:"error_paths_covered,omitempty"`
	MutationScore    float64  `json:"mutation_score_pct,omitempty"`
	MutationKilled   int      `json:"mutation_killed,omitempty"`
	MutationTotal    int      `json:"mutation_total,omitempty"`
	PromptTokens     int      `json:"prompt_tokens,omitempty"`
	CompletionTokens int      `json:"completion_tokens,omitempty"`
	TokenEfficiency  float64  `json:"token_efficiency_tokens_per_test,omitempty"`
	// Naturalness is omitted when the pipeline skipped naturalness analysis
	// (e.g. the test file failed to parse) to keep the JSON schema
	// distinguishable from "analysed but scored zero".
	Naturalness *JSONNaturalness `json:"naturalness,omitempty"`
}

// JSONNaturalness mirrors naturalness.Result but uses value types so that
// downstream tooling does not have to reason about absence-vs-zero for
// every individual metric.
type JSONNaturalness struct {
	TestCount              int     `json:"test_count"`
	AssertionRatio         float64 `json:"assertion_ratio"`
	NoAssertionsPct        float64 `json:"no_assertions_pct"`
	DuplicateAssertionsPct float64 `json:"duplicate_assertions_pct"`
	NilOnlyAssertionsPct   float64 `json:"nil_only_assertions_pct"`
	ErrorAssertionsPct     float64 `json:"error_assertions_pct"`
	TestNameScore          float64 `json:"test_name_score"`
	VarNameScore           float64 `json:"var_name_score"`
}

// JSONConfig captures the knobs that affect reproducibility of a run.
type JSONConfig struct {
	CoverageTarget    float64 `json:"coverage_target"`
	MaxRetries        int     `json:"max_retries"`
	MaxCoverageIter   int     `json:"max_coverage_iter"`
	RaceDetection     bool    `json:"race_detection"`
	MutationEnabled   bool    `json:"mutation_enabled"`
	CacheEnabled      bool    `json:"cache_enabled"`
	SmartDiffEnabled  bool    `json:"smart_diff_enabled"`
	CoverageAnalysis  bool    `json:"coverage_analysis"`
	ValidationEnabled bool    `json:"validation_enabled"`
	TimeoutSeconds    int     `json:"timeout_seconds"`
	MaxContextTokens  int     `json:"max_context_tokens,omitempty"`
	ExcludeFilesCount int     `json:"exclude_files_count,omitempty"`

	// Ablation knobs. Name the configuration that produced this run so
	// the aggregator can group results by human-readable labels.
	AblationConfig            string `json:"ablation_config,omitempty"`
	TypesEnabled              bool   `json:"types_enabled"`
	StructuredFeedbackEnabled bool   `json:"structured_feedback_enabled"`
	PruningEnabled            bool   `json:"pruning_enabled"`
	NaturalnessEnabled        bool   `json:"naturalness_enabled"`
}

// GenerateJSON writes the JSON experimental record next to other reports and
// returns the resulting path. The file name is derived from the timestamp so
// that successive runs accumulate an audit trail inside outputDir.
func GenerateJSON(run JSONRun, outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = "."
	}
	fileName := fmt.Sprintf("testgen-report-%s.json", run.Timestamp.Format("2006-01-02-150405"))
	filePath := filepath.Join(outputDir, fileName)

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal json report: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("write json report: %w", err)
	}
	return filePath, nil
}

// floatPtr returns a pointer to v. Helper for optional totals.
func floatPtr(v float64) *float64 { return &v }

// BuildTotals is a convenience aggregator that computes run-level totals
// from a slice of JSONFile records. It centralises the "count everything
// non-zero" rules so that callers (main.go) stay trivial.
func BuildTotals(files []JSONFile) JSONTotals {
	t := JSONTotals{FilesProcessed: len(files)}

	var covSum float64
	covN := 0
	for _, f := range files {
		t.TestsGenerated += f.TestsTotal
		t.TestsValidated += f.TestsPassed
		if f.DiffCoverage > 0 {
			covSum += f.DiffCoverage
			covN++
		}
		t.MutationsKilled += f.MutationKilled
		t.MutationsTotal += f.MutationTotal
		t.BranchesCovered += f.BranchesCovered
		t.BranchesTotal += f.BranchesTotal
		t.ErrorPathsCovered += f.ErrorPathsCov
		t.ErrorPathsTotal += f.ErrorPathsTotal
		t.PromptTokens += f.PromptTokens
		t.CompletionTokens += f.CompletionTokens
	}

	if covN > 0 {
		t.DiffCoveragePct = floatPtr(covSum / float64(covN))
	}
	if t.MutationsTotal > 0 {
		t.MutationScorePct = floatPtr(float64(t.MutationsKilled) / float64(t.MutationsTotal) * 100)
	}
	if t.BranchesTotal > 0 {
		t.BranchCoveragePct = floatPtr(float64(t.BranchesCovered) / float64(t.BranchesTotal) * 100)
	}
	if t.ErrorPathsTotal > 0 {
		t.ErrorPathCoveragePct = floatPtr(float64(t.ErrorPathsCovered) / float64(t.ErrorPathsTotal) * 100)
	}
	if t.TestsValidated > 0 && (t.PromptTokens+t.CompletionTokens) > 0 {
		eff := float64(t.PromptTokens+t.CompletionTokens) / float64(t.TestsValidated)
		t.TokenEfficiency = floatPtr(eff)
	}

	// Aggregate naturalness across files that reported it.
	var natN int
	var agg JSONNaturalness
	for _, f := range files {
		if f.Naturalness == nil || f.Naturalness.TestCount == 0 {
			continue
		}
		natN++
		agg.TestCount += f.Naturalness.TestCount
		agg.AssertionRatio += f.Naturalness.AssertionRatio
		agg.NoAssertionsPct += f.Naturalness.NoAssertionsPct
		agg.DuplicateAssertionsPct += f.Naturalness.DuplicateAssertionsPct
		agg.NilOnlyAssertionsPct += f.Naturalness.NilOnlyAssertionsPct
		agg.ErrorAssertionsPct += f.Naturalness.ErrorAssertionsPct
		agg.TestNameScore += f.Naturalness.TestNameScore
		agg.VarNameScore += f.Naturalness.VarNameScore
	}
	if natN > 0 {
		d := float64(natN)
		agg.AssertionRatio /= d
		agg.NoAssertionsPct /= d
		agg.DuplicateAssertionsPct /= d
		agg.NilOnlyAssertionsPct /= d
		agg.ErrorAssertionsPct /= d
		agg.TestNameScore /= d
		agg.VarNameScore /= d
		t.Naturalness = &agg
	}
	return t
}
