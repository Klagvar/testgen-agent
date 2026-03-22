// Tests pattern: errors.Is/As, wrapped errors, fmt.Errorf %w
package sample

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrValidation = errors.New("validation error")
)

// ValidateAge checks age and returns a wrapped error if invalid.
func ValidateAge(age int) error {
	if age < 0 {
		return fmt.Errorf("negative age %d: %w", age, ErrValidation)
	}
	if age > 150 {
		return fmt.Errorf("unrealistic age %d: %w", age, ErrValidation)
	}
	return nil
}

// LookupUser returns a wrapped not-found error for unknown users.
func LookupUser(id int) (string, error) {
	users := map[int]string{1: "Alice", 2: "Bob"}
	name, ok := users[id]
	if !ok {
		return "", fmt.Errorf("user %d: %w", id, ErrNotFound)
	}
	return name, nil
}

// CheckPermission returns a forbidden error if role is not admin.
func CheckPermission(role string) error {
	if role != "admin" {
		return fmt.Errorf("role %q: %w", role, ErrForbidden)
	}
	return nil
}

// UnwrapAndClassify checks what kind of error it is using errors.Is.
func UnwrapAndClassify(err error) string {
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
