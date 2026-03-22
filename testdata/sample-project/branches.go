// Tests: AST-based branch analysis (if/switch/select), MutReturn operator
package sample

import (
	"fmt"
	"strings"
	"sync"
)

// Classify returns a category string based on the score.
// Tests: multi-branch switch, AST branch detection.
func Classify(score int) string {
	switch {
	case score >= 90:
		return "excellent"
	case score >= 70:
		return "good"
	case score >= 50:
		return "average"
	case score >= 0:
		return "poor"
	default:
		return "invalid"
	}
}

// ParseCommand parses a "verb:arg" command string.
// Tests: multi-branch if/else-if, error returns for MutReturn.
func ParseCommand(input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("empty command: %w", ErrValidation)
	}

	parts := strings.SplitN(input, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid format %q: %w", input, ErrValidation)
	}

	verb := strings.TrimSpace(parts[0])
	arg := strings.TrimSpace(parts[1])

	if verb == "" {
		return "", "", fmt.Errorf("empty verb: %w", ErrValidation)
	}

	return verb, arg, nil
}

// SafeCounter is a concurrent counter (tests race detection).
type SafeCounter struct {
	mu    sync.Mutex
	count int
}

func (c *SafeCounter) Increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
}

func (c *SafeCounter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// Abs returns the absolute value. Tests: simple if, MutReturn for 0.
func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// IsEmpty returns true if the string is empty. Tests: MutReturn for bool.
func IsEmpty(s string) bool {
	return len(s) == 0
}

// Sign returns -1, 0, or 1. Tests: MutReturn for multiple return paths.
func Sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}
