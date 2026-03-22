package sample

import (
	"errors"
	"testing"
)

func TestLCM(t *testing.T) {
	tests := []struct {
		name    string
		a       int
		b       int
		want    int
		wantErr bool
		err     error
	}{
		{
			name:    "happy path",
			a:       12,
			b:       18,
			want:    36,
			wantErr: false,
		},
		{
			name:    "zero a",
			a:       0,
			b:       5,
			want:    0,
			wantErr: true,
			err:     errors.New("LCM undefined for zero"),
		},
		{
			name:    "zero b",
			a:       5,
			b:       0,
			want:    0,
			wantErr: true,
			err:     errors.New("LCM undefined for zero"),
		},
		{
			name:    "both zero",
			a:       0,
			b:       0,
			want:    0,
			wantErr: true,
			err:     errors.New("LCM undefined for zero"),
		},
		{
			name:    "same numbers",
			a:       7,
			b:       7,
			want:    7,
			wantErr: false,
		},
		{
			name:    "coprime numbers",
			a:       5,
			b:       7,
			want:    35,
			wantErr: false,
		},
		{
			name:    "one is factor of other",
			a:       4,
			b:       8,
			want:    8,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LCM(tt.a, tt.b)
			if tt.wantErr {
				if err == nil {
					t.Errorf("LCM(%d, %d) expected error, got none", tt.a, tt.b)
				} else if err.Error() != tt.err.Error() {
					t.Errorf("LCM(%d, %d) error = %v, wantErr %v", tt.a, tt.b, err, tt.err)
				}
			} else {
				if err != nil {
					t.Errorf("LCM(%d, %d) unexpected error: %v", tt.a, tt.b, err)
				}
				if got != tt.want {
					t.Errorf("LCM(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
				}
			}
		})
	}
}

func TestFibonacci(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		want    int
		wantErr bool
		err     error
	}{
		{
			name:    "negative input",
			n:       -1,
			want:    0,
			wantErr: true,
			err:     errors.New("negative index"),
		},
		{
			name:    "zero input",
			n:       0,
			want:    0,
			wantErr: false,
		},
		{
			name:    "one input",
			n:       1,
			want:    1,
			wantErr: false,
		},
		{
			name:    "small positive input",
			n:       2,
			want:    1,
			wantErr: false,
		},
		{
			name:    "small positive input",
			n:       3,
			want:    2,
			wantErr: false,
		},
		{
			name:    "small positive input",
			n:       4,
			want:    3,
			wantErr: false,
		},
		{
			name:    "small positive input",
			n:       5,
			want:    5,
			wantErr: false,
		},
		{
			name:    "medium positive input",
			n:       10,
			want:    55,
			wantErr: false,
		},
		{
			name:    "large positive input",
			n:       20,
			want:    6765,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Fibonacci(tt.n)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Fibonacci(%d) expected error, got none", tt.n)
				} else if err.Error() != tt.err.Error() {
					t.Errorf("Fibonacci(%d) error = %v, wantErr %v", tt.n, err, tt.err)
				}
			} else {
				if err != nil {
					t.Errorf("Fibonacci(%d) unexpected error: %v", tt.n, err)
				}
				if got != tt.want {
					t.Errorf("Fibonacci(%d) = %d, want %d", tt.n, got, tt.want)
				}
			}
		})
	}
}
