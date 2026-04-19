package benchmark

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/ablation"
	"github.com/gizatulin/testgen-agent/internal/report"
)

// LoadedRepo bundles everything the aggregator needs to know about a
// single repository's results: the metadata manifest and one ablation.Row
// per configuration. The row carries the JSONRun totals already parsed
// by internal/ablation.LoadReports.
type LoadedRepo struct {
	Name string
	Rows []ablation.Row
}

// LoadAll walks outDir and returns one LoadedRepo per subdirectory that
// contains a valid set of testgen-agent JSON reports.
func LoadAll(outDir string) ([]LoadedRepo, error) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return nil, fmt.Errorf("read out: %w", err)
	}
	var out []LoadedRepo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		repoDir := filepath.Join(outDir, e.Name())
		rows, err := ablation.LoadReports(repoDir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if len(rows) == 0 {
			continue
		}
		out = append(out, LoadedRepo{Name: e.Name(), Rows: rows})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// AveragePerConfig collapses a multi-repo benchmark into one synthetic
// row per ablation configuration: every numeric metric is averaged over
// the repositories that reported it. Count-like fields (files,
// tests_generated, tests_validated, duration) are summed.
//
// Configurations that appear in only a subset of repositories are still
// returned; their averages simply use the smaller denominator.
func AveragePerConfig(repos []LoadedRepo) []ablation.Row {
	type accum struct {
		sum   ablation.Row
		count struct {
			diffCov, branchCov, errPath, mutation, tokEff int
			assertRatio, noAssert, nilOnly, errAsserts    int
			testName, varName                             int
		}
	}
	agg := make(map[string]*accum)
	order := []string{}

	for _, repo := range repos {
		for _, row := range repo.Rows {
			a, ok := agg[row.Config]
			if !ok {
				a = &accum{sum: ablation.Row{Config: row.Config}}
				agg[row.Config] = a
				order = append(order, row.Config)
			}
			a.sum.FilesProcessed += row.FilesProcessed
			a.sum.TestsGenerated += row.TestsGenerated
			a.sum.TestsValidated += row.TestsValidated
			a.sum.DurationSec += row.DurationSec
			addPtr(&a.sum.DiffCovPct, row.DiffCovPct, &a.count.diffCov)
			addPtr(&a.sum.BranchCovPct, row.BranchCovPct, &a.count.branchCov)
			addPtr(&a.sum.ErrorPathCovPct, row.ErrorPathCovPct, &a.count.errPath)
			addPtr(&a.sum.MutationPct, row.MutationPct, &a.count.mutation)
			addPtr(&a.sum.TokenEfficiency, row.TokenEfficiency, &a.count.tokEff)
			addPtr(&a.sum.AssertionRatio, row.AssertionRatio, &a.count.assertRatio)
			addPtr(&a.sum.NoAssertionsPct, row.NoAssertionsPct, &a.count.noAssert)
			addPtr(&a.sum.NilOnlyPct, row.NilOnlyPct, &a.count.nilOnly)
			addPtr(&a.sum.ErrorAssertsPct, row.ErrorAssertsPct, &a.count.errAsserts)
			addPtr(&a.sum.TestNameScore, row.TestNameScore, &a.count.testName)
			addPtr(&a.sum.VarNameScore, row.VarNameScore, &a.count.varName)
		}
	}

	rows := make([]ablation.Row, 0, len(order))
	for _, name := range order {
		a := agg[name]
		divPtr(a.sum.DiffCovPct, a.count.diffCov)
		divPtr(a.sum.BranchCovPct, a.count.branchCov)
		divPtr(a.sum.ErrorPathCovPct, a.count.errPath)
		divPtr(a.sum.MutationPct, a.count.mutation)
		divPtr(a.sum.TokenEfficiency, a.count.tokEff)
		divPtr(a.sum.AssertionRatio, a.count.assertRatio)
		divPtr(a.sum.NoAssertionsPct, a.count.noAssert)
		divPtr(a.sum.NilOnlyPct, a.count.nilOnly)
		divPtr(a.sum.ErrorAssertsPct, a.count.errAsserts)
		divPtr(a.sum.TestNameScore, a.count.testName)
		divPtr(a.sum.VarNameScore, a.count.varName)
		rows = append(rows, a.sum)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return configRank(rows[i].Config) < configRank(rows[j].Config)
	})
	return rows
}

func configRank(name string) int {
	for i, c := range ablation.DefaultConfigs {
		if c.Name == name {
			return i
		}
	}
	return 1000 + int(strhash(name)%1000)
}

func strhash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func addPtr(dst **float64, src *float64, count *int) {
	if src == nil {
		return
	}
	if *dst == nil {
		v := 0.0
		*dst = &v
	}
	**dst += *src
	*count++
}

func divPtr(p *float64, count int) {
	if p == nil || count == 0 {
		return
	}
	*p = *p / float64(count)
}

// Metric identifies a single numeric field of ablation.Row. Used for
// building matrix views.
type Metric string

const (
	MetricDiffCov         Metric = "diff_cov"
	MetricBranchCov       Metric = "branch_cov"
	MetricErrorPathCov    Metric = "error_path_cov"
	MetricMutation        Metric = "mutation"
	MetricTokenEfficiency Metric = "token_efficiency"
	MetricAssertionRatio  Metric = "assertion_ratio"
	MetricNoAssertions    Metric = "no_assertions_pct"
	MetricNilOnly         Metric = "nil_only_pct"
	MetricErrorAsserts    Metric = "error_asserts_pct"
	MetricTestName        Metric = "test_name_score"
	MetricVarName         Metric = "var_name_score"
	MetricDuration        Metric = "duration_s"
	MetricTestsValidated  Metric = "tests_validated"
)

// AllMetrics lists every supported matrix metric in presentation order.
func AllMetrics() []Metric {
	return []Metric{
		MetricDiffCov,
		MetricBranchCov,
		MetricErrorPathCov,
		MetricMutation,
		MetricTokenEfficiency,
		MetricAssertionRatio,
		MetricNoAssertions,
		MetricNilOnly,
		MetricErrorAsserts,
		MetricTestName,
		MetricVarName,
		MetricDuration,
		MetricTestsValidated,
	}
}

// pickMetric returns the row-level value of the given metric or nil when
// the row did not record it.
func pickMetric(row ablation.Row, m Metric) *float64 {
	switch m {
	case MetricDiffCov:
		return row.DiffCovPct
	case MetricBranchCov:
		return row.BranchCovPct
	case MetricErrorPathCov:
		return row.ErrorPathCovPct
	case MetricMutation:
		return row.MutationPct
	case MetricTokenEfficiency:
		return row.TokenEfficiency
	case MetricAssertionRatio:
		return row.AssertionRatio
	case MetricNoAssertions:
		return row.NoAssertionsPct
	case MetricNilOnly:
		return row.NilOnlyPct
	case MetricErrorAsserts:
		return row.ErrorAssertsPct
	case MetricTestName:
		return row.TestNameScore
	case MetricVarName:
		return row.VarNameScore
	case MetricDuration:
		d := row.DurationSec
		return &d
	case MetricTestsValidated:
		d := float64(row.TestsValidated)
		return &d
	}
	return nil
}

// Matrix is a repos × configs view of a single metric.
type Matrix struct {
	Metric  Metric
	Configs []string      // column order
	Repos   []string      // row order
	Cells   [][]*float64  // Cells[i][j] = metric for Repos[i] × Configs[j]
}

// BuildMatrix projects LoadedRepos onto a single metric. Column order is
// the union of configs found, sorted by configRank (baseline first).
func BuildMatrix(repos []LoadedRepo, m Metric) Matrix {
	configSet := map[string]struct{}{}
	for _, r := range repos {
		for _, row := range r.Rows {
			configSet[row.Config] = struct{}{}
		}
	}
	configs := make([]string, 0, len(configSet))
	for c := range configSet {
		configs = append(configs, c)
	}
	sort.SliceStable(configs, func(i, j int) bool { return configRank(configs[i]) < configRank(configs[j]) })

	cells := make([][]*float64, len(repos))
	repoNames := make([]string, len(repos))
	for i, r := range repos {
		repoNames[i] = r.Name
		cells[i] = make([]*float64, len(configs))
		byCfg := make(map[string]ablation.Row)
		for _, row := range r.Rows {
			byCfg[row.Config] = row
		}
		for j, cfg := range configs {
			if row, ok := byCfg[cfg]; ok {
				cells[i][j] = pickMetric(row, m)
			}
		}
	}
	return Matrix{Metric: m, Configs: configs, Repos: repoNames, Cells: cells}
}

// RenderSummaryMarkdown writes the "per-config averaged across repos"
// table. It reuses the single-repo renderer from internal/ablation so
// the presentation layer stays consistent.
func RenderSummaryMarkdown(w io.Writer, repos []LoadedRepo) error {
	rows := AveragePerConfig(repos)
	return ablation.RenderMarkdown(w, rows)
}

// RenderSummaryCSV — CSV counterpart of RenderSummaryMarkdown.
func RenderSummaryCSV(w io.Writer, repos []LoadedRepo) error {
	rows := AveragePerConfig(repos)
	return ablation.RenderCSV(w, rows)
}

// RenderSummaryLaTeX — LaTeX counterpart of RenderSummaryMarkdown.
func RenderSummaryLaTeX(w io.Writer, repos []LoadedRepo) error {
	rows := AveragePerConfig(repos)
	return ablation.RenderLaTeX(w, rows)
}

// RenderMatrixMarkdown writes a repos × configs table for a single metric.
func RenderMatrixMarkdown(w io.Writer, mx Matrix) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Metric:** `%s`\n\n", mx.Metric))
	sb.WriteString("| Repo |")
	for _, c := range mx.Configs {
		sb.WriteString(" " + c + " |")
	}
	sb.WriteByte('\n')
	sb.WriteString("|------|")
	for range mx.Configs {
		sb.WriteString("----:|")
	}
	sb.WriteByte('\n')
	for i, r := range mx.Repos {
		sb.WriteString("| " + r + " |")
		for _, v := range mx.Cells[i] {
			sb.WriteByte(' ')
			sb.WriteString(fmtMetric(v, mx.Metric))
			sb.WriteString(" |")
		}
		sb.WriteByte('\n')
	}
	_, err := io.WriteString(w, sb.String())
	return err
}

// RenderMatrixCSV — CSV counterpart.
func RenderMatrixCSV(w io.Writer, mx Matrix) error {
	var sb strings.Builder
	sb.WriteString("repo")
	for _, c := range mx.Configs {
		sb.WriteString("," + c)
	}
	sb.WriteByte('\n')
	for i, r := range mx.Repos {
		sb.WriteString(r)
		for _, v := range mx.Cells[i] {
			sb.WriteByte(',')
			if v != nil {
				sb.WriteString(fmt.Sprintf("%.4f", *v))
			}
		}
		sb.WriteByte('\n')
	}
	_, err := io.WriteString(w, sb.String())
	return err
}

// RenderMatrixLaTeX — minimal LaTeX tabular for a single-metric matrix.
func RenderMatrixLaTeX(w io.Writer, mx Matrix) error {
	var sb strings.Builder
	sb.WriteString("\\begin{tabular}{l")
	for range mx.Configs {
		sb.WriteString(" r")
	}
	sb.WriteString("}\n\\hline\n")
	sb.WriteString("Repo")
	for _, c := range mx.Configs {
		sb.WriteString(" & " + escapeLaTeX(c))
	}
	sb.WriteString(" \\\\\n\\hline\n")
	for i, r := range mx.Repos {
		sb.WriteString(escapeLaTeX(r))
		for _, v := range mx.Cells[i] {
			sb.WriteString(" & ")
			sb.WriteString(fmtMetric(v, mx.Metric))
		}
		sb.WriteString(" \\\\\n")
	}
	sb.WriteString("\\hline\n\\end{tabular}\n")
	_, err := io.WriteString(w, sb.String())
	return err
}

func fmtMetric(v *float64, m Metric) string {
	if v == nil {
		return "—"
	}
	switch m {
	case MetricDiffCov, MetricBranchCov, MetricErrorPathCov, MetricMutation,
		MetricNoAssertions, MetricNilOnly, MetricErrorAsserts:
		return fmt.Sprintf("%.1f%%", *v)
	case MetricTokenEfficiency, MetricDuration, MetricTestsValidated:
		return fmt.Sprintf("%.0f", *v)
	case MetricAssertionRatio:
		return fmt.Sprintf("%.2f", *v)
	case MetricTestName, MetricVarName:
		return fmt.Sprintf("%.1f", *v)
	}
	return fmt.Sprintf("%.3f", *v)
}

func escapeLaTeX(s string) string {
	replacer := strings.NewReplacer("_", "\\_", "&", "\\&", "#", "\\#", "%", "\\%")
	return replacer.Replace(s)
}

// Dump writes a structured JSON snapshot of (summary, matrices) for
// machine-readable consumption. Used by cmd/benchmark-report with
// `--format json`.
func Dump(w io.Writer, repos []LoadedRepo) error {
	summary := AveragePerConfig(repos)
	matrices := make(map[string]Matrix, len(AllMetrics()))
	for _, m := range AllMetrics() {
		matrices[string(m)] = BuildMatrix(repos, m)
	}
	payload := struct {
		Summary  []ablation.Row    `json:"summary"`
		Matrices map[string]Matrix `json:"matrices"`
	}{
		Summary:  summary,
		Matrices: matrices,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// Ensure we keep report.JSONRun depended on so that go mod tidy does not
// drop it — aggregate relies on internal/ablation which in turn parses
// JSONRun.
var _ = (*report.JSONRun)(nil)
