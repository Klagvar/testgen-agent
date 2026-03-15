// Package merger реализует AST-слияние тестов:
// берёт существующий _test.go файл и сгенерированный код,
// извлекает только новые тест-функции и вставляет их в конец,
// объединяя импорты.
package merger

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

// MergeResult — результат слияния.
type MergeResult struct {
	Code      string   // итоговый код
	Added     []string // имена добавленных тест-функций
	Skipped   []string // имена пропущенных (уже существуют)
}

// funcSource — исходный код одной функции.
type funcSource struct {
	Name   string
	Source string // исходный код функции
}

// Merge объединяет существующий тестовый файл с новыми сгенерированными тестами.
// Если existing пуст — возвращает generated as-is.
func Merge(existing, generated string) (*MergeResult, error) {
	if strings.TrimSpace(existing) == "" {
		return &MergeResult{Code: generated}, nil
	}

	if strings.TrimSpace(generated) == "" {
		return &MergeResult{Code: existing}, nil
	}

	fset := token.NewFileSet()

	existingFile, err := parser.ParseFile(fset, "existing_test.go", existing, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse existing tests: %w", err)
	}

	genFset := token.NewFileSet()
	genFile, err := parser.ParseFile(genFset, "generated_test.go", generated, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse generated tests: %w", err)
	}

	result := &MergeResult{}

	// Собираем имена существующих функций
	existingFuncs := make(map[string]bool)
	for _, decl := range existingFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		existingFuncs[fn.Name.Name] = true
	}

	// Собираем импорты из обоих файлов (union)
	allImports := make(map[string]string) // path -> alias (empty = no alias)
	for _, imp := range existingFile.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		allImports[path] = alias
	}
	for _, imp := range genFile.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		if _, exists := allImports[path]; !exists {
			allImports[path] = alias
		}
	}

	// Извлекаем исходный код новых функций из generated текста
	genLines := strings.Split(generated, "\n")
	var newFuncs []funcSource

	for _, decl := range genFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if existingFuncs[fn.Name.Name] {
			result.Skipped = append(result.Skipped, fn.Name.Name)
			continue
		}

		// Извлекаем исходный код функции из generated текста
		startLine := genFset.Position(fn.Pos()).Line
		endLine := genFset.Position(fn.End()).Line

		// Включаем doc-комментарий если есть
		if fn.Doc != nil {
			docStart := genFset.Position(fn.Doc.Pos()).Line
			if docStart < startLine {
				startLine = docStart
			}
		}

		if startLine >= 1 && endLine <= len(genLines) {
			funcCode := strings.Join(genLines[startLine-1:endLine], "\n")
			newFuncs = append(newFuncs, funcSource{
				Name:   fn.Name.Name,
				Source: funcCode,
			})
			result.Added = append(result.Added, fn.Name.Name)
		}
	}

	// Также извлекаем новые top-level var/const/type из generated
	existingTopLevel := collectTopLevelNames(existingFile)
	var newDecls []string

	for _, decl := range genFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok == token.IMPORT {
			continue
		}

		hasConflict := false
		for _, spec := range genDecl.Specs {
			switch s := spec.(type) {
			case *ast.ValueSpec:
				for _, name := range s.Names {
					if existingTopLevel[name.Name] {
						hasConflict = true
					}
				}
			case *ast.TypeSpec:
				if existingTopLevel[s.Name.Name] {
					hasConflict = true
				}
			}
		}

		if !hasConflict {
			startLine := genFset.Position(genDecl.Pos()).Line
			endLine := genFset.Position(genDecl.End()).Line
			if startLine >= 1 && endLine <= len(genLines) {
				declCode := strings.Join(genLines[startLine-1:endLine], "\n")
				newDecls = append(newDecls, declCode)
			}
		}
	}

	// Строим итоговый файл
	var sb strings.Builder

	// Package
	sb.WriteString(fmt.Sprintf("package %s\n\n", existingFile.Name.Name))

	// Imports (отсортированные)
	if len(allImports) > 0 {
		sb.WriteString("import (\n")

		var paths []string
		for path := range allImports {
			paths = append(paths, path)
		}
		sort.Strings(paths)

		for _, path := range paths {
			alias := allImports[path]
			if alias != "" && alias != "." && alias != "_" {
				sb.WriteString(fmt.Sprintf("\t%s \"%s\"\n", alias, path))
			} else if alias == "." || alias == "_" {
				sb.WriteString(fmt.Sprintf("\t%s \"%s\"\n", alias, path))
			} else {
				sb.WriteString(fmt.Sprintf("\t\"%s\"\n", path))
			}
		}

		sb.WriteString(")\n")
	}

	// Существующий код (без package и imports)
	existingBody := extractBodyAfterImports(existing, existingFile, fset)
	if existingBody != "" {
		sb.WriteString("\n")
		sb.WriteString(existingBody)
	}

	// Новые top-level declarations
	for _, decl := range newDecls {
		sb.WriteString("\n\n")
		sb.WriteString(decl)
	}

	// Новые функции
	for _, fn := range newFuncs {
		sb.WriteString("\n\n")
		sb.WriteString(fn.Source)
	}

	sb.WriteString("\n")

	// Форматируем через go/format для чистоты
	formatted, err := format.Source([]byte(sb.String()))
	if err != nil {
		// Если форматирование не удалось — возвращаем как есть
		result.Code = sb.String()
		return result, nil
	}

	result.Code = string(formatted)
	return result, nil
}

// extractBodyAfterImports возвращает исходный код файла после package и import.
func extractBodyAfterImports(src string, file *ast.File, fset *token.FileSet) string {
	lines := strings.Split(src, "\n")

	// Находим конец импортов
	lastImportEnd := 0
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		end := fset.Position(genDecl.End()).Line
		if end > lastImportEnd {
			lastImportEnd = end
		}
	}

	// Если нет импортов, начинаем после package
	if lastImportEnd == 0 {
		lastImportEnd = fset.Position(file.Name.End()).Line
	}

	// Берём всё после импортов
	if lastImportEnd < len(lines) {
		body := strings.Join(lines[lastImportEnd:], "\n")
		return strings.TrimLeft(body, "\n\r")
	}

	return ""
}

// collectTopLevelNames собирает все top-level имена (var, const, type).
func collectTopLevelNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			switch s := spec.(type) {
			case *ast.ValueSpec:
				for _, name := range s.Names {
					names[name.Name] = true
				}
			case *ast.TypeSpec:
				names[s.Name.Name] = true
			}
		}
	}
	return names
}

// ExtractNewFuncNames возвращает имена функций из generated, которых нет в existing.
func ExtractNewFuncNames(existing, generated string) ([]string, error) {
	fset := token.NewFileSet()

	existingFile, err := parser.ParseFile(fset, "existing_test.go", existing, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	genFile, err := parser.ParseFile(fset, "generated_test.go", generated, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	existingFuncs := make(map[string]bool)
	for _, decl := range existingFile.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			existingFuncs[fn.Name.Name] = true
		}
	}

	var newFuncs []string
	for _, decl := range genFile.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if !existingFuncs[fn.Name.Name] {
				newFuncs = append(newFuncs, fn.Name.Name)
			}
		}
	}

	return newFuncs, nil
}
