// cmd/ablate orchestrates ablation experiments by invoking testgen-agent
// once per configuration and collecting the per-run JSON reports into a
// single output directory.
//
// Usage:
//
//	ablate --agent ./testgen-agent --repo ./dataset/foo \
//	       --base origin/main --model qwen3-coder:30b \
//	       --configs full,no-types,no-pruning \
//	       --out ./ablation-results/foo
//
// The harness writes:
//
//   - <out>/<config>.json    per-run testgen-agent JSON report
//   - <out>/index.json       list of RunRecord entries (one per config)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gizatulin/testgen-agent/internal/ablation"
)

func main() {
	agentBin := flag.String("agent", "", "Path to the testgen-agent binary (required)")
	repoPath := flag.String("repo", ".", "Path to the target repository")
	baseBranch := flag.String("base", "main", "Base branch for git diff")
	model := flag.String("model", "", "LLM model override (passed to agent)")
	outDir := flag.String("out", "./ablation-results", "Directory for per-configuration JSON reports")
	configsCSV := flag.String("configs", "", "Comma-separated list of ablation configs (empty = all defaults)")
	listConfigs := flag.Bool("list", false, "List known ablation configurations and exit")
	flag.Parse()

	if *listConfigs {
		printConfigList()
		return
	}
	if *agentBin == "" {
		fmt.Fprintln(os.Stderr, "❌ --agent is required")
		os.Exit(2)
	}
	if _, err := os.Stat(*agentBin); err != nil {
		fmt.Fprintf(os.Stderr, "❌ agent binary not found: %v\n", err)
		os.Exit(2)
	}

	names := splitCSV(*configsCSV)
	configs, missing := ablation.SelectConfigs(names)
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "❌ unknown configurations: %s\n", strings.Join(missing, ", "))
		os.Exit(2)
	}

	absOut, err := filepath.Abs(*outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ resolve --out: %v\n", err)
		os.Exit(2)
	}
	if err := os.MkdirAll(absOut, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "❌ create --out: %v\n", err)
		os.Exit(2)
	}

	runner := ablation.Runner{
		Opts: ablation.RunOptions{
			AgentBin:   *agentBin,
			RepoPath:   *repoPath,
			BaseBranch: *baseBranch,
			Model:      *model,
			Report:     "json",
			OutDir:     absOut,
			Stdout:     os.Stdout,
			Stderr:     os.Stderr,
		},
	}

	var records []ablation.RunRecord
	started := time.Now()
	for _, cfg := range configs {
		fmt.Printf("▶️  Running configuration: %s (%s)\n", cfg.Name, cfg.Description)
		rec := runner.Run(cfg)
		records = append(records, rec)
		fmt.Printf("   ✔ exit=%d  duration=%s  report=%s  err=%s\n\n",
			rec.ExitCode, rec.Duration.Round(time.Second), rec.ReportPath, rec.Err)
	}

	idxPath := filepath.Join(absOut, "index.json")
	idx := struct {
		Started     time.Time            `json:"started"`
		Finished    time.Time            `json:"finished"`
		RepoPath    string               `json:"repo_path"`
		BaseBranch  string               `json:"base_branch"`
		Model       string               `json:"model"`
		AgentBin    string               `json:"agent_bin"`
		TotalRuns   int                  `json:"total_runs"`
		Records     []ablation.RunRecord `json:"records"`
	}{
		Started:    started,
		Finished:   time.Now(),
		RepoPath:   *repoPath,
		BaseBranch: *baseBranch,
		Model:      *model,
		AgentBin:   *agentBin,
		TotalRuns:  len(records),
		Records:    records,
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ marshal index: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(idxPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "❌ write index.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("📦 Index: %s\n", idxPath)
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printConfigList() {
	fmt.Println("Known ablation configurations:")
	for _, c := range ablation.DefaultConfigs {
		flags := strings.Join(c.Flags, " ")
		if flags == "" {
			flags = "(none)"
		}
		fmt.Printf("  %-25s  %s\n                              flags: %s\n",
			c.Name, c.Description, flags)
	}
}
