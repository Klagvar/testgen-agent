package pruner

import (
	"strings"
	"testing"
)

const sampleTestOutput = `=== RUN   TestAdd
=== RUN   TestAdd/positive_numbers
=== RUN   TestAdd/negative_numbers
=== RUN   TestAdd/overflow
--- PASS: TestAdd (0.00s)
    --- PASS: TestAdd/positive_numbers (0.00s)
    --- PASS: TestAdd/negative_numbers (0.00s)
    --- PASS: TestAdd/overflow (0.00s)
=== RUN   TestReverse
=== RUN   TestReverse/simple
=== RUN   TestReverse/unicode_string
=== RUN   TestReverse/empty
--- FAIL: TestReverse (0.00s)
    --- PASS: TestReverse/simple (0.00s)
    --- FAIL: TestReverse/unicode_string (0.00s)
    --- PASS: TestReverse/empty (0.00s)
=== RUN   TestCapitalize
--- FAIL: TestCapitalize (0.00s)
FAIL
`

func TestParseTestOutput(t *testing.T) {
	results := ParseTestOutput(sampleTestOutput)

	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}

	// Count pass/fail
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	if passed < 5 {
		t.Errorf("expected at least 5 passed, got %d", passed)
	}
	if failed < 3 {
		t.Errorf("expected at least 3 failed, got %d", failed)
	}
}

func TestFailingTopLevel(t *testing.T) {
	results := ParseTestOutput(sampleTestOutput)
	failing := FailingTopLevel(results)

	if len(failing) != 2 {
		t.Errorf("expected 2 failing top-level tests, got %d: %v", len(failing), failing)
	}

	failSet := make(map[string]bool)
	for _, n := range failing {
		failSet[n] = true
	}

	if !failSet["TestReverse"] {
		t.Error("expected TestReverse to be failing")
	}
	if !failSet["TestCapitalize"] {
		t.Error("expected TestCapitalize to be failing")
	}
}

func TestFailingSubTests(t *testing.T) {
	results := ParseTestOutput(sampleTestOutput)
	subs := FailingSubTests(results)

	if len(subs["TestReverse"]) != 1 {
		t.Errorf("expected 1 failing sub-test in TestReverse, got %d", len(subs["TestReverse"]))
	}

	if subs["TestReverse"][0] != "unicode_string" {
		t.Errorf("expected unicode_string, got %s", subs["TestReverse"][0])
	}
}

func TestAllSubTestsFailing(t *testing.T) {
	results := ParseTestOutput(sampleTestOutput)

	if AllSubTestsFailing(results, "TestReverse") {
		t.Error("TestReverse has passing sub-tests, should not be 'all failing'")
	}

	if !AllSubTestsFailing(results, "TestCapitalize") {
		t.Error("TestCapitalize has no sub-tests (all fail at top level)")
	}
}

const sampleTestCode = `package sample

import (
	"testing"
)

func TestAdd(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{name: "positive", a: 1, b: 2, want: 3},
		{name: "negative", a: -1, b: -2, want: -3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Add(tt.a, tt.b); got != tt.want {
				t.Errorf("Add() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		name string
		input string
		want  string
	}{
		{name: "simple", input: "hello", want: "olleh"},
		{name: "unicode_string", input: "hello", want: "WRONG"},
		{name: "empty", input: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Reverse(tt.input); got != tt.want {
				t.Errorf("Reverse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCapitalize(t *testing.T) {
	if got := Capitalize("hello"); got != "WRONG" {
		t.Errorf("got %v", got)
	}
}
`

const sampleTestOutputForPrune = `=== RUN   TestAdd
=== RUN   TestAdd/positive
=== RUN   TestAdd/negative
--- PASS: TestAdd (0.00s)
    --- PASS: TestAdd/positive (0.00s)
    --- PASS: TestAdd/negative (0.00s)
=== RUN   TestReverse
=== RUN   TestReverse/simple
=== RUN   TestReverse/unicode_string
=== RUN   TestReverse/empty
--- FAIL: TestReverse (0.00s)
    --- PASS: TestReverse/simple (0.00s)
    --- FAIL: TestReverse/unicode_string (0.00s)
    --- PASS: TestReverse/empty (0.00s)
=== RUN   TestCapitalize
--- FAIL: TestCapitalize (0.00s)
FAIL
`

func TestPrune_RemoveFailingFunction(t *testing.T) {
	result, err := Prune(sampleTestCode, sampleTestOutputForPrune)
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}

	// TestCapitalize should be completely removed (no sub-tests, all failed)
	if strings.Contains(result.Code, "TestCapitalize") {
		t.Error("TestCapitalize should have been removed")
	}

	// TestAdd should be preserved (all passed)
	if !strings.Contains(result.Code, "TestAdd") {
		t.Error("TestAdd should be preserved")
	}

	if len(result.RemovedFuncs) == 0 {
		t.Error("expected at least one removed function")
	}

	t.Logf("Removed funcs: %v", result.RemovedFuncs)
	t.Logf("Removed sub-tests: %d", result.RemovedSubTests)
	t.Logf("Kept tests: %d", result.KeptTests)
}

func TestPrune_RemoveTableCase(t *testing.T) {
	result, err := Prune(sampleTestCode, sampleTestOutputForPrune)
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}

	// TestReverse should be kept (has passing sub-tests)
	if !strings.Contains(result.Code, "TestReverse") {
		t.Error("TestReverse should be preserved (has passing sub-tests)")
	}

	// The "unicode_string" case should be removed from the table
	if strings.Contains(result.Code, "unicode_string") {
		t.Error("unicode_string case should have been removed from table")
	}

	// Other cases should remain
	if !strings.Contains(result.Code, "simple") {
		t.Error("simple case should be preserved")
	}
	if !strings.Contains(result.Code, "empty") {
		t.Error("empty case should be preserved")
	}

	t.Logf("Removed sub-tests: %d", result.RemovedSubTests)
	t.Logf("Code:\n%s", result.Code)
}

func TestPrune_NothingToRemove(t *testing.T) {
	allPassOutput := `=== RUN   TestAdd
--- PASS: TestAdd (0.00s)
PASS
`
	result, err := Prune(sampleTestCode, allPassOutput)
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}

	if len(result.RemovedFuncs) != 0 {
		t.Errorf("expected 0 removed funcs, got %d", len(result.RemovedFuncs))
	}
}

func TestPrune_NoTestResults(t *testing.T) {
	_, err := Prune(sampleTestCode, "some random output")
	if err == nil {
		t.Error("expected error for no test results")
	}
}
