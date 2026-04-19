// Package testjson parses the stream of events emitted by `go test -json`.
//
// The output of `go test -json` is a sequence of JSON objects, one per line,
// each describing an event (test start, output line, test finish, etc.).
// This format is robust to parallel test execution (t.Parallel), because each
// event is explicitly tagged with its test name, unlike the interleaved text
// produced by `go test -v`.
//
// See: https://pkg.go.dev/cmd/test2json
package testjson

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// Event mirrors the structure of a single `go test -json` event.
//
// Action is one of: "start", "run", "pause", "cont", "pass", "bench",
// "fail", "output", "skip". For a detailed description of each action,
// see the test2json documentation.
type Event struct {
	Time    time.Time `json:",omitempty"`
	Action  string    `json:",omitempty"`
	Package string    `json:",omitempty"`
	Test    string    `json:",omitempty"`
	Elapsed float64   `json:",omitempty"`
	Output  string    `json:",omitempty"`
}

// ParseResult holds the outcome of parsing a test2json stream.
type ParseResult struct {
	// Events is the ordered list of parsed test events.
	Events []Event
	// NonJSON contains lines that failed to parse as JSON, joined by "\n".
	// This typically happens when `go test` produces build/compile errors
	// before the JSON stream begins (those lines are emitted as plain text).
	NonJSON string
}

// Parse reads an entire test2json stream.
//
// Each line is attempted as a JSON object. Lines that are not valid JSON
// (for example build errors emitted before the JSON stream starts) are
// collected into ParseResult.NonJSON so callers can still show compilation
// diagnostics.
//
// The function never returns an error for malformed lines; it only returns
// an error if the underlying reader fails.
func Parse(r io.Reader) (ParseResult, error) {
	res := ParseResult{}
	var nonJSON []string

	scanner := bufio.NewScanner(r)
	// Test output can contain very long lines (stack traces, diffs).
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if line[0] != '{' {
			nonJSON = append(nonJSON, string(line))
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			nonJSON = append(nonJSON, string(line))
			continue
		}
		res.Events = append(res.Events, ev)
	}
	if err := scanner.Err(); err != nil {
		return res, err
	}
	if len(nonJSON) > 0 {
		res.NonJSON = strings.Join(nonJSON, "\n")
	}
	return res, nil
}

// Result aggregates events for a single test (or sub-test) into a summary.
type Result struct {
	// Name is the fully-qualified test name: "TestFoo" or "TestFoo/sub_name".
	Name string
	// Package is the package path of the test.
	Package string
	// Passed reports whether the test ended with "pass".
	Passed bool
	// Skipped reports whether the test ended with "skip".
	Skipped bool
	// Elapsed is the duration reported by go test (seconds).
	Elapsed float64
	// Output collects all "output" events that were associated with this test,
	// in the order they were produced.
	Output []string
}

// Aggregate reduces a stream of events to a list of per-test results.
//
// Only events with a non-empty Test field are considered (package-level events
// are ignored). The returned slice preserves the order in which tests finished.
func Aggregate(events []Event) []Result {
	// key: package + "\x00" + test name
	type key struct{ pkg, name string }

	byTest := make(map[key]*Result)
	order := make([]key, 0)

	for _, ev := range events {
		if ev.Test == "" {
			continue
		}
		k := key{pkg: ev.Package, name: ev.Test}
		r, ok := byTest[k]
		if !ok {
			r = &Result{Name: ev.Test, Package: ev.Package}
			byTest[k] = r
			order = append(order, k)
		}
		switch ev.Action {
		case "output":
			if ev.Output != "" {
				r.Output = append(r.Output, ev.Output)
			}
		case "pass":
			r.Passed = true
			r.Elapsed = ev.Elapsed
		case "fail":
			r.Passed = false
			r.Elapsed = ev.Elapsed
		case "skip":
			r.Skipped = true
			r.Elapsed = ev.Elapsed
		}
	}

	out := make([]Result, 0, len(order))
	for _, k := range order {
		out = append(out, *byTest[k])
	}
	return out
}

// HasEvents reports whether the stream produced at least one test event.
// This is a convenience used by callers that need to decide between the
// structured parser and a legacy textual fallback.
func (p ParseResult) HasEvents() bool {
	for _, ev := range p.Events {
		if ev.Test != "" {
			return true
		}
	}
	return false
}
