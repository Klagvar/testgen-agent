package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Model != "gpt-4o-mini" {
		t.Errorf("Model = %q, want gpt-4o-mini", cfg.Model)
	}
	if cfg.CoverageThreshold != 80.0 {
		t.Errorf("CoverageThreshold = %f, want 80.0", cfg.CoverageThreshold)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.Timeout != 300 {
		t.Errorf("Timeout = %d, want 300", cfg.Timeout)
	}
}

func TestLoad_NoFile(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// Should return defaults
	if cfg.Model != "gpt-4o-mini" {
		t.Errorf("expected default model, got %q", cfg.Model)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()

	yamlContent := `
model: qwen3-coder:30b
api_url: http://localhost:11434/v1
coverage_threshold: 90
max_retries: 5
timeout_seconds: 600

exclude:
  - "vendor/**"
  - "generated/**"
  - "*_mock.go"

custom_prompt: |
  Always use table-driven tests.

mutation: true
race_detection: true
`

	err := os.WriteFile(filepath.Join(dir, ".testgen.yml"), []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Model != "qwen3-coder:30b" {
		t.Errorf("Model = %q, want qwen3-coder:30b", cfg.Model)
	}
	if cfg.APIURL != "http://localhost:11434/v1" {
		t.Errorf("APIURL = %q", cfg.APIURL)
	}
	if cfg.CoverageThreshold != 90 {
		t.Errorf("CoverageThreshold = %f, want 90", cfg.CoverageThreshold)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
	if cfg.Timeout != 600 {
		t.Errorf("Timeout = %d, want 600", cfg.Timeout)
	}
	if len(cfg.Exclude) != 3 {
		t.Errorf("Exclude length = %d, want 3", len(cfg.Exclude))
	}
	if !cfg.Mutation {
		t.Error("Mutation should be true")
	}
	if !cfg.Race {
		t.Error("Race should be true")
	}
	if cfg.CustomPrompt == "" {
		t.Error("CustomPrompt should not be empty")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, ".testgen.yml"), []byte("{{{{invalid"), 0644)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = Load(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoad_ParentSearch(t *testing.T) {
	// Create dir structure: parent/.testgen.yml, parent/child/
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	os.MkdirAll(child, 0755)

	yamlContent := `model: test-model`
	os.WriteFile(filepath.Join(parent, ".testgen.yml"), []byte(yamlContent), 0644)

	cfg, err := Load(child)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Model != "test-model" {
		t.Errorf("Model = %q, want test-model (should find parent config)", cfg.Model)
	}
}

func TestShouldExclude(t *testing.T) {
	cfg := &Config{
		Exclude: []string{
			"vendor/**",
			"*_mock.go",
			"generated/**",
		},
	}

	tests := []struct {
		path    string
		exclude bool
	}{
		{"vendor/pkg/foo.go", true},
		{"internal/service.go", false},
		{"internal/repo_mock.go", true},
		{"generated/models.go", true},
		{"main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := cfg.ShouldExclude(tt.path)
			if got != tt.exclude {
				t.Errorf("ShouldExclude(%q) = %v, want %v", tt.path, got, tt.exclude)
			}
		})
	}
}

func TestShouldExclude_IncludeOnly(t *testing.T) {
	cfg := &Config{
		IncludeOnly: []string{
			"internal/**",
			"cmd/**",
		},
	}

	tests := []struct {
		path    string
		exclude bool
	}{
		{"internal/service.go", false},
		{"cmd/agent/main.go", false},
		{"vendor/pkg/foo.go", true},
		{"utils/helper.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := cfg.ShouldExclude(tt.path)
			if got != tt.exclude {
				t.Errorf("ShouldExclude(%q) = %v, want %v", tt.path, got, tt.exclude)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()
	errs := cfg.Validate()
	if len(errs) != 0 {
		t.Errorf("default config should be valid, got errors: %v", errs)
	}
}

func TestValidate_Errors(t *testing.T) {
	cfg := &Config{
		MaxRetries:        0,
		CoverageThreshold: 150,
		Timeout:           5,
		ReportFormat:      "xml",
	}

	errs := cfg.Validate()
	if len(errs) != 4 {
		t.Errorf("expected 4 errors, got %d: %v", len(errs), errs)
	}
}

func TestString(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Exclude = []string{"vendor/**"}
	cfg.CustomPrompt = "Use table-driven tests"

	s := cfg.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		match   bool
	}{
		{"foo.go", "*.go", true},
		{"foo.txt", "*.go", false},
		{"vendor/pkg/foo.go", "vendor/**", true},
		{"repo_mock.go", "*_mock.go", true},
		{"internal/service.go", "*_mock.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.pattern, func(t *testing.T) {
			got := matchGlob(tt.path, tt.pattern)
			if got != tt.match {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.match)
			}
		})
	}
}
