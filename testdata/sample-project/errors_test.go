package sample

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidateAge(t *testing.T) {
	tests := []struct {
		name      string
		age       int
		wantErr   bool
		wantErrIs error
	}{
		{
			name:      "valid age",
			age:       25,
			wantErr:   false,
			wantErrIs: nil,
		},
		{
			name:      "negative age",
			age:       -1,
			wantErr:   true,
			wantErrIs: ErrValidation,
		},
		{
			name:      "age over 150",
			age:       151,
			wantErr:   true,
			wantErrIs: ErrValidation,
		},
		{
			name:      "age exactly 0",
			age:       0,
			wantErr:   false,
			wantErrIs: nil,
		},
		{
			name:      "age exactly 150",
			age:       150,
			wantErr:   false,
			wantErrIs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAge(tt.age)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateAge() error = nil, want error")
					return
				}
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("ValidateAge() error = %v, wantErrIs = %v", err, tt.wantErrIs)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateAge() error = %v, want nil", err)
				}
			}
		})
	}
}

func TestLookupUser(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		want      string
		wantErr   bool
		wantErrIs error
	}{
		{
			name:      "valid id",
			id:        "123",
			want:      "User_123",
			wantErr:   false,
			wantErrIs: nil,
		},
		{
			name:      "empty id",
			id:        "",
			want:      "",
			wantErr:   true,
			wantErrIs: ErrValidation,
		},
		{
			name:      "blocked user",
			id:        "blocked",
			want:      "",
			wantErr:   true,
			wantErrIs: ErrForbidden,
		},
		{
			name:      "ghost user",
			id:        "ghost",
			want:      "",
			wantErr:   true,
			wantErrIs: ErrNotFound,
		},
		{
			name:      "id with special characters",
			id:        "user@123",
			want:      "User_user@123",
			wantErr:   false,
			wantErrIs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LookupUser(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("LookupUser() error = nil, want error")
					return
				}
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("LookupUser() error = %v, wantErrIs = %v", err, tt.wantErrIs)
				}
			} else {
				if err != nil {
					t.Errorf("LookupUser() error = %v, want nil", err)
					return
				}
				if got != tt.want {
					t.Errorf("LookupUser() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "ok",
		},
		{
			name: "not found error",
			err:  ErrNotFound,
			want: "not_found",
		},
		{
			name: "forbidden error",
			err:  ErrForbidden,
			want: "forbidden",
		},
		{
			name: "validation error",
			err:  ErrValidation,
			want: "validation",
		},
		{
			name: "wrapped not found error",
			err:  fmt.Errorf("wrapped: %w", ErrNotFound),
			want: "not_found",
		},
		{
			name: "wrapped forbidden error",
			err:  fmt.Errorf("wrapped: %w", ErrForbidden),
			want: "forbidden",
		},
		{
			name: "wrapped validation error",
			err:  fmt.Errorf("wrapped: %w", ErrValidation),
			want: "validation",
		},
		{
			name: "unknown error",
			err:  errors.New("some unknown error"),
			want: "unknown",
		},
		{
			name: "wrapped unknown error",
			err:  fmt.Errorf("wrapped unknown: %w", errors.New("some unknown error")),
			want: "unknown",
		},
		{
			name: "nil wrapped error",
			err:  fmt.Errorf("wrapped: %w", nil),
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.want {
				t.Errorf("ClassifyError() = %v, want %v", got, tt.want)
			}
		})
	}
}
