package mutation

import (
	"strings"
	"testing"
)

const sampleSrc = `package calc

import "errors"

func Add(a, b int) (int, error) {
	result := a + b
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, errors.New("integer overflow")
	}
	return result, nil
}

func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
`

func TestGenerateMutants(t *testing.T) {
	mutants, err := GenerateMutants(sampleSrc, "calc.go", nil)
	if err != nil {
		t.Fatalf("GenerateMutants error: %v", err)
	}

	if len(mutants) == 0 {
		t.Fatal("expected at least some mutants")
	}

	t.Logf("Generated %d mutants:", len(mutants))
	for _, m := range mutants {
		t.Logf("  #%d [%s] %s:%d  %s → %s  (func %s)",
			m.ID, m.Type, m.File, m.Line, m.Original, m.Replacement, m.FuncName)
	}

	// Should have arithmetic mutants for + and /
	hasArithmetic := false
	hasComparison := false
	hasLogical := false

	for _, m := range mutants {
		switch m.Type {
		case MutArithmetic:
			hasArithmetic = true
		case MutComparison:
			hasComparison = true
		case MutLogical:
			hasLogical = true
		}
	}

	if !hasArithmetic {
		t.Error("expected arithmetic mutants (+ ↔ -)")
	}
	if !hasComparison {
		t.Error("expected comparison mutants (< ↔ <=, == ↔ !=)")
	}
	if !hasLogical {
		t.Error("expected logical mutants (&& ↔ ||)")
	}
}

func TestGenerateMutants_FilterByFunc(t *testing.T) {
	mutants, err := GenerateMutants(sampleSrc, "calc.go", []string{"Divide"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	for _, m := range mutants {
		if m.FuncName != "Divide" {
			t.Errorf("mutant #%d is in func %s, expected only Divide", m.ID, m.FuncName)
		}
	}

	if len(mutants) == 0 {
		t.Error("expected at least one mutant for Divide")
	}

	t.Logf("Divide mutants: %d", len(mutants))
}

func TestApplyMutant_Arithmetic(t *testing.T) {
	m := &Mutant{
		ID:          1,
		Type:        MutArithmetic,
		File:        "calc.go",
		Line:        6, // result := a + b
		Original:    "+",
		Replacement: "-",
		FuncName:    "Add",
	}

	mutated, err := applyMutant(sampleSrc, m)
	if err != nil {
		t.Fatalf("applyMutant error: %v", err)
	}

	// Mutated code should have - instead of +
	if !strings.Contains(mutated, "a - b") {
		t.Error("mutated code should contain 'a - b'")
	}
	if strings.Contains(mutated, "a + b") {
		t.Error("mutated code should NOT contain 'a + b'")
	}

	t.Logf("Mutated:\n%s", mutated)
}

func TestApplyMutant_Comparison(t *testing.T) {
	m := &Mutant{
		ID:          1,
		Type:        MutComparison,
		File:        "calc.go",
		Line:        14, // if b == 0
		Original:    "==",
		Replacement: "!=",
		FuncName:    "Divide",
	}

	mutated, err := applyMutant(sampleSrc, m)
	if err != nil {
		t.Fatalf("applyMutant error: %v", err)
	}

	if !strings.Contains(mutated, "b != 0") {
		t.Error("mutated code should contain 'b != 0'")
	}

	t.Logf("Mutated:\n%s", mutated)
}

func TestFormatResult(t *testing.T) {
	r := &Result{
		Mutants: []Mutant{
			{ID: 1, Type: MutArithmetic, FuncName: "Add", Line: 6, Original: "+", Replacement: "-", Killed: true},
			{ID: 2, Type: MutComparison, FuncName: "Divide", Line: 14, Original: "==", Replacement: "!=", Killed: true},
			{ID: 3, Type: MutComparison, FuncName: "Abs", Line: 20, Original: "<", Replacement: "<=", Killed: false},
			{ID: 4, Type: MutLogical, FuncName: "Add", Line: 7, Original: "&&", Replacement: "||", Killed: true},
		},
		Total:         4,
		Killed:        3,
		Survived:      1,
		MutationScore: 75.0,
	}

	md := FormatResult(r)
	t.Logf("Report:\n%s", md)

	if !strings.Contains(md, "75.0%") {
		t.Error("should contain mutation score")
	}
	if !strings.Contains(md, "Survived Mutants") {
		t.Error("should list survived mutants")
	}
	if !strings.Contains(md, "Abs") {
		t.Error("should mention Abs as survived")
	}
}

const returnSrc = `package example

import "errors"

func GetError() error {
	return nil
}

func GetNum() int {
	return 0
}

func GetFlag() bool {
	return true
}

func GetName() string {
	return ""
}

func GetOne() int {
	return 1
}

var _ = errors.New
`

func TestGenerateMutants_ReturnNil(t *testing.T) {
	mutants, err := GenerateMutants(returnSrc, "example.go", []string{"GetError"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	found := false
	for _, m := range mutants {
		if m.Type == MutReturn && m.Original == "nil" {
			found = true
			if m.Replacement != `errors.New("mutant")` {
				t.Errorf("expected replacement errors.New(\"mutant\"), got %s", m.Replacement)
			}
		}
	}
	if !found {
		t.Error("expected MutReturn mutant for return nil")
	}
}

func TestGenerateMutants_ReturnZero(t *testing.T) {
	mutants, err := GenerateMutants(returnSrc, "example.go", []string{"GetNum"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	found := false
	for _, m := range mutants {
		if m.Type == MutReturn && m.Original == "0" && m.Replacement == "1" {
			found = true
		}
	}
	if !found {
		t.Error("expected MutReturn mutant for return 0 → 1")
	}
}

func TestGenerateMutants_ReturnTrue(t *testing.T) {
	mutants, err := GenerateMutants(returnSrc, "example.go", []string{"GetFlag"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	found := false
	for _, m := range mutants {
		if m.Type == MutReturn && m.Original == "true" && m.Replacement == "false" {
			found = true
		}
	}
	if !found {
		t.Error("expected MutReturn mutant for return true → false")
	}
}

func TestGenerateMutants_ReturnEmptyString(t *testing.T) {
	mutants, err := GenerateMutants(returnSrc, "example.go", []string{"GetName"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	found := false
	for _, m := range mutants {
		if m.Type == MutReturn && m.Original == `""` && m.Replacement == `"mutant"` {
			found = true
		}
	}
	if !found {
		t.Error(`expected MutReturn mutant for return "" → "mutant"`)
	}
}

func TestApplyMutant_ReturnNil(t *testing.T) {
	m := &Mutant{
		ID:          1,
		Type:        MutReturn,
		File:        "example.go",
		Line:        6,
		Original:    "nil",
		Replacement: `errors.New("mutant")`,
		FuncName:    "GetError",
	}

	mutated, err := applyMutant(returnSrc, m)
	if err != nil {
		t.Fatalf("applyMutant error: %v", err)
	}

	if !strings.Contains(mutated, `errors.New("mutant")`) {
		t.Error(`mutated code should contain errors.New("mutant")`)
	}
	t.Logf("Mutated:\n%s", mutated)
}

func TestApplyMutant_ReturnZero(t *testing.T) {
	m := &Mutant{
		ID:          1,
		Type:        MutReturn,
		File:        "example.go",
		Line:        10,
		Original:    "0",
		Replacement: "1",
		FuncName:    "GetNum",
	}

	mutated, err := applyMutant(returnSrc, m)
	if err != nil {
		t.Fatalf("applyMutant error: %v", err)
	}

	if !strings.Contains(mutated, "return 1") {
		t.Error("mutated code should contain 'return 1'")
	}
	t.Logf("Mutated:\n%s", mutated)
}

func TestStringToToken(t *testing.T) {
	tests := map[string]bool{
		"+": true, "-": true, "*": true, "/": true,
		"<": true, "<=": true, ">": true, ">=": true,
		"==": true, "!=": true, "&&": true, "||": true,
		"??": false,
	}

	for s, valid := range tests {
		tok := stringToToken(s)
		if valid && tok.String() != s {
			t.Errorf("stringToToken(%q) = %s, want %s", s, tok, s)
		}
	}
}
