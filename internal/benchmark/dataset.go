// Package benchmark implements the multi-repository harness used for the
// thesis' experimental evaluation. It is deliberately dataset-agnostic:
// the list of repositories, base commits and configurations is supplied
// by the caller (usually via a YAML file), so the same binary can be
// used with any collection of Go projects.
package benchmark

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Dataset is the in-memory representation of a benchmark dataset.yaml
// file. Everything in WorkDir is treated as a clone-cache so that
// repeated invocations do not re-clone large repositories.
type Dataset struct {
	// WorkDir is the directory used as a clone cache. Relative paths are
	// resolved against the dataset file's directory at load time.
	WorkDir string `yaml:"workdir"`

	// Repos enumerates the repositories included in the benchmark.
	Repos []Repo `yaml:"repos"`
}

// Repo describes a single repository to benchmark. `Name` must be unique
// within a dataset; it is used as the subdirectory name under both the
// clone cache and the output directory.
type Repo struct {
	Name    string   `yaml:"name"`
	URL     string   `yaml:"url"`
	Base    string   `yaml:"base"`
	Head    string   `yaml:"head"`
	Subdir  string   `yaml:"subdir,omitempty"`
	Exclude []string `yaml:"exclude,omitempty"`
}

// LoadDataset parses path, applies light normalisation and validates the
// structure. Returned errors describe the first problem encountered so
// the operator can fix them one at a time.
func LoadDataset(path string) (*Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dataset: %w", err)
	}
	var ds Dataset
	if err := yaml.Unmarshal(data, &ds); err != nil {
		return nil, fmt.Errorf("parse dataset: %w", err)
	}
	base := filepath.Dir(path)
	if ds.WorkDir == "" {
		ds.WorkDir = filepath.Join(base, "benchmark-checkouts")
	} else if !filepath.IsAbs(ds.WorkDir) {
		ds.WorkDir = filepath.Join(base, ds.WorkDir)
	}
	if err := ds.Validate(); err != nil {
		return nil, err
	}
	return &ds, nil
}

// Validate reports the first structural problem in ds. Required fields:
// non-empty Repos, unique Name, URL, Base, Head for every entry.
func (d *Dataset) Validate() error {
	if len(d.Repos) == 0 {
		return fmt.Errorf("dataset contains no repos")
	}
	seen := make(map[string]struct{}, len(d.Repos))
	for i, r := range d.Repos {
		if r.Name == "" {
			return fmt.Errorf("repo[%d]: name is required", i)
		}
		if _, dup := seen[r.Name]; dup {
			return fmt.Errorf("repo[%d]: duplicate name %q", i, r.Name)
		}
		seen[r.Name] = struct{}{}
		if r.URL == "" {
			return fmt.Errorf("repo %q: url is required", r.Name)
		}
		if r.Base == "" {
			return fmt.Errorf("repo %q: base is required", r.Name)
		}
		if r.Head == "" {
			return fmt.Errorf("repo %q: head is required", r.Name)
		}
		if strings.ContainsAny(r.Name, `/\`) {
			return fmt.Errorf("repo %q: name must not contain path separators", r.Name)
		}
	}
	return nil
}

// AgentDir returns the directory where the agent should be invoked for
// repo: cloneDir joined with Repo.Subdir (empty Subdir ⇒ cloneDir).
func AgentDir(cloneDir string, r Repo) string {
	if r.Subdir == "" {
		return cloneDir
	}
	return filepath.Join(cloneDir, r.Subdir)
}
