package sample

import (
	"errors"
	"testing"
)

func TestReverse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty string", "", ""},
		{"Single character", "a", "a"},
		{"Two characters", "ab", "ba"},
		{"Normal string", "hello", "olleh"},
		{"String with spaces", "hello world", "dlrow olleh"},
		{"Unicode characters", "café", "éfac"},
		{"Mixed Unicode", "héllo wørld", "dlrøw olléh"},
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
		{"Empty string", "", true},
		{"Single character", "a", true},
		{"Simple palindrome", "aba", true},
		{"Simple non-palindrome", "abc", false},
		{"Case insensitive", "Aba", true},
		{"With spaces", "A man a plan a canal Panama", true},
		{"With punctuation", "Was it a car or a cat I saw?", true},
		{"With numbers", "12321", true},
		{"With mixed characters", "A1B2b1a", true},
		{"Non-palindrome with letters", "hello", false},
		{"Only non-letters", "!@#$%", true},
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
		hasError bool
	}{
		{"Negative maxLen", "hello", -1, "", true},
		{"Zero maxLen", "hello", 0, "...", false},
		{"MaxLen greater than string length", "hello", 10, "hello", false},
		{"Exact match", "hello", 5, "hello", false},
		{"Truncate", "hello", 3, "hel...", false},
		{"Unicode truncation", "café", 3, "caf...", false},
		{"Truncate with spaces", "hello world", 5, "hello...", false},
		{"Single rune", "a", 1, "a", false},
		{"Single rune truncate", "a", 0, "...", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Truncate(tt.input, tt.maxLen)
			if tt.hasError {
				if err == nil {
					t.Errorf("Truncate(%q, %d) expected error, got nil", tt.input, tt.maxLen)
				}
			} else {
				if err != nil {
					t.Errorf("Truncate(%q, %d) unexpected error: %v", tt.input, tt.maxLen, err)
				}
				if result != tt.expected {
					t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
				}
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
		{"Empty string", "", 0},
		{"Single word", "hello", 1},
		{"Multiple words", "hello world", 2},
		{"Multiple spaces", "  hello   world  ", 2},
		{"Tabs and newlines", "hello\t\nworld", 2},
		{"Only spaces", "   ", 0},
		{"Single character", "a", 1},
		{"Multiple single characters", "a b c d e", 5},
		{"Leading and trailing spaces", " hello world ", 2},
		{"Unicode spaces", "héllo wörld", 2},
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

func TestJoinWith(t *testing.T) {
	tests := []struct {
		name     string
		sep      string
		parts    []string
		expected string
	}{
		{"Empty parts", ",", []string{}, ""},
		{"Single empty part", ",", []string{""}, ""},
		{"Single non-empty part", ",", []string{"hello"}, "hello"},
		{"Multiple parts", ",", []string{"hello", "world"}, "hello,world"},
		{"Empty separator", "", []string{"hello", "world"}, "helloworld"},
		{"Empty parts with separator", ",", []string{"", "hello", "", "world", ""}, "hello,world"},
		{"Multiple empty parts", ",", []string{"", "", ""}, ""},
		{"Mixed empty and non-empty", ",", []string{"", "hello", "", "world"}, "hello,world"},
		{"Unicode parts", "-", []string{"héllo", "wörld"}, "héllo-wörld"},
		{"Complex separator", " | ", []string{"a", "b", "c"}, "a | b | c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinWith(tt.sep, tt.parts...)
			if result != tt.expected {
				t.Errorf("JoinWith(%q, %v) = %q, want %q", tt.sep, tt.parts, result, tt.expected)
			}
		})
	}
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
		hasError bool
	}{
		{"Empty input", "", nil, true},
		{"Empty input with whitespace", "   ", nil, true},
		{"Single field", "hello", []string{"hello"}, false},
		{"Two fields", "hello,world", []string{"hello", "world"}, false},
		{"Three fields", "a,b,c", []string{"a", "b", "c"}, false},
		{"Fields with spaces", " hello , world ", []string{"hello", "world"}, false},
		{"Fields with multiple spaces", "  a  ,  b  ,  c  ", []string{"a", "b", "c"}, false},
		{"Empty fields", "a,,c", []string{"a", "", "c"}, false},
		{"Only commas", ",,", []string{"", "", ""}, false},
		{"Mixed spaces and commas", " a , b , c ", []string{"a", "b", "c"}, false},
		{"Unicode fields", "héllo,wörld", []string{"héllo", "wörld"}, false},
		{"Trailing comma", "hello,", []string{"hello", ""}, false},
		{"Leading comma", ",hello", []string{"", "hello"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCSV(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("ParseCSV(%q) expected error, got nil", tt.input)
				}
				if !errors.Is(err, err) { // Just checking that we have an error
					t.Logf("ParseCSV(%q) got error: %v", tt.input, err)
				}
			} else {
				if err != nil {
					t.Errorf("ParseCSV(%q) unexpected error: %v", tt.input, err)
				}
				if len(result) != len(tt.expected) {
					t.Errorf("ParseCSV(%q) length mismatch: got %d, want %d", tt.input, len(result), len(tt.expected))
				}
				for i, expected := range tt.expected {
					if result[i] != expected {
						t.Errorf("ParseCSV(%q)[%d] = %q, want %q", tt.input, i, result[i], expected)
					}
				}
			}
		})
	}
}
