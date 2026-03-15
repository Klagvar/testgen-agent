package dedup

import (
	"strings"
	"testing"
)

func TestDedup_NoDuplicates(t *testing.T) {
	code := `package calc_test

import "testing"

func TestAdd(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{name: "positive", a: 1, b: 2, want: 3},
		{name: "negative", a: -1, b: -2, want: -3},
		{name: "zero", a: 0, b: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Add(tt.a, tt.b); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}
`

	result, err := Dedup(code)
	if err != nil {
		t.Fatalf("Dedup error: %v", err)
	}

	if result.Removed != 0 {
		t.Errorf("expected 0 removed, got %d", result.Removed)
	}
}

func TestDedup_WithDuplicates(t *testing.T) {
	code := `package calc_test

import "testing"

func TestAdd(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{name: "positive", a: 1, b: 2, want: 3},
		{name: "also positive", a: 1, b: 2, want: 3},
		{name: "negative", a: -1, b: -2, want: -3},
		{name: "same as negative", a: -1, b: -2, want: -3},
		{name: "zero", a: 0, b: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Add(tt.a, tt.b); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}
`

	result, err := Dedup(code)
	if err != nil {
		t.Fatalf("Dedup error: %v", err)
	}

	if result.Removed != 2 {
		t.Errorf("expected 2 removed, got %d", result.Removed)
	}

	// Check that deduplicated code still has the unique cases
	if !strings.Contains(result.Code, "positive") {
		t.Error("should keep 'positive' case")
	}
	if !strings.Contains(result.Code, "negative") {
		t.Error("should keep 'negative' case")
	}
	if !strings.Contains(result.Code, "zero") {
		t.Error("should keep 'zero' case")
	}

	// Should not have "also positive" or "same as negative"
	if strings.Contains(result.Code, "also positive") {
		t.Error("should remove 'also positive' duplicate")
	}
	if strings.Contains(result.Code, "same as negative") {
		t.Error("should remove 'same as negative' duplicate")
	}
}

func TestDedup_DifferentNames_SameValues(t *testing.T) {
	code := `package calc_test

import "testing"

func TestMultiply(t *testing.T) {
	tests := []struct {
		name string
		x    int
		y    int
		want int
	}{
		{name: "two times three", x: 2, y: 3, want: 6},
		{name: "2*3", x: 2, y: 3, want: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.x * tt.y
		})
	}
}
`

	result, err := Dedup(code)
	if err != nil {
		t.Fatalf("Dedup error: %v", err)
	}

	if result.Removed != 1 {
		t.Errorf("expected 1 removed (same values, different name), got %d", result.Removed)
	}
}

func TestDedup_NoTableDriven(t *testing.T) {
	code := `package calc_test

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(1, 2); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}
`

	result, err := Dedup(code)
	if err != nil {
		t.Fatalf("Dedup error: %v", err)
	}

	if result.Removed != 0 {
		t.Errorf("expected 0 removed for non-table test, got %d", result.Removed)
	}
}

func TestDedup_MultipleFunctions(t *testing.T) {
	code := `package calc_test

import "testing"

func TestAdd(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{name: "case1", a: 1, b: 2, want: 3},
		{name: "case1_dup", a: 1, b: 2, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}

func TestSub(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{name: "case1", a: 5, b: 3, want: 2},
		{name: "case1_dup", a: 5, b: 3, want: 2},
		{name: "case2", a: 10, b: 4, want: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}
`

	result, err := Dedup(code)
	if err != nil {
		t.Fatalf("Dedup error: %v", err)
	}

	if result.Removed != 2 {
		t.Errorf("expected 2 removed across both functions, got %d", result.Removed)
	}
}

func TestDedup_InvalidCode(t *testing.T) {
	_, err := Dedup("not valid go code {{{{")
	if err == nil {
		t.Error("expected error for invalid code")
	}
}

func TestDedup_EmptySlice(t *testing.T) {
	code := `package calc_test

import "testing"

func TestEmpty(t *testing.T) {
	tests := []struct {
		name string
	}{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}
`

	result, err := Dedup(code)
	if err != nil {
		t.Fatalf("Dedup error: %v", err)
	}

	if result.Removed != 0 {
		t.Errorf("expected 0 removed for empty slice, got %d", result.Removed)
	}
}
