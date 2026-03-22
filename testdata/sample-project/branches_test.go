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
			name:     "invalid score less than 0",
			score:    -1,
			expected: "invalid",
		},
		{
			name:     "fail score less than 50",
			score:    49,
			expected: "fail",
		},
		{
			name:     "pass score between 50 and 69",
			score:    65,
			expected: "pass",
		},
		{
			name:     "good score between 70 and 89",
			score:    85,
			expected: "good",
		},
		{
			name:     "excellent score 90 and above",
			score:    95,
			expected: "excellent",
		},
		{
			name:     "boundary case score 0",
			score:    0,
			expected: "fail",
		},
		{
			name:     "boundary case score 50",
			score:    50,
			expected: "pass",
		},
		{
			name:     "boundary case score 70",
			score:    70,
			expected: "good",
		},
		{
			name:     "boundary case score 90",
			score:    90,
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
			name:     "positive number",
			n:        5,
			expected: 5,
		},
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
			name:     "maximum int",
			n:        1<<31 - 1,
			expected: 1<<31 - 1,
		},
		{
			name:     "minimum int",
			n:        -1 << 31,
			expected: 1 << 31,
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
			name:     "negative number",
			n:        -5,
			expected: -1,
		},
		{
			name:     "zero",
			n:        0,
			expected: 0,
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
			name:     "b greater than a",
			a:        5,
			b:        10,
			expected: 10,
		},
		{
			name:     "a equals b",
			a:        5,
			b:        5,
			expected: 5,
		},
		{
			name:     "negative a and positive b",
			a:        -10,
			b:        5,
			expected: 5,
		},
		{
			name:     "both negative",
			a:        -10,
			b:        -5,
			expected: -5,
		},
		{
			name:     "maximum int",
			a:        1<<31 - 1,
			b:        5,
			expected: 1<<31 - 1,
		},
		{
			name:     "minimum int",
			a:        -1 << 31,
			b:        5,
			expected: 5,
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
			name:     "val less than min",
			val:      5,
			min:      10,
			max:      20,
			expected: 10,
		},
		{
			name:     "val greater than max",
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
		{
			name:     "min equals max",
			val:      15,
			min:      10,
			max:      10,
			expected: 10,
		},
		{
			name:     "negative values",
			val:      -5,
			min:      -10,
			max:      -2,
			expected: -5,
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
			s:        "   ",
			expected: false,
		},
		{
			name:     "string with special characters",
			s:        "!@#$%",
			expected: false,
		},
		{
			name:     "unicode string",
			s:        "hello 世界",
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
