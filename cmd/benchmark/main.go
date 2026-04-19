// cmd/benchmark is the multi-repository experimental harness: it reads a
// dataset.yaml, clones / checks out every listed repository and runs
// testgen-agent once per (repo × ablation config), producing per-run
// JSON reports plus a top-level benchmark-index.json.
//
// Usage:
//
//	benchmark --dataset ./benchmark/dataset.yaml \
//	          --agent ./testgen-agent \
//	          --model qwen3-coder:30b \
//	          --configs full,no-types,no-pruning \
//	          --out ./benchmark-results
//
// For interactive development on an already-cloned working copy pass
// `--skip-clone`; in that mode dataset.repos[].url is treated as a local
// path.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/ablation"
	"github.com/gizatulin/testgen-agent/internal/benchmark"
)

func main() {
	datasetPath := flag.String("dataset", "", "Path to dataset.yaml (required)")
	agentBin := flag.String("agent", "", "Path to the testgen-agent binary (required)")
	outDir := flag.String("out", "./benchmark-results", "Output directory for per-repo JSON reports")
	model := flag.String("model", "", "LLM model (forwarded to agent as --model)")
	configsCSV := flag.String("configs", "", "Comma-separated list of ablation configs (empty = all defaults)")
	extraArgs := flag.String("extra", "", "Extra args forwarded to the agent (space-separated)")
	skipClone := flag.Bool("skip-clone", false, "Treat dataset.repos[].url as a local path already checked out")
	listOnly := flag.Bool("list", false, "Print dataset contents and exit")
	flag.Parse()

	if *datasetPath == "" {
		fmt.Fprintln(os.Stderr, "❌ --dataset is required")
		os.Exit(2)
	}
	ds, err := benchmark.LoadDataset(*datasetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(2)
	}

	if *listOnly {
		fmt.Printf("Dataset (%d repos, workdir=%s):\n", len(ds.Repos), ds.WorkDir)
		for i, r := range ds.Repos {
			fmt.Printf("  %d. %-24s  %s\n     base=%s head=%s subdir=%q\n",
				i+1, r.Name, r.URL, r.Base, r.Head, r.Subdir)
		}
		return
	}

	if *agentBin == "" {
		fmt.Fprintln(os.Stderr, "❌ --agent is required")
		os.Exit(2)
	}

	configs, missing := ablation.SelectConfigs(splitCSV(*configsCSV))
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "❌ unknown configurations: %s\n", strings.Join(missing, ", "))
		os.Exit(2)
	}

	runner := benchmark.NewRunner(benchmark.Options{
		AgentBin:  *agentBin,
		Configs:   configs,
		Model:     *model,
		OutDir:    *outDir,
		ExtraArgs: splitWS(*extraArgs),
		SkipClone: *skipClone,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})

	results, err := runner.RunAll(ds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ benchmark: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n═══ Summary ═══")
	for _, rr := range results {
		if rr.Err != "" {
			fmt.Printf("  ❌ %-24s  %s\n", rr.Repo.Name, rr.Err)
			continue
		}
		ok, fail := 0, 0
		for _, run := range rr.Runs {
			if run.Err == "" {
				ok++
			} else {
				fail++
			}
		}
		fmt.Printf("  ✅ %-24s  %d ok / %d fail  (%s)\n",
			rr.Repo.Name, ok, fail, rr.Duration.Round(1_000_000_000))
	}
	fmt.Printf("\n📦 Results in %s\n", *outDir)
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

func splitWS(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}
