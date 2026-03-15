// Package analyzer использует go/ast для анализа Go-файлов:
// поиск функций по номерам строк, извлечение сигнатур, типов, зависимостей.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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

// TypeInfo — информация о type-декларации (struct, interface, alias).
type TypeInfo struct {
	Name       string       // имя типа
	Kind       string       // "struct", "interface", "alias", "other"
	Fields     []FieldInfo  // поля структуры
	Methods    []MethodInfo // методы интерфейса
	Underlying string       // базовый тип для алиасов
	Source     string       // исходный код декларации
}

// FieldInfo — поле структуры.
type FieldInfo struct {
	Name string
	Type string
	Tag  string // struct tag (если есть)
}

// MethodInfo — метод интерфейса.
type MethodInfo struct {
	Name      string
	Signature string // (params) returns
}

// FileAnalysis — результат анализа одного Go-файла.
type FileAnalysis struct {
	Package   string      // имя пакета
	Imports   []string    // список импортов
	Functions []FuncInfo  // все функции в файле
	Types     []TypeInfo  // type-декларации (struct, interface, alias)
	FilePath  string      // путь к файлу
}

// PackageAnalysis — результат анализа всего пакета (все .go файлы).
type PackageAnalysis struct {
	Package   string            // имя пакета
	Files     []*FileAnalysis   // анализ каждого файла
	AllTypes  []TypeInfo        // все типы из всех файлов пакета
	AllFuncs  []FuncInfo        // все функции из всех файлов пакета
	FuncIndex map[string]FuncInfo // имя → FuncInfo (для быстрого поиска)
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

	lines := strings.Split(src, "\n")

	// Извлекаем типы (struct, interface, alias)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			ti := extractTypeInfo(fset, typeSpec, lines)
			analysis.Types = append(analysis.Types, ti)
		}
	}

	// Извлекаем функции
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

// AnalyzePackage анализирует все .go файлы в директории пакета.
// Возвращает агрегированный результат с типами и функциями из всех файлов.
func AnalyzePackage(dir string) (*PackageAnalysis, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}

	pkg := &PackageAnalysis{
		FuncIndex: make(map[string]FuncInfo),
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		filePath := filepath.Join(dir, name)
		analysis, err := AnalyzeFile(filePath)
		if err != nil {
			continue // пропускаем файлы с ошибками парсинга
		}

		if pkg.Package == "" {
			pkg.Package = analysis.Package
		}

		pkg.Files = append(pkg.Files, analysis)
		pkg.AllTypes = append(pkg.AllTypes, analysis.Types...)

		for _, fn := range analysis.Functions {
			pkg.AllFuncs = append(pkg.AllFuncs, fn)
			// Для методов: ключ = "ReceiverType.MethodName"
			if fn.Receiver != "" {
				recvName := strings.TrimPrefix(fn.Receiver, "*")
				pkg.FuncIndex[recvName+"."+fn.Name] = fn
			}
			pkg.FuncIndex[fn.Name] = fn
		}
	}

	return pkg, nil
}

// FindUsedTypes определяет, какие типы из пакета использует данная функция.
// Проверяет параметры, возвращаемые значения и ресивер.
func FindUsedTypes(fn FuncInfo, allTypes []TypeInfo) []TypeInfo {
	// Собираем имена типов из сигнатуры функции
	typeNames := make(map[string]bool)

	// Ресивер
	if fn.Receiver != "" {
		name := strings.TrimPrefix(fn.Receiver, "*")
		typeNames[name] = true
	}

	// Параметры
	for _, p := range fn.Params {
		extractTypeNames(p.Type, typeNames)
	}

	// Возвраты
	for _, r := range fn.Returns {
		extractTypeNames(r, typeNames)
	}

	// Фильтруем
	var used []TypeInfo
	for _, ti := range allTypes {
		if typeNames[ti.Name] {
			used = append(used, ti)
			// Рекурсивно: если структура содержит другие пользовательские типы
			for _, field := range ti.Fields {
				extractTypeNames(field.Type, typeNames)
			}
		}
	}

	// Второй проход для рекурсивных зависимостей
	for _, ti := range allTypes {
		if typeNames[ti.Name] {
			found := false
			for _, u := range used {
				if u.Name == ti.Name {
					found = true
					break
				}
			}
			if !found {
				used = append(used, ti)
			}
		}
	}

	return used
}

// FindCalledFunctions находит функции из пакета, вызываемые внутри данной функции.
// Возвращает список FuncInfo вызываемых функций.
func FindCalledFunctions(fn FuncInfo, pkg *PackageAnalysis) []FuncInfo {
	if fn.Body == "" {
		return nil
	}

	// Парсим тело функции для поиска вызовов
	// Оборачиваем тело в package + func для парсинга
	wrapped := fmt.Sprintf("package tmp\n%s", fn.Body)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		return nil
	}

	calledNames := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fun := callExpr.Fun.(type) {
		case *ast.Ident:
			// Простой вызов: Helper()
			calledNames[fun.Name] = true
		case *ast.SelectorExpr:
			// Вызов метода: obj.Method() или pkg.Func()
			if ident, ok := fun.X.(*ast.Ident); ok {
				calledNames[ident.Name+"."+fun.Sel.Name] = true
				calledNames[fun.Sel.Name] = true
			}
		}

		return true
	})

	// Сопоставляем с функциями пакета
	var called []FuncInfo
	seen := make(map[string]bool)

	for name := range calledNames {
		if fi, ok := pkg.FuncIndex[name]; ok {
			if fi.Name != fn.Name && !seen[fi.Name] { // не рекурсия
				called = append(called, fi)
				seen[fi.Name] = true
			}
		}
	}

	return called
}

// extractTypeNames извлекает имена пользовательских типов из строки типа.
func extractTypeNames(typeStr string, names map[string]bool) {
	// Убираем модификаторы: *, [], map[...]
	clean := typeStr
	clean = strings.TrimPrefix(clean, "*")
	clean = strings.TrimPrefix(clean, "[]")
	clean = strings.TrimPrefix(clean, "...")

	// Убираем map[Key]
	if strings.HasPrefix(clean, "map[") {
		idx := strings.Index(clean, "]")
		if idx > 0 {
			keyType := clean[4:idx]
			extractTypeNames(keyType, names)
			clean = clean[idx+1:]
		}
	}

	// Пропускаем встроенные типы и пакеты
	if isBuiltinType(clean) || strings.Contains(clean, ".") {
		return
	}

	if clean != "" && clean[0] >= 'A' && clean[0] <= 'Z' {
		names[clean] = true
	}
}

// isBuiltinType проверяет, является ли тип встроенным.
func isBuiltinType(t string) bool {
	builtins := map[string]bool{
		"bool": true, "byte": true, "complex64": true, "complex128": true,
		"error": true, "float32": true, "float64": true,
		"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
		"rune": true, "string": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"uintptr": true, "any": true, "comparable": true,
		"interface{}": true,
	}
	return builtins[t]
}

// extractTypeInfo извлекает информацию о type-декларации.
func extractTypeInfo(fset *token.FileSet, ts *ast.TypeSpec, lines []string) TypeInfo {
	ti := TypeInfo{
		Name: ts.Name.Name,
	}

	startPos := fset.Position(ts.Pos())
	endPos := fset.Position(ts.End())

	// Исходный код
	if startPos.Line >= 1 && endPos.Line <= len(lines) {
		ti.Source = strings.Join(lines[startPos.Line-1:endPos.Line], "\n")
	}

	switch t := ts.Type.(type) {
	case *ast.StructType:
		ti.Kind = "struct"
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				fi := FieldInfo{
					Type: exprToString(field.Type),
				}
				if field.Tag != nil {
					fi.Tag = field.Tag.Value
				}
				if len(field.Names) == 0 {
					// Embedded field
					fi.Name = fi.Type
				} else {
					for _, name := range field.Names {
						f := fi
						f.Name = name.Name
						ti.Fields = append(ti.Fields, f)
					}
					continue
				}
				ti.Fields = append(ti.Fields, fi)
			}
		}

	case *ast.InterfaceType:
		ti.Kind = "interface"
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				if len(method.Names) == 0 {
					// Embedded interface
					continue
				}
				mi := MethodInfo{
					Name: method.Names[0].Name,
				}
				if ft, ok := method.Type.(*ast.FuncType); ok {
					mi.Signature = funcTypeToString(ft)
				}
				ti.Methods = append(ti.Methods, mi)
			}
		}

	case *ast.Ident:
		ti.Kind = "alias"
		ti.Underlying = t.Name

	case *ast.SelectorExpr:
		ti.Kind = "alias"
		ti.Underlying = exprToString(t)

	default:
		ti.Kind = "other"
		ti.Underlying = exprToString(ts.Type)
	}

	return ti
}

// funcTypeToString преобразует ast.FuncType в строковую сигнатуру.
func funcTypeToString(ft *ast.FuncType) string {
	var sb strings.Builder
	sb.WriteString("(")

	if ft.Params != nil {
		for i, field := range ft.Params.List {
			if i > 0 {
				sb.WriteString(", ")
			}
			typeName := exprToString(field.Type)
			if len(field.Names) > 0 {
				for j, name := range field.Names {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(name.Name + " " + typeName)
				}
			} else {
				sb.WriteString(typeName)
			}
		}
	}

	sb.WriteString(")")

	if ft.Results != nil && len(ft.Results.List) > 0 {
		sb.WriteString(" ")
		if len(ft.Results.List) == 1 && len(ft.Results.List[0].Names) == 0 {
			sb.WriteString(exprToString(ft.Results.List[0].Type))
		} else {
			sb.WriteString("(")
			for i, field := range ft.Results.List {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(exprToString(field.Type))
			}
			sb.WriteString(")")
		}
	}

	return sb.String()
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
