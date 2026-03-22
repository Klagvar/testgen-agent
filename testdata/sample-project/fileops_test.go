package sample

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfig_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.txt")
	content := `key1=value1
key2=value2
key3=value3`
	err := os.WriteFile(configPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	result, err := ReadConfig(configPath)
	if err != nil {
		t.Fatalf("ReadConfig returned error: %v", err)
	}

	expected := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range expected {
		if result[k] != v {
			t.Errorf("ReadConfig: expected %s=%s, got %s=%s", k, v, k, result[k])
		}
	}
}

func TestReadConfig_FileNotFound(t *testing.T) {
	_, err := ReadConfig("/non/existent/file")
	if err == nil {
		t.Error("ReadConfig should return error for non-existent file")
	}
}

func TestReadConfig_EmptyLinesAndComments(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.txt")
	content := `key1=value1

# This is a comment
key2=value2
  # Another comment with spaces
key3=value3
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	result, err := ReadConfig(configPath)
	if err != nil {
		t.Fatalf("ReadConfig returned error: %v", err)
	}

	expected := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range expected {
		if result[k] != v {
			t.Errorf("ReadConfig: expected %s=%s, got %s=%s", k, v, k, result[k])
		}
	}
}

func TestReadConfig_UnbalancedKeyValue(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.txt")
	content := `key1=value1
key2
key3=value3`
	err := os.WriteFile(configPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	result, err := ReadConfig(configPath)
	if err != nil {
		t.Fatalf("ReadConfig returned error: %v", err)
	}

	expected := map[string]string{
		"key1": "value1",
		"key3": "value3",
	}
	for k, v := range expected {
		if result[k] != v {
			t.Errorf("ReadConfig: expected %s=%s, got %s=%s", k, v, k, result[k])
		}
	}
	if _, exists := result["key2"]; exists {
		t.Errorf("ReadConfig: key2 should not exist in result")
	}
}

func TestWriteConfig_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.txt")
	config := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	err := WriteConfig(configPath, config)
	if err != nil {
		t.Fatalf("WriteConfig returned error: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	expectedContent := "key1=value1\nkey2=value2\nkey3=value3\n"
	if string(content) != expectedContent {
		t.Errorf("WriteConfig: expected %q, got %q", expectedContent, string(content))
	}
}

func TestWriteConfig_EmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.txt")
	config := map[string]string{}

	err := WriteConfig(configPath, config)
	if err != nil {
		t.Fatalf("WriteConfig returned error: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(content) != "" {
		t.Errorf("WriteConfig: expected empty content, got %q", string(content))
	}
}

func TestListGoFiles_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	// Create test go files
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte(""), 0644)
	// Create a non-go file
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte(""), 0644)
	// Create a subdirectory
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "subdir", "file3.go"), []byte(""), 0644)

	files, err := ListGoFiles(tmpDir)
	if err != nil {
		t.Fatalf("ListGoFiles returned error: %v", err)
	}

	expected := []string{"file1.go", "file2.go"}
	for _, expectedFile := range expected {
		found := false
		for _, actualFile := range files {
			if actualFile == expectedFile {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListGoFiles: expected file %s not found", expectedFile)
		}
	}

	// Check that we don't include subdirectory files or non-go files
	for _, file := range files {
		if file == "file3.go" || file == "file.txt" {
			t.Errorf("ListGoFiles: should not include %s", file)
		}
	}
}

func TestListGoFiles_FileNotFound(t *testing.T) {
	_, err := ListGoFiles("/non/existent/directory")
	if err == nil {
		t.Error("ListGoFiles should return error for non-existent directory")
	}
}

func TestListGoFiles_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	files, err := ListGoFiles(tmpDir)
	if err != nil {
		t.Fatalf("ListGoFiles returned error: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("ListGoFiles: expected empty slice, got %v", files)
	}
}

func TestGetEnvOrDefault_EnvSet(t *testing.T) {
	t.Setenv("TEST_KEY", "env_value")
	result := GetEnvOrDefault("TEST_KEY", "default_value")
	if result != "env_value" {
		t.Errorf("GetEnvOrDefault: expected 'env_value', got %q", result)
	}
}

func TestGetEnvOrDefault_EnvNotSet(t *testing.T) {
	result := GetEnvOrDefault("NON_EXISTENT_KEY", "default_value")
	if result != "default_value" {
		t.Errorf("GetEnvOrDefault: expected 'default_value', got %q", result)
	}
}

func TestGetEnvOrDefault_EnvEmpty(t *testing.T) {
	t.Setenv("EMPTY_KEY", "")
	result := GetEnvOrDefault("EMPTY_KEY", "default_value")
	if result != "default_value" {
		t.Errorf("GetEnvOrDefault: expected 'default_value', got %q", result)
	}
}

func TestLoadDatabaseURL_EnvSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/prod")
	result := LoadDatabaseURL()
	if result != "postgres://user:pass@host:5432/prod" {
		t.Errorf("LoadDatabaseURL: expected 'postgres://user:pass@host:5432/prod', got %q", result)
	}
}

func TestLoadDatabaseURL_EnvNotSet(t *testing.T) {
	result := LoadDatabaseURL()
	if result != "postgres://localhost:5432/dev" {
		t.Errorf("LoadDatabaseURL: expected 'postgres://localhost:5432/dev', got %q", result)
	}
}

func TestLoadDatabaseURL_EnvEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	result := LoadDatabaseURL()
	if result != "postgres://localhost:5432/dev" {
		t.Errorf("LoadDatabaseURL: expected 'postgres://localhost:5432/dev', got %q", result)
	}
}
