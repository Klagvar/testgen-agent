package ablation

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/report"
)

// Row is a single line in the aggregated ablation table. One Row per
// configuration in the input directory.
type Row struct {
	Config           string
	FilesProcessed   int
	TestsGenerated   int
	TestsValidated   int
	DiffCovPct       *float64
	BranchCovPct     *float64
	ErrorPathCovPct  *float64
	MutationPct      *float64
	TokenEfficiency  *float64
	DurationSec      float64
	AssertionRatio   *float64
	NoAssertionsPct  *float64
	NilOnlyPct       *float64
	ErrorAssertsPct  *float64
	TestNameScore    *float64
	VarNameScore     *float64
}

// LoadReports walks dir for *.json files, skipping `index.json`, and
// returns one Row per report (ordered by the configuration name inside
// the report, falling back to the file name). A report whose
// config.ablation_config field is empty is tagged with its file name
// minus the .json extension.
func LoadReports(dir string) ([]Row, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var rows []Row
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || name == "index.json" {
			continue
		}
		path := filepath.Join(dir, name)
		row, err := loadOne(path, strings.TrimSuffix(name, ".json"))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return configOrder(rows[i].Config) < configOrder(rows[j].Config) })
	return rows, nil
}

// configOrder gives "full" index 0 so it always appears first in tables.
// Unknown configs sort after known ones, alphabetically.
func configOrder(name string) int {
	for i, c := range DefaultConfigs {
		if c.Name == name {
			return i
		}
	}
	return 100 + int(strhash(name)%100)
}

func strhash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func loadOne(path, fallbackName string) (Row, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Row{}, err
	}
	var run report.JSONRun
	if err := json.Unmarshal(data, &run); err != nil {
		return Row{}, err
	}
	row := Row{
		Config:          fallbackName,
		FilesProcessed:  run.Totals.FilesProcessed,
		TestsGenerated:  run.Totals.TestsGenerated,
		TestsValidated:  run.Totals.TestsValidated,
		DiffCovPct:      run.Totals.DiffCoveragePct,
		BranchCovPct:    run.Totals.BranchCoveragePct,
		ErrorPathCovPct: run.Totals.ErrorPathCoveragePct,
		MutationPct:     run.Totals.MutationScorePct,
		TokenEfficiency: run.Totals.TokenEfficiency,
		DurationSec:     run.DurationSec,
	}
	if run.Config != nil && run.Config.AblationConfig != "" {
		row.Config = run.Config.AblationConfig
	}
	if run.Totals.Naturalness != nil {
		n := run.Totals.Naturalness
		row.AssertionRatio = &n.AssertionRatio
		row.NoAssertionsPct = &n.NoAssertionsPct
		row.NilOnlyPct = &n.NilOnlyAssertionsPct
		row.ErrorAssertsPct = &n.ErrorAssertionsPct
		row.TestNameScore = &n.TestNameScore
		row.VarNameScore = &n.VarNameScore
	}
	return row, nil
}

// RenderMarkdown writes a GitHub-flavoured markdown table.
func RenderMarkdown(w io.Writer, rows []Row) error {
	const header = "| Config | Files | Gen | Val | DiffCov | Branch | Err-path | Mut | Tok/test | Assert/test | NoA% | NilOnly% | TestNameScore | VarNameScore | Dur(s) |\n" +
		"|--------|------:|----:|----:|--------:|-------:|---------:|----:|---------:|------------:|-----:|---------:|--------------:|-------------:|-------:|\n"
	if _, err := fmt.Fprint(w, header); err != nil {
		return err
	}
	for _, r := range rows {
		_, err := fmt.Fprintf(w, "| %s | %d | %d | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %.1f |\n",
			r.Config, r.FilesProcessed, r.TestsGenerated, r.TestsValidated,
			fmtPct(r.DiffCovPct), fmtPct(r.BranchCovPct), fmtPct(r.ErrorPathCovPct),
			fmtPct(r.MutationPct), fmtFloat(r.TokenEfficiency, 0),
			fmtFloat(r.AssertionRatio, 2), fmtPct(r.NoAssertionsPct), fmtPct(r.NilOnlyPct),
			fmtFloat(r.TestNameScore, 1), fmtFloat(r.VarNameScore, 1),
			r.DurationSec,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// RenderCSV writes the same data in CSV form, ready for Excel / pandas.
func RenderCSV(w io.Writer, rows []Row) error {
	_, err := fmt.Fprintln(w, "config,files,tests_generated,tests_validated,diff_cov_pct,branch_cov_pct,error_path_cov_pct,mutation_pct,tokens_per_test,assertions_per_test,no_assertions_pct,nil_only_pct,error_assertions_pct,test_name_score,var_name_score,duration_s")
	if err != nil {
		return err
	}
	for _, r := range rows {
		_, err := fmt.Fprintf(w, "%s,%d,%d,%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%.1f\n",
			r.Config, r.FilesProcessed, r.TestsGenerated, r.TestsValidated,
			csvFloat(r.DiffCovPct), csvFloat(r.BranchCovPct), csvFloat(r.ErrorPathCovPct),
			csvFloat(r.MutationPct), csvFloat(r.TokenEfficiency),
			csvFloat(r.AssertionRatio), csvFloat(r.NoAssertionsPct), csvFloat(r.NilOnlyPct),
			csvFloat(r.ErrorAssertsPct), csvFloat(r.TestNameScore), csvFloat(r.VarNameScore),
			r.DurationSec,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// RenderLaTeX writes a tabular environment suitable for a LaTeX table.
// Intended for direct inclusion in the thesis' experimental chapter.
func RenderLaTeX(w io.Writer, rows []Row) error {
	if _, err := fmt.Fprintln(w, "\\begin{tabular}{l r r r r r r r r}"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\\hline"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Config & Files & Gen & Val & DiffCov & Branch & ErrPath & Mut & Tok/test \\\\"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\\hline"); err != nil {
		return err
	}
	for _, r := range rows {
		_, err := fmt.Fprintf(w, "%s & %d & %d & %d & %s & %s & %s & %s & %s \\\\\n",
			escapeLaTeX(r.Config), r.FilesProcessed, r.TestsGenerated, r.TestsValidated,
			fmtPct(r.DiffCovPct), fmtPct(r.BranchCovPct), fmtPct(r.ErrorPathCovPct),
			fmtPct(r.MutationPct), fmtFloat(r.TokenEfficiency, 0),
		)
		if err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\\hline"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "\\end{tabular}")
	return err
}

func fmtPct(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", *v)
}

func fmtFloat(v *float64, digits int) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.*f", digits, *v)
}

func csvFloat(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.4f", *v)
}

func escapeLaTeX(s string) string {
	replacer := strings.NewReplacer("_", "\\_", "&", "\\&", "#", "\\#", "%", "\\%")
	return replacer.Replace(s)
}
