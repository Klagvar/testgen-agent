// Package dedup удаляет дублирующиеся тест-кейсы из сгенерированного кода.
// Анализирует table-driven тесты: если два кейса имеют одинаковые входные
// данные и ожидаемые значения — оставляет один.
package dedup

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

// Result — результат дедупликации.
type Result struct {
	Code          string   // очищенный код
	Removed       int      // количество удалённых дубликатов
	RemovedNames  []string // имена/описания удалённых кейсов
	TotalBefore   int      // всего кейсов до
	TotalAfter    int      // всего кейсов после
}

// Dedup анализирует тестовый код и удаляет дублирующиеся table-driven test cases.
func Dedup(code string) (*Result, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", code, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse test code: %w", err)
	}

	result := &Result{}
	modified := false

	// Обходим все функции
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Ищем table-driven test patterns
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

	// Форматируем модифицированный AST
	var buf strings.Builder
	if err := format.Node(&buf, fset, file); err != nil {
		// Если формат не удался — возвращаем оригинал
		result.Code = code
		result.Removed = 0
		return result, nil
	}

	result.Code = buf.String()
	return result, nil
}

// deduplicateCompositeLit ищет table-driven test слайс и удаляет дубликаты.
// Возвращает количество удалённых и их имена.
func deduplicateCompositeLit(stmt ast.Stmt, fset *token.FileSet, src string) (int, []string) {
	// Ищем: varName := []struct{...}{...}
	assignStmt, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assignStmt.Rhs) == 0 {
		return 0, nil
	}

	compLit, ok := assignStmt.Rhs[0].(*ast.CompositeLit)
	if !ok {
		return 0, nil
	}

	// Проверяем что это слайс структур
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

	// Извлекаем fingerprints для каждого элемента
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

	// Находим дубликаты
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

	// Удаляем дубликаты из CompositeLit (с конца, чтобы индексы не смещались)
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

// computeFingerprint вычисляет «отпечаток» тест-кейса по значениям полей.
// Игнорирует имя кейса (поле "name") — сравнивает только input/output.
func computeFingerprint(cl *ast.CompositeLit, fset *token.FileSet, src string) string {
	var parts []string

	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			// Positional: используем как есть
			parts = append(parts, nodeToString(elt, fset, src))
			continue
		}

		keyName := ""
		if ident, ok := kv.Key.(*ast.Ident); ok {
			keyName = ident.Name
		}

		// Пропускаем поле "name" — оно не влияет на логику теста
		if strings.EqualFold(keyName, "name") {
			continue
		}

		parts = append(parts, keyName+"="+nodeToString(kv.Value, fset, src))
	}

	return strings.Join(parts, "|")
}

// extractCaseName извлекает имя тест-кейса (поле "name").
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

// nodeToString извлекает строковое представление AST-узла из исходного кода.
func nodeToString(node ast.Node, fset *token.FileSet, src string) string {
	start := fset.Position(node.Pos())
	end := fset.Position(node.End())

	if start.Offset >= 0 && end.Offset <= len(src) && start.Offset < end.Offset {
		return strings.TrimSpace(src[start.Offset:end.Offset])
	}

	// Fallback: форматируем через go/format
	var buf strings.Builder
	format.Node(&buf, fset, node)
	return buf.String()
}
