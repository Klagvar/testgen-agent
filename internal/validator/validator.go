// Package validator verifies generated tests:
// compilation, execution, and error analysis.
package validator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Result holds the test validation outcome.
type Result struct {
	// Compilation
	CompileOK    bool   // whether it compiled
	CompileError string // compilation error (if any)

	// Test execution
	TestsOK    bool   // all tests passed
	TestOutput string // go test output
	TestError  string // test run errors

	// Race detector
	HasRaces    bool   // data races detected
	RaceDetails string // data race details

	// Statistics
	Passed   int           // number of passing tests
	Failed   int           // number of failing tests
	Duration time.Duration // execution time
}

// IsValid returns true if tests compile and pass.
func (r *Result) IsValid() bool {
	return r.CompileOK && r.TestsOK
}

// Summary returns a brief description of the result.
func (r *Result) Summary() string {
	if !r.CompileOK {
		return fmt.Sprintf("❌ Compilation error:\n%s", r.CompileError)
	}
	if !r.TestsOK {
		return fmt.Sprintf("⚠️  Tests failed (%d passed, %d failed):\n%s", r.Passed, r.Failed, r.TestError)
	}
	if r.HasRaces {
		return fmt.Sprintf("⚠️  Tests passed but DATA RACE detected (%d passed, %s):\n%s",
			r.Passed, r.Duration, r.RaceDetails)
	}
	return fmt.Sprintf("✅ All tests passed (%d passed, %s)", r.Passed, r.Duration)
}

// FormatFile runs goimports on a file to auto-fix imports.
// Falls back to go fmt if goimports is not installed.
func FormatFile(filePath string) error {
	// Try goimports (fixes unused and missing imports)
	cmd := exec.Command("goimports", "-w", filePath)
	if err := cmd.Run(); err != nil {
		// Fallback to go fmt (at least formats)
		cmd = exec.Command("go", "fmt", filePath)
		return cmd.Run()
	}
	return nil
}

// findModuleRoot finds the nearest go.mod upward from the directory.
// Returns the directory containing go.mod or an empty string.
func findModuleRoot(dir string) string {
	current := dir
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

// Validate checks a generated test file.
// repoDir is the repository path, testFile is the test file path.
func Validate(repoDir string, testFile string) *Result {
	result := &Result{}
	start := time.Now()

	// Step 0: Auto-format (goimports fixes imports automatically)
	_ = FormatFile(testFile)

	// Determine the test file directory
	testDir := filepath.Dir(testFile)

	// Find the Go module root for this test file.
	// If testdata/sample-project has its own go.mod, use that
	// instead of the main project's go.mod.
	moduleRoot := findModuleRoot(testDir)
	if moduleRoot == "" {
		moduleRoot = repoDir
	}

	// Step 1: Check compilation
	compileErr := runGoCommand(moduleRoot, testDir, "build")
	if compileErr != "" {
		result.CompileOK = false
		result.CompileError = compileErr
		result.Duration = time.Since(start)
		return result
	}
	result.CompileOK = true

	// Step 2: Run tests
	testOutput, testErr := runGoTest(moduleRoot, testDir)
	result.TestOutput = testOutput
	result.Duration = time.Since(start)

	if testErr == "" {
		result.TestsOK = true
		result.Passed = countTests(testOutput, "PASS")
		return result
	}

	// Parse failing test results
	result.TestsOK = false
	result.TestError = testErr
	result.Passed = countTests(testOutput, "PASS")
	result.Failed = countTests(testOutput, "FAIL")

	return result
}

// ValidateWithRace checks a test file with the data race detector enabled.
// Runs go test -race -v.
func ValidateWithRace(repoDir string, testFile string) *Result {
	result := &Result{}
	start := time.Now()

	_ = FormatFile(testFile)

	testDir := filepath.Dir(testFile)
	moduleRoot := findModuleRoot(testDir)
	if moduleRoot == "" {
		moduleRoot = repoDir
	}

	// Compile check
	compileErr := runGoCommand(moduleRoot, testDir, "build")
	if compileErr != "" {
		result.CompileOK = false
		result.CompileError = compileErr
		result.Duration = time.Since(start)
		return result
	}
	result.CompileOK = true

	// Run tests with -race
	testOutput, testErr := runGoTestRace(moduleRoot, testDir)
	result.TestOutput = testOutput
	result.Duration = time.Since(start)

	// Check for data races
	if strings.Contains(testOutput, "WARNING: DATA RACE") {
		result.HasRaces = true
		result.RaceDetails = extractRaceDetails(testOutput)
	}

	if testErr == "" {
		result.TestsOK = true
		result.Passed = countTests(testOutput, "PASS")
		return result
	}

	result.TestsOK = false
	result.TestError = testErr
	result.Passed = countTests(testOutput, "PASS")
	result.Failed = countTests(testOutput, "FAIL")

	return result
}

// runGoTestRace runs go test -race and returns the output.
func runGoTestRace(moduleRoot, pkgDir string) (output string, errMsg string) {
	if !isRaceSupported() {
		return "", "race detector unavailable (CGO disabled or unsupported platform)"
	}

	relPkg, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		relPkg = "."
	}
	pkgPath := "./" + filepath.ToSlash(relPkg)
	if pkgPath == "./" {
		pkgPath = "."
	}

	cmd := exec.Command("go", "test", "-race", "-v", "-count=1", "-timeout", "60s", pkgPath)
	cmd.Dir = moduleRoot

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if err != nil {
		return outputStr, extractTestErrors(outputStr)
	}
	return outputStr, ""
}

func isRaceSupported() bool {
	cmd := exec.Command("go", "env", "CGO_ENABLED")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}

// extractRaceDetails extracts data race information from go test -race output.
func extractRaceDetails(output string) string {
	var details []string
	lines := strings.Split(output, "\n")
	inRace := false

	for _, line := range lines {
		if strings.Contains(line, "WARNING: DATA RACE") {
			inRace = true
		}
		if inRace {
			details = append(details, line)
			// Race block ends with empty line or goroutine info
			if strings.TrimSpace(line) == "" && len(details) > 3 {
				inRace = false
			}
		}
	}

	if len(details) > 30 {
		details = details[:30]
		details = append(details, "... (truncated)")
	}

	return strings.Join(details, "\n")
}

// runGoCommand runs go <command> in the specified directory.
func runGoCommand(moduleRoot, pkgDir, command string) string {
	// Determine relative package path from module root
	relPkg, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		relPkg = "."
	}
	// Convert to Go package format: ./path/to/pkg
	pkgPath := "./" + filepath.ToSlash(relPkg)
	if pkgPath == "./" {
		pkgPath = "."
	}

	cmd := exec.Command("go", command, pkgPath)
	cmd.Dir = moduleRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output))
	}
	return ""
}

// runGoTest runs go test and returns output and error.
func runGoTest(moduleRoot, pkgDir string) (output string, errMsg string) {
	relPkg, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		relPkg = "."
	}
	pkgPath := "./" + filepath.ToSlash(relPkg)
	if pkgPath == "./" {
		pkgPath = "."
	}

	cmd := exec.Command("go", "test", "-v", "-count=1", "-timeout", "30s", pkgPath)
	cmd.Dir = moduleRoot

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if err != nil {
		return outputStr, extractTestErrors(outputStr)
	}
	return outputStr, ""
}

// extractTestErrors extracts error messages from go test output.
func extractTestErrors(output string) string {
	var errors []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Compilation errors
		if strings.Contains(trimmed, ": ") && (strings.Contains(trimmed, ".go:") || strings.Contains(trimmed, "cannot") || strings.Contains(trimmed, "undefined")) {
			errors = append(errors, trimmed)
		}
		// Failing tests
		if strings.HasPrefix(trimmed, "--- FAIL:") {
			errors = append(errors, trimmed)
		}
		// t.Errorf / t.Fatalf output
		if strings.Contains(trimmed, "Error Trace:") || strings.Contains(trimmed, "Error:") {
			errors = append(errors, trimmed)
		}
		// Direct test errors
		if strings.HasPrefix(trimmed, "FAIL") {
			errors = append(errors, trimmed)
		}
	}

	if len(errors) == 0 {
		return output // Return full output if we couldn't parse it
	}

	return strings.Join(errors, "\n")
}

// countTests counts the number of tests with the given status in go test -v output.
func countTests(output, status string) int {
	count := 0
	prefix := "--- " + status + ":"
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(strings.TrimSpace(line), prefix) {
			count++
		}
	}
	return count
}
