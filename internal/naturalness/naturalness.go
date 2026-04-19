// Package naturalness computes readability / style metrics for generated
// Go test files. The goal is to distinguish test suites that look "hand
// written" from ones that were clearly machine-produced (no assertions,
// duplicated checks, cryptic names, only-nil assertions).
//
// The design intentionally mirrors the naturalness metrics proposed by
// ASTER (Pizzorno et al., 2025) while specialising each check for the Go
// idioms used in practice: stdlib testing, github.com/stretchr/testify
// and gotest.tools/v3/assert.
//
// Everything is static: we parse *_test.go files once with go/ast and
// aggregate per-test observations into a single Result.
package naturalness

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// Result aggregates naturalness metrics computed over a single test file.
//
// Percentage fields are expressed on the 0..100 scale (not 0..1) so that
// they can be rendered directly in Markdown / JSON reports without
// post-processing. A zero TestCount means the file had no *testing.T
// top-level test functions; consumers should treat such a Result as
// "not applicable" rather than "0%".
type Result struct {
	TestCount              int     `json:"test_count"`
	AssertionRatio         float64 `json:"assertion_ratio"`          // assertions per test, mean
	NoAssertionsPct        float64 `json:"no_assertions_pct"`        // % of tests with zero assertions
	DuplicateAssertionsPct float64 `json:"duplicate_assertions_pct"` // % of tests with repeated asserts
	NilOnlyAssertionsPct   float64 `json:"nil_only_assertions_pct"`  // % of tests whose ONLY assertion is nil/not-nil
	ErrorAssertionsPct     float64 `json:"error_assertions_pct"`     // % of tests that assert on error
	TestNameScore          float64 `json:"test_name_score"`          // 0..100, closer to 100 = test name matches focal function
	VarNameScore           float64 `json:"var_name_score"`           // 0..100, closer to 100 = variable names match their types/values
}

// Analyze parses testPath and computes naturalness metrics over every
// top-level *testing.T test function it contains. focalNames is the list
// of function/method names present in the source-under-test file; empty
// slice disables TestNameScore.
//
// Returned errors originate from the Go parser only; if no tests are
// present the function returns a zero Result and nil error.
func Analyze(testPath string, focalNames []string) (Result, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, testPath, nil, parser.ParseComments)
	if err != nil {
		return Result{}, err
	}
	return analyzeFile(file, focalNames), nil
}

// AnalyzeSource analyses an in-memory test file. Convenient for unit tests
// of this package and for cases where the caller already parsed the file.
func AnalyzeSource(src string, focalNames []string) (Result, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		return Result{}, err
	}
	return analyzeFile(file, focalNames), nil
}

// analyzeFile walks f and produces a Result. It expects f to be the AST
// of a single *_test.go file; non-test files produce a zero Result.
func analyzeFile(f *ast.File, focalNames []string) Result {
	var (
		tests             []*ast.FuncDecl
		totalAsserts      int
		noAssertCount     int
		dupCount          int
		nilOnlyCount      int
		errAssertCount    int
		testNameScoreSum  float64
		testNameScoreN    int
		varNameScoreSum   float64
		varNameScoreN     int
	)

	focalIndex := buildFocalIndex(focalNames)

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !isTopLevelTest(fn) {
			continue
		}
		tests = append(tests, fn)

		obs := inspectTestFunc(fn)
		totalAsserts += obs.assertCount
		if obs.assertCount == 0 {
			noAssertCount++
		}
		if obs.hasDuplicate {
			dupCount++
		}
		if obs.nilOnly {
			nilOnlyCount++
		}
		if obs.hasErrorAssert {
			errAssertCount++
		}

		if len(focalIndex) > 0 {
			if score, ok := scoreTestName(fn.Name.Name, focalIndex); ok {
				testNameScoreSum += score
				testNameScoreN++
			}
		}

		if n, sum := scoreVarNames(fn); n > 0 {
			varNameScoreSum += sum
			varNameScoreN += n
		}
	}

	r := Result{TestCount: len(tests)}
	if r.TestCount == 0 {
		return r
	}

	n := float64(r.TestCount)
	r.AssertionRatio = float64(totalAsserts) / n
	r.NoAssertionsPct = float64(noAssertCount) / n * 100
	r.DuplicateAssertionsPct = float64(dupCount) / n * 100
	r.NilOnlyAssertionsPct = float64(nilOnlyCount) / n * 100
	r.ErrorAssertionsPct = float64(errAssertCount) / n * 100

	if testNameScoreN > 0 {
		r.TestNameScore = testNameScoreSum / float64(testNameScoreN)
	}
	if varNameScoreN > 0 {
		r.VarNameScore = varNameScoreSum / float64(varNameScoreN)
	}
	return r
}

// isTopLevelTest reports whether fn is a top-level `func TestXxx(t *testing.T)`
// declaration. Helpers (TestMain, subtest closures, benchmarks, fuzz) are
// excluded so that the assertion ratio is not diluted by non-test functions.
func isTopLevelTest(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Name == nil {
		return false
	}
	name := fn.Name.Name
	if !strings.HasPrefix(name, "Test") || name == "TestMain" {
		return false
	}
	if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	p := fn.Type.Params.List[0]
	star, ok := p.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return id.Name == "testing" && sel.Sel.Name == "T"
}

// testObservations captures per-test aggregates used by analyzeFile.
type testObservations struct {
	assertCount    int
	hasDuplicate   bool
	nilOnly        bool
	hasErrorAssert bool
}

// inspectTestFunc walks the body of a test function and gathers assertion
// statistics. Table-driven and sub-test patterns are handled uniformly —
// every assertion seen anywhere in the AST counts once, regardless of
// nesting depth.
func inspectTestFunc(fn *ast.FuncDecl) testObservations {
	var obs testObservations
	if fn.Body == nil {
		return obs
	}
	seen := make(map[string]int)
	kinds := make(map[string]int)

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		a, ok := classifyAssertion(call)
		if !ok {
			return true
		}
		obs.assertCount++
		seen[canonicalSignature(call)]++
		kinds[a.Kind]++
		if a.IsError {
			obs.hasErrorAssert = true
		}
		return true
	})

	for _, c := range seen {
		if c >= 2 {
			obs.hasDuplicate = true
			break
		}
	}

	if obs.assertCount > 0 {
		nilKinds := kinds[kindNil]
		if nilKinds == obs.assertCount {
			obs.nilOnly = true
		}
	}

	return obs
}

// canonicalSignature returns a whitespace-normalised textual form of call
// suitable for duplicate detection. Two calls with identical arguments
// produce identical signatures even if formatting differs.
func canonicalSignature(call *ast.CallExpr) string {
	var sb strings.Builder
	writeExpr(&sb, call.Fun)
	sb.WriteByte('(')
	for i, arg := range call.Args {
		if i > 0 {
			sb.WriteByte(',')
		}
		writeExpr(&sb, arg)
	}
	sb.WriteByte(')')
	return sb.String()
}

// writeExpr serialises a small subset of expressions into sb. The goal is
// not fidelity (go/format would do that) but a cheap, stable key for
// duplicate detection: identifiers, selectors, literals and calls cover
// all assertion arguments we care about.
func writeExpr(sb *strings.Builder, e ast.Expr) {
	switch x := e.(type) {
	case *ast.Ident:
		sb.WriteString(x.Name)
	case *ast.SelectorExpr:
		writeExpr(sb, x.X)
		sb.WriteByte('.')
		sb.WriteString(x.Sel.Name)
	case *ast.BasicLit:
		sb.WriteString(x.Value)
	case *ast.CallExpr:
		writeExpr(sb, x.Fun)
		sb.WriteByte('(')
		for i, a := range x.Args {
			if i > 0 {
				sb.WriteByte(',')
			}
			writeExpr(sb, a)
		}
		sb.WriteByte(')')
	case *ast.StarExpr:
		sb.WriteByte('*')
		writeExpr(sb, x.X)
	case *ast.UnaryExpr:
		sb.WriteString(x.Op.String())
		writeExpr(sb, x.X)
	case *ast.BinaryExpr:
		writeExpr(sb, x.X)
		sb.WriteString(x.Op.String())
		writeExpr(sb, x.Y)
	case *ast.IndexExpr:
		writeExpr(sb, x.X)
		sb.WriteByte('[')
		writeExpr(sb, x.Index)
		sb.WriteByte(']')
	default:
		sb.WriteByte('?')
	}
}
