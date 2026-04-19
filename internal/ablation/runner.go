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
}

// RunRecord describes the outcome of running a single configuration.
type RunRecord struct {
	Config     string        `json:"config"`
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

// Run executes cfg and returns the resulting record. It does not abort
// the caller on non-zero exit — many ablations can legitimately exit
// with a warning code (e.g. partial validation) and we still want the
// JSON report that was produced.
func (r Runner) Run(cfg Config) RunRecord {
	rec := RunRecord{Config: cfg.Name}

	report := r.Opts.Report
	if report == "" {
		report = "json"
	}
	if err := os.MkdirAll(r.Opts.OutDir, 0755); err != nil {
		rec.Err = fmt.Sprintf("mkdir out: %v", err)
		return rec
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
	// stable (<config>.json).
	if report == "json" {
		if src := findLatestJSONReport(r.Opts.RepoPath); src != "" {
			dst := filepath.Join(r.Opts.OutDir, cfg.Name+".json")
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
