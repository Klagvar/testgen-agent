package sample

import (
	"errors"
	"testing"
)

func TestMarshalProfile(t *testing.T) {
	tests := []struct {
		name     string
		input    Profile
		expected string
		hasError bool
	}{
		{
			name: "valid profile with all fields",
			input: Profile{
				Name:     "John Doe",
				Email:    "john@example.com",
				Age:      30,
				IsActive: true,
			},
			expected: `{"name":"John Doe","email":"john@example.com","age":30,"is_active":true}`,
		},
		{
			name: "profile with omitempty fields empty",
			input: Profile{
				Name:     "Jane Smith",
				Email:    "",
				Age:      0,
				IsActive: false,
			},
			expected: `{"name":"Jane Smith","is_active":false}`,
		},
		{
			name: "profile with only required fields",
			input: Profile{
				Name:     "Alice",
				Email:    "",
				Age:      0,
				IsActive: true,
			},
			expected: `{"name":"Alice","is_active":true}`,
		},
		{
			name: "empty profile",
			input: Profile{
				Name:     "",
				Email:    "",
				Age:      0,
				IsActive: false,
			},
			expected: `{"name":"","is_active":false}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MarshalProfile(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, result)
				}
			}
		})
	}
}

func TestUnmarshalProfile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Profile
		hasError bool
	}{
		{
			name:  "valid json with all fields",
			input: `{"name":"John Doe","email":"john@example.com","age":30,"is_active":true}`,
			expected: Profile{
				Name:     "John Doe",
				Email:    "john@example.com",
				Age:      30,
				IsActive: true,
			},
		},
		{
			name:  "valid json with omitempty fields omitted",
			input: `{"name":"Jane Smith","is_active":false}`,
			expected: Profile{
				Name:     "Jane Smith",
				Email:    "",
				Age:      0,
				IsActive: false,
			},
		},
		{
			name:     "invalid json",
			input:    `{"name":"John","invalid json`,
			expected: Profile{},
			hasError: true,
		},
		{
			name:     "empty json",
			input:    `{}`,
			expected: Profile{},
		},
		{
			name:  "json with extra fields",
			input: `{"name":"John","email":"john@example.com","age":30,"is_active":true,"extra":"field"}`,
			expected: Profile{
				Name:     "John",
				Email:    "john@example.com",
				Age:      30,
				IsActive: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := UnmarshalProfile(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if !errors.Is(err, err) { // Just checking error is not nil
					// We're only verifying that error is returned
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("expected %+v, got %+v", tt.expected, result)
				}
			}
		})
	}
}

func TestFormatMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected string
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: "{}",
		},
		{
			name:     "single key-value",
			input:    map[string]string{"key": "value"},
			expected: "{key=value}",
		},
		{
			name:     "multiple key-values sorted",
			input:    map[string]string{"z": "value_z", "a": "value_a", "m": "value_m"},
			expected: "{a=value_a, m=value_m, z=value_z}",
		},
		{
			name:     "map with empty string values",
			input:    map[string]string{"empty": "", "nil": "nil"},
			expected: "{empty=, nil=nil}",
		},
		{
			name:     "map with special characters",
			input:    map[string]string{"key&": "value&", "key+": "value+"},
			expected: "{key&=value&, key+=value+}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMap(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCountKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]int
		expected int
	}{
		{
			name:     "empty map",
			input:    map[string]int{},
			expected: 0,
		},
		{
			name:     "single key",
			input:    map[string]int{"key1": 1},
			expected: 1,
		},
		{
			name:     "multiple keys",
			input:    map[string]int{"a": 1, "b": 2, "c": 3},
			expected: 3,
		},
		{
			name:     "nil map",
			input:    nil,
			expected: 0,
		},
		{
			name:     "map with zero values",
			input:    map[string]int{"zero": 0, "negative": -1},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountKeys(tt.input)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}
