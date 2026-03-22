package sample

import (
	"testing"
)

func TestReverse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single character",
			input:    "a",
			expected: "a",
		},
		{
			name:     "normal string",
			input:    "hello",
			expected: "olleh",
		},
		{
			name:     "palindrome",
			input:    "racecar",
			expected: "racecar",
		},

		{
			name:     "mixed unicode",
			input:    "Hello 世界",
			expected: "界世 olleH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Reverse(tt.input)
			if result != tt.expected {
				t.Errorf("Reverse(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsPalindrome(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: true,
		},
		{
			name:     "single character",
			input:    "a",
			expected: true,
		},
		{
			name:     "palindrome lowercase",
			input:    "racecar",
			expected: true,
		},
		{
			name:     "palindrome uppercase",
			input:    "RACECAR",
			expected: true,
		},
		{
			name:     "palindrome mixed case",
			input:    "RaceCar",
			expected: true,
		},
		{
			name:     "not palindrome",
			input:    "hello",
			expected: false,
		},
		{
			name:     "not palindrome mixed case",
			input:    "Hello",
			expected: false,
		},
		{
			name:     "palindrome with spaces",
			input:    "A man a plan a canal Panama",
			expected: false,
		},
		{
			name:     "palindrome with punctuation",
			input:    "Was it a car or a cat I saw?",
			expected: false,
		},
		{
			name:     "unicode palindrome",
			input:    "上海海上",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPalindrome(tt.input)
			if result != tt.expected {
				t.Errorf("IsPalindrome(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{

		{
			name:     "maxLen greater than string",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact match",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},

		{
			name:     "maxLen 1",
			input:    "hello",
			maxLen:   1,
			expected: "h",
		},
		{
			name:     "maxLen 3",
			input:    "hello",
			maxLen:   3,
			expected: "hel",
		},
		{
			name:     "maxLen 2",
			input:    "hello",
			maxLen:   2,
			expected: "he",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "single word",
			input:    "hello",
			expected: 1,
		},
		{
			name:     "multiple words",
			input:    "hello world",
			expected: 2,
		},
		{
			name:     "multiple spaces",
			input:    "hello   world",
			expected: 2,
		},
		{
			name:     "tabs and newlines",
			input:    "hello\tworld\n\ntest",
			expected: 3,
		},
		{
			name:     "leading and trailing spaces",
			input:    " hello world ",
			expected: 2,
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: 0,
		},
		{
			name:     "unicode words",
			input:    "Hello 世界",
			expected: 2,
		},
		{
			name:     "single character",
			input:    "a",
			expected: 1,
		},
		{
			name:     "mixed whitespace",
			input:    "  hello \t\n  world  \t\n  ",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountWords(tt.input)
			if result != tt.expected {
				t.Errorf("CountWords(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single word",
			input:    "Hello",
			expected: "hello",
		},
		{
			name:     "camel case",
			input:    "HelloWorld",
			expected: "hello_world",
		},

		{
			name:     "number in middle",
			input:    "Version2Test",
			expected: "version2_test",
		},
		{
			name:     "single character",
			input:    "A",
			expected: "a",
		},
		{
			name:     "already snake_case",
			input:    "hello_world",
			expected: "hello_world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CamelToSnake(tt.input)
			if result != tt.expected {
				t.Errorf("CamelToSnake(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestWrapLines(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		expected string
	}{
		{
			name:     "zero width",
			text:     "hello",
			width:    0,
			expected: "hello",
		},
		{
			name:     "negative width",
			text:     "hello",
			width:    -1,
			expected: "hello",
		},
		{
			name:     "empty text",
			text:     "",
			width:    10,
			expected: "",
		},
		{
			name:     "single word fits",
			text:     "hello",
			width:    10,
			expected: "hello",
		},
		{
			name:     "single word too long",
			text:     "hello",
			width:    3,
			expected: "hello",
		},
		{
			name:     "multiple words exact width",
			text:     "hello world",
			width:    11,
			expected: "hello world",
		},

		{
			name:     "multiple words multiple wrapping",
			text:     "hello world test example",
			width:    8,
			expected: "hello\nworld\ntest\nexample",
		},
		{
			name:     "word longer than width",
			text:     "hello world superlongword",
			width:    5,
			expected: "hello\nworld\nsuperlongword",
		},
		{
			name:     "multiple spaces",
			text:     "hello    world   test",
			width:    8,
			expected: "hello\nworld\ntest",
		},
		{
			name:     "tabs and newlines",
			text:     "hello\tworld\n\ntest",
			width:    8,
			expected: "hello\nworld\ntest",
		},
		{
			name:     "exact fit",
			text:     "hello world",
			width:    5,
			expected: "hello\nworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapLines(tt.text, tt.width)
			if result != tt.expected {
				t.Errorf("WrapLines(%q, %d) = %q, want %q", tt.text, tt.width, result, tt.expected)
			}
		})
	}
}
