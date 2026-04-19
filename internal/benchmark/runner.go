package benchmark

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gizatulin/testgen-agent/internal/ablation"
)

// Options controls a benchmark sweep. Everything except AgentBin has a
// sensible default so callers can instantiate the runner with a single
// line.
type Options struct {
	AgentBin  string             // required: path to testgen-agent binary
	Configs   []ablation.Config  // which ablation configs to run per repo
	Model     string             // forwarded as --model when non-empty
	OutDir    string             // root directory for <repo>/<config>.json
	ExtraArgs []string           // forwarded verbatim to the agent
	Stdout    io.Writer          // where to stream subprocess stdout
	Stderr    io.Writer          // where to stream subprocess stderr
	// SkipClone bypasses the Cloner and treats repo.URL as a local path
	// that is already checked out at repo.Head. Useful for tests and for
	// iterating on local copies without network access.
	SkipClone bool
}

// RepoResult describes the outcome of benchmarking a single repository.
type RepoResult struct {
	Repo     Repo                 `json:"repo"`
	CloneDir string               `json:"clone_dir"`
	AgentDir string               `json:"agent_dir"`
	Runs     []ablation.RunRecord `json:"runs"`
	Err      string               `json:"error,omitempty"`
	Duration time.Duration        `json:"duration"`
}

// Runner executes a Dataset by iterating repositories × ablation
// configurations and invoking the testgen-agent binary once per pair.
type Runner struct {
	Opts    Options
	Cloner  *Cloner
}

// NewRunner returns a Runner with a default Cloner. Callers can override
// both after construction.
func NewRunner(opts Options) *Runner {
	return &Runner{Opts: opts, Cloner: NewCloner()}
}

// RunAll benchmarks every repository in ds. The function is fail-soft:
// an error with one repository does not abort the rest — its RepoResult
// just carries the Err field.
func (r *Runner) RunAll(ds *Dataset) ([]RepoResult, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(r.Opts.OutDir, 0755); err != nil {
		return nil, fmt.Errorf("create out: %w", err)
	}
	configs := r.Opts.Configs
	if len(configs) == 0 {
		configs = ablation.DefaultConfigs
	}
	var results []RepoResult
	for _, repo := range ds.Repos {
		r.logf("\n═══ %s (%s) ═══\n", repo.Name, repo.URL)
		rr := r.runOne(repo, ds.WorkDir, configs)
		results = append(results, rr)
		if err := r.writeRepoManifest(rr); err != nil {
			r.logf("⚠️  write %s/repo.json: %v\n", repo.Name, err)
		}
	}
	if err := r.writeIndex(ds, results); err != nil {
		return results, fmt.Errorf("write index: %w", err)
	}
	return results, nil
}

// runOne benchmarks a single repository. The caller receives a fully
// populated RepoResult even when individual ablation runs fail.
func (r *Runner) runOne(repo Repo, workDir string, configs []ablation.Config) RepoResult {
	start := time.Now()
	res := RepoResult{Repo: repo}

	var cloneDir string
	if r.Opts.SkipClone {
		cloneDir = repo.URL
	} else {
		dir, err := r.Cloner.Ensure(repo, workDir)
		if err != nil {
			res.Err = err.Error()
			res.Duration = time.Since(start)
			return res
		}
		cloneDir = dir
	}
	res.CloneDir = cloneDir
	res.AgentDir = AgentDir(cloneDir, repo)

	outDir := filepath.Join(r.Opts.OutDir, repo.Name)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		res.Err = fmt.Sprintf("create repo out: %v", err)
		res.Duration = time.Since(start)
		return res
	}

	runner := ablation.Runner{Opts: ablation.RunOptions{
		AgentBin:   r.Opts.AgentBin,
		RepoPath:   res.AgentDir,
		BaseBranch: repo.Base,
		Model:      r.Opts.Model,
		Report:     "json",
		OutDir:     outDir,
		ExtraArgs:  r.Opts.ExtraArgs,
		Stdout:     fileFromWriter(r.Opts.Stdout),
		Stderr:     fileFromWriter(r.Opts.Stderr),
	}}

	for _, cfg := range configs {
		r.logf("▶️  %s / %s\n", repo.Name, cfg.Name)
		rec := runner.Run(cfg)
		res.Runs = append(res.Runs, rec)
	}
	res.Duration = time.Since(start)
	return res
}

func (r *Runner) validate() error {
	if r.Opts.AgentBin == "" {
		return fmt.Errorf("Options.AgentBin is required")
	}
	if _, err := os.Stat(r.Opts.AgentBin); err != nil {
		return fmt.Errorf("agent binary: %w", err)
	}
	if r.Opts.OutDir == "" {
		return fmt.Errorf("Options.OutDir is required")
	}
	return nil
}

func (r *Runner) logf(format string, args ...any) {
	if r.Opts.Stdout == nil {
		return
	}
	fmt.Fprintf(r.Opts.Stdout, format, args...)
}

// writeRepoManifest persists a self-describing manifest for a repo so
// that the aggregator (and later re-runs) have access to provenance data
// without re-parsing the dataset.
func (r *Runner) writeRepoManifest(rr RepoResult) error {
	if r.Opts.OutDir == "" {
		return nil
	}
	path := filepath.Join(r.Opts.OutDir, rr.Repo.Name, "repo.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// writeIndex writes a top-level index summarising all runs.
func (r *Runner) writeIndex(ds *Dataset, results []RepoResult) error {
	idx := struct {
		WorkDir   string        `json:"work_dir"`
		Model     string        `json:"model"`
		AgentBin  string        `json:"agent_bin"`
		Timestamp time.Time     `json:"timestamp"`
		Results   []RepoResult  `json:"results"`
	}{
		WorkDir:   ds.WorkDir,
		Model:     r.Opts.Model,
		AgentBin:  r.Opts.AgentBin,
		Timestamp: time.Now(),
		Results:   results,
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.Opts.OutDir, "benchmark-index.json"), data, 0644)
}

// fileFromWriter is a tiny shim: ablation.RunOptions expects *os.File for
// stdout/stderr (to match exec.Cmd semantics) but the benchmark API lets
// the caller pass a generic io.Writer for flexibility. We pass through
// *os.File unchanged and otherwise return nil (ablation.Runner then
// discards output), which is acceptable for an experimental harness.
func fileFromWriter(w io.Writer) *os.File {
	if f, ok := w.(*os.File); ok {
		return f
	}
	return nil
}
