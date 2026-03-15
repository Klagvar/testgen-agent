package sample

import (
	"errors"
	"strings"
	"unicode"
)

// Reverse переворачивает строку, корректно обрабатывая Unicode.
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// IsPalindrome проверяет, является ли строка палиндромом (без учёта регистра).
func IsPalindrome(s string) bool {
	s = strings.ToLower(s)
	reversed := Reverse(s)
	return s == reversed
}

// Truncate обрезает строку до maxLen символов и добавляет "..." если обрезана.
// Возвращает ошибку если maxLen < 0.
func Truncate(s string, maxLen int) (string, error) {
	if maxLen < 0 {
		return "", errors.New("maxLen must be non-negative")
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s, nil
	}
	if maxLen <= 3 {
		return string(runes[:maxLen]), nil
	}
	return string(runes[:maxLen-3]) + "...", nil
}

// CountWords подсчитывает количество слов в строке.
// Слова разделены пробельными символами.
func CountWords(s string) int {
	return len(strings.Fields(s))
}

// Capitalize делает первую букву каждого слова заглавной.
func Capitalize(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			runes := []rune(w)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}
