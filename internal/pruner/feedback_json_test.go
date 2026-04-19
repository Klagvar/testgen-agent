package pruner

import (
	"strings"
	"testing"
)

// sampleJSONStream is a miniature go test -json output with one passing top-level
// test, one failing top-level test with a mix of passing/failing sub-tests, and
// one top-level test that fails directly (no sub-tests).
const sampleJSONStream = `{"Action":"run","Test":"TestAdd"}
{"Action":"run","Test":"TestAdd/positive"}
{"Action":"pass","Test":"TestAdd/positive","Elapsed":0.001}
{"Action":"run","Test":"TestAdd/negative"}
{"Action":"pass","Test":"TestAdd/negative","Elapsed":0.001}
{"Action":"pass","Test":"TestAdd","Elapsed":0.003}
{"Action":"run","Test":"TestReverse"}
{"Action":"run","Test":"TestReverse/simple"}
{"Action":"pass","Test":"TestReverse/simple","Elapsed":0.001}
{"Action":"run","Test":"TestReverse/unicode_string"}
{"Action":"output","Test":"TestReverse/unicode_string","Output":"    r_test.go:42: Reverse() = \"olleh\", want \"WRONG\"\n"}
{"Action":"fail","Test":"TestReverse/unicode_string","Elapsed":0.001}
{"Action":"fail","Test":"TestReverse","Elapsed":0.003}
{"Action":"run","Test":"TestCapitalize"}
{"Action":"output","Test":"TestCapitalize","Output":"    c_test.go:12: expected \"HELLO\", got \"hello\"\n"}
{"Action":"fail","Test":"TestCapitalize","Elapsed":0.001}
`

func TestParseTestOutput_JSON(t *testing.T) {
	results := ParseTestOutput(sampleJSONStream)
	if len(results) == 0 {
		t.Fatal("expected results from JSON stream")
	}

	pass, fail := 0, 0
	for _, r := range results {
		if r.Passed {
			pass++
		} else {
			fail++
		}
	}
	if pass < 3 {
		t.Errorf("expected at least 3 passing tests, got %d", pass)
	}
	if fail < 3 {
		t.Errorf("expected at least 3 failing tests, got %d", fail)
	}
}

func TestParseStructuredFeedback_JSON_ExpectedGotFromPerTestOutput(t *testing.T) {
	fb := ParseStructuredFeedback(sampleJSONStream)
	if len(fb) == 0 {
		t.Fatal("expected feedback")
	}

	var cap *TestFeedback
	for i := range fb {
		if fb[i].Name == "TestCapitalize" {
			cap = &fb[i]
			break
		}
	}
	if cap == nil {
		t.Fatal("TestCapitalize feedback not found")
	}
	if cap.Passed {
		t.Error("TestCapitalize must be failed")
	}
	if cap.Expected != "HELLO" || cap.Got != "hello" {
		t.Errorf("expected/got mismatch: %+v", cap)
	}
	if !strings.Contains(cap.Line, "c_test.go") {
		t.Errorf("expected source line for TestCapitalize, got %q", cap.Line)
	}
}

func TestParseStructuredFeedback_JSON_InterleavedParallelDoesNotMixOutput(t *testing.T) {
	// Two failing tests run in parallel; their output is intentionally
	// interleaved. The JSON parser must still attribute each message
	// to its own test.
	interleaved := `{"Action":"run","Test":"TestA"}
{"Action":"run","Test":"TestB"}
{"Action":"output","Test":"TestA","Output":"    a_test.go:5: expected 1, got 2\n"}
{"Action":"output","Test":"TestB","Output":"    b_test.go:7: expected \"x\", got \"y\"\n"}
{"Action":"output","Test":"TestA","Output":"more A context\n"}
{"Action":"fail","Test":"TestB","Elapsed":0.001}
{"Action":"fail","Test":"TestA","Elapsed":0.002}
`

	fb := ParseStructuredFeedback(interleaved)
	byName := map[string]TestFeedback{}
	for _, f := range fb {
		byName[f.Name] = f
	}

	a, ok := byName["TestA"]
	if !ok {
		t.Fatal("TestA missing from feedback")
	}
	if !strings.Contains(a.Error, "a_test.go:5") {
		t.Errorf("TestA error should mention its own file: %q", a.Error)
	}
	if strings.Contains(a.Error, "b_test.go") {
		t.Errorf("TestA error must not leak TestB output: %q", a.Error)
	}

	b, ok := byName["TestB"]
	if !ok {
		t.Fatal("TestB missing from feedback")
	}
	if b.Expected != "x" || b.Got != "y" {
		t.Errorf("TestB expected/got mismatch: %+v", b)
	}
}

func TestParseStructuredFeedback_JSON_SkippedTestsDropped(t *testing.T) {
	stream := `{"Action":"run","Test":"TestOne"}
{"Action":"pass","Test":"TestOne"}
{"Action":"run","Test":"TestSkip"}
{"Action":"skip","Test":"TestSkip"}
`
	fb := ParseStructuredFeedback(stream)
	for _, f := range fb {
		if f.Name == "TestSkip" {
			t.Errorf("skipped tests must not appear in feedback: %+v", f)
		}
	}
}
