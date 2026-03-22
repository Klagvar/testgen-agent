package sample

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidateAge_HappyPath(t *testing.T) {
	tests := []struct {
		name string
		age  int
		want error
	}{
		{
			name: "valid age 0",
			age:  0,
			want: nil,
		},
		{
			name: "valid age 50",
			age:  50,
			want: nil,
		},
		{
			name: "valid age 150",
			age:  150,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAge(tt.age)
			if got != tt.want {
				t.Errorf("ValidateAge(%d) = %v, want %v", tt.age, got, tt.want)
			}
		})
	}
}

func TestValidateAge_NegativeAge(t *testing.T) {
	got := ValidateAge(-1)
	if got == nil {
		t.Error("ValidateAge(-1) = nil, want error")
	}

	if !errors.Is(got, ErrValidation) {
		t.Errorf("ValidateAge(-1) error does not wrap ErrValidation")
	}

	expectedMsg := "negative age: validation failed"
	if got.Error() != expectedMsg {
		t.Errorf("ValidateAge(-1) = %q, want %q", got.Error(), expectedMsg)
	}
}

func TestValidateAge_UnrealisticAge(t *testing.T) {
	tests := []struct {
		name string
		age  int
		want string
	}{
		{
			name: "age 151",
			age:  151,
			want: "unrealistic age 151: validation failed",
		},
		{
			name: "age 200",
			age:  200,
			want: "unrealistic age 200: validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAge(tt.age)
			if got == nil {
				t.Errorf("ValidateAge(%d) = nil, want error", tt.age)
			}

			if !errors.Is(got, ErrValidation) {
				t.Errorf("ValidateAge(%d) error does not wrap ErrValidation", tt.age)
			}

			if got.Error() != tt.want {
				t.Errorf("ValidateAge(%d) = %q, want %q", tt.age, got.Error(), tt.want)
			}
		})
	}
}

func TestLookupUser_HappyPath(t *testing.T) {
	got, err := LookupUser("valid")
	if err != nil {
		t.Errorf("LookupUser(\"valid\") returned error: %v", err)
	}

	expected := "User_valid"
	if got != expected {
		t.Errorf("LookupUser(\"valid\") = %q, want %q", got, expected)
	}
}

func TestLookupUser_EmptyID(t *testing.T) {
	got, err := LookupUser("")
	if err == nil {
		t.Error("LookupUser(\"\") returned nil error, want error")
	}

	if !errors.Is(err, ErrValidation) {
		t.Errorf("LookupUser(\"\") error does not wrap ErrValidation")
	}

	expectedMsg := "empty id: validation failed"
	if err.Error() != expectedMsg {
		t.Errorf("LookupUser(\"\") = %q, want %q", err.Error(), expectedMsg)
	}

	if got != "" {
		t.Errorf("LookupUser(\"\") = %q, want empty string", got)
	}
}

func TestLookupUser_BlockedUser(t *testing.T) {
	got, err := LookupUser("blocked")
	if err == nil {
		t.Error("LookupUser(\"blocked\") returned nil error, want error")
	}

	if !errors.Is(err, ErrForbidden) {
		t.Errorf("LookupUser(\"blocked\") error does not wrap ErrForbidden")
	}

	expectedMsg := "user blocked: forbidden"
	if err.Error() != expectedMsg {
		t.Errorf("LookupUser(\"blocked\") = %q, want %q", err.Error(), expectedMsg)
	}

	if got != "" {
		t.Errorf("LookupUser(\"blocked\") = %q, want empty string", got)
	}
}

func TestLookupUser_GhostUser(t *testing.T) {
	got, err := LookupUser("ghost")
	if err == nil {
		t.Error("LookupUser(\"ghost\") returned nil error, want error")
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("LookupUser(\"ghost\") error does not wrap ErrNotFound")
	}

	expectedMsg := "user ghost: not found"
	if err.Error() != expectedMsg {
		t.Errorf("LookupUser(\"ghost\") = %q, want %q", err.Error(), expectedMsg)
	}

	if got != "" {
		t.Errorf("LookupUser(\"ghost\") = %q, want empty string", got)
	}
}

func TestClassifyError_HappyPath(t *testing.T) {
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
			name: "ErrNotFound",
			err:  ErrNotFound,
			want: "not_found",
		},
		{
			name: "ErrForbidden",
			err:  ErrForbidden,
			want: "forbidden",
		},
		{
			name: "ErrValidation",
			err:  ErrValidation,
			want: "validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.want {
				t.Errorf("ClassifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestClassifyError_WrappedErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "wrapped ErrNotFound",
			err:  fmt.Errorf("wrapped: %w", ErrNotFound),
			want: "not_found",
		},
		{
			name: "wrapped ErrForbidden",
			err:  fmt.Errorf("wrapped: %w", ErrForbidden),
			want: "forbidden",
		},
		{
			name: "wrapped ErrValidation",
			err:  fmt.Errorf("wrapped: %w", ErrValidation),
			want: "validation",
		},
		{
			name: "unknown error",
			err:  errors.New("unknown error"),
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.want {
				t.Errorf("ClassifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
