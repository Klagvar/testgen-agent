// Package dedup removes duplicate test cases from generated code.
// Analyzes table-driven tests: if two cases have identical input
// data and expected values, keeps only one.
package dedup

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

// Result holds the deduplication result.
type Result struct {
	Code          string   // cleaned code
	Removed       int      // number of removed duplicates
	RemovedNames  []string // names/descriptions of removed cases
	TotalBefore   int      // total cases before
	TotalAfter    int      // total cases after
}

// Dedup analyzes test code and removes duplicate table-driven test cases.
func Dedup(code string) (*Result, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", code, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse test code: %w", err)
	}

	result := &Result{}
	modified := false

	// Walk all functions
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Look for table-driven test patterns
		if funcDecl.Body == nil {
			continue
		}

		for _, stmt := range funcDecl.Body.List {
			removed, names := deduplicateCompositeLit(stmt, fset, code)
			if removed > 0 {
				result.Removed += removed
				result.RemovedNames = append(result.RemovedNames, names...)
				modified = true
			}
		}
	}

	if !modified {
		result.Code = code
		return result, nil
	}

	// Format the modified AST
	var buf strings.Builder
	if err := format.Node(&buf, fset, file); err != nil {
		// If formatting failed — return original
		result.Code = code
		result.Removed = 0
		return result, nil
	}

	result.Code = buf.String()
	return result, nil
}

// deduplicateCompositeLit finds a table-driven test slice and removes duplicates.
// Returns the number of removed items and their names.
func deduplicateCompositeLit(stmt ast.Stmt, fset *token.FileSet, src string) (int, []string) {
	// Look for: varName := []struct{...}{...}
	assignStmt, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assignStmt.Rhs) == 0 {
		return 0, nil
	}

	compLit, ok := assignStmt.Rhs[0].(*ast.CompositeLit)
	if !ok {
		return 0, nil
	}

	// Verify it's a slice of structs
	arrayType, ok := compLit.Type.(*ast.ArrayType)
	if !ok {
		return 0, nil
	}
	if _, ok := arrayType.Elt.(*ast.StructType); !ok {
		return 0, nil
	}

	if len(compLit.Elts) < 2 {
		return 0, nil
	}

	// Extract fingerprints for each element
	type caseInfo struct {
		index       int
		fingerprint string
		name        string
		element     ast.Expr
	}

	var cases []caseInfo

	for i, elt := range compLit.Elts {
		cl, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}

		fp := computeFingerprint(cl, fset, src)
		name := extractCaseName(cl, fset, src)

		cases = append(cases, caseInfo{
			index:       i,
			fingerprint: fp,
			name:        name,
			element:     elt,
		})
	}

	// Find duplicates
	seen := make(map[string]int) // fingerprint → first index
	var toRemove []int
	var removedNames []string

	for _, c := range cases {
		if _, exists := seen[c.fingerprint]; exists {
			toRemove = append(toRemove, c.index)
			removedNames = append(removedNames, c.name)
		} else {
			seen[c.fingerprint] = c.index
		}
	}

	if len(toRemove) == 0 {
		return 0, nil
	}

	// Remove duplicates from CompositeLit (from end so indices don't shift)
	removeSet := make(map[int]bool)
	for _, idx := range toRemove {
		removeSet[idx] = true
	}

	var newElts []ast.Expr
	for i, elt := range compLit.Elts {
		if !removeSet[i] {
			newElts = append(newElts, elt)
		}
	}
	compLit.Elts = newElts

	return len(toRemove), removedNames
}

// computeFingerprint computes a fingerprint for a test case based on field values.
// Ignores the case name ("name" field) — compares only input/output.
func computeFingerprint(cl *ast.CompositeLit, fset *token.FileSet, src string) string {
	var parts []string

	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			// Positional: use as-is
			parts = append(parts, nodeToString(elt, fset, src))
			continue
		}

		keyName := ""
		if ident, ok := kv.Key.(*ast.Ident); ok {
			keyName = ident.Name
		}

		// Skip "name" field — it doesn't affect test logic
		if strings.EqualFold(keyName, "name") {
			continue
		}

		parts = append(parts, keyName+"="+nodeToString(kv.Value, fset, src))
	}

	return strings.Join(parts, "|")
}

// extractCaseName extracts the test case name ("name" field).
func extractCaseName(cl *ast.CompositeLit, fset *token.FileSet, src string) string {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if ident, ok := kv.Key.(*ast.Ident); ok {
			if strings.EqualFold(ident.Name, "name") {
				return nodeToString(kv.Value, fset, src)
			}
		}
	}
	return "<unnamed>"
}

// nodeToString extracts the string representation of an AST node from source code.
func nodeToString(node ast.Node, fset *token.FileSet, src string) string {
	start := fset.Position(node.Pos())
	end := fset.Position(node.End())

	if start.Offset >= 0 && end.Offset <= len(src) && start.Offset < end.Offset {
		return strings.TrimSpace(src[start.Offset:end.Offset])
	}

	// Fallback: format via go/format
	var buf strings.Builder
	format.Node(&buf, fset, node)
	return buf.String()
}
