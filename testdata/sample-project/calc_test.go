package sample

import (
	"testing"
)

func TestPower(t *testing.T) {
	tests := []struct {
		name    string
		base    int
		exp     int
		want    int
		wantErr bool
	}{
		{"normal case", 2, 3, 8, false},
		{"zero exponent", 5, 0, 1, false},
		{"base one", 1, 10, 1, false},
		{"base zero", 0, 5, 0, false},
		{"negative exponent", 2, -1, 0, true},
		{"negative exponent zero base", 0, -1, 0, true},
		{"large base", 10, 2, 100, false},
		{"large exponent", 2, 10, 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Power(tt.base, tt.exp)
			if (err != nil) != tt.wantErr {
				t.Errorf("Power(%d, %d) error = %v, wantErr %v", tt.base, tt.exp, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Power(%d, %d) = %d, want %d", tt.base, tt.exp, got, tt.want)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		name    string
		x       int
		min     int
		max     int
		want    int
		wantErr bool
	}{
		{"within range", 5, 1, 10, 5, false},
		{"below min", 0, 1, 10, 1, false},
		{"above max", 15, 1, 10, 10, false},
		{"min equals max", 5, 5, 5, 5, false},
		{"min greater than max", 5, 10, 5, 0, true},
		{"min equals max, x equals min", 5, 5, 5, 5, false},
		{"negative values", -5, -10, -1, -5, false},
		{"x equals min", 1, 1, 10, 1, false},
		{"x equals max", 10, 1, 10, 10, false},
		{"both negative range", -5, -10, -1, -5, false},
		{"min greater than max with negatives", -5, -1, -10, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Clamp(tt.x, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("Clamp(%d, %d, %d) error = %v, wantErr %v", tt.x, tt.min, tt.max, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Clamp(%d, %d, %d) = %d, want %d", tt.x, tt.min, tt.max, got, tt.want)
			}
		})
	}
}

func TestGCD(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"both positive", 12, 8, 4},
		{"first zero", 0, 5, 5},
		{"second zero", 5, 0, 5},
		{"both zero", 0, 0, 0},
		{"first negative", -12, 8, 4},
		{"second negative", 12, -8, 4},
		{"both negative", -12, -8, 4},
		{"coprime numbers", 7, 11, 1},
		{"equal numbers", 5, 5, 5},
		{"one is one", 1, 100, 1},
		{"large numbers", 1000, 1500, 500},
		{"one is zero", 0, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GCD(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("GCD(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
