package pruner

import (
	"fmt"
	"regexp"
	"strings"
)

// TestFeedback holds a structured summary of a single test's failure.
type TestFeedback struct {
	Name     string
	Passed   bool
	Expected string
	Got      string
	Error    string
	Line     string
}

// ParseStructuredFeedback parses go test -v output and returns structured
// feedback for each test, with expected/got values extracted where possible.
func ParseStructuredFeedback(output string) []TestFeedback {
	results := ParseTestOutput(output)
	if len(results) == 0 {
		return nil
	}

	errorCtx := extractErrorContext(output)

	var feedback []TestFeedback
	for _, r := range results {
		fb := TestFeedback{
			Name:   r.Name,
			Passed: r.Passed,
		}
		if !r.Passed {
			if ctx, ok := errorCtx[r.Name]; ok {
				fb.Expected = ctx.expected
				fb.Got = ctx.got
				fb.Error = ctx.errMsg
				fb.Line = ctx.line
			}
		}
		feedback = append(feedback, fb)
	}
	return feedback
}

type errorContext struct {
	expected string
	got      string
	errMsg   string
	line     string
}

var (
	// Match: expected "X" got "Y" or expected "X", got "Y"
	quotedExpGotRe = regexp.MustCompile(`(?i)(?:expected|want)\s+"([^"]*)",?\s+(?:got|but got)\s+"([^"]*)"`)
	// Match: got "X", want "Y"
	quotedGotWantRe = regexp.MustCompile(`(?i)got\s+"([^"]*)",?\s+want\s+"([^"]*)"`)
	// Unquoted fallback: expected X, got Y (single tokens, strip trailing comma/paren)
	unquotedExpGotRe  = regexp.MustCompile(`(?i)(?:expected|want)\s+([^\s,]+),?\s+(?:got|but got)\s+([^\s,]+)`)
	unquotedGotWantRe = regexp.MustCompile(`(?i)got\s+([^\s,]+),?\s+want\s+([^\s,]+)`)
	lineRe            = regexp.MustCompile(`(\w+_test\.go:\d+):`)
)

func extractErrorContext(output string) map[string]errorContext {
	result := make(map[string]errorContext)
	lines := strings.Split(output, "\n")

	currentTest := ""
	var currentErrors []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "=== RUN") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				currentTest = parts[2]
			}
			currentErrors = nil
			continue
		}

		if strings.HasPrefix(trimmed, "--- FAIL:") || strings.HasPrefix(trimmed, "--- PASS:") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				testName := parts[2]
				if strings.HasPrefix(trimmed, "--- FAIL:") && len(currentErrors) > 0 {
					ctx := errorContext{errMsg: strings.Join(currentErrors, "; ")}

					fullErr := strings.Join(currentErrors, " ")
					if m := quotedExpGotRe.FindStringSubmatch(fullErr); len(m) >= 3 {
						ctx.expected = m[1]
						ctx.got = m[2]
					} else if m := quotedGotWantRe.FindStringSubmatch(fullErr); len(m) >= 3 {
						ctx.got = m[1]
						ctx.expected = m[2]
					} else if m := unquotedExpGotRe.FindStringSubmatch(fullErr); len(m) >= 3 {
						ctx.expected = m[1]
						ctx.got = m[2]
					} else if m := unquotedGotWantRe.FindStringSubmatch(fullErr); len(m) >= 3 {
						ctx.got = m[1]
						ctx.expected = m[2]
					}
					if m := lineRe.FindStringSubmatch(fullErr); len(m) >= 2 {
						ctx.line = m[1]
					}

					result[testName] = ctx
				}
				currentErrors = nil
			}
			continue
		}

		if currentTest != "" && trimmed != "" && !strings.HasPrefix(trimmed, "===") {
			currentErrors = append(currentErrors, trimmed)
		}
	}

	return result
}

// FormatCompactFeedback formats test feedback as a compact summary for the LLM.
func FormatCompactFeedback(feedback []TestFeedback) string {
	var sb strings.Builder
	for _, fb := range feedback {
		if fb.Passed {
			sb.WriteString(fmt.Sprintf("PASS %s\n", fb.Name))
		} else {
			sb.WriteString(fmt.Sprintf("FAIL %s", fb.Name))
			if fb.Expected != "" && fb.Got != "" {
				sb.WriteString(fmt.Sprintf(": expected %q, got %q", fb.Expected, fb.Got))
			} else if fb.Error != "" {
				errShort := fb.Error
				if len(errShort) > 150 {
					errShort = errShort[:150] + "..."
				}
				sb.WriteString(fmt.Sprintf(": %s", errShort))
			}
			if fb.Line != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", fb.Line))
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
