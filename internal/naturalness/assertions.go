package naturalness

import (
	"go/ast"
	"strings"
)

// Assertion kinds we recognise. The set is intentionally coarse: we care
// whether a call is *some* sort of assertion and, within that, whether
// it's a nil-check or an error-check. Finer-grained taxonomy would
// complicate the metric without improving its discriminative power.
const (
	kindEqual    = "equal"
	kindNil      = "nil"
	kindError    = "error"
	kindBool     = "bool"
	kindContains = "contains"
	kindLen      = "len"
	kindPanic    = "panic"
	kindFatal    = "fatal" // stdlib t.Fatal / t.Error without further context
	kindOther    = "other"
)

// Assertion describes a classified call. Exported for tests.
type Assertion struct {
	Kind    string
	IsError bool // true if this assertion checks an `error` value
}

// classifyAssertion decides whether call looks like a test assertion. It
// recognises three idioms:
//
//  1. stdlib testing: `t.Error`, `t.Errorf`, `t.Fatal`, `t.Fatalf`
//     (rule of thumb: inside a `TestXxx(t *testing.T)` everything that
//     *fails* the test is an assertion).
//  2. testify/assert and testify/require: `Equal`, `Nil`, `Error`, …
//  3. gotest.tools/v3/assert: `Check`, `Assert`, `Equal`, `ErrorContains`, …
//
// We treat "assertion" as a structural property of the call and do not
// type-check — matching by receiver name and method name is both cheaper
// and robust enough for auto-generated tests.
func classifyAssertion(call *ast.CallExpr) (Assertion, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return Assertion{}, false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return Assertion{}, false
	}
	method := sel.Sel.Name
	recv := ident.Name

	// 1. stdlib testing.T reporting — these inherently assert.
	if recv == "t" || recv == "tb" || strings.HasSuffix(recv, "T") {
		switch method {
		case "Error", "Errorf", "Fatal", "Fatalf":
			return Assertion{Kind: kindFatal, IsError: looksLikeErrorArg(call)}, true
		}
	}

	// 2. Testify-style: receiver is typically `assert` or `require`.
	if isAssertReceiver(recv) {
		kind, isErr := testifyKind(method, call)
		if kind == "" {
			return Assertion{}, false
		}
		return Assertion{Kind: kind, IsError: isErr}, true
	}

	// 3. gotest.tools/assert — dotted `assert.Equal`, `assert.DeepEqual`, …
	if recv == "assert" {
		kind, isErr := gotestToolsKind(method, call)
		if kind != "" {
			return Assertion{Kind: kind, IsError: isErr}, true
		}
	}

	return Assertion{}, false
}

// isAssertReceiver reports whether recv looks like a testify assert / require
// package alias. We accept the common aliases and their conventional ident
// names.
func isAssertReceiver(recv string) bool {
	switch recv {
	case "assert", "require", "a", "r":
		return true
	}
	return false
}

// testifyKind maps a testify call like `assert.Equal` to one of our
// canonical kinds. Returns ("", false) for unknown methods so the caller
// can decide whether to keep searching.
func testifyKind(method string, call *ast.CallExpr) (string, bool) {
	switch method {
	case "Equal", "EqualValues", "Exactly", "Same", "NotSame",
		"JSONEq", "YAMLEq", "InDelta", "InEpsilon":
		return kindEqual, false
	case "NotEqual", "NotEqualValues":
		return kindEqual, false
	case "Nil", "NotNil", "Empty", "NotEmpty", "Zero", "NotZero":
		return kindNil, false
	case "Error", "NoError", "ErrorIs", "ErrorAs", "ErrorContains", "EqualError":
		return kindError, true
	case "True", "False":
		return kindBool, false
	case "Contains", "NotContains", "Subset", "NotSubset",
		"ElementsMatch", "Regexp", "NotRegexp":
		return kindContains, false
	case "Len", "Greater", "GreaterOrEqual", "Less", "LessOrEqual":
		return kindLen, false
	case "Panics", "NotPanics", "PanicsWithValue", "PanicsWithError":
		return kindPanic, false
	case "FileExists", "NoFileExists", "DirExists", "NoDirExists":
		return kindOther, false
	case "Fail", "FailNow":
		return kindFatal, false
	}
	return "", false
}

// gotestToolsKind is the gotest.tools/v3/assert counterpart of testifyKind.
func gotestToolsKind(method string, call *ast.CallExpr) (string, bool) {
	switch method {
	case "Check", "Assert":
		return classifyGotestToolsCheck(call), looksLikeErrorArg(call)
	case "Equal", "DeepEqual":
		return kindEqual, false
	case "NilError":
		return kindError, true
	case "Error", "ErrorContains", "ErrorIs", "ErrorType":
		return kindError, true
	}
	return "", false
}

// classifyGotestToolsCheck inspects the first argument of an
// `assert.Check(t, <expr>)` call to refine the assertion kind. Most uses
// reduce to a boolean expression so we default to kindBool.
func classifyGotestToolsCheck(call *ast.CallExpr) string {
	if len(call.Args) < 2 {
		return kindBool
	}
	switch call.Args[1].(type) {
	case *ast.BinaryExpr:
		return kindEqual
	case *ast.CallExpr:
		return kindOther
	}
	return kindBool
}

// looksLikeErrorArg reports whether at least one argument of call looks
// like an error value. We use syntactic cues only:
//
//   - an identifier named err / e / er
//   - a call whose selector ends in Error / Err
//
// False positives here are harmless (they only inflate the "error
// assertion" percentage slightly), and the alternative of full type
// checking is too expensive for a style metric.
func looksLikeErrorArg(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		if looksLikeErrorExpr(a) {
			return true
		}
	}
	return false
}

func looksLikeErrorExpr(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		name := strings.ToLower(x.Name)
		return name == "err" || name == "e" || name == "er" || strings.HasSuffix(name, "err")
	case *ast.SelectorExpr:
		lower := strings.ToLower(x.Sel.Name)
		if lower == "error" || strings.HasSuffix(lower, "err") {
			return true
		}
		return looksLikeErrorExpr(x.X)
	case *ast.CallExpr:
		return looksLikeErrorExpr(x.Fun)
	}
	return false
}
