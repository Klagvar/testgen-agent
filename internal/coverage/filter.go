package coverage

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// FilterExecutableLines removes non-executable lines from the set of changed lines.
// Non-executable lines include: blank lines, comments, package declarations,
// import blocks, and lines containing only braces.
func FilterExecutableLines(filePath string, lines []int) []int {
	if len(lines) == 0 {
		return lines
	}

	src, err := os.ReadFile(filePath)
	if err != nil {
		return lines
	}

	return FilterExecutableLinesFromSource(string(src), lines)
}

// FilterExecutableLinesFromSource works like FilterExecutableLines but accepts
// source code directly (useful for testing without files).
func FilterExecutableLinesFromSource(src string, lines []int) []int {
	if len(lines) == 0 {
		return lines
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "filter.go", src, parser.ParseComments)
	if err != nil {
		return filterExecutableLinesTextFallback(src, lines)
	}

	exec := executableLinesFromAST(fset, file)
	removeBraceOnlyLines(src, exec)

	var result []int
	for _, l := range lines {
		if exec[l] {
			result = append(result, l)
		}
	}
	return result
}

func executableLinesFromAST(fset *token.FileSet, file *ast.File) map[int]bool {
	exec := make(map[int]bool)
	addRange := func(start, end token.Pos) {
		if !start.IsValid() || !end.IsValid() {
			return
		}
		sl := fset.Position(start).Line
		el := fset.Position(end).Line
		for l := sl; l <= el; l++ {
			exec[l] = true
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Body != nil {
				addRange(x.Pos(), x.Body.Lbrace)
			}
		case *ast.AssignStmt:
			addRange(x.Pos(), x.End())
		case *ast.ExprStmt:
			addRange(x.Pos(), x.End())
		case *ast.ReturnStmt:
			addRange(x.Pos(), x.End())
		case *ast.IfStmt:
			addRange(x.Pos(), x.End())
		case *ast.ForStmt:
			addRange(x.Pos(), x.End())
		case *ast.RangeStmt:
			addRange(x.Pos(), x.End())
		case *ast.SwitchStmt:
			addRange(x.Pos(), x.End())
		case *ast.TypeSwitchStmt:
			addRange(x.Pos(), x.End())
		case *ast.SelectStmt:
			addRange(x.Pos(), x.End())
		case *ast.DeferStmt:
			addRange(x.Pos(), x.End())
		case *ast.GoStmt:
			addRange(x.Pos(), x.End())
		case *ast.SendStmt:
			addRange(x.Pos(), x.End())
		case *ast.IncDecStmt:
			addRange(x.Pos(), x.End())
		case *ast.BranchStmt:
			addRange(x.Pos(), x.End())
		case *ast.DeclStmt:
			addRange(x.Pos(), x.End())
		}
		return true
	})
	return exec
}

// removeBraceOnlyLines drops lines that contain only `{` or `}` (after trim).
// Statement Pos..End often spans closing braces; those lines are not separate
// coverage counters in Go.
func removeBraceOnlyLines(src string, exec map[int]bool) {
	srcLines := strings.Split(src, "\n")
	for l := range exec {
		if l < 1 || l > len(srcLines) {
			delete(exec, l)
			continue
		}
		trimmed := strings.TrimSpace(srcLines[l-1])
		if trimmed == "{" || trimmed == "}" {
			delete(exec, l)
		}
	}
}

func filterExecutableLinesTextFallback(src string, lines []int) []int {
	srcLines := strings.Split(src, "\n")
	lineSet := make(map[int]bool, len(lines))
	for _, l := range lines {
		lineSet[l] = true
	}

	inBlockComment := false
	inImportBlock := false
	nonExec := make(map[int]bool)

	for i, line := range srcLines {
		lineNum := i + 1
		if !lineSet[lineNum] {
			continue
		}

		trimmed := strings.TrimSpace(line)

		if inBlockComment {
			nonExec[lineNum] = true
			if strings.Contains(trimmed, "*/") {
				inBlockComment = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "/*") {
			nonExec[lineNum] = true
			if !strings.Contains(trimmed, "*/") {
				inBlockComment = true
			}
			continue
		}

		if inImportBlock {
			nonExec[lineNum] = true
			if trimmed == ")" {
				inImportBlock = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "import (") || trimmed == "import (" {
			nonExec[lineNum] = true
			inImportBlock = true
			continue
		}

		if trimmed == "" ||
			strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "package ") ||
			strings.HasPrefix(trimmed, "import \"") ||
			trimmed == "{" ||
			trimmed == "}" ||
			trimmed == "})" ||
			trimmed == ")," ||
			trimmed == ")" {
			nonExec[lineNum] = true
			continue
		}
	}

	var result []int
	for _, l := range lines {
		if !nonExec[l] {
			result = append(result, l)
		}
	}
	return result
}
