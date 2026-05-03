// Package ablation defines the catalogue of ablation configurations used
// by the experimental harness (cmd/ablate) and the aggregator
// (cmd/ablate-report). Each configuration is a named set of testgen-agent
// CLI flags that selectively disables one pipeline component.
//
// The baseline configuration, "full", passes no extra flags and therefore
// represents the full pipeline. Every other configuration isolates one
// component by disabling it, so that the difference between its metrics
// and those of "full" quantifies the component's contribution.
package ablation

// Config is a named set of CLI flags that, when appended to a
// testgen-agent invocation, disables a specific pipeline component.
type Config struct {
	Name        string
	Description string
	Flags       []string
}

// DefaultConfigs enumerates the ablation configurations we use in the
// thesis' experimental chapter. The list is ordered from the least
// invasive configuration (full) to the most invasive (no-pruning), which
// matches the order of presentation in the results table.
var DefaultConfigs = []Config{
	{
		Name:        "full",
		Description: "Full pipeline (baseline)",
		Flags:       nil,
	},
	{
		Name:        "no-types",
		Description: "go/types-based analysis disabled (syntactic fallback)",
		Flags:       []string{"--no-types"},
	},
	{
		Name:        "no-smart-diff",
		Description: "Per-function git-based diff disabled (process every function)",
		Flags:       []string{"--no-smart-diff"},
	},
	{
		Name:        "no-structured-feedback",
		Description: "Repair prompt receives raw stderr instead of parsed failures",
		Flags:       []string{"--no-structured-feedback"},
	},
	{
		Name:        "no-pruning",
		Description: "Failing tests are not pruned after retry budget is exhausted",
		Flags:       []string{"--no-pruning"},
	},
	{
		Name:        "no-coverage",
		Description: "Iterative coverage-gap loop disabled",
		Flags:       []string{"--no-coverage"},
	},
	// no-cache is intentionally absent: the function-level cache is an
	// engineering optimisation that only activates on a *re-run* of the
	// agent against a previously processed repository. The benchmark
	// harness resets the working tree between every run (see
	// benchmark.runOne), so .testgen-cache.json never carries over and
	// "no-cache" would be tautologically equivalent to "full". Cache
	// efficiency is measured in a separate micro-experiment described in
	// the experiment plan, not through ablation.
}

// FindConfig returns the configuration with the given name, or (Config{}, false)
// if it does not exist in DefaultConfigs.
func FindConfig(name string) (Config, bool) {
	for _, c := range DefaultConfigs {
		if c.Name == name {
			return c, true
		}
	}
	return Config{}, false
}

// SelectConfigs filters DefaultConfigs by the provided names. Unknown names
// are reported through the second return value so the caller can fail
// early with a readable error.
func SelectConfigs(names []string) ([]Config, []string) {
	if len(names) == 0 {
		return DefaultConfigs, nil
	}
	var out []Config
	var missing []string
	for _, n := range names {
		if c, ok := FindConfig(n); ok {
			out = append(out, c)
		} else {
			missing = append(missing, n)
		}
	}
	return out, missing
}
