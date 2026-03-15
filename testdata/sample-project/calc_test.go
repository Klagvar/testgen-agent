package sample

import (
	"math"
	"testing"
)

func TestAdd(t *testing.T) {
	tests := []struct {
		name    string
		a, b    int
		want    int
		wantErr bool
	}{
		{"positive", 2, 3, 5, false},
		{"negative", -1, -2, -3, false},
		{"zero", 0, 0, 0, false},
		{"mixed", -5, 3, -2, false},
		{"overflow positive", math.MaxInt64, 1, 0, true},
		{"overflow negative", math.MinInt64, -1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Add(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add(%d, %d) error = %v, wantErr %v", tt.a, tt.b, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSubtract(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"positive", 5, 3, 2},
		{"negative", -1, -2, 1},
		{"zero", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Subtract(tt.a, tt.b); got != tt.want {
				t.Errorf("Subtract(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestDivide(t *testing.T) {
	tests := []struct {
		name    string
		a, b    int
		want    int
		wantErr bool
	}{
		{"normal", 10, 2, 5, false},
		{"integer division", 7, 2, 3, false},
		{"division by zero", 1, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Divide(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Divide(%d, %d) error = %v, wantErr %v", tt.a, tt.b, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Divide(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestMultiply(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"positive", 3, 4, 12},
		{"negative", -3, 4, -12},
		{"zero", 0, 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Multiply(tt.a, tt.b); got != tt.want {
				t.Errorf("Multiply(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		name    string
		x       float64
		want    float64
		wantErr bool
	}{
		{"positive", 4.0, 2.0, false},
		{"zero", 0.0, 0.0, false},
		{"negative", -1.0, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sqrt(tt.x)
			if (err != nil) != tt.wantErr {
				t.Errorf("Sqrt(%f) error = %v, wantErr %v", tt.x, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Sqrt(%f) = %f, want %f", tt.x, got, tt.want)
			}
		})
	}
}

func TestModulo(t *testing.T) {
	tests := []struct {
		name    string
		a, b    int
		want    int
		wantErr bool
	}{
		{"positive", 7, 3, 1, false},
		{"negative dividend", -7, 3, -1, false},
		{"negative divisor", 7, -3, 1, false},
		{"both negative", -7, -3, -1, false},
		{"exact division", 6, 3, 0, false},
		{"division by zero", 5, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Modulo(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("Modulo(%d, %d) error = %v, wantErr %v", tt.a, tt.b, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Modulo(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		name string
		x    int
		want int
	}{
		{"positive", 42, 42},
		{"negative", -42, 42},
		{"zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Abs(tt.x); got != tt.want {
				t.Errorf("Abs(%d) = %d, want %d", tt.x, got, tt.want)
			}
		})
	}
}
