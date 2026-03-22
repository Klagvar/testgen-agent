package sample

import (
	"testing"
)

func TestMarshalProfile(t *testing.T) {
	tests := []struct {
		name     string
		input    Profile
		expected string
	}{
		{
			name: "Empty profile",
			input: Profile{
				Name:     "",
				Email:    "",
				Age:      0,
				IsActive: false,
			},
			expected: `{"name":"","is_active":false}`,
		},
		{
			name: "Profile with all fields",
			input: Profile{
				Name:     "John Doe",
				Email:    "john@example.com",
				Age:      30,
				IsActive: true,
			},
			expected: `{"name":"John Doe","email":"john@example.com","age":30,"is_active":true}`,
		},
		{
			name: "Profile with omitempty fields",
			input: Profile{
				Name:     "Jane Smith",
				Email:    "",
				Age:      0,
				IsActive: true,
			},
			expected: `{"name":"Jane Smith","is_active":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MarshalProfile(tt.input)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestUnmarshalProfile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Profile
		wantErr  bool
	}{
		{
			name:  "Valid profile",
			input: `{"name":"John Doe","email":"john@example.com","age":30,"is_active":true}`,
			expected: Profile{
				Name:     "John Doe",
				Email:    "john@example.com",
				Age:      30,
				IsActive: true,
			},
			wantErr: false,
		},
		{
			name:  "Profile with omitempty fields",
			input: `{"name":"Jane Smith","is_active":true}`,
			expected: Profile{
				Name:     "Jane Smith",
				Email:    "",
				Age:      0,
				IsActive: true,
			},
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			input:   `{"name":"John Doe",}`,
			wantErr: true,
		},
		{
			name:    "Empty JSON",
			input:   `{}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := UnmarshalProfile(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result.Name != tt.expected.Name {
				t.Errorf("Name: expected %q, got %q", tt.expected.Name, result.Name)
			}
			if result.Email != tt.expected.Email {
				t.Errorf("Email: expected %q, got %q", tt.expected.Email, result.Email)
			}
			if result.Age != tt.expected.Age {
				t.Errorf("Age: expected %d, got %d", tt.expected.Age, result.Age)
			}
			if result.IsActive != tt.expected.IsActive {
				t.Errorf("IsActive: expected %t, got %t", tt.expected.IsActive, result.IsActive)
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
			name:     "Empty map",
			input:    map[string]string{},
			expected: "{}",
		},
		{
			name:     "Single key-value",
			input:    map[string]string{"key": "value"},
			expected: "{key=value}",
		},
		{
			name:     "Multiple key-values",
			input:    map[string]string{"z": "1", "a": "2", "b": "3"},
			expected: "{a=2, b=3, z=1}",
		},
		{
			name:     "Key-value with special characters",
			input:    map[string]string{"key with space": "value with space"},
			expected: "{key with space=value with space}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMap(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
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
			name:     "Empty map",
			input:    map[string]int{},
			expected: 0,
		},
		{
			name:     "Single key",
			input:    map[string]int{"key": 1},
			expected: 1,
		},
		{
			name:     "Multiple keys",
			input:    map[string]int{"a": 1, "b": 2, "c": 3},
			expected: 3,
		},
		{
			name:     "Keys with zero values",
			input:    map[string]int{"a": 0, "b": 0},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountKeys(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}
