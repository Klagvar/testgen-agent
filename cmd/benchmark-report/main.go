// cmd/benchmark-report turns a benchmark results directory into a
// thesis-ready comparison table. Two output modes are supported:
//
//   1. Summary (default): one row per ablation configuration with every
//      metric averaged across repositories in the benchmark.
//   2. Matrix (`--matrix <metric>`): a repos × configs table for a
//      single metric, useful for per-project analysis.
//
// Supported formats: markdown (default), csv, latex, json.
//
// Usage:
//
//	benchmark-report --in ./benchmark-results
//	benchmark-report --in ./benchmark-results --format csv
//	benchmark-report --in ./benchmark-results --matrix diff_cov --format latex
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/benchmark"
)

func main() {
	in := flag.String("in", "./benchmark-results", "Directory produced by cmd/benchmark")
	format := flag.String("format", "markdown", "Output format: markdown|csv|latex|json")
	matrix := flag.String("matrix", "", "Render a repos × configs matrix for this metric (e.g. diff_cov)")
	list := flag.Bool("list-metrics", false, "Print the metrics supported by --matrix and exit")
	flag.Parse()

	if *list {
		for _, m := range benchmark.AllMetrics() {
			fmt.Println(m)
		}
		return
	}

	repos, err := benchmark.LoadAll(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ load: %v\n", err)
		os.Exit(1)
	}
	if len(repos) == 0 {
		fmt.Fprintln(os.Stderr, "⚠️  no per-repo JSON reports found")
		os.Exit(2)
	}

	f := strings.ToLower(*format)
	if *matrix != "" {
		m := benchmark.Metric(*matrix)
		if !isKnownMetric(m) {
			fmt.Fprintf(os.Stderr, "❌ unknown --matrix metric %q (use --list-metrics to list supported)\n", *matrix)
			os.Exit(2)
		}
		mx := benchmark.BuildMatrix(repos, m)
		err = renderMatrix(f, mx)
	} else {
		err = renderSummary(f, repos)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ render: %v\n", err)
		os.Exit(1)
	}
}

func renderSummary(format string, repos []benchmark.LoadedRepo) error {
	switch format {
	case "md", "markdown":
		return benchmark.RenderSummaryMarkdown(os.Stdout, repos)
	case "csv":
		return benchmark.RenderSummaryCSV(os.Stdout, repos)
	case "tex", "latex":
		return benchmark.RenderSummaryLaTeX(os.Stdout, repos)
	case "json":
		return benchmark.Dump(os.Stdout, repos)
	}
	return fmt.Errorf("unknown format %q", format)
}

func renderMatrix(format string, mx benchmark.Matrix) error {
	switch format {
	case "md", "markdown":
		return benchmark.RenderMatrixMarkdown(os.Stdout, mx)
	case "csv":
		return benchmark.RenderMatrixCSV(os.Stdout, mx)
	case "tex", "latex":
		return benchmark.RenderMatrixLaTeX(os.Stdout, mx)
	}
	return fmt.Errorf("format %q not supported for --matrix", format)
}

func isKnownMetric(m benchmark.Metric) bool {
	for _, k := range benchmark.AllMetrics() {
		if k == m {
			return true
		}
	}
	return false
}
