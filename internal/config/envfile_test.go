package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	content := `# Comment
TESTGEN_ENV_TEST_A=hello
TESTGEN_ENV_TEST_B="quoted value"
TESTGEN_ENV_TEST_C='single quoted'

# Blank lines are fine
TESTGEN_ENV_TEST_D = spaces around equals
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("TESTGEN_ENV_TEST_A")
	os.Unsetenv("TESTGEN_ENV_TEST_B")
	os.Unsetenv("TESTGEN_ENV_TEST_C")
	os.Unsetenv("TESTGEN_ENV_TEST_D")

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile error: %v", err)
	}

	tests := []struct {
		key  string
		want string
	}{
		{"TESTGEN_ENV_TEST_A", "hello"},
		{"TESTGEN_ENV_TEST_B", "quoted value"},
		{"TESTGEN_ENV_TEST_C", "single quoted"},
		{"TESTGEN_ENV_TEST_D", "spaces around equals"},
	}

	for _, tt := range tests {
		got := os.Getenv(tt.key)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.key, got, tt.want)
		}
		os.Unsetenv(tt.key)
	}
}

func TestLoadEnvFile_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	content := "TESTGEN_ENV_TEST_EXISTING=from-file\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	os.Setenv("TESTGEN_ENV_TEST_EXISTING", "original")
	defer os.Unsetenv("TESTGEN_ENV_TEST_EXISTING")

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile error: %v", err)
	}

	got := os.Getenv("TESTGEN_ENV_TEST_EXISTING")
	if got != "original" {
		t.Errorf("should not overwrite existing env: got %q, want %q", got, "original")
	}
}

func TestLoadEnvFile_MissingFile(t *testing.T) {
	err := LoadEnvFile("/nonexistent/path/.env")
	if err != nil {
		t.Errorf("missing file should return nil, got: %v", err)
	}
}
