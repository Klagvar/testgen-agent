package validator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempPackage creates a minimal Go package in a temp directory and
// returns the path to the created test file.
func writeTempPackage(t *testing.T, srcFiles map[string]string) (moduleDir, testFile string) {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	var tf string
	for name, content := range srcFiles {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if strings.HasSuffix(name, "_test.go") {
			tf = p
		}
	}
	return dir, tf
}

func hasGo(t *testing.T) {
	t.Helper()
	if err := exec.Command("go", "version").Run(); err != nil {
		t.Skip("go toolchain not available")
	}
}

func TestValidate_AllPass(t *testing.T) {
	hasGo(t)

	src := `package testmod

func Add(a, b int) int { return a + b }
`
	test := `package testmod

import "testing"

func TestAdd_JSONBasic(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("1+2 should be 3")
	}
}

func TestAdd_JSONTableDriven(t *testing.T) {
	cases := []struct {
		name    string
		a, b, w int
	}{
		{"zeros", 0, 0, 0},
		{"positive", 2, 3, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Add(c.a, c.b); got != c.w {
				t.Errorf("got %d, want %d", got, c.w)
			}
		})
	}
}
`
	dir, tf := writeTempPackage(t, map[string]string{
		"add.go":      src,
		"add_test.go": test,
	})

	res := Validate(dir, tf)

	if !res.CompileOK {
		t.Fatalf("expected CompileOK, got %q", res.CompileError)
	}
	if !res.TestsOK {
		t.Fatalf("expected TestsOK, got: %s", res.TestError)
	}
	// Top-level TestAdd_JSONTableDriven plus its two sub-tests plus TestAdd_JSONBasic == 4.
	if res.Passed < 3 {
		t.Errorf("expected at least 3 passing tests, got %d (output:\n%s)", res.Passed, res.TestOutput)
	}
	if res.Failed != 0 {
		t.Errorf("expected 0 failing tests, got %d", res.Failed)
	}
	// TestOutput must be a JSON stream (contains events), not plain -v text.
	if !strings.Contains(res.TestOutput, `"Action"`) {
		t.Errorf("expected TestOutput to be a go test -json stream, got:\n%s", res.TestOutput)
	}
}

func TestValidate_TestFailurePopulatesCounts(t *testing.T) {
	hasGo(t)

	src := `package testmod

func Mul(a, b int) int { return a * b }
`
	test := `package testmod

import "testing"

func TestMul_OK(t *testing.T) {
	if Mul(2, 3) != 6 {
		t.Error("wrong")
	}
}

func TestMul_Broken(t *testing.T) {
	if Mul(2, 3) != 99 {
		t.Errorf("expected 99, got %d", Mul(2, 3))
	}
}
`
	dir, tf := writeTempPackage(t, map[string]string{
		"mul.go":      src,
		"mul_test.go": test,
	})

	res := Validate(dir, tf)

	if !res.CompileOK {
		t.Fatalf("expected CompileOK, got %q", res.CompileError)
	}
	if res.TestsOK {
		t.Fatal("expected TestsOK=false for a failing test")
	}
	if res.Passed != 1 {
		t.Errorf("expected 1 passing test, got %d", res.Passed)
	}
	if res.Failed < 1 {
		t.Errorf("expected at least 1 failing test, got %d", res.Failed)
	}
	if !strings.Contains(res.TestError, "TestMul_Broken") {
		t.Errorf("TestError should mention failing test name, got: %s", res.TestError)
	}
}

func TestValidate_CompileError(t *testing.T) {
	hasGo(t)

	src := `package testmod

func Bad() { notDefined() }
`
	test := `package testmod

import "testing"

func TestBad(t *testing.T) { Bad() }
`
	dir, tf := writeTempPackage(t, map[string]string{
		"bad.go":      src,
		"bad_test.go": test,
	})

	res := Validate(dir, tf)

	if res.CompileOK {
		t.Fatal("expected compile error, got CompileOK")
	}
	if res.CompileError == "" {
		t.Error("CompileError should be populated")
	}
}

func TestExtractTestErrors_JSONStream(t *testing.T) {
	stream := `{"Action":"run","Test":"TestA"}
{"Action":"output","Test":"TestA","Output":"    a_test.go:12: expected 1, got 2\n"}
{"Action":"fail","Test":"TestA","Elapsed":0.01}
`
	got := extractTestErrors(stream)
	if !strings.Contains(got, "TestA") {
		t.Errorf("expected failing test name in summary, got %q", got)
	}
	if !strings.Contains(got, "expected 1, got 2") {
		t.Errorf("expected failing message in summary, got %q", got)
	}
}

func TestExtractTestErrors_FallbackText(t *testing.T) {
	text := `=== RUN   TestX
--- FAIL: TestX (0.00s)
    foo_test.go:7: bad
FAIL
`
	got := extractTestErrors(text)
	if !strings.Contains(got, "--- FAIL: TestX") {
		t.Errorf("legacy fallback should extract --- FAIL lines, got %q", got)
	}
}

