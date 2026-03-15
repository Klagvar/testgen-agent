// Package gitdiff сравнивает функции между текущей веткой и base branch.
// Если тело функции не изменилось (только whitespace / комментарии) — можно
// пропустить вызов LLM, даже если функция попала в diff.
//
// Использует `git show <branch>:<file>` для получения кода из base branch,
// затем парсит AST обеих версий и сравнивает нормализованные тела функций.
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

// CompareResult — результат сравнения affected functions с базовой веткой.
type CompareResult struct {
	Changed   []analyzer.FuncInfo // действительно изменённые функции
	Unchanged []analyzer.FuncInfo // не изменились (whitespace/комменты)
	New       []analyzer.FuncInfo // новые функции (нет в base)
}

// GetBaseFileContent получает содержимое файла из base branch через git show.
func GetBaseFileContent(repoDir, baseBranch, filePath string) (string, error) {
	// git show origin/main:path/to/file.go
	ref := baseBranch + ":" + filePath
	cmd := exec.Command("git", "show", ref)
	cmd.Dir = repoDir

	output, err := cmd.Output()
	if err != nil {
		// Файл не существует в base branch — все функции новые
		return "", nil
	}

	return string(output), nil
}

// FilterChanged сравнивает affected functions с их версиями из base branch.
// Возвращает CompareResult, разделяя функции на changed/unchanged/new.
func FilterChanged(
	affectedFuncs []analyzer.FuncInfo,
	repoDir, baseBranch, filePath string,
) (*CompareResult, error) {
	result := &CompareResult{}

	// Получаем файл из base branch
	baseContent, err := GetBaseFileContent(repoDir, baseBranch, filePath)
	if err != nil {
		// Ошибка git — считаем все функции изменёнными
		result.Changed = affectedFuncs
		return result, fmt.Errorf("git show failed: %w", err)
	}

	if baseContent == "" {
		// Файл новый — все функции новые
		result.New = affectedFuncs
		return result, nil
	}

	// Парсим base-версию файла
	baseFuncs, err := parseFunctions(baseContent)
	if err != nil {
		// Не парсится — считаем все изменёнными
		result.Changed = affectedFuncs
		return result, nil
	}

	// Строим индекс: funcKey → normalizedBody
	baseIndex := make(map[string]string)
	for _, fn := range baseFuncs {
		key := funcKey(fn)
		body, _ := normalizeBody(fn.Body)
		baseIndex[key] = body
	}

	// Сравниваем каждую affected function
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

// funcKey формирует уникальный ключ для функции (с учётом ресивера).
func funcKey(fn analyzer.FuncInfo) string {
	if fn.Receiver != "" {
		return fn.Receiver + "." + fn.Name
	}
	return fn.Name
}

// parseFunctions парсит исходный код Go и извлекает функции.
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

// extractFuncBasic извлекает базовую информацию о функции для сравнения.
func extractFuncBasic(fset *token.FileSet, fn *ast.FuncDecl, lines []string) analyzer.FuncInfo {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	fi := analyzer.FuncInfo{
		Name: fn.Name.Name,
	}

	// Ресивер
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		fi.Receiver = exprToString(fn.Recv.List[0].Type)
	}

	// Тело функции
	if startPos.Line >= 1 && endPos.Line <= len(lines) {
		bodyLines := lines[startPos.Line-1 : endPos.Line]
		fi.Body = strings.Join(bodyLines, "\n")
	}

	return fi
}

// normalizeBody нормализует тело функции для сравнения:
// - Убирает комментарии
// - Нормализует whitespace через go/format
// - Результат позволяет сравнивать логику, игнорируя форматирование
func normalizeBody(body string) (string, error) {
	if body == "" {
		return "", nil
	}

	// Оборачиваем тело в package для парсинга
	wrapped := "package tmp\n" + body
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", wrapped, 0) // без ParseComments — комменты убираются
	if err != nil {
		// Если не парсится — сравниваем как есть (строковое сравнение)
		return normalizeString(body), nil
	}

	// Форматируем AST обратно в код (нормализует whitespace)
	var buf strings.Builder
	if err := format.Node(&buf, fset, file); err != nil {
		return normalizeString(body), nil
	}

	// Убираем "package tmp\n" обёртку
	result := buf.String()
	if idx := strings.Index(result, "\n"); idx >= 0 {
		result = result[idx+1:]
	}

	// Убираем пустые строки для стабильного сравнения
	return stripBlankLines(strings.TrimSpace(result)), nil
}

// stripBlankLines убирает пустые строки из текста.
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

// normalizeString выполняет простую нормализацию строки.
func normalizeString(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Пропускаем пустые строки и строки-комментарии
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

// exprToString конвертирует AST-выражение типа в строку.
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
