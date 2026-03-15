package merger

import (
	"strings"
	"testing"
)

const existingCode = `package sample

import (
	"testing"
)

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Error("expected 3")
	}
}

func TestSubtract(t *testing.T) {
	if Subtract(5, 3) != 2 {
		t.Error("expected 2")
	}
}
`

const generatedCode = `package sample

import (
	"testing"
	"math"
)

func TestAdd(t *testing.T) {
	// LLM redefined this - should be skipped
	if Add(2, 3) != 5 {
		t.Error("expected 5")
	}
}

func TestMultiply(t *testing.T) {
	if Multiply(3, 4) != 12 {
		t.Error("expected 12")
	}
}

func TestSqrt(t *testing.T) {
	result := math.Sqrt(16)
	if result != 4.0 {
		t.Errorf("expected 4.0, got %f", result)
	}
}
`

func TestMerge_Basic(t *testing.T) {
	result, err := Merge(existingCode, generatedCode)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}

	// TestAdd should be skipped (exists)
	if len(result.Skipped) != 1 || result.Skipped[0] != "TestAdd" {
		t.Errorf("Skipped = %v, want [TestAdd]", result.Skipped)
	}

	// TestMultiply and TestSqrt should be added
	if len(result.Added) != 2 {
		t.Errorf("Added = %v, want 2 functions", result.Added)
	}

	// Result should contain all 4 functions
	if !strings.Contains(result.Code, "TestAdd") {
		t.Error("result should contain TestAdd")
	}
	if !strings.Contains(result.Code, "TestSubtract") {
		t.Error("result should contain TestSubtract")
	}
	if !strings.Contains(result.Code, "TestMultiply") {
		t.Error("result should contain TestMultiply")
	}
	if !strings.Contains(result.Code, "TestSqrt") {
		t.Error("result should contain TestSqrt")
	}

	// math import should be added
	if !strings.Contains(result.Code, `"math"`) {
		t.Error("result should contain math import")
	}

	// Original TestAdd body should be preserved (not LLM version)
	if strings.Contains(result.Code, "expected 5") {
		t.Error("original TestAdd should be preserved, not LLM version")
	}
	if !strings.Contains(result.Code, "expected 3") {
		t.Error("original TestAdd body should be preserved")
	}

	t.Logf("Added: %v", result.Added)
	t.Logf("Skipped: %v", result.Skipped)
}

func TestMerge_EmptyExisting(t *testing.T) {
	result, err := Merge("", generatedCode)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}

	if result.Code != generatedCode {
		t.Error("with empty existing, should return generated as-is")
	}
}

func TestMerge_EmptyGenerated(t *testing.T) {
	result, err := Merge(existingCode, "")
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}

	if result.Code != existingCode {
		t.Error("with empty generated, should return existing as-is")
	}
}

func TestMerge_ImportsUnion(t *testing.T) {
	existing := `package p
import "testing"
func TestA(t *testing.T) {}
`
	generated := `package p
import (
	"testing"
	"fmt"
	"math"
)
func TestB(t *testing.T) {}
`

	result, err := Merge(existing, generated)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}

	if !strings.Contains(result.Code, `"fmt"`) {
		t.Error("result should contain fmt import")
	}
	if !strings.Contains(result.Code, `"math"`) {
		t.Error("result should contain math import")
	}
	if !strings.Contains(result.Code, `"testing"`) {
		t.Error("result should contain testing import")
	}
}

func TestMerge_AllExist(t *testing.T) {
	generated := `package sample
import "testing"
func TestAdd(t *testing.T) {
	// new version
}
func TestSubtract(t *testing.T) {
	// new version
}
`

	result, err := Merge(existingCode, generated)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}

	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want none", result.Added)
	}
	if len(result.Skipped) != 2 {
		t.Errorf("Skipped = %v, want 2", result.Skipped)
	}
}

func TestExtractNewFuncNames(t *testing.T) {
	newFuncs, err := ExtractNewFuncNames(existingCode, generatedCode)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(newFuncs) != 2 {
		t.Errorf("expected 2 new functions, got %d: %v", len(newFuncs), newFuncs)
	}

	expected := map[string]bool{"TestMultiply": true, "TestSqrt": true}
	for _, name := range newFuncs {
		if !expected[name] {
			t.Errorf("unexpected new func: %s", name)
		}
	}
}
