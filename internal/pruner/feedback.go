package pruner

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/testjson"
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

// ParseStructuredFeedback parses go test output and returns structured
// feedback for each test, with expected/got values extracted where possible.
//
// Like ParseTestOutput, it prefers the `go test -json` event stream (robust
// to parallel execution) and falls back to the legacy textual parser when
// the input is plain `go test -v` output.
func ParseStructuredFeedback(output string) []TestFeedback {
	if looksLikeJSONStream(output) {
		parsed, err := testjson.Parse(strings.NewReader(output))
		if err == nil && parsed.HasEvents() {
			return structuredFeedbackFromJSON(parsed.Events)
		}
	}
	return structuredFeedbackFromText(output)
}

// structuredFeedbackFromJSON builds feedback from a parsed testjson stream.
// This is the robust path: per-test output is already partitioned by test
// name, so expected/got extraction cannot drift between tests even under
// t.Parallel.
func structuredFeedbackFromJSON(events []testjson.Event) []TestFeedback {
	aggregated := testjson.Aggregate(events)
	out := make([]TestFeedback, 0, len(aggregated))
	for _, r := range aggregated {
		if r.Skipped {
			continue
		}
		fb := TestFeedback{
			Name:   r.Name,
			Passed: r.Passed,
		}
		if !r.Passed {
			// Concatenate output lines and extract expected/got and source line.
			joined := strings.TrimSpace(strings.Join(r.Output, ""))
			fb.Error = truncate(joined, 500)
			exp, got := extractExpectedGot(joined)
			fb.Expected = exp
			fb.Got = got
			fb.Line = extractSourceLine(joined)
		}
		out = append(out, fb)
	}
	return out
}

// structuredFeedbackFromText is the legacy textual parser, kept for callers
// that still feed `go test -v` output (and existing tests).
func structuredFeedbackFromText(output string) []TestFeedback {
	results := parseTestOutputLegacy(output)
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

// extractExpectedGot tries to extract a single (expected, got) pair from an
// error message, preferring quoted matches over unquoted ones.
func extractExpectedGot(s string) (expected string, got string) {
	if m := quotedExpGotRe.FindStringSubmatch(s); len(m) >= 3 {
		return m[1], m[2]
	}
	if m := quotedGotWantRe.FindStringSubmatch(s); len(m) >= 3 {
		return m[2], m[1]
	}
	if m := unquotedExpGotRe.FindStringSubmatch(s); len(m) >= 3 {
		return m[1], m[2]
	}
	if m := unquotedGotWantRe.FindStringSubmatch(s); len(m) >= 3 {
		return m[2], m[1]
	}
	return "", ""
}

// extractSourceLine extracts the file:line reference from a test failure
// message (e.g. "foo_test.go:42").
func extractSourceLine(s string) string {
	if m := lineRe.FindStringSubmatch(s); len(m) >= 2 {
		return m[1]
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

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
					ctx.expected, ctx.got = extractExpectedGot(fullErr)
					ctx.line = extractSourceLine(fullErr)
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
