package prompt

import (
	"strings"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

func sampleRequest() TestGenRequest {
	return TestGenRequest{
		PackageName: "calc",
		FilePath:    "calc.go",
		Imports:     []string{"errors", "math"},
		TargetFuncs: []analyzer.FuncInfo{
			{
				Name:       "Add",
				Signature:  "func Add(a int, b int) (int, error)",
				Params:     []analyzer.Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
				Returns:    []string{"int", "error"},
				DocComment: "Add sums two numbers. Supports overflow checking.\n",
				Body: `func Add(a, b int) (int, error) {
	result := a + b
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, errors.New("integer overflow")
	}
	return result, nil
}`,
				StartLine: 9,
				EndLine:   15,
			},
			{
				Name:       "Multiply",
				Signature:  "func Multiply(a int, b int) int",
				Params:     []analyzer.Param{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
				Returns:    []string{"int"},
				DocComment: "Multiply multiplies two numbers.\n",
				Body: `func Multiply(a, b int) int {
	return a * b
}`,
				StartLine: 31,
				EndLine:   33,
			},
			{
				Name:      "Sqrt",
				Signature: "func Sqrt(x float64) (float64, error)",
				Params:    []analyzer.Param{{Name: "x", Type: "float64"}},
				Returns:   []string{"float64", "error"},
				Body: `func Sqrt(x float64) (float64, error) {
	if x < 0 {
		return 0, errors.New("negative number")
	}
	return math.Sqrt(x), nil
}`,
				StartLine: 36,
				EndLine:   41,
			},
		},
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	sys := BuildSystemPrompt()

	if sys == "" {
		t.Fatal("system prompt is empty")
	}

	checks := []string{
		"table-driven",
		"testing",
		"Boundary",
		"t.Errorf",
		"package",
	}
	for _, check := range checks {
		if !strings.Contains(sys, check) {
			t.Errorf("system prompt does not contain %q", check)
		}
	}

	t.Logf("System prompt (%d chars):\n%s", len(sys), sys)
}

func TestBuildUserPrompt_ContainsFunctions(t *testing.T) {
	req := sampleRequest()
	prompt := BuildUserPrompt(req)

	for _, fn := range req.TargetFuncs {
		if !strings.Contains(prompt, fn.Name) {
			t.Errorf("prompt does not contain function %q", fn.Name)
		}
		if !strings.Contains(prompt, fn.Signature) {
			t.Errorf("prompt does not contain signature %q", fn.Signature)
		}
	}

	if !strings.Contains(prompt, "calc") {
		t.Error("prompt does not contain package name")
	}

	if !strings.Contains(prompt, "errors") || !strings.Contains(prompt, "math") {
		t.Error("prompt does not contain imports")
	}

	t.Logf("User prompt (%d chars)", len(prompt))
}

func TestBuildUserPrompt_ContainsBody(t *testing.T) {
	req := sampleRequest()
	prompt := BuildUserPrompt(req)

	if !strings.Contains(prompt, "result := a + b") {
		t.Error("prompt does not contain Add body")
	}
	if !strings.Contains(prompt, "return a * b") {
		t.Error("prompt does not contain Multiply body")
	}
	if !strings.Contains(prompt, "math.Sqrt(x)") {
		t.Error("prompt does not contain Sqrt body")
	}
}

func TestBuildUserPrompt_ContainsBranches(t *testing.T) {
	req := sampleRequest()
	prompt := BuildUserPrompt(req)

	if !strings.Contains(prompt, "Condition") {
		t.Error("prompt does not contain branch analysis for Add")
	}

	if !strings.Contains(prompt, "x < 0") {
		t.Error("prompt does not contain condition x < 0 for Sqrt")
	}
}

func TestBuildUserPrompt_ContainsDocComment(t *testing.T) {
	req := sampleRequest()
	prompt := BuildUserPrompt(req)

	if !strings.Contains(prompt, "overflow") {
		t.Error("prompt does not contain Add documentation")
	}
	if !strings.Contains(prompt, "multiplies") {
		t.Error("prompt does not contain Multiply documentation")
	}
}

func TestBuildUserPrompt_WithExistingTestNames(t *testing.T) {
	req := sampleRequest()
	req.ExistingTestNames = []string{"TestAdd", "TestMultiply_Basic"}
	prompt := BuildUserPrompt(req)

	if !strings.Contains(prompt, "Existing Tests") {
		t.Error("prompt does not contain existing tests section")
	}
	if !strings.Contains(prompt, "DO NOT REDECLARE") {
		t.Error("prompt does not contain redeclaration warning")
	}
	if !strings.Contains(prompt, "TestAdd") {
		t.Error("prompt does not contain existing test name TestAdd")
	}
	if !strings.Contains(prompt, "TestMultiply_Basic") {
		t.Error("prompt does not contain existing test name TestMultiply_Basic")
	}
	if strings.Contains(prompt, "// existing test") {
		t.Error("prompt should not contain full test body")
	}
}

func TestBuildUserPrompt_WithoutExistingTests(t *testing.T) {
	req := sampleRequest()
	req.ExistingTestNames = nil
	prompt := BuildUserPrompt(req)

	if strings.Contains(prompt, "Existing Tests") {
		t.Error("prompt contains existing tests section when there are none")
	}
}

func TestExtractTestFuncNames(t *testing.T) {
	src := `package foo_test

import "testing"

func TestA(t *testing.T) {}
func TestB(t *testing.T) {}
func helperFunc() {}
`
	names := ExtractTestFuncNames(src)
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d: %v", len(names), names)
	}
	if names[0] != "TestA" || names[1] != "TestB" || names[2] != "helperFunc" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestExtractTestFuncNames_Empty(t *testing.T) {
	names := ExtractTestFuncNames("")
	if names != nil {
		t.Errorf("expected nil for empty input, got %v", names)
	}
}

func TestBuildMessages(t *testing.T) {
	req := sampleRequest()
	msgs := BuildMessages(req)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	if msgs[0].Role != "system" {
		t.Errorf("first message should be system, got %q", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("second message should be user, got %q", msgs[1].Role)
	}

	if msgs[0].Content == "" || msgs[1].Content == "" {
		t.Error("message content must not be empty")
	}
}

func TestAnalyzeBranches(t *testing.T) {
	body := `func Example() {
	if x > 0 {
		doSomething()
	} else if x == 0 {
		doNothing()
	} else {
		doOther()
	}
	switch mode {
	case "fast":
		fast()
	case "slow":
		slow()
	}
	if err != nil {
		return err
	}
}`

	branches := analyzeBranches(body)
	t.Logf("Found branches: %v", branches)

	if len(branches) < 5 {
		t.Errorf("expected >= 5 branches, got %d", len(branches))
	}

	hasIf := false
	hasElseIf := false
	hasElse := false
	hasSwitch := false
	hasCase := false
	hasErrCheck := false

	for _, b := range branches {
		if strings.HasPrefix(b, "Condition:") {
			hasIf = true
		}
		if strings.HasPrefix(b, "Else-if:") {
			hasElseIf = true
		}
		if b == "Else branch" {
			hasElse = true
		}
		if b == "Switch statement" {
			hasSwitch = true
		}
		if strings.HasPrefix(b, "Case:") {
			hasCase = true
		}
		if strings.Contains(b, "err != nil") {
			hasErrCheck = true
		}
	}

	if !hasIf {
		t.Error("condition if not found")
	}
	if !hasElseIf {
		t.Error("else if not found")
	}
	if !hasElse {
		t.Error("else branch not found")
	}
	if !hasSwitch {
		t.Error("switch not found")
	}
	if !hasCase {
		t.Error("case not found")
	}
	if !hasErrCheck {
		t.Error("error check not found")
	}
}
