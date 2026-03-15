package sample

import (
	"errors"
	"math"
	"testing"
)

// TestAdd_HappyPath tests the Add function with normal inputs.
func TestAdd_HappyPath(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{"Positive numbers", 1, 2, 3},
		{"Negative numbers", -1, -2, -3},
		{"Mixed numbers", -1, 2, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Add(tt.a, tt.b)
			if err != nil {
				t.Errorf("Add(%d, %d) = unexpected error: %v", tt.a, tt.b, err)
			}
			if result != tt.expected {
				t.Errorf("Add(%d, %d) = %d; want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestAdd_Overflow tests the Add function with overflow scenarios.
func TestAdd_Overflow(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected error
	}{
		{"Positive overflow", math.MaxInt64, 1, errors.New("integer overflow")},
		{"Negative overflow", math.MinInt64, -1, errors.New("integer overflow")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Add(tt.a, tt.b)
			if err == nil || err.Error() != tt.expected.Error() {
				t.Errorf("Add(%d, %d) = %v; want %v", tt.a, tt.b, err, tt.expected)
			}
		})
	}
}

// TestMultiply_HappyPath tests the Multiply function with normal inputs.
func TestMultiply_HappyPath(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{"Positive numbers", 2, 3, 6},
		{"Negative numbers", -2, -3, 6},
		{"Mixed numbers", -2, 3, -6},
		{"Zero case", 0, 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Multiply(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Multiply(%d, %d) = %d; want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestSqrt_HappyPath tests the Sqrt function with normal inputs.
func TestSqrt_HappyPath(t *testing.T) {
	tests := []struct {
		name     string
		x        float64
		expected float64
	}{
		{"Positive number", 9.0, 3.0},
		{"Zero case", 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Sqrt(tt.x)
			if err != nil {
				t.Errorf("Sqrt(%f) = unexpected error: %v", tt.x, err)
			}
			if result != tt.expected {
				t.Errorf("Sqrt(%f) = %f; want %f", tt.x, result, tt.expected)
			}
		})
	}
}

// TestSqrt_NegativeNumber tests the Sqrt function with negative input.
func TestSqrt_NegativeNumber(t *testing.T) {
	tests := []struct {
		name     string
		x        float64
		expected error
	}{
		{"Negative number", -1.0, errors.New("negative number")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Sqrt(tt.x)
			if err == nil || err.Error() != tt.expected.Error() {
				t.Errorf("Sqrt(%f) = %v; want %v", tt.x, err, tt.expected)
			}
		})
	}
}
