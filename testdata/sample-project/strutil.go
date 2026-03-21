package sample

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Reverse returns the input string reversed, preserving valid UTF-8.
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// IsPalindrome checks whether a string reads the same forwards and backwards.
// Comparison is case-insensitive and ignores non-letter characters.
func IsPalindrome(s string) bool {
	var letters []rune
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters = append(letters, unicode.ToLower(r))
		}
	}
	for i, j := 0, len(letters)-1; i < j; i, j = i+1, j-1 {
		if letters[i] != letters[j] {
			return false
		}
	}
	return true
}

// Truncate shortens a string to maxLen runes and appends "..." if truncated.
// Returns an error if maxLen is negative.
func Truncate(s string, maxLen int) (string, error) {
	if maxLen < 0 {
		return "", errors.New("maxLen must be non-negative")
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s, nil
	}
	runes := []rune(s)
	return string(runes[:maxLen]) + "...", nil
}

// CountWords returns the number of whitespace-separated words in a string.
func CountWords(s string) int {
	return len(strings.Fields(s))
}

// JoinWith concatenates the given parts using sep as a separator.
// Empty parts are skipped.
func JoinWith(sep string, parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, sep)
}

// ParseCSV splits a comma-separated line into trimmed fields.
// Returns an error if the input is empty.
func ParseCSV(line string) ([]string, error) {
	if strings.TrimSpace(line) == "" {
		return nil, errors.New("empty input")
	}
	parts := strings.Split(line, ",")
	result := make([]string, len(parts))
	for i, p := range parts {
		result[i] = strings.TrimSpace(p)
	}
	return result, nil
}
