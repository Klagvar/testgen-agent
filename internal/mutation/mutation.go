// Package mutation implements mutation testing:
// injects mutations into source code, runs tests,
// and checks whether the tests detect these mutations.
//
// Mutation Score = killed / total × 100%
package mutation

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MutationType represents a mutation type.
type MutationType string

const (
	MutArithmetic  MutationType = "arithmetic"  // + ↔ -, * ↔ /
	MutComparison  MutationType = "comparison"  // < ↔ <=, == ↔ !=
	MutLogical     MutationType = "logical"     // && ↔ ||
	MutReturn      MutationType = "return"      // return nil → return err
	MutCondNegate  MutationType = "cond_negate" // if cond → if !cond
)

// Mutant represents a single mutation.
type Mutant struct {
	ID          int          // sequential number
	Type        MutationType // mutation type
	File        string       // file
	Line        int          // line
	Original    string       // original operator/expression
	Replacement string       // mutated variant
	FuncName    string       // function containing the mutation
	Killed      bool         // true = tests failed (mutant killed)
	Error       string       // execution error (if any)
}

// Result holds the mutation testing result.
type Result struct {
	Mutants       []Mutant
	Total         int
	Killed        int
	Survived      int
	Errors        int
	MutationScore float64 // killed / (total - errors) * 100
}

// RunMutationTests performs mutation testing.
// sourceFile is the path to a .go file, funcNames are functions to mutate (nil = all),
// moduleRoot is the module root (where go.mod is).
//
// Mutations are applied to temporary copies of the package so the original
// source is never modified, even if the process crashes mid-run.
func RunMutationTests(sourceFile string, funcNames []string, moduleRoot string) (*Result, error) {
	originalBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	original := string(originalBytes)

	mutants, err := GenerateMutants(original, sourceFile, funcNames)
	if err != nil {
		return nil, fmt.Errorf("generate mutants: %w", err)
	}

	result := &Result{Total: len(mutants)}

	pkgDir := filepath.Dir(sourceFile)
	sourceBase := filepath.Base(sourceFile)

	for i := range mutants {
		m := &mutants[i]

		mutatedCode, err := applyMutant(original, m)
		if err != nil {
			m.Error = err.Error()
			result.Errors++
			continue
		}

		tmpRoot, tmpPkg, err := copyPackageToTemp(pkgDir, moduleRoot)
		if err != nil {
			m.Error = fmt.Sprintf("copy to temp: %v", err)
			result.Errors++
			continue
		}

		tmpSourceFile := filepath.Join(tmpPkg, sourceBase)
		if err := os.WriteFile(tmpSourceFile, []byte(mutatedCode), 0644); err != nil {
			os.RemoveAll(tmpRoot)
			m.Error = err.Error()
			result.Errors++
			continue
		}

		killed := runTestsForMutant(tmpRoot, tmpPkg)
		m.Killed = killed

		if killed {
			result.Killed++
		} else {
			result.Survived++
		}

		os.RemoveAll(tmpRoot)
	}

	result.Mutants = mutants

	effective := result.Total - result.Errors
	if effective > 0 {
		result.MutationScore = float64(result.Killed) / float64(effective) * 100.0
	}

	return result, nil
}

// copyPackageToTemp copies Go source files from pkgDir into a temporary
// directory together with go.mod/go.sum from moduleRoot so that `go test`
// can run in isolation.  Returns (tmpRoot, tmpPkgDir, error).
func copyPackageToTemp(pkgDir, moduleRoot string) (string, string, error) {
	tmpRoot, err := os.MkdirTemp("", "testgen-mutation-*")
	if err != nil {
		return "", "", err
	}

	for _, name := range []string{"go.mod", "go.sum"} {
		src := filepath.Join(moduleRoot, name)
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(tmpRoot, name), data, 0644)
	}

	relPath, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil || relPath == "." {
		return tmpRoot, tmpRoot, copyGoFiles(pkgDir, tmpRoot)
	}

	targetDir := filepath.Join(tmpRoot, relPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		os.RemoveAll(tmpRoot)
		return "", "", err
	}

	if err := copyGoFiles(pkgDir, targetDir); err != nil {
		os.RemoveAll(tmpRoot)
		return "", "", err
	}

	return tmpRoot, targetDir, nil
}

func copyGoFiles(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dstDir, entry.Name()), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// GenerateMutants creates a list of potential mutations for a file.
func GenerateMutants(src, filename string, funcNames []string) ([]Mutant, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	funcFilter := make(map[string]bool)
	for _, fn := range funcNames {
		funcFilter[fn] = true
	}

	var mutants []Mutant
	id := 0

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Filter by function names
		if len(funcFilter) > 0 && !funcFilter[funcDecl.Name.Name] {
			continue
		}

		// Walk AST to find mutable nodes
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			if n == nil {
				return false
			}

			switch node := n.(type) {
			case *ast.BinaryExpr:
				muts := binaryMutations(node, fset, funcDecl.Name.Name)
				for _, m := range muts {
					id++
					m.ID = id
					m.File = filename
					mutants = append(mutants, m)
				}

		case *ast.UnaryExpr:
			muts := unaryMutations(node, fset, funcDecl.Name.Name)
			for _, m := range muts {
				id++
				m.ID = id
				m.File = filename
				mutants = append(mutants, m)
			}

		case *ast.ReturnStmt:
			muts := returnMutations(node, fset, funcDecl.Name.Name)
			for _, m := range muts {
				id++
				m.ID = id
				m.File = filename
				mutants = append(mutants, m)
			}
		}

			return true
		})
	}

	return mutants, nil
}

// binaryMutations generates mutations for binary operators.
func binaryMutations(expr *ast.BinaryExpr, fset *token.FileSet, funcName string) []Mutant {
	var mutants []Mutant
	line := fset.Position(expr.OpPos).Line

	replacements := map[token.Token]token.Token{
		// Arithmetic
		token.ADD: token.SUB,
		token.SUB: token.ADD,
		token.MUL: token.QUO,
		token.QUO: token.MUL,

		// Comparisons
		token.LSS: token.LEQ,
		token.LEQ: token.LSS,
		token.GTR: token.GEQ,
		token.GEQ: token.GTR,
		token.EQL: token.NEQ,
		token.NEQ: token.EQL,

		// Logical
		token.LAND: token.LOR,
		token.LOR:  token.LAND,
	}

	if replacement, ok := replacements[expr.Op]; ok {
		mutType := classifyBinaryOp(expr.Op)
		mutants = append(mutants, Mutant{
			Type:        mutType,
			Line:        line,
			Original:    expr.Op.String(),
			Replacement: replacement.String(),
			FuncName:    funcName,
		})
	}

	return mutants
}

// unaryMutations generates mutations for unary operators.
func unaryMutations(expr *ast.UnaryExpr, fset *token.FileSet, funcName string) []Mutant {
	var mutants []Mutant
	line := fset.Position(expr.OpPos).Line

	// !x → x (negation removal)
	if expr.Op == token.NOT {
		mutants = append(mutants, Mutant{
			Type:        MutCondNegate,
			Line:        line,
			Original:    "!",
			Replacement: "" ,
			FuncName:    funcName,
		})
	}

	return mutants
}

// returnMutations generates mutations for return statement literals.
func returnMutations(stmt *ast.ReturnStmt, fset *token.FileSet, funcName string) []Mutant {
	var mutants []Mutant
	line := fset.Position(stmt.Pos()).Line

	for _, result := range stmt.Results {
		switch v := result.(type) {
		case *ast.Ident:
			switch v.Name {
			case "nil":
				mutants = append(mutants, Mutant{
					Type:        MutReturn,
					Line:        line,
					Original:    "nil",
					Replacement: `errors.New("mutant")`,
					FuncName:    funcName,
				})
			case "true":
				mutants = append(mutants, Mutant{
					Type:        MutReturn,
					Line:        line,
					Original:    "true",
					Replacement: "false",
					FuncName:    funcName,
				})
			case "false":
				mutants = append(mutants, Mutant{
					Type:        MutReturn,
					Line:        line,
					Original:    "false",
					Replacement: "true",
					FuncName:    funcName,
				})
			}
		case *ast.BasicLit:
			switch v.Kind {
			case token.INT:
				switch v.Value {
				case "0":
					mutants = append(mutants, Mutant{
						Type:        MutReturn,
						Line:        line,
						Original:    "0",
						Replacement: "1",
						FuncName:    funcName,
					})
				case "1":
					mutants = append(mutants, Mutant{
						Type:        MutReturn,
						Line:        line,
						Original:    "1",
						Replacement: "0",
						FuncName:    funcName,
					})
				}
			case token.STRING:
				if v.Value == `""` {
					mutants = append(mutants, Mutant{
						Type:        MutReturn,
						Line:        line,
						Original:    `""`,
						Replacement: `"mutant"`,
						FuncName:    funcName,
					})
				}
			}
		}
	}

	return mutants
}

// classifyBinaryOp classifies the mutation type by operator.
func classifyBinaryOp(op token.Token) MutationType {
	switch op {
	case token.ADD, token.SUB, token.MUL, token.QUO:
		return MutArithmetic
	case token.LSS, token.LEQ, token.GTR, token.GEQ, token.EQL, token.NEQ:
		return MutComparison
	case token.LAND, token.LOR:
		return MutLogical
	default:
		return "other"
	}
}

// applyMutant applies a mutation to source code via AST manipulation.
func applyMutant(src string, m *Mutant) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, m.File, src, parser.ParseComments)
	if err != nil {
		return "", err
	}

	applied := false

	ast.Inspect(file, func(n ast.Node) bool {
		if applied || n == nil {
			return false
		}

		switch node := n.(type) {
		case *ast.BinaryExpr:
			line := fset.Position(node.OpPos).Line
			if line == m.Line && node.Op.String() == m.Original {
				// Replace operator
				newOp := stringToToken(m.Replacement)
				if newOp != token.ILLEGAL {
					node.Op = newOp
					applied = true
				}
			}

		case *ast.UnaryExpr:
			line := fset.Position(node.OpPos).Line
			if line == m.Line && m.Type == MutCondNegate && node.Op == token.NOT {
				// For !x → x we need to replace UnaryExpr with its operand
				// This is harder via AST, so we use text replacement
				applied = true
			}
		}

		return true
	})

	// For NOT removal — use text replacement
	if m.Type == MutCondNegate && m.Replacement == "" {
		lines := strings.Split(src, "\n")
		if m.Line >= 1 && m.Line <= len(lines) {
			lineContent := lines[m.Line-1]
			// Replace the first "!" before the expression
			mutated := strings.Replace(lineContent, "!", "", 1)
			lines[m.Line-1] = mutated
			return strings.Join(lines, "\n"), nil
		}
	}

	if m.Type == MutReturn {
		lines := strings.Split(src, "\n")
		if m.Line >= 1 && m.Line <= len(lines) {
			lineContent := lines[m.Line-1]
			mutated := strings.Replace(lineContent, m.Original, m.Replacement, 1)
			if mutated != lineContent {
				lines[m.Line-1] = mutated
				return strings.Join(lines, "\n"), nil
			}
		}
	}

	if !applied {
		return "", fmt.Errorf("could not apply mutant #%d at line %d", m.ID, m.Line)
	}

	var buf strings.Builder
	if err := format.Node(&buf, fset, file); err != nil {
		return "", fmt.Errorf("format mutated code: %w", err)
	}

	return buf.String(), nil
}

// stringToToken converts an operator string to token.Token.
func stringToToken(s string) token.Token {
	tokenMap := map[string]token.Token{
		"+":  token.ADD,
		"-":  token.SUB,
		"*":  token.MUL,
		"/":  token.QUO,
		"<":  token.LSS,
		"<=": token.LEQ,
		">":  token.GTR,
		">=": token.GEQ,
		"==": token.EQL,
		"!=": token.NEQ,
		"&&": token.LAND,
		"||": token.LOR,
	}
	if t, ok := tokenMap[s]; ok {
		return t
	}
	return token.ILLEGAL
}

// runTestsForMutant runs go test and returns true if tests failed (mutant killed).
func runTestsForMutant(moduleRoot, pkgDir string) bool {
	cmd := exec.Command("go", "test", "-count=1", "-timeout=30s", "./...")
	cmd.Dir = moduleRoot

	// If pkgDir differs from moduleRoot, use relative path
	if pkgDir != moduleRoot {
		rel, err := filepath.Rel(moduleRoot, pkgDir)
		if err == nil {
			cmd = exec.Command("go", "test", "-count=1", "-timeout=30s", "./"+filepath.ToSlash(rel)+"/...")
			cmd.Dir = moduleRoot
		}
	}

	output, err := cmd.CombinedOutput()
	_ = output

	// If tests failed (exit code != 0) → mutant killed
	if err != nil {
		return true
	}

	// Tests passed → mutant survived (tests are weak)
	return false
}

// FormatResult formats the result as a Markdown table.
func FormatResult(r *Result) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## 🧬 Mutation Testing Results\n\n"))
	sb.WriteString(fmt.Sprintf("**Mutation Score: %.1f%%** (%d killed / %d total",
		r.MutationScore, r.Killed, r.Total))
	if r.Errors > 0 {
		sb.WriteString(fmt.Sprintf(", %d errors", r.Errors))
	}
	sb.WriteString(")\n\n")

	if r.Survived > 0 {
		sb.WriteString("### ⚠️ Survived Mutants (tests did NOT catch these)\n\n")
		sb.WriteString("| # | Function | Line | Mutation | Original | Replacement |\n")
		sb.WriteString("|---|----------|------|----------|----------|-------------|\n")

		for _, m := range r.Mutants {
			if !m.Killed && m.Error == "" {
				sb.WriteString(fmt.Sprintf("| %d | %s | %d | %s | `%s` | `%s` |\n",
					m.ID, m.FuncName, m.Line, m.Type, m.Original, m.Replacement))
			}
		}
		sb.WriteString("\n")
	}

	// Summary by type
	typeStats := make(map[MutationType][2]int) // [killed, total]
	for _, m := range r.Mutants {
		if m.Error != "" {
			continue
		}
		stats := typeStats[m.Type]
		stats[1]++
		if m.Killed {
			stats[0]++
		}
		typeStats[m.Type] = stats
	}

	sb.WriteString("### By Mutation Type\n\n")
	sb.WriteString("| Type | Killed | Total | Score |\n")
	sb.WriteString("|------|--------|-------|-------|\n")

	for _, mt := range []MutationType{MutArithmetic, MutComparison, MutLogical, MutCondNegate, MutReturn} {
		stats, ok := typeStats[mt]
		if !ok {
			continue
		}
		score := 0.0
		if stats[1] > 0 {
			score = float64(stats[0]) / float64(stats[1]) * 100
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.0f%% |\n", mt, stats[0], stats[1], score))
	}

	return sb.String()
}
