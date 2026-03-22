package sample

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrValidation = errors.New("validation failed")
)

func ValidateAge(age int) error {
	if age < 0 {
		return fmt.Errorf("negative age: %w", ErrValidation)
	}
	if age > 150 {
		return fmt.Errorf("unrealistic age %d: %w", age, ErrValidation)
	}
	return nil
}

func LookupUser(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("empty id: %w", ErrValidation)
	}
	if id == "blocked" {
		return "", fmt.Errorf("user blocked: %w", ErrForbidden)
	}
	if id == "ghost" {
		return "", fmt.Errorf("user %s: %w", id, ErrNotFound)
	}
	return "User_" + id, nil
}

func ClassifyError(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, ErrNotFound) {
		return "not_found"
	}
	if errors.Is(err, ErrForbidden) {
		return "forbidden"
	}
	if errors.Is(err, ErrValidation) {
		return "validation"
	}
	return "unknown"
}
