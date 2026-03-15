// Package mutation реализует мутационное тестирование:
// внедряет мутации в исходный код, запускает тесты
// и проверяет, обнаруживают ли тесты эти мутации.
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

// MutationType — тип мутации.
type MutationType string

const (
	MutArithmetic  MutationType = "arithmetic"  // + ↔ -, * ↔ /
	MutComparison  MutationType = "comparison"  // < ↔ <=, == ↔ !=
	MutLogical     MutationType = "logical"     // && ↔ ||
	MutReturn      MutationType = "return"      // return nil → return err
	MutCondNegate  MutationType = "cond_negate" // if cond → if !cond
)

// Mutant — одна мутация.
type Mutant struct {
	ID          int          // порядковый номер
	Type        MutationType // тип мутации
	File        string       // файл
	Line        int          // строка
	Original    string       // оригинальный оператор / выражение
	Replacement string       // мутированный вариант
	FuncName    string       // функция, в которой мутация
	Killed      bool         // true = тесты упали (мутант убит)
	Error       string       // ошибка выполнения (если есть)
}

// Result — результат мутационного тестирования.
type Result struct {
	Mutants       []Mutant
	Total         int
	Killed        int
	Survived      int
	Errors        int
	MutationScore float64 // killed / (total - errors) * 100
}

// RunMutationTests проводит мутационное тестирование.
// sourceFile — путь к .go файлу, funcNames — функции для мутирования (nil = все),
// moduleRoot — корень модуля (где go.mod).
func RunMutationTests(sourceFile string, funcNames []string, moduleRoot string) (*Result, error) {
	// Читаем оригинальный файл
	originalBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	original := string(originalBytes)

	// Генерируем мутанты
	mutants, err := GenerateMutants(original, sourceFile, funcNames)
	if err != nil {
		return nil, fmt.Errorf("generate mutants: %w", err)
	}

	result := &Result{
		Total: len(mutants),
	}

	pkgDir := filepath.Dir(sourceFile)

	for i := range mutants {
		m := &mutants[i]

		// Применяем мутацию
		mutatedCode, err := applyMutant(original, m)
		if err != nil {
			m.Error = err.Error()
			result.Errors++
			continue
		}

		// Записываем мутированный файл
		if err := os.WriteFile(sourceFile, []byte(mutatedCode), 0644); err != nil {
			m.Error = err.Error()
			result.Errors++
			continue
		}

		// Запускаем тесты
		killed := runTestsForMutant(moduleRoot, pkgDir)
		m.Killed = killed

		if killed {
			result.Killed++
		} else {
			result.Survived++
		}

		// Восстанавливаем оригинал
		os.WriteFile(sourceFile, originalBytes, 0644)
	}

	result.Mutants = mutants

	effective := result.Total - result.Errors
	if effective > 0 {
		result.MutationScore = float64(result.Killed) / float64(effective) * 100.0
	}

	return result, nil
}

// GenerateMutants создаёт список потенциальных мутаций для файла.
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

		// Фильтр по именам функций
		if len(funcFilter) > 0 && !funcFilter[funcDecl.Name.Name] {
			continue
		}

		// Обход AST для поиска мутируемых узлов
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
			}

			return true
		})
	}

	return mutants, nil
}

// binaryMutations генерирует мутации для бинарных операторов.
func binaryMutations(expr *ast.BinaryExpr, fset *token.FileSet, funcName string) []Mutant {
	var mutants []Mutant
	line := fset.Position(expr.OpPos).Line

	replacements := map[token.Token]token.Token{
		// Арифметические
		token.ADD: token.SUB,
		token.SUB: token.ADD,
		token.MUL: token.QUO,
		token.QUO: token.MUL,

		// Сравнения
		token.LSS: token.LEQ,
		token.LEQ: token.LSS,
		token.GTR: token.GEQ,
		token.GEQ: token.GTR,
		token.EQL: token.NEQ,
		token.NEQ: token.EQL,

		// Логические
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

// unaryMutations генерирует мутации для унарных операторов.
func unaryMutations(expr *ast.UnaryExpr, fset *token.FileSet, funcName string) []Mutant {
	var mutants []Mutant
	line := fset.Position(expr.OpPos).Line

	// !x → x (удаление отрицания)
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

// classifyBinaryOp определяет тип мутации по оператору.
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

// applyMutant применяет мутацию к исходному коду через AST-манипуляцию.
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
				// Заменяем оператор
				newOp := stringToToken(m.Replacement)
				if newOp != token.ILLEGAL {
					node.Op = newOp
					applied = true
				}
			}

		case *ast.UnaryExpr:
			line := fset.Position(node.OpPos).Line
			if line == m.Line && m.Type == MutCondNegate && node.Op == token.NOT {
				// Для !x → x нужно заменить UnaryExpr на его операнд
				// Это сложнее через AST, поэтому используем текстовую замену
				applied = true
			}
		}

		return true
	})

	// Для случая с удалением NOT — используем текстовую замену
	if m.Type == MutCondNegate && m.Replacement == "" {
		lines := strings.Split(src, "\n")
		if m.Line >= 1 && m.Line <= len(lines) {
			lineContent := lines[m.Line-1]
			// Заменяем первый "!" перед выражением
			mutated := strings.Replace(lineContent, "!", "", 1)
			lines[m.Line-1] = mutated
			return strings.Join(lines, "\n"), nil
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

// stringToToken конвертирует строку оператора в token.Token.
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

// runTestsForMutant запускает go test и возвращает true если тесты упали (мутант убит).
func runTestsForMutant(moduleRoot, pkgDir string) bool {
	cmd := exec.Command("go", "test", "-count=1", "-timeout=30s", "./...")
	cmd.Dir = moduleRoot

	// Если pkgDir отличается от moduleRoot, используем относительный путь
	if pkgDir != moduleRoot {
		rel, err := filepath.Rel(moduleRoot, pkgDir)
		if err == nil {
			cmd = exec.Command("go", "test", "-count=1", "-timeout=30s", "./"+filepath.ToSlash(rel)+"/...")
			cmd.Dir = moduleRoot
		}
	}

	output, err := cmd.CombinedOutput()
	_ = output

	// Если тесты упали (exit code != 0) → мутант убит
	if err != nil {
		return true
	}

	// Тесты прошли → мутант выжил (тесты слабые)
	return false
}

// FormatResult форматирует результат в Markdown-таблицу.
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
