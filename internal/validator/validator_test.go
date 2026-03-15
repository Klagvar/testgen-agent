package validator

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper function to create a temporary directory with a go.mod file
func setupTestDir(t *testing.T) (string, func()) {
	dir, err := ioutil.TempDir("", "validator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer t.Cleanup(func() { os.RemoveAll(dir) })

	goModContent := []byte("module testmod")
	err = ioutil.WriteFile(filepath.Join(dir, "go.mod"), goModContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	return dir, func() { os.RemoveAll(dir) }
}

// Helper function to create a file with content
func createFile(t *testing.T, path string, content []byte) {
	err := ioutil.WriteFile(path, content, 0644)
	if err != nil {
		t.Fatalf("Failed to write file %s: %v", path, err)
	}
}

// TestFindModuleRoot_HappyPath tests finding go.mod in the current directory
func TestFindModuleRoot_HappyPath(t *testing.T) {
	dir, cleanup := setupTestDir(t)
	defer cleanup()

	root := findModuleRoot(dir)
	if root != dir {
		t.Errorf("Expected %s but got %s", dir, root)
	}
}

// TestFindModuleRoot_NoGoMod tests finding no go.mod in the directory
func TestFindModuleRoot_NoGoMod(t *testing.T) {
	dir, cleanup := setupTestDir(t)
	defer cleanup()

	root := findModuleRoot("/")
	if root != "" {
		t.Errorf("Expected empty string but got %s", root)
	}
}

// TestFindModuleRoot_GoModInParent tests finding go.mod in the parent directory
func TestFindModuleRoot_GoModInParent(t *testing.T) {
	parentDir, cleanup := setupTestDir(t)
	defer cleanup()

	testFile := filepath.Join(parentDir, "test")
	createFile(t, testFile, []byte("some content"))

	root := findModuleRoot(testFile)
	if root != parentDir {
		t.Errorf("Expected %s but got %s", parentDir, root)
	}
}

// TestValidate_HappyPath tests successful validation
func TestValidate_HappyPath(t *testing.T) {
	repoDir, cleanup := setupTestDir(t)
	defer cleanup()

	testFile := filepath.Join(repoDir, "test.go")
	createFile(t, testFile, []byte("package main\nfunc TestExample(t *testing.T) {}\n"))

	result := Validate(repoDir, testFile)
	if !result.CompileOK {
		t.Errorf("Expected CompileOK=true but got false")
	}
	if !result.TestsOK {
		t.Errorf("Expected TestsOK=true but got false")
	}
}

// TestValidate_CompileError tests validation with a compile error
func TestValidate_CompileError(t *testing.T) {
	repoDir, cleanup := setupTestDir(t)
	defer cleanup()

	testFile := filepath.Join(repoDir, "test.go")
	createFile(t, testFile, []byte("package main\nfunc TestExample(t testing.T) {}\n"))

	result := Validate(repoDir, testFile)
	if result.CompileOK {
		t.Errorf("Expected CompileOK=false but got true")
	}
	if result.TestsOK {
		t.Errorf("Expected TestsOK=false but got true")
	}
}

// TestValidate_TestError tests a failed test after successful compile
func TestValidate_TestError(t *testing.T) {
	repoDir, cleanup := setupTestDir(t)
	defer cleanup()

	testFile := filepath.Join(repoDir, "test.go")
	createFile(t, testFile, []byte("package main\nimport \"testing\"\nfunc TestExample(t *testing.T) { t.Fail() }\n"))

	result := Validate(repoDir, testFile)
	if !result.CompileOK {
		t.Errorf("Expected CompileOK=true but got false")
	}
	if result.TestsOK {
		t.Errorf("Expected TestsOK=false but got true")
	}
}

// TestRunGoCommand_GoModInCurrent tests running a command with go.mod in the current directory
func TestRunGoCommand_GoModInCurrent(t *testing.T) {
	dir, cleanup := setupTestDir(t)
	defer cleanup()

	output := runGoCommand(dir, dir, "list")
	if output == "" || strings.Contains(output, "module not found") {
		t.Errorf("Expected valid output but got %s", output)
	}
}

// TestRunGoCommand_NoMod tests running a command with no module
func TestRunGoCommand_NoMod(t *testing.T) {
	testFile := filepath.Join(os.TempDir(), "temp.go")
	createFile(t, testFile, []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"Hello\") }\n"))

	output := runGoCommand(os.TempDir(), os.TempDir(), "list")
	if output != "" {
		t.Errorf("Expected empty output but got %s", output)
	}
}

// TestRunGoTest_GoModInCurrent tests running a test with go.mod in the current directory
func TestRunGoTest_GoModInCurrent(t *testing.T) {
	dir, cleanup := setupTestDir(t)
	defer cleanup()

	testFile := filepath.Join(dir, "test.go")
	createFile(t, testFile, []byte("package main\nimport \"testing\"\nfunc TestExample(t *testing.T) { t.Fail() }\n"))

	output, err := runGoTest(dir, dir)
	if output == "" || strings.Contains(output, "PASS") {
		t.Errorf("Expected error output but got %s", output)
	}
	if err == "" {
		t.Errorf("Expected an error message but got empty")
	}
}

// TestRunGoTest_NoMod tests running a test with no module
func TestRunGoTest_NoMod(t *testing.T) {
	testFile := filepath.Join(os.TempDir(), "temp.go")
	createFile(t, testFile, []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"Hello\") }\n"))

	output, err := runGoTest(os.TempDir(), os.TempDir())
	if output != "" || strings.Contains(output, "PASS") {
		t.Errorf("Expected empty output but got %s", output)
	}
	if err == "" {
		t.Errorf("Expected an error message but got empty")
	}
}
