package sample

import (
	"errors"
	"testing"
)

func TestReverse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "single character",
			in:   "a",
			want: "a",
		},
		{
			name: "normal string",
			in:   "hello",
			want: "olleh",
		},
		{
			name: "unicode string",
			in:   "привет",
			want: "тевирп",
		},
		{
			name: "mixed unicode",
			in:   "Hello 世界",
			want: "界世 olleH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Reverse(tt.in)
			if got != tt.want {
				t.Errorf("Reverse(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsPalindrome(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{
			name: "empty string",
			in:   "",
			want: true,
		},
		{
			name: "single character",
			in:   "a",
			want: true,
		},
		{
			name: "palindrome lowercase",
			in:   "level",
			want: true,
		},
		{
			name: "palindrome mixed case",
			in:   "Level",
			want: true,
		},
		{
			name: "not palindrome",
			in:   "hello",
			want: false,
		},
		{
			name: "palindrome with spaces",
			in:   "A man a plan a canal Panama",
			want: true,
		},
		{
			name: "palindrome with punctuation",
			in:   "Was it a car or a cat I saw?",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPalindrome(tt.in)
			if got != tt.want {
				t.Errorf("IsPalindrome(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		maxLen  int
		want    string
		wantErr error
	}{
		{
			name:    "negative maxLen",
			in:      "hello",
			maxLen:  -1,
			want:    "",
			wantErr: errors.New("maxLen must be non-negative"),
		},
		{
			name:    "maxLen 0",
			in:      "hello",
			maxLen:  0,
			want:    "",
			wantErr: nil,
		},
		{
			name:    "string shorter than maxLen",
			in:      "hello",
			maxLen:  10,
			want:    "hello",
			wantErr: nil,
		},
		{
			name:    "string equal to maxLen",
			in:      "hello",
			maxLen:  5,
			want:    "hello",
			wantErr: nil,
		},
		{
			name:    "maxLen less than 3",
			in:      "hello",
			maxLen:  2,
			want:    "he",
			wantErr: nil,
		},
		{
			name:    "normal truncation",
			in:      "hello world",
			maxLen:  8,
			want:    "hello...",
			wantErr: nil,
		},
		{
			name:    "unicode truncation",
			in:      "привет мир",
			maxLen:  6,
			want:    "привет...",
			wantErr: nil,
		},
		{
			name:    "maxLen equals 3",
			in:      "hello",
			maxLen:  3,
			want:    "hel",
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Truncate(tt.in, tt.maxLen)
			if err != nil && tt.wantErr == nil {
				t.Errorf("Truncate(%q, %d) error = %v, wantErr %v", tt.in, tt.maxLen, err, tt.wantErr)
				return
			}
			if err != nil && tt.wantErr != nil && err.Error() != tt.wantErr.Error() {
				t.Errorf("Truncate(%q, %d) error = %v, wantErr %v", tt.in, tt.maxLen, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{
			name: "empty string",
			in:   "",
			want: 0,
		},
		{
			name: "single word",
			in:   "hello",
			want: 1,
		},
		{
			name: "multiple words",
			in:   "hello world",
			want: 2,
		},
		{
			name: "multiple spaces",
			in:   " hello   world  ",
			want: 2,
		},
		{
			name: "tabs and newlines",
			in:   "hello\t\nworld",
			want: 2,
		},
		{
			name: "unicode words",
			in:   "привет мир",
			want: 2,
		},
		{
			name: "only spaces",
			in:   "   ",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountWords(tt.in)
			if got != tt.want {
				t.Errorf("CountWords(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "single word",
			in:   "hello",
			want: "Hello",
		},
		{
			name: "multiple words",
			in:   "hello world",
			want: "Hello World",
		},
		{
			name: "existing capitalization",
			in:   "HELLO WORLD",
			want: "Hello World",
		},
		{
			name: "mixed case with spaces",
			in:   "  hello   world  ",
			want: "Hello World",
		},
		{
			name: "unicode",
			in:   "привет мир",
			want: "Привет Мир",
		},
		{
			name: "single character",
			in:   "a",
			want: "A",
		},
		{
			name: "empty words",
			in:   " hello  world ",
			want: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Capitalize(tt.in)
			if got != tt.want {
				t.Errorf("Capitalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
