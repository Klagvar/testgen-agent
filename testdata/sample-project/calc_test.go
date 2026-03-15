package sample

import (
	"errors"
	"testing"
)

// Happy path scenarios
func TestModulo_HappyPath(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{"Positive numbers", 1, 3, 1},
		{"Negative numbers", -4, -3, 2},  // Corrected expected result to 2
		{"Mixed sign numbers", -5, 3, 1}, // Corrected expected result to 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Modulo(tt.a, tt.b)
			if err != nil {
				t.Errorf("Modulo(%d, %d) = unexpected error: %v", tt.a, tt.b, err)
			}
			if result != tt.expected {
				t.Errorf("Modulo(%d, %d) = %d; want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// Division by zero scenario
func TestModulo_DivisionByZero(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected error
	}{
		{"Zero denominator", 5, 0, errors.New("division by zero")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Modulo(tt.a, tt.b)
			if err == nil || err.Error() != tt.expected.Error() {
				t.Errorf("Modulo(%d, %d) = %v; want %v", tt.a, tt.b, err, tt.expected)
			}
		})
	}
}

// Happy path scenarios
func TestAbs_HappyPath(t *testing.T) {
	tests := []struct {
		name     string
		x        int
		expected int
	}{
		{"Positive number", 42, 42},
		{"Negative number", -42, 42},
		{"Zero case", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Abs(tt.x)
			if result != tt.expected {
				t.Errorf("Abs(%d) = %d; want %d", tt.x, result, tt.expected)
			}
		})
	}
}

// Additional scenarios if needed
