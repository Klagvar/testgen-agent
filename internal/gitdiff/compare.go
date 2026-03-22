// Package gitdiff compares functions between the current branch and the base branch.
// If a function body has not changed (only whitespace/comments), the LLM call
// can be skipped even if the function appears in the diff.
//
// Uses `git show <branch>:<file>` to retrieve code from the base branch,
// then parses the AST of both versions and compares normalized function bodies.
package gitdiff

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os/exec"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

// CompareResult holds the comparison result of affected functions against the base branch.
type CompareResult struct {
	Changed   []analyzer.FuncInfo // actually changed functions
	Unchanged []analyzer.FuncInfo // unchanged (whitespace/comments only)
	New       []analyzer.FuncInfo // new functions (not in base)
}

// GetBaseFileContent retrieves file content from the base branch via git show.
func GetBaseFileContent(repoDir, baseBranch, filePath string) (string, error) {
	// git show origin/main:path/to/file.go
	ref := baseBranch + ":" + filePath
	cmd := exec.Command("git", "show", ref)
	cmd.Dir = repoDir

	output, err := cmd.Output()
	if err != nil {
		// File does not exist in base branch — all functions are new
		return "", nil
	}

	return string(output), nil
}

// FilterChanged compares affected functions with their base branch versions.
// Returns CompareResult, splitting functions into changed/unchanged/new.
func FilterChanged(
	affectedFuncs []analyzer.FuncInfo,
	repoDir, baseBranch, filePath string,
) (*CompareResult, error) {
	result := &CompareResult{}

	// Get file from base branch
	baseContent, err := GetBaseFileContent(repoDir, baseBranch, filePath)
	if err != nil {
		// Git error — treat all functions as changed
		result.Changed = affectedFuncs
		return result, fmt.Errorf("git show failed: %w", err)
	}

	if baseContent == "" {
		// File is new — all functions are new
		result.New = affectedFuncs
		return result, nil
	}

	// Parse base version of the file
	baseFuncs, err := parseFunctions(baseContent)
	if err != nil {
		// Parse failed — treat all as changed
		result.Changed = affectedFuncs
		return result, nil
	}

	// Build index: funcKey → normalizedBody
	baseIndex := make(map[string]string)
	for _, fn := range baseFuncs {
		key := funcKey(fn)
		body, _ := normalizeBody(fn.Body)
		baseIndex[key] = body
	}

	// Compare each affected function
	for _, fn := range affectedFuncs {
		key := funcKey(fn)
		baseBody, exists := baseIndex[key]

		if !exists {
			result.New = append(result.New, fn)
			continue
		}

		currentBody, _ := normalizeBody(fn.Body)

		if currentBody == baseBody {
			result.Unchanged = append(result.Unchanged, fn)
		} else {
			result.Changed = append(result.Changed, fn)
		}
	}

	return result, nil
}

// funcKey builds a unique key for a function (including receiver).
func funcKey(fn analyzer.FuncInfo) string {
	if fn.Receiver != "" {
		return fn.Receiver + "." + fn.Name
	}
	return fn.Name
}

// parseFunctions parses Go source code and extracts functions.
func parseFunctions(src string) ([]analyzer.FuncInfo, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(src, "\n")
	var funcs []analyzer.FuncInfo

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		fi := extractFuncBasic(fset, funcDecl, lines)
		funcs = append(funcs, fi)
	}

	return funcs, nil
}

// extractFuncBasic extracts basic function info for comparison.
func extractFuncBasic(fset *token.FileSet, fn *ast.FuncDecl, lines []string) analyzer.FuncInfo {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	fi := analyzer.FuncInfo{
		Name: fn.Name.Name,
	}

	// Receiver
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		fi.Receiver = exprToString(fn.Recv.List[0].Type)
	}

	// Function body
	if startPos.Line >= 1 && endPos.Line <= len(lines) {
		bodyLines := lines[startPos.Line-1 : endPos.Line]
		fi.Body = strings.Join(bodyLines, "\n")
	}

	return fi
}

// normalizeBody normalizes a function body for comparison:
// - Strips comments
// - Normalizes whitespace via go/format
// - The result allows comparing logic while ignoring formatting
func normalizeBody(body string) (string, error) {
	if body == "" {
		return "", nil
	}

	// Wrap body in package for parsing
	wrapped := "package tmp\n" + body
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", wrapped, 0) // without ParseComments — comments are stripped
	if err != nil {
		// If parsing fails — compare as-is (string comparison)
		return normalizeString(body), nil
	}

	// Format AST back to code (normalizes whitespace)
	var buf strings.Builder
	if err := format.Node(&buf, fset, file); err != nil {
		return normalizeString(body), nil
	}

	// Remove "package tmp\n" wrapper
	result := buf.String()
	if idx := strings.Index(result, "\n"); idx >= 0 {
		result = result[idx+1:]
	}

	// Remove blank lines for stable comparison
	return stripBlankLines(strings.TrimSpace(result)), nil
}

// stripBlankLines removes blank lines from text.
func stripBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// normalizeString performs simple string normalization.
func normalizeString(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip blank lines and comment lines
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		result = append(result, trimmed)
	}
	return strings.Join(result, "\n")
}

// exprToString converts an AST type expression to a string.
func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	default:
		return fmt.Sprintf("%T", expr)
	}
}
