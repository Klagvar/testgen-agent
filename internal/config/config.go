// Package config loads project settings from .testgen.yml.
// Priority: CLI flags > env variables > .testgen.yml > defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const configFileName = ".testgen.yml"

// StringOrSlice allows a YAML field to be either a single string or a list of strings.
type StringOrSlice []string

func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*s = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*s = list
		return nil
	default:
		return fmt.Errorf("expected string or list, got %v", value.Kind)
	}
}

// Config holds agent settings.
type Config struct {
	// LLM settings
	Model  string `yaml:"model"`
	APIURL string `yaml:"api_url"`
	APIKey string `yaml:"api_key"`

	// Generation settings
	CoverageThreshold float64 `yaml:"coverage_threshold"`
	MaxRetries        int     `yaml:"max_retries"`
	MaxCoverageIter   int     `yaml:"max_coverage_iterations"`
	Timeout           int     `yaml:"timeout_seconds"`

	// Filtering
	Exclude     StringOrSlice `yaml:"exclude"`
	IncludeOnly StringOrSlice `yaml:"include_only"`

	// Features
	Mutation     bool `yaml:"mutation"`
	Race         bool `yaml:"race_detection"`
	NoCache      bool `yaml:"no_cache"`
	NoSmartDiff  bool `yaml:"no_smart_diff"`
	NoValidate   bool `yaml:"no_validate"`
	NoCoverage   bool `yaml:"no_coverage"`

	// Prompt customization
	CustomPrompt     string `yaml:"custom_prompt"`
	MaxContextTokens int    `yaml:"max_context_tokens"`

	// Output
	ReportFormat string `yaml:"report_format"` // "text", "html", "json"
	OutDir       string `yaml:"out_dir"`
}

// DefaultConfig returns config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:             "gpt-4o-mini",
		APIURL:            "https://api.openai.com/v1",
		CoverageThreshold: 80.0,
		MaxRetries:        3,
		MaxCoverageIter:   2,
		Timeout:           300,
		ReportFormat:      "text",
	}
}

// Load reads .testgen.yml from the given directory (and parents).
// Returns default config if file not found.
func Load(dir string) (*Config, error) {
	cfg := DefaultConfig()

	path := findConfigFile(dir)
	if path == "" {
		return &cfg, nil // no config file — use defaults
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return &cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return &cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Apply defaults for zero values
	def := DefaultConfig()
	if cfg.CoverageThreshold == 0 {
		cfg.CoverageThreshold = def.CoverageThreshold
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = def.MaxRetries
	}
	if cfg.MaxCoverageIter == 0 {
		cfg.MaxCoverageIter = def.MaxCoverageIter
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = def.Timeout
	}
	if cfg.ReportFormat == "" {
		cfg.ReportFormat = def.ReportFormat
	}

	return &cfg, nil
}

// findConfigFile searches for .testgen.yml starting from dir, going up.
func findConfigFile(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	for {
		path := filepath.Join(absDir, configFileName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(absDir)
		if parent == absDir {
			return ""
		}
		absDir = parent
	}
}

// DefaultExcludes are always applied unless the file is in include_only.
var DefaultExcludes = []string{
	"*.pb.go",
	"*_generated.go",
	"*_gen.go",
	"vendor/**",
}

// ShouldExclude checks if a file path should be excluded based on config.
func (c *Config) ShouldExclude(filePath string) bool {
	// Normalize to forward slashes
	normalized := filepath.ToSlash(filePath)

	// Check default excludes first
	for _, pattern := range DefaultExcludes {
		if matchGlob(normalized, pattern) {
			return true
		}
	}

	// Check user-defined exclude patterns
	for _, pattern := range c.Exclude {
		pattern = filepath.ToSlash(pattern)
		if matchGlob(normalized, pattern) {
			return true
		}
	}

	// Check include_only (if specified)
	if len(c.IncludeOnly) > 0 {
		for _, pattern := range c.IncludeOnly {
			pattern = filepath.ToSlash(pattern)
			if matchGlob(normalized, pattern) {
				return false // included
			}
		}
		return true // not in include_only → exclude
	}

	return false
}

// matchGlob performs simple glob matching.
// Supports: * (any chars), ** (any path), ? (single char).
func matchGlob(path, pattern string) bool {
	// Simple cases
	if pattern == "*" || pattern == "**" {
		return true
	}

	// Use filepath.Match for single-segment patterns
	if !strings.Contains(pattern, "**") {
		matched, _ := filepath.Match(pattern, path)
		if matched {
			return true
		}
		// Also try matching just the filename
		matched, _ = filepath.Match(pattern, filepath.Base(path))
		return matched
	}

	// Handle ** (match any number of path segments)
	parts := strings.Split(pattern, "**")
	if len(parts) == 2 {
		prefix := strings.TrimSuffix(parts[0], "/")
		suffix := strings.TrimPrefix(parts[1], "/")

		if prefix != "" && !strings.HasPrefix(path, prefix) {
			return false
		}
		if suffix != "" {
			// Match suffix against any part of the remaining path
			if suffix == "*" {
				return true
			}
			matched, _ := filepath.Match(suffix, filepath.Base(path))
			return matched
		}
		return true
	}

	return false
}

// Validate checks config for obvious errors.
func (c *Config) Validate() []string {
	var errs []string

	if c.MaxRetries < 1 {
		errs = append(errs, "max_retries must be >= 1")
	}
	if c.CoverageThreshold < 0 || c.CoverageThreshold > 100 {
		errs = append(errs, "coverage_threshold must be 0-100")
	}
	if c.Timeout < 10 {
		errs = append(errs, "timeout_seconds must be >= 10")
	}
	if c.ReportFormat != "" && c.ReportFormat != "text" &&
		c.ReportFormat != "html" && c.ReportFormat != "json" {
		errs = append(errs, "report_format must be one of: text, html, json")
	}

	return errs
}

// String returns a human-readable summary of the config.
func (c *Config) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  model: %s\n", c.Model))
	sb.WriteString(fmt.Sprintf("  api_url: %s\n", c.APIURL))
	sb.WriteString(fmt.Sprintf("  coverage: %.0f%%\n", c.CoverageThreshold))
	sb.WriteString(fmt.Sprintf("  retries: %d\n", c.MaxRetries))
	if len(c.Exclude) > 0 {
		sb.WriteString(fmt.Sprintf("  exclude: %s\n", strings.Join(c.Exclude, ", ")))
	}
	if c.CustomPrompt != "" {
		sb.WriteString(fmt.Sprintf("  custom_prompt: %d chars\n", len(c.CustomPrompt)))
	}
	return sb.String()
}
