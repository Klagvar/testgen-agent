package pruner

import (
	"strings"
	"testing"
)

func TestParseStructuredFeedback_ExpectedGot(t *testing.T) {
	output := `=== RUN   TestReverse
=== RUN   TestReverse/empty
=== RUN   TestReverse/palindrome
    reverse_test.go:25: expected "aba" got "ab a"
--- FAIL: TestReverse/palindrome (0.00s)
--- PASS: TestReverse/empty (0.00s)
--- FAIL: TestReverse (0.00s)
`
	fb := ParseStructuredFeedback(output)
	if len(fb) == 0 {
		t.Fatal("expected feedback, got none")
	}

	var failFB *TestFeedback
	for i := range fb {
		if fb[i].Name == "TestReverse/palindrome" {
			failFB = &fb[i]
			break
		}
	}
	if failFB == nil {
		t.Fatal("TestReverse/palindrome not found in feedback")
	}
	if failFB.Passed {
		t.Error("expected Passed=false")
	}
	if failFB.Expected != "aba" {
		t.Errorf("Expected=%q, want %q", failFB.Expected, "aba")
	}
	if failFB.Got != "ab a" {
		t.Errorf("Got=%q, want %q", failFB.Got, "ab a")
	}
}

func TestParseStructuredFeedback_GotWant(t *testing.T) {
	output := `=== RUN   TestAdd
    calc_test.go:10: got 5, want 4
--- FAIL: TestAdd (0.00s)
`
	fb := ParseStructuredFeedback(output)
	if len(fb) == 0 {
		t.Fatal("expected feedback")
	}
	if fb[0].Expected != "4" {
		t.Errorf("Expected=%q, want %q", fb[0].Expected, "4")
	}
	if fb[0].Got != "5" {
		t.Errorf("Got=%q, want %q", fb[0].Got, "5")
	}
}

func TestParseStructuredFeedback_AllPassing(t *testing.T) {
	output := `=== RUN   TestAdd
--- PASS: TestAdd (0.00s)
=== RUN   TestSub
--- PASS: TestSub (0.00s)
`
	fb := ParseStructuredFeedback(output)
	for _, f := range fb {
		if !f.Passed {
			t.Errorf("%s should be Passed", f.Name)
		}
	}
}

func TestFormatCompactFeedback(t *testing.T) {
	feedback := []TestFeedback{
		{Name: "TestAdd", Passed: true},
		{Name: "TestSub", Passed: false, Expected: "3", Got: "5", Line: "calc_test.go:20"},
		{Name: "TestMul", Passed: false, Error: "index out of range"},
	}
	result := FormatCompactFeedback(feedback)

	if !strings.Contains(result, "PASS TestAdd") {
		t.Error("missing PASS TestAdd")
	}
	if !strings.Contains(result, "FAIL TestSub") {
		t.Error("missing FAIL TestSub")
	}
	if !strings.Contains(result, `expected "3"`) {
		t.Error("missing expected value")
	}
	if !strings.Contains(result, `got "5"`) {
		t.Error("missing got value")
	}
	if !strings.Contains(result, "calc_test.go:20") {
		t.Error("missing line info")
	}
	if !strings.Contains(result, "FAIL TestMul") {
		t.Error("missing FAIL TestMul")
	}
	if !strings.Contains(result, "index out of range") {
		t.Error("missing error for TestMul")
	}
}

func TestParseStructuredFeedback_Empty(t *testing.T) {
	fb := ParseStructuredFeedback("")
	if fb != nil {
		t.Errorf("expected nil, got %d entries", len(fb))
	}
}
