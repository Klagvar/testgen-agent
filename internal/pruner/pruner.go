// Package pruner analyzes go test output and removes
// failing tests from the generated file.
//
// Strategy:
// 1. Parse go test output → determine failing test names
// 2. For table-driven tests: determine failing sub-test names
// 3. AST: remove failing test functions entirely or individual
//    entries from the test table if passing cases can be preserved
// 4. Reformat and return the cleaned code
package pruner

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"regexp"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/testjson"
)

// TestResult holds the result of a single test.
type TestResult struct {
	Name   string // full name: TestFoo or TestFoo/sub_case
	Passed bool
}

// Pre-compiled regexes for the legacy textual fallback parser.
var (
	legacyPassRe = regexp.MustCompile(`--- PASS: (\S+)\s`)
	legacyFailRe = regexp.MustCompile(`--- FAIL: (\S+)\s`)
)

// ParseTestOutput parses go test output and returns results for each test
// (including sub-tests).
//
// The parser first tries to interpret the input as a stream of `go test -json`
// events (which is the robust path: each event carries an explicit Test name,
// so parallel / interleaved output is handled correctly). If the input contains
// no JSON events, the parser falls back to the legacy `go test -v` regex parser
// so that older callers and tests continue to work.
func ParseTestOutput(output string) []TestResult {
	if looksLikeJSONStream(output) {
		parsed, err := testjson.Parse(strings.NewReader(output))
		if err == nil && parsed.HasEvents() {
			return fromJSONEvents(parsed.Events)
		}
	}
	return parseTestOutputLegacy(output)
}

// fromJSONEvents converts testjson.Aggregate output to []TestResult while
// dropping non-terminal events (only pass/fail/skip produce a result).
func fromJSONEvents(events []testjson.Event) []TestResult {
	aggregated := testjson.Aggregate(events)
	out := make([]TestResult, 0, len(aggregated))
	for _, r := range aggregated {
		if r.Skipped {
			// Skipped tests do not affect pruning decisions.
			continue
		}
		out = append(out, TestResult{Name: r.Name, Passed: r.Passed})
	}
	return out
}

// parseTestOutputLegacy extracts test results from the textual `go test -v`
// output. Retained so that tools, fixtures, and existing tests that still feed
// the text format keep working.
func parseTestOutputLegacy(output string) []TestResult {
	var results []TestResult
	for _, match := range legacyPassRe.FindAllStringSubmatch(output, -1) {
		results = append(results, TestResult{Name: match[1], Passed: true})
	}
	for _, match := range legacyFailRe.FindAllStringSubmatch(output, -1) {
		results = append(results, TestResult{Name: match[1], Passed: false})
	}
	return results
}

// looksLikeJSONStream reports whether the input appears to be a go test -json
// stream. Detection is intentionally cheap: a stream of events always contains
// at least one line starting with `{"Action":`.
func looksLikeJSONStream(output string) bool {
	return strings.Contains(output, `{"Action":`) || strings.Contains(output, `{"Time":`)
}

// FailingTopLevel returns the names of top-level test functions
// that have at least one failing sub-test.
func FailingTopLevel(results []TestResult) []string {
	failing := make(map[string]bool)

	for _, r := range results {
		if r.Passed {
			continue
		}
		// TestFoo/bar/baz → TestFoo
		topLevel := strings.SplitN(r.Name, "/", 2)[0]
		failing[topLevel] = true
	}

	var names []string
	for name := range failing {
		names = append(names, name)
	}
	return names
}

// FailingSubTests returns a map: TestFuncName → []failingSubTestNames.
func FailingSubTests(results []TestResult) map[string][]string {
	failing := make(map[string][]string)

	for _, r := range results {
		if r.Passed {
			continue
		}
		parts := strings.SplitN(r.Name, "/", 2)
		if len(parts) != 2 {
			continue // top-level fail, not a sub-test
		}
		topLevel := parts[0]
		subTest := parts[1]
		failing[topLevel] = append(failing[topLevel], subTest)
	}

	return failing
}

// AllSubTestsFailing checks whether all sub-tests of the given top-level test failed.
func AllSubTestsFailing(results []TestResult, topLevelName string) bool {
	totalSubs := 0
	failedSubs := 0

	for _, r := range results {
		parts := strings.SplitN(r.Name, "/", 2)
		if parts[0] != topLevelName || len(parts) < 2 {
			continue
		}
		totalSubs++
		if !r.Passed {
			failedSubs++
		}
	}

	// If no sub-tests, the top-level test itself failed
	if totalSubs == 0 {
		return true
	}

	return totalSubs == failedSubs
}

// PruneResult holds the pruning result.
type PruneResult struct {
	Code            string   // cleaned code
	RemovedFuncs    []string // removed test functions
	RemovedSubTests int      // removed sub-test cases from table-driven tests
	KeptTests       int      // number of remaining tests
}

// Prune removes failing tests from generated code.
// Strategy:
// - If all sub-tests in a test function failed → remove the entire function
// - If only some sub-tests failed → try to remove
//   specific entries from the table-driven test (composite literals)
// - If it's not a table-driven test → remove the entire function
func Prune(source string, testOutput string) (*PruneResult, error) {
	results := ParseTestOutput(testOutput)

	if len(results) == 0 {
		return nil, fmt.Errorf("no test results found in output")
	}

	// Check if there are any failing tests
	failingTop := FailingTopLevel(results)
	if len(failingTop) == 0 {
		return &PruneResult{Code: source}, nil // all OK
	}

	failingSubs := FailingSubTests(results)

	// Parse AST
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse test file: %w", err)
	}

	result := &PruneResult{}
	var declsToRemove []*ast.FuncDecl

	for _, topName := range failingTop {
		// Find function in AST
		funcDecl := findFunc(node, topName)
		if funcDecl == nil {
			continue
		}

		if AllSubTestsFailing(results, topName) {
			// All sub-tests failed → remove entire function
			declsToRemove = append(declsToRemove, funcDecl)
			result.RemovedFuncs = append(result.RemovedFuncs, topName)
			continue
		}

		// Some sub-tests failed → try to remove from table
		subs := failingSubs[topName]
		removed := removeTableCases(fset, funcDecl, subs)
		result.RemovedSubTests += removed

		if removed == 0 {
			// Could not remove individual cases → remove entire function
			declsToRemove = append(declsToRemove, funcDecl)
			result.RemovedFuncs = append(result.RemovedFuncs, topName)
		}
	}

	// Remove marked functions from AST
	removeFuncs(node, declsToRemove)

	// Count remaining tests
	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if strings.HasPrefix(fn.Name.Name, "Test") {
				result.KeptTests++
			}
		}
	}

	// Format result
	var buf strings.Builder
	if err := format.Node(&buf, fset, node); err != nil {
		return nil, fmt.Errorf("format pruned code: %w", err)
	}

	result.Code = buf.String()
	return result, nil
}

// findFunc finds a function by name in the AST.
func findFunc(node *ast.File, name string) *ast.FuncDecl {
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

// removeFuncs removes functions from Decls.
func removeFuncs(node *ast.File, toRemove []*ast.FuncDecl) {
	removeSet := make(map[*ast.FuncDecl]bool)
	for _, fn := range toRemove {
		removeSet[fn] = true
	}

	var filtered []ast.Decl
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && removeSet[fn] {
			continue
		}
		filtered = append(filtered, decl)
	}
	node.Decls = filtered
}

// removeTableCases attempts to remove specific sub-test cases
// from a table-driven test. Looks for an array/slice with composite literals
// that have a "name" field matching the failing sub-test name.
//
// Returns the number of removed cases.
func removeTableCases(fset *token.FileSet, funcDecl *ast.FuncDecl, failingSubs []string) int {
	if funcDecl.Body == nil {
		return 0
	}

	failingSet := make(map[string]bool)
	for _, name := range failingSubs {
		failingSet[name] = true
		// Also normalize: go test replaces spaces with _
		failingSet[strings.ReplaceAll(name, "_", " ")] = true
	}

	removed := 0

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		// Look for a slice of composite literals (test table)
		// Check for []struct{...}{...} or tests := []struct{...}{...}
		if len(compLit.Elts) == 0 {
			return true
		}

		// Check that elements are composite literals with a "name" field
		var filteredElts []ast.Expr
		for _, elt := range compLit.Elts {
			innerLit, ok := elt.(*ast.CompositeLit)
			if !ok {
				filteredElts = append(filteredElts, elt)
				continue
			}

			caseName := extractTestCaseName(innerLit)
			if caseName == "" {
				filteredElts = append(filteredElts, elt)
				continue
			}

			// Normalize: go test replaces spaces with _
			normalizedName := strings.ReplaceAll(caseName, " ", "_")

			if failingSet[caseName] || failingSet[normalizedName] {
				removed++
				continue // skip this case
			}

			filteredElts = append(filteredElts, elt)
		}

		if removed > 0 {
			compLit.Elts = filteredElts
		}

		return true
	})

	return removed
}

// extractTestCaseName extracts the test case name from a composite literal.
// Looks for a "name" or "Name" field of type string.
func extractTestCaseName(lit *ast.CompositeLit) string {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		ident, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		if strings.ToLower(ident.Name) != "name" {
			continue
		}

		basicLit, ok := kv.Value.(*ast.BasicLit)
		if !ok {
			continue
		}

		if basicLit.Kind != token.STRING {
			continue
		}

		// Remove quotes
		name := strings.Trim(basicLit.Value, "\"`")
		return name
	}

	return ""
}
