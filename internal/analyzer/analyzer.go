// Package analyzer использует go/ast для анализа Go-файлов:
// поиск функций по номерам строк, извлечение сигнатур, типов, зависимостей.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// FuncInfo содержит информацию об одной функции, извлечённую из AST.
type FuncInfo struct {
	Name       string   // имя функции
	Receiver   string   // ресивер (для методов), например "*Calculator"
	StartLine  int      // первая строка функции
	EndLine    int      // последняя строка функции
	Signature  string   // полная сигнатура: func(a int, b int) (int, error)
	Params     []Param  // параметры
	Returns    []string // типы возвращаемых значений
	Body       string   // тело функции (исходный код)
	DocComment string   // комментарий над функцией
}

// Param — параметр функции.
type Param struct {
	Name string
	Type string
}

// FileAnalysis — результат анализа одного Go-файла.
type FileAnalysis struct {
	Package   string      // имя пакета
	Imports   []string    // список импортов
	Functions []FuncInfo  // все функции в файле
	FilePath  string      // путь к файлу
}

// AnalyzeFile парсит Go-файл и возвращает информацию о всех функциях.
func AnalyzeFile(filePath string) (*FileAnalysis, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл %s: %w", filePath, err)
	}

	return AnalyzeSource(filePath, string(src))
}

// AnalyzeSource парсит исходный код Go и возвращает информацию о всех функциях.
func AnalyzeSource(filename, src string) (*FileAnalysis, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга Go-файла: %w", err)
	}

	analysis := &FileAnalysis{
		Package:  file.Name.Name,
		FilePath: filename,
	}

	// Извлекаем импорты
	for _, imp := range file.Imports {
		path := imp.Path.Value // строка в кавычках, например `"fmt"`
		path = strings.Trim(path, `"`)
		analysis.Imports = append(analysis.Imports, path)
	}

	// Извлекаем функции
	lines := strings.Split(src, "\n")

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		fi := extractFuncInfo(fset, funcDecl, lines)
		analysis.Functions = append(analysis.Functions, fi)
	}

	return analysis, nil
}

// extractFuncInfo извлекает информацию о функции из AST-узла.
func extractFuncInfo(fset *token.FileSet, fn *ast.FuncDecl, lines []string) FuncInfo {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	fi := FuncInfo{
		Name:      fn.Name.Name,
		StartLine: startPos.Line,
		EndLine:   endPos.Line,
	}

	// Ресивер (для методов)
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		fi.Receiver = exprToString(fn.Recv.List[0].Type)
	}

	// Параметры
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			typeName := exprToString(field.Type)
			if len(field.Names) == 0 {
				fi.Params = append(fi.Params, Param{Type: typeName})
			} else {
				for _, name := range field.Names {
					fi.Params = append(fi.Params, Param{
						Name: name.Name,
						Type: typeName,
					})
				}
			}
		}
	}

	// Возвращаемые типы
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			fi.Returns = append(fi.Returns, exprToString(field.Type))
		}
	}

	// Сигнатура
	fi.Signature = buildSignature(fi)

	// Тело функции (исходный код из строк файла)
	if startPos.Line >= 1 && endPos.Line <= len(lines) {
		bodyLines := lines[startPos.Line-1 : endPos.Line]
		fi.Body = strings.Join(bodyLines, "\n")
	}

	// Документация
	if fn.Doc != nil {
		fi.DocComment = fn.Doc.Text()
	}

	return fi
}

// buildSignature строит строковую сигнатуру функции.
func buildSignature(fi FuncInfo) string {
	var sb strings.Builder

	sb.WriteString("func ")

	if fi.Receiver != "" {
		sb.WriteString("(")
		sb.WriteString(fi.Receiver)
		sb.WriteString(") ")
	}

	sb.WriteString(fi.Name)
	sb.WriteString("(")

	for i, p := range fi.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		if p.Name != "" {
			sb.WriteString(p.Name)
			sb.WriteString(" ")
		}
		sb.WriteString(p.Type)
	}

	sb.WriteString(")")

	if len(fi.Returns) > 0 {
		sb.WriteString(" ")
		if len(fi.Returns) == 1 {
			sb.WriteString(fi.Returns[0])
		} else {
			sb.WriteString("(")
			sb.WriteString(strings.Join(fi.Returns, ", "))
			sb.WriteString(")")
		}
	}

	return sb.String()
}

// FindFunctionsByLines находит функции, которые пересекаются с данными номерами строк.
// Это ключевая функция: берём изменённые строки из diff → находим затронутые функции.
func FindFunctionsByLines(analysis *FileAnalysis, changedLines []int) []FuncInfo {
	lineSet := make(map[int]bool)
	for _, l := range changedLines {
		lineSet[l] = true
	}

	var result []FuncInfo
	for _, fn := range analysis.Functions {
		for line := fn.StartLine; line <= fn.EndLine; line++ {
			if lineSet[line] {
				result = append(result, fn)
				break
			}
		}
	}

	return result
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
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + exprToString(t.Elt)
		}
		return "[...]" + exprToString(t.Elt)
	case *ast.MapType:
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + exprToString(t.Elt)
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + exprToString(t.Value)
	default:
		return fmt.Sprintf("%T", expr)
	}
}
