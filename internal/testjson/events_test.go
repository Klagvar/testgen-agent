package testjson

import (
	"strings"
	"testing"
)

func TestParse_SimpleStream(t *testing.T) {
	stream := `{"Action":"run","Test":"TestA"}
{"Action":"output","Test":"TestA","Output":"=== RUN   TestA\n"}
{"Action":"pass","Test":"TestA","Elapsed":0.01}
{"Action":"run","Test":"TestB"}
{"Action":"output","Test":"TestB","Output":"--- FAIL: TestB\n"}
{"Action":"output","Test":"TestB","Output":"    b_test.go:12: expected 1, got 2\n"}
{"Action":"fail","Test":"TestB","Elapsed":0.02}
`
	res, err := Parse(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Events) != 7 {
		t.Fatalf("got %d events, want 7", len(res.Events))
	}
	if !res.HasEvents() {
		t.Error("HasEvents should be true")
	}
	if res.NonJSON != "" {
		t.Errorf("unexpected non-JSON: %q", res.NonJSON)
	}
}

func TestParse_CollectsNonJSONLines(t *testing.T) {
	stream := `# some compile output
./foo.go:5:7: undefined: bar
{"Action":"run","Test":"TestA"}
{"Action":"pass","Test":"TestA"}
FAIL    example.com/foo [build failed]
`
	res, err := Parse(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(res.Events))
	}
	if !strings.Contains(res.NonJSON, "undefined: bar") {
		t.Errorf("NonJSON should contain compile error, got %q", res.NonJSON)
	}
	if !strings.Contains(res.NonJSON, "build failed") {
		t.Errorf("NonJSON should contain build failure line, got %q", res.NonJSON)
	}
}

func TestParse_IgnoresMalformedJSON(t *testing.T) {
	stream := `{"Action":"run","Test":"TestA"}
{broken json
{"Action":"pass","Test":"TestA"}
`
	res, err := Parse(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(res.Events))
	}
	if !strings.Contains(res.NonJSON, "{broken json") {
		t.Errorf("NonJSON should contain broken line, got %q", res.NonJSON)
	}
}

func TestParse_EmptyStream(t *testing.T) {
	res, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Events) != 0 {
		t.Fatalf("expected no events, got %d", len(res.Events))
	}
	if res.HasEvents() {
		t.Error("HasEvents should be false for empty stream")
	}
}

func TestAggregate_PassAndFail(t *testing.T) {
	events := []Event{
		{Action: "run", Test: "TestA"},
		{Action: "output", Test: "TestA", Output: "=== RUN   TestA\n"},
		{Action: "pass", Test: "TestA", Elapsed: 0.015},
		{Action: "run", Test: "TestB"},
		{Action: "output", Test: "TestB", Output: "expected 1, got 2\n"},
		{Action: "fail", Test: "TestB", Elapsed: 0.03},
	}
	results := Aggregate(events)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Name != "TestA" || !results[0].Passed {
		t.Errorf("TestA not passed: %+v", results[0])
	}
	if results[0].Elapsed != 0.015 {
		t.Errorf("TestA elapsed = %v, want 0.015", results[0].Elapsed)
	}
	if results[1].Name != "TestB" || results[1].Passed {
		t.Errorf("TestB should not be passed: %+v", results[1])
	}
	if len(results[1].Output) != 1 || !strings.Contains(results[1].Output[0], "expected 1") {
		t.Errorf("TestB output not captured: %+v", results[1].Output)
	}
}

func TestAggregate_SubTests(t *testing.T) {
	events := []Event{
		{Action: "run", Test: "TestFoo"},
		{Action: "run", Test: "TestFoo/case_one"},
		{Action: "pass", Test: "TestFoo/case_one", Elapsed: 0.001},
		{Action: "run", Test: "TestFoo/case two"},
		{Action: "fail", Test: "TestFoo/case two", Elapsed: 0.002},
		{Action: "fail", Test: "TestFoo", Elapsed: 0.003},
	}
	results := Aggregate(events)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	// Order preserved by first-seen test.
	want := []struct {
		name   string
		passed bool
	}{
		{"TestFoo", false},
		{"TestFoo/case_one", true},
		{"TestFoo/case two", false},
	}
	for i, w := range want {
		if results[i].Name != w.name || results[i].Passed != w.passed {
			t.Errorf("result[%d] = %+v, want name=%s passed=%v", i, results[i], w.name, w.passed)
		}
	}
}

func TestAggregate_Skipped(t *testing.T) {
	events := []Event{
		{Action: "run", Test: "TestSkip"},
		{Action: "skip", Test: "TestSkip"},
	}
	results := Aggregate(events)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !results[0].Skipped {
		t.Errorf("TestSkip should be skipped: %+v", results[0])
	}
	if results[0].Passed {
		t.Errorf("skipped test should not be marked passed")
	}
}

func TestAggregate_IgnoresPackageLevelEvents(t *testing.T) {
	events := []Event{
		{Action: "start", Package: "example.com/foo"},
		{Action: "run", Test: "TestA"},
		{Action: "pass", Test: "TestA"},
		{Action: "output", Package: "example.com/foo", Output: "PASS\n"},
		{Action: "pass", Package: "example.com/foo", Elapsed: 0.1},
	}
	results := Aggregate(events)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (package events should be ignored), %+v", len(results), results)
	}
}

func TestParse_InterleavedParallel(t *testing.T) {
	// Simulates two parallel tests producing interleaved output.
	// The critical property: each event is explicitly tagged with its test,
	// so aggregation correctly attributes output.
	stream := `{"Action":"run","Test":"TestA"}
{"Action":"run","Test":"TestB"}
{"Action":"output","Test":"TestA","Output":"A: step 1\n"}
{"Action":"output","Test":"TestB","Output":"B: step 1\n"}
{"Action":"output","Test":"TestA","Output":"A: step 2\n"}
{"Action":"pass","Test":"TestB","Elapsed":0.01}
{"Action":"pass","Test":"TestA","Elapsed":0.02}
`
	res, err := Parse(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	results := Aggregate(res.Events)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// TestA has two output lines, TestB has one — even though they were interleaved.
	byName := map[string]Result{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if got := len(byName["TestA"].Output); got != 2 {
		t.Errorf("TestA output lines = %d, want 2", got)
	}
	if got := len(byName["TestB"].Output); got != 1 {
		t.Errorf("TestB output lines = %d, want 1", got)
	}
}
