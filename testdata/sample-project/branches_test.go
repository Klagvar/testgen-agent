package sample

import (
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		score    int
		expected string
	}{
		{
			name:     "negative score",
			score:    -1,
			expected: "invalid",
		},
		{
			name:     "score less than 50",
			score:    49,
			expected: "fail",
		},
		{
			name:     "score exactly 50",
			score:    50,
			expected: "pass",
		},
		{
			name:     "score less than 70",
			score:    69,
			expected: "pass",
		},
		{
			name:     "score exactly 70",
			score:    70,
			expected: "good",
		},
		{
			name:     "score less than 90",
			score:    89,
			expected: "good",
		},
		{
			name:     "score exactly 90",
			score:    90,
			expected: "excellent",
		},
		{
			name:     "score greater than 90",
			score:    95,
			expected: "excellent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.score)
			if result != tt.expected {
				t.Errorf("Classify(%d) = %v, want %v", tt.score, result, tt.expected)
			}
		})
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		expected int
	}{
		{
			name:     "negative number",
			n:        -5,
			expected: 5,
		},
		{
			name:     "zero",
			n:        0,
			expected: 0,
		},
		{
			name:     "positive number",
			n:        5,
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Abs(tt.n)
			if result != tt.expected {
				t.Errorf("Abs(%d) = %v, want %v", tt.n, result, tt.expected)
			}
		})
	}
}

func TestSign(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		expected int
	}{
		{
			name:     "positive number",
			n:        5,
			expected: 1,
		},
		{
			name:     "zero",
			n:        0,
			expected: 0,
		},
		{
			name:     "negative number",
			n:        -5,
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Sign(tt.n)
			if result != tt.expected {
				t.Errorf("Sign(%d) = %v, want %v", tt.n, result, tt.expected)
			}
		})
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{
			name:     "a greater than b",
			a:        10,
			b:        5,
			expected: 10,
		},
		{
			name:     "a equal to b",
			a:        5,
			b:        5,
			expected: 5,
		},
		{
			name:     "a less than b",
			a:        5,
			b:        10,
			expected: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Max(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Max(%d, %d) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		name     string
		val      int
		min      int
		max      int
		expected int
	}{
		{
			name:     "val below min",
			val:      5,
			min:      10,
			max:      20,
			expected: 10,
		},
		{
			name:     "val above max",
			val:      25,
			min:      10,
			max:      20,
			expected: 20,
		},
		{
			name:     "val within range",
			val:      15,
			min:      10,
			max:      20,
			expected: 15,
		},
		{
			name:     "val equals min",
			val:      10,
			min:      10,
			max:      20,
			expected: 10,
		},
		{
			name:     "val equals max",
			val:      20,
			min:      10,
			max:      20,
			expected: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Clamp(tt.val, tt.min, tt.max)
			if result != tt.expected {
				t.Errorf("Clamp(%d, %d, %d) = %v, want %v", tt.val, tt.min, tt.max, result, tt.expected)
			}
		})
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		expected bool
	}{
		{
			name:     "empty string",
			s:        "",
			expected: true,
		},
		{
			name:     "non-empty string",
			s:        "hello",
			expected: false,
		},
		{
			name:     "string with spaces",
			s:        " ",
			expected: false,
		},
		{
			name:     "string with special characters",
			s:        "!\n\t",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsEmpty(tt.s)
			if result != tt.expected {
				t.Errorf("IsEmpty(%q) = %v, want %v", tt.s, result, tt.expected)
			}
		})
	}
}
