package ablation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunOptions controls a single ablation invocation of the testgen-agent
// binary. They are intentionally small — anything that is not strictly
// needed to reproduce a run belongs in .testgen.yml.
type RunOptions struct {
	AgentBin   string   // path to the testgen-agent binary (required)
	RepoPath   string   // repository to generate tests for
	BaseBranch string   // --base
	Model      string   // --model, empty uses whatever the config says
	Report     string   // --report value, defaults to "json"
	OutDir     string   // where per-config JSON reports are written
	ExtraArgs  []string // forwarded verbatim to the agent
	Stdout     *os.File // where to stream the agent's stdout (nil = discard)
	Stderr     *os.File // where to stream the agent's stderr (nil = discard)
	// Runs is the number of times each configuration is executed; values
	// below 1 fall back to a single run. When >1 the runner suffixes
	// every report with "-runN.json" and forwards --run-index to the
	// agent so downstream aggregators can group repeats.
	Runs int
	// SeedBase, when non-zero, is forwarded as --seed for the first run
	// and incremented for each subsequent repetition. Zero leaves seed
	// at the agent's default (typically unset).
	SeedBase int
	// Reset, when non-nil, is invoked immediately before each runOnce
	// invocation. The callback is expected to return the working tree
	// to a pristine state so that consecutive runs of different
	// configurations (or repeated runs of the same configuration) do
	// not leak cache hits, generated test files, or merger artefacts
	// between each other. Failures from Reset are recorded on the
	// RunRecord and the run is skipped.
	Reset func() error
}

// RunRecord describes the outcome of running a single configuration.
type RunRecord struct {
	Config     string        `json:"config"`
	RunIndex   int           `json:"run_index,omitempty"`
	ReportPath string        `json:"report_path"`
	ExitCode   int           `json:"exit_code"`
	Duration   time.Duration `json:"duration"`
	Err        string        `json:"error,omitempty"`
}

// Runner executes testgen-agent once per configuration, tagging each run
// with an --ablation-config label so that the resulting JSON reports can
// be grouped by the aggregator later.
type Runner struct {
	Opts RunOptions
}

// Run executes cfg once. It does not abort the caller on non-zero exit
// — many ablations can legitimately exit with a warning code (e.g.
// partial validation) and we still want the JSON report that was
// produced.
func (r Runner) Run(cfg Config) RunRecord {
	return r.runOnce(cfg, 0)
}

// RunRepeated executes cfg either once (Opts.Runs <= 1) or Opts.Runs
// times, returning one RunRecord per execution. Repeated runs are
// useful when the underlying LLM call is non-deterministic and the
// caller wants to estimate variance.
func (r Runner) RunRepeated(cfg Config) []RunRecord {
	n := r.Opts.Runs
	if n < 1 {
		n = 1
	}
	out := make([]RunRecord, 0, n)
	for i := 1; i <= n; i++ {
		idx := 0
		if n > 1 {
			idx = i
		}
		out = append(out, r.runOnce(cfg, idx))
	}
	return out
}

// runOnce executes cfg with a given run index. When runIndex > 0 it is
// forwarded to the agent as --run-index and used as a suffix in the
// archived report file name so multiple runs of the same configuration
// do not overwrite each other.
func (r Runner) runOnce(cfg Config, runIndex int) RunRecord {
	rec := RunRecord{Config: cfg.Name, RunIndex: runIndex}

	report := r.Opts.Report
	if report == "" {
		report = "json"
	}
	if err := os.MkdirAll(r.Opts.OutDir, 0755); err != nil {
		rec.Err = fmt.Sprintf("mkdir out: %v", err)
		return rec
	}

	if r.Opts.Reset != nil {
		if err := r.Opts.Reset(); err != nil {
			rec.Err = fmt.Sprintf("reset working tree: %v", err)
			return rec
		}
	}

	args := []string{
		"--repo", r.Opts.RepoPath,
		"--base", r.Opts.BaseBranch,
		"--report", report,
		"--ablation-config", cfg.Name,
	}
	if r.Opts.Model != "" {
		args = append(args, "--model", r.Opts.Model)
	}
	if runIndex > 0 {
		args = append(args, "--run-index", fmt.Sprintf("%d", runIndex))
	}
	if r.Opts.SeedBase != 0 {
		seed := r.Opts.SeedBase
		if runIndex > 0 {
			seed = r.Opts.SeedBase + runIndex - 1
		}
		args = append(args, "--seed", fmt.Sprintf("%d", seed))
	}
	args = append(args, cfg.Flags...)
	args = append(args, r.Opts.ExtraArgs...)

	start := time.Now()
	cmd := exec.Command(r.Opts.AgentBin, args...)
	if r.Opts.Stdout != nil {
		cmd.Stdout = r.Opts.Stdout
	}
	if r.Opts.Stderr != nil {
		cmd.Stderr = r.Opts.Stderr
	}
	err := cmd.Run()
	rec.Duration = time.Since(start)
	if cmd.ProcessState != nil {
		rec.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil && rec.ExitCode == 0 {
		rec.Err = err.Error()
	}

	// Locate the most recent JSON report produced in the repo (testgen-agent
	// writes testgen-report-<timestamp>.json into --repo). Copy it into
	// OutDir so that ablation artifacts are self-contained and naming is
	// stable. For repeated runs the file name is suffixed with "-runN" so
	// the previous run's report is preserved.
	if report == "json" {
		if src := findLatestJSONReport(r.Opts.RepoPath); src != "" {
			name := cfg.Name + ".json"
			if runIndex > 0 {
				name = fmt.Sprintf("%s-run%d.json", cfg.Name, runIndex)
			}
			dst := filepath.Join(r.Opts.OutDir, name)
			if copyErr := copyFile(src, dst); copyErr == nil {
				rec.ReportPath = dst
				_ = os.Remove(src)
			} else if rec.Err == "" {
				rec.Err = fmt.Sprintf("copy report: %v", copyErr)
			}
		}
	}
	return rec
}

// findLatestJSONReport returns the most recent testgen-report-*.json file
// in dir, or "" when no matching file exists. Mirrors the naming used by
// internal/report.GenerateJSON.
func findLatestJSONReport(dir string) string {
	matches, err := filepath.Glob(filepath.Join(dir, "testgen-report-*.json"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	latest := matches[0]
	latestMod := fileModTime(latest)
	for _, m := range matches[1:] {
		if t := fileModTime(m); t.After(latestMod) {
			latest = m
			latestMod = t
		}
	}
	return latest
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// copyFile copies src to dst preserving file mode. Used instead of os.Rename
// because the destination may be on a different filesystem.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
