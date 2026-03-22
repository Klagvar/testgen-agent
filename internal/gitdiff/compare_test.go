package gitdiff

import (
	"testing"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

func TestNormalizeBody_IgnoresComments(t *testing.T) {
	body1 := `func Add(a, b int) int {
	// sum of two numbers
	return a + b
}`
	body2 := `func Add(a, b int) int {
	// sum of two numbers (different comment)
	return a + b
}`

	norm1, _ := normalizeBody(body1)
	norm2, _ := normalizeBody(body2)

	if norm1 != norm2 {
		t.Errorf("normalized bodies should be equal when only comments differ\n  body1: %q\n  body2: %q", norm1, norm2)
	}
}

func TestNormalizeBody_DetectsLogicChange(t *testing.T) {
	body1 := `func Add(a, b int) int {
	return a + b
}`
	body2 := `func Add(a, b int) int {
	return a - b
}`

	norm1, _ := normalizeBody(body1)
	norm2, _ := normalizeBody(body2)

	if norm1 == norm2 {
		t.Error("normalized bodies should differ when logic changes")
	}
}

func TestNormalizeBody_IgnoresWhitespace(t *testing.T) {
	body1 := `func Foo() int {
	x := 1
	return x
}`
	body2 := `func Foo() int {
	x := 1

	return x
}`

	norm1, _ := normalizeBody(body1)
	norm2, _ := normalizeBody(body2)

	if norm1 != norm2 {
		t.Errorf("should ignore blank line differences\n  body1: %q\n  body2: %q", norm1, norm2)
	}
}

func TestNormalizeBody_Empty(t *testing.T) {
	norm, _ := normalizeBody("")
	if norm != "" {
		t.Errorf("empty body should normalize to empty, got: %q", norm)
	}
}

func TestParseFunctions(t *testing.T) {
	src := `package calc

func Add(a, b int) int {
	return a + b
}

func Subtract(a, b int) int {
	return a - b
}
`

	funcs, err := parseFunctions(src)
	if err != nil {
		t.Fatalf("parseFunctions error: %v", err)
	}

	if len(funcs) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(funcs))
	}

	if funcs[0].Name != "Add" {
		t.Errorf("expected first function 'Add', got %q", funcs[0].Name)
	}
	if funcs[1].Name != "Subtract" {
		t.Errorf("expected second function 'Subtract', got %q", funcs[1].Name)
	}
}

func TestParseFunctions_WithReceiver(t *testing.T) {
	src := `package calc

type Calculator struct{}

func (c *Calculator) Add(a, b int) int {
	return a + b
}

func FreeFunc() {}
`

	funcs, err := parseFunctions(src)
	if err != nil {
		t.Fatalf("parseFunctions error: %v", err)
	}

	if len(funcs) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(funcs))
	}

	if funcs[0].Name != "Add" || funcs[0].Receiver != "*Calculator" {
		t.Errorf("expected Add with *Calculator receiver, got %q with %q", funcs[0].Name, funcs[0].Receiver)
	}
}

func TestFuncKey(t *testing.T) {
	tests := []struct {
		name     string
		fn       analyzer.FuncInfo
		expected string
	}{
		{
			name:     "free function",
			fn:       analyzer.FuncInfo{Name: "Add"},
			expected: "Add",
		},
		{
			name:     "method with pointer receiver",
			fn:       analyzer.FuncInfo{Name: "Process", Receiver: "*Service"},
			expected: "*Service.Process",
		},
		{
			name:     "method with value receiver",
			fn:       analyzer.FuncInfo{Name: "String", Receiver: "MyType"},
			expected: "MyType.String",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := funcKey(tt.fn)
			if got != tt.expected {
				t.Errorf("funcKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFilterChanged_AllNew(t *testing.T) {
	// Simulating: base content is empty (new file)
	funcs := []analyzer.FuncInfo{
		{Name: "Foo", Body: "func Foo() {}"},
		{Name: "Bar", Body: "func Bar() {}"},
	}

	// We can't easily call FilterChanged without a git repo,
	// but we can test the logic by directly using parseFunctions + comparison

	// Test with empty base
	result := filterChangedWithBase(funcs, "")

	if len(result.New) != 2 {
		t.Errorf("expected 2 new functions, got %d", len(result.New))
	}
	if len(result.Changed) != 0 {
		t.Errorf("expected 0 changed functions, got %d", len(result.Changed))
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("expected 0 unchanged functions, got %d", len(result.Unchanged))
	}
}

func TestFilterChanged_SomeUnchanged(t *testing.T) {
	baseSrc := `package calc

func Add(a, b int) int {
	return a + b
}

func Subtract(a, b int) int {
	return a - b
}
`

	affected := []analyzer.FuncInfo{
		{
			Name: "Add",
			Body: `func Add(a, b int) int {
	return a + b
}`,
		},
		{
			Name: "Subtract",
			Body: `func Subtract(a, b int) int {
	return a * b
}`,
		},
	}

	result := filterChangedWithBase(affected, baseSrc)

	if len(result.Unchanged) != 1 || result.Unchanged[0].Name != "Add" {
		t.Errorf("expected Add to be unchanged, got %v", namesOf(result.Unchanged))
	}
	if len(result.Changed) != 1 || result.Changed[0].Name != "Subtract" {
		t.Errorf("expected Subtract to be changed, got %v", namesOf(result.Changed))
	}
}

func TestFilterChanged_OnlyCommentChanged(t *testing.T) {
	baseSrc := `package calc

func Add(a, b int) int {
	// old comment
	return a + b
}
`

	affected := []analyzer.FuncInfo{
		{
			Name: "Add",
			Body: `func Add(a, b int) int {
	// brand new comment with lots of detail
	return a + b
}`,
		},
	}

	result := filterChangedWithBase(affected, baseSrc)

	if len(result.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged (comment-only change), got %d unchanged, %d changed",
			len(result.Unchanged), len(result.Changed))
	}
}

func TestFilterChanged_NewFunction(t *testing.T) {
	baseSrc := `package calc

func Add(a, b int) int {
	return a + b
}
`

	affected := []analyzer.FuncInfo{
		{
			Name: "Add",
			Body: `func Add(a, b int) int {
	return a + b
}`,
		},
		{
			Name: "Multiply",
			Body: `func Multiply(a, b int) int {
	return a * b
}`,
		},
	}

	result := filterChangedWithBase(affected, baseSrc)

	if len(result.Unchanged) != 1 || result.Unchanged[0].Name != "Add" {
		t.Errorf("expected Add unchanged, got %v", namesOf(result.Unchanged))
	}
	if len(result.New) != 1 || result.New[0].Name != "Multiply" {
		t.Errorf("expected Multiply as new, got %v", namesOf(result.New))
	}
}

func TestNormalizeString(t *testing.T) {
	input := `  func Foo() {
	// comment
	x := 1

	return x
  }`

	result := normalizeString(input)

	if result == "" {
		t.Error("normalizeString should not return empty for valid input")
	}

	// Should not contain comment
	if contains(result, "// comment") {
		t.Error("normalizeString should strip comments")
	}
}

// --- helpers ---

// filterChangedWithBase is a test wrapper: instead of calling git, it accepts baseSrc directly.
func filterChangedWithBase(affectedFuncs []analyzer.FuncInfo, baseSrc string) *CompareResult {
	result := &CompareResult{}

	if baseSrc == "" {
		result.New = affectedFuncs
		return result
	}

	baseFuncs, err := parseFunctions(baseSrc)
	if err != nil {
		result.Changed = affectedFuncs
		return result
	}

	baseIndex := make(map[string]string)
	for _, fn := range baseFuncs {
		key := funcKey(fn)
		body, _ := normalizeBody(fn.Body)
		baseIndex[key] = body
	}

	for _, fn := range affectedFuncs {
		key := funcKey(fn)
		baseBody, exists := baseIndex[key]

		if !exists {
			result.New = append(result.New, fn)
			continue
		}

		currentBody, _ := normalizeBody(fn.Body)

		if currentBody == baseBody {
			result.Unchanged = append(result.Unchanged, fn)
		} else {
			result.Changed = append(result.Changed, fn)
		}
	}

	return result
}

func namesOf(funcs []analyzer.FuncInfo) []string {
	names := make([]string, len(funcs))
	for i, f := range funcs {
		names[i] = f.Name
	}
	return names
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
