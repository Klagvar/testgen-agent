// cmd/ablate-report aggregates JSON reports produced by cmd/ablate into
// a single comparison table, ready for inclusion in the thesis'
// experimental chapter. Supported output formats are Markdown (default),
// CSV, and LaTeX tabular.
//
// Usage:
//
//	ablate-report --in ./ablation-results/foo --format markdown
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/ablation"
)

func main() {
	in := getArg("--in", "./ablation-results")
	format := strings.ToLower(getArg("--format", "markdown"))

	rows, err := ablation.LoadReports(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ load: %v\n", err)
		os.Exit(1)
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "⚠️  no JSON reports found")
		os.Exit(2)
	}

	switch format {
	case "md", "markdown":
		err = ablation.RenderMarkdown(os.Stdout, rows)
	case "csv":
		err = ablation.RenderCSV(os.Stdout, rows)
	case "tex", "latex":
		err = ablation.RenderLaTeX(os.Stdout, rows)
	default:
		fmt.Fprintf(os.Stderr, "❌ unknown --format %q (expected markdown|csv|latex)\n", format)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ render: %v\n", err)
		os.Exit(1)
	}
}

// getArg is a tiny, dependency-free flag parser so that the aggregator
// has no transitive dependency on flag.Parse ordering when someone embeds
// it in a shell script with piped stdin. Both `--key=value` and
// `--key value` are accepted.
func getArg(name, def string) string {
	args := os.Args[1:]
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"=")
		}
	}
	return def
}
