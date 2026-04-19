// Package branchcov computes branch coverage and error-path coverage for
// changed functions by combining an AST walk with a Go coverage profile.
//
// Branch coverage is defined as the fraction of executable branches
// (if / else if / else / switch case / type switch case / select case /
// default case) whose first statement is recorded as executed in the
// coverage profile produced by `go test -coverprofile`.
//
// Error-path coverage is the restriction of branch coverage to the subset
// of branches that guard classic Go error handling (`if err != nil { ... }`
// and its short-statement form `if err := f(); err != nil { ... }`). This
// metric is reported separately because error paths are commonly under-tested
// and diff coverage alone does not expose that asymmetry.
package branchcov

import (
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/gizatulin/testgen-agent/internal/coverage"
)

// Kind enumerates the syntactic kinds of branches recognised by the analyser.
type Kind string

const (
	KindIf         Kind = "if"
	KindElseIf     Kind = "else-if"
	KindElse       Kind = "else"
	KindSwitchCase Kind = "switch-case"
	KindDefault    Kind = "default"
	KindTypeCase   Kind = "type-case"
	KindSelectCase Kind = "select-case"
)

// Branch describes a single executable branch in source code.
type Branch struct {
	Kind Kind
	// Line is the line on which the branch body begins (i.e. the line to
	// check against a coverage profile). For `if` / `else if` this is the
	// first statement inside the brace; for a switch/select case it is the
	// line of the first case statement.
	Line int
	// Function is the name of the enclosing function or method, for
	// reporting. Methods use the bare method name (no receiver).
	Function string
	// IsErrorPath is true for branches whose guard is the classic Go
	// error-handling idiom (`if err != nil { ... }`).
	IsErrorPath bool
}

// Analyze parses the Go source file and returns every branch that belongs to
// one of the named functions. Pass an empty funcNames map to include every
// function in the file.
//
// The returned lines are absolute file-level line numbers, suitable for
// direct look-up against a coverage profile parsed with
// coverage.ParseProfile.
func Analyze(filePath string, funcNames map[string]bool) ([]Branch, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return nil, err
	}

	var out []Branch
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		if len(funcNames) > 0 && !funcNames[fd.Name.Name] {
			continue
		}
		out = append(out, walkBranches(fset, fd.Body, fd.Name.Name)...)
	}
	return out, nil
}

func walkBranches(fset *token.FileSet, body *ast.BlockStmt, fnName string) []Branch {
	var out []Branch

	// Track IfStmt nodes that were already recorded as part of an else-if
	// chain so that the top-level traversal does not double-count them when
	// it visits them as independent if-statements.
	handled := make(map[*ast.IfStmt]struct{})

	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt:
			if _, seen := handled[x]; seen {
				return true // already recorded, but keep walking into body
			}
			out = append(out, Branch{
				Kind:        KindIf,
				Line:        bodyStartLine(fset, x.Body),
				Function:    fnName,
				IsErrorPath: isErrorCheck(x),
			})
			// Walk the else chain explicitly, marking every nested IfStmt
			// as "handled" so that Inspect does not re-record them.
			for e := x.Else; e != nil; {
				switch t := e.(type) {
				case *ast.IfStmt:
					handled[t] = struct{}{}
					out = append(out, Branch{
						Kind:        KindElseIf,
						Line:        bodyStartLine(fset, t.Body),
						Function:    fnName,
						IsErrorPath: isErrorCheck(t),
					})
					e = t.Else
				case *ast.BlockStmt:
					out = append(out, Branch{
						Kind:     KindElse,
						Line:     bodyStartLine(fset, t),
						Function: fnName,
					})
					e = nil
				default:
					e = nil
				}
			}
		case *ast.SwitchStmt:
			collectCaseClauses(fset, x.Body, fnName, KindSwitchCase, &out)
		case *ast.TypeSwitchStmt:
			collectCaseClauses(fset, x.Body, fnName, KindTypeCase, &out)
		case *ast.SelectStmt:
			for _, stmt := range x.Body.List {
				cl, ok := stmt.(*ast.CommClause)
				if !ok {
					continue
				}
				kind := KindSelectCase
				if cl.Comm == nil {
					kind = KindDefault
				}
				out = append(out, Branch{
					Kind:     kind,
					Line:     caseStartLine(fset, cl.Body, cl.Pos()),
					Function: fnName,
				})
			}
		}
		return true
	})

	return out
}

func collectCaseClauses(fset *token.FileSet, body *ast.BlockStmt, fnName string, caseKind Kind, out *[]Branch) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		kind := caseKind
		if len(cc.List) == 0 {
			kind = KindDefault
		}
		*out = append(*out, Branch{
			Kind:     kind,
			Line:     caseStartLine(fset, cc.Body, cc.Pos()),
			Function: fnName,
		})
	}
}

// bodyStartLine returns the line on which the first statement of a block lives.
// Falls back to the opening brace line if the body is empty.
func bodyStartLine(fset *token.FileSet, body *ast.BlockStmt) int {
	if body == nil {
		return 0
	}
	if len(body.List) > 0 {
		return fset.Position(body.List[0].Pos()).Line
	}
	return fset.Position(body.Lbrace).Line
}

// caseStartLine returns the line of the first statement of a case clause.
// Falls back to the case keyword line when the case body is empty (e.g. a
// fall-through placeholder).
func caseStartLine(fset *token.FileSet, caseBody []ast.Stmt, clausePos token.Pos) int {
	if len(caseBody) > 0 {
		return fset.Position(caseBody[0].Pos()).Line
	}
	return fset.Position(clausePos).Line
}

// isErrorCheck reports whether an IfStmt represents the classic
// `if err != nil` idiom (either directly or after a short statement
// assignment).
func isErrorCheck(ifs *ast.IfStmt) bool {
	be, ok := ifs.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if be.Op != token.NEQ && be.Op != token.EQL {
		return false
	}
	// One side must be a nil identifier, the other an "err" identifier.
	left, lOk := be.X.(*ast.Ident)
	right, rOk := be.Y.(*ast.Ident)
	if !lOk || !rOk {
		return false
	}
	hasNil := left.Name == "nil" || right.Name == "nil"
	hasErr := left.Name == "err" || right.Name == "err"
	return hasNil && hasErr
}

// Result is the coverage-agnostic summary returned by Calculate.
type Result struct {
	// Total branches discovered in the analysed functions.
	Total int
	// Covered is the count of branches whose body line is executed by at
	// least one test run.
	Covered int
	// Coverage is Covered / Total expressed as a percentage (100 when Total
	// is zero).
	Coverage float64

	// ErrorPathsTotal is the subset of Total restricted to error-path
	// branches.
	ErrorPathsTotal int
	// ErrorPathsCovered is the subset of Covered restricted to error-path
	// branches.
	ErrorPathsCovered int
	// ErrorPathCoverage is ErrorPathsCovered / ErrorPathsTotal expressed
	// as a percentage (100 when ErrorPathsTotal is zero).
	ErrorPathCoverage float64
}

// Calculate evaluates branch and error-path coverage for the given branches
// against the coverage profile blocks. fileSuffix is the same path suffix used
// elsewhere to match source files against the profile (see
// coverage.CoveredLines).
func Calculate(branches []Branch, blocks []coverage.CoverageBlock, fileSuffix string) Result {
	covered := coverage.CoveredLines(blocks, fileSuffix)
	var res Result

	for _, b := range branches {
		res.Total++
		if b.IsErrorPath {
			res.ErrorPathsTotal++
		}
		if b.Line > 0 && covered[b.Line] {
			res.Covered++
			if b.IsErrorPath {
				res.ErrorPathsCovered++
			}
		}
	}

	if res.Total > 0 {
		res.Coverage = float64(res.Covered) / float64(res.Total) * 100.0
	} else {
		res.Coverage = 100.0
	}
	if res.ErrorPathsTotal > 0 {
		res.ErrorPathCoverage = float64(res.ErrorPathsCovered) / float64(res.ErrorPathsTotal) * 100.0
	} else {
		res.ErrorPathCoverage = 100.0
	}

	return res
}
