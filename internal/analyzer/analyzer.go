// Package analyzer uses go/ast for Go file analysis:
// finding functions by line numbers, extracting signatures, types, and dependencies.
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

// FuncInfo contains information about a single function extracted from AST.
type FuncInfo struct {
	Name       string   // function name
	Receiver   string   // receiver (for methods), e.g. "*Calculator"
	StartLine  int      // first line of the function
	EndLine    int      // last line of the function
	Signature  string   // full signature: func(a int, b int) (int, error)
	Params     []Param  // parameters
	Returns    []string // return types
	Body       string   // function body (source code)
	DocComment string   // doc comment above the function
	TypeParams string   // generic type params, e.g. "[T any, U comparable]"
}

// Param represents a function parameter.
type Param struct {
	Name string
	Type string
}

// TypeInfo holds information about a type declaration (struct, interface, alias).
type TypeInfo struct {
	Name       string       // type name
	Kind       string       // "struct", "interface", "alias", "other"
	Fields     []FieldInfo  // struct fields
	Methods    []MethodInfo // interface methods
	Underlying string       // underlying type for aliases
	Source     string       // source code of the declaration
	TypeParams string       // generic type params, e.g. "[T comparable]"
}

// FieldInfo represents a struct field.
type FieldInfo struct {
	Name string
	Type string
	Tag  string // struct tag (if any)
}

// MethodInfo represents an interface method.
type MethodInfo struct {
	Name      string
	Signature string // (params) returns
}

// FileAnalysis holds the analysis result for a single Go file.
type FileAnalysis struct {
	Package   string      // package name
	Imports   []string    // import list
	Functions []FuncInfo  // all functions in the file
	Types     []TypeInfo  // type declarations (struct, interface, alias)
	FilePath  string      // file path
}

// PackageAnalysis holds the analysis result for an entire package (all .go files).
type PackageAnalysis struct {
	Package   string            // package name
	Files     []*FileAnalysis   // analysis of each file
	AllTypes  []TypeInfo        // all types from all package files
	AllFuncs  []FuncInfo        // all functions from all package files
	FuncIndex map[string]FuncInfo // name → FuncInfo (for quick lookup)
}

// AnalyzeFile parses a Go file and returns information about all functions.
func AnalyzeFile(filePath string) (*FileAnalysis, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return AnalyzeSource(filePath, string(src))
}

// AnalyzeSource parses Go source code and returns information about all functions.
func AnalyzeSource(filename, src string) (*FileAnalysis, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("error parsing Go file: %w", err)
	}

	analysis := &FileAnalysis{
		Package:  file.Name.Name,
		FilePath: filename,
	}

	// Extract imports
	for _, imp := range file.Imports {
		path := imp.Path.Value // quoted string, e.g. "fmt"
		path = strings.Trim(path, `"`)
		analysis.Imports = append(analysis.Imports, path)
	}

	lines := strings.Split(src, "\n")

	// Extract types (struct, interface, alias)
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

	// Extract functions
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

// AnalyzePackage analyzes all .go files in the package directory.
// Returns an aggregated result with types and functions from all files.
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
			continue // skip files that fail to parse
		}

		if pkg.Package == "" {
			pkg.Package = analysis.Package
		}

		pkg.Files = append(pkg.Files, analysis)
		pkg.AllTypes = append(pkg.AllTypes, analysis.Types...)

		for _, fn := range analysis.Functions {
			pkg.AllFuncs = append(pkg.AllFuncs, fn)
			// For methods: key = "ReceiverType.MethodName"
			if fn.Receiver != "" {
				recvName := strings.TrimPrefix(fn.Receiver, "*")
				pkg.FuncIndex[recvName+"."+fn.Name] = fn
			}
			pkg.FuncIndex[fn.Name] = fn
		}
	}

	return pkg, nil
}

// FindUsedTypes determines which package types are used by the given function.
// Checks parameters, return values, and receiver.
func FindUsedTypes(fn FuncInfo, allTypes []TypeInfo) []TypeInfo {
	// Collect type names from the function signature
	typeNames := make(map[string]bool)

	// Receiver
	if fn.Receiver != "" {
		name := strings.TrimPrefix(fn.Receiver, "*")
		typeNames[name] = true
	}

	// Parameters
	for _, p := range fn.Params {
		extractTypeNames(p.Type, typeNames)
	}

	// Returns
	for _, r := range fn.Returns {
		extractTypeNames(r, typeNames)
	}

	// Filter
	var used []TypeInfo
	for _, ti := range allTypes {
		if typeNames[ti.Name] {
			used = append(used, ti)
			// Recursive: if struct contains other user-defined types
			for _, field := range ti.Fields {
				extractTypeNames(field.Type, typeNames)
			}
		}
	}

	// Second pass for recursive dependencies
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

// FindCalledFunctions finds package functions called within the given function.
// Returns a list of FuncInfo for called functions.
func FindCalledFunctions(fn FuncInfo, pkg *PackageAnalysis) []FuncInfo {
	if fn.Body == "" {
		return nil
	}

	// Parse function body to find calls
	// Wrap body in package + func for parsing
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
			// Simple call: Helper()
			calledNames[fun.Name] = true
		case *ast.SelectorExpr:
			// Method call: obj.Method() or pkg.Func()
			if ident, ok := fun.X.(*ast.Ident); ok {
				calledNames[ident.Name+"."+fun.Sel.Name] = true
				calledNames[fun.Sel.Name] = true
			}
		}

		return true
	})

	// Match against package functions
	var called []FuncInfo
	seen := make(map[string]bool)

	for name := range calledNames {
		if fi, ok := pkg.FuncIndex[name]; ok {
			if fi.Name != fn.Name && !seen[fi.Name] { // not recursion
				called = append(called, fi)
				seen[fi.Name] = true
			}
		}
	}

	return called
}

// extractTypeNames extracts user-defined type names from a type string.
func extractTypeNames(typeStr string, names map[string]bool) {
	// Remove modifiers: *, [], map[...]
	clean := typeStr
	clean = strings.TrimPrefix(clean, "*")
	clean = strings.TrimPrefix(clean, "[]")
	clean = strings.TrimPrefix(clean, "...")

	// Remove map[Key]
	if strings.HasPrefix(clean, "map[") {
		idx := strings.Index(clean, "]")
		if idx > 0 {
			keyType := clean[4:idx]
			extractTypeNames(keyType, names)
			clean = clean[idx+1:]
		}
	}

	// Skip built-in types and packages
	if isBuiltinType(clean) || strings.Contains(clean, ".") {
		return
	}

	if clean != "" && clean[0] >= 'A' && clean[0] <= 'Z' {
		names[clean] = true
	}
}

// isBuiltinType checks whether the type is built-in.
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

// extractTypeInfo extracts information about a type declaration.
func extractTypeInfo(fset *token.FileSet, ts *ast.TypeSpec, lines []string) TypeInfo {
	ti := TypeInfo{
		Name: ts.Name.Name,
	}

	if ts.TypeParams != nil && len(ts.TypeParams.List) > 0 {
		ti.TypeParams = typeParamsToString(ts.TypeParams)
	}

	startPos := fset.Position(ts.Pos())
	endPos := fset.Position(ts.End())

	// Source code
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

// funcTypeToString converts ast.FuncType to a string signature.
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

// extractFuncInfo extracts function information from an AST node.
func extractFuncInfo(fset *token.FileSet, fn *ast.FuncDecl, lines []string) FuncInfo {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	fi := FuncInfo{
		Name:      fn.Name.Name,
		StartLine: startPos.Line,
		EndLine:   endPos.Line,
	}

	// Receiver (for methods)
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		fi.Receiver = exprToString(fn.Recv.List[0].Type)
	}

	// Parameters
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

	// Return types
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			fi.Returns = append(fi.Returns, exprToString(field.Type))
		}
	}

	// Type parameters (generics)
	if fn.Type.TypeParams != nil {
		fi.TypeParams = typeParamsToString(fn.Type.TypeParams)
	}

	// Signature
	fi.Signature = buildSignature(fi)

	// Function body (source code from file lines)
	if startPos.Line >= 1 && endPos.Line <= len(lines) {
		bodyLines := lines[startPos.Line-1 : endPos.Line]
		fi.Body = strings.Join(bodyLines, "\n")
	}

	// Documentation
	if fn.Doc != nil {
		fi.DocComment = fn.Doc.Text()
	}

	return fi
}

// typeParamsToString converts a FieldList of type parameters to "[T any, U comparable]" form.
func typeParamsToString(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	var parts []string
	for _, field := range fl.List {
		constraint := exprToString(field.Type)
		for _, name := range field.Names {
			parts = append(parts, name.Name+" "+constraint)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// buildSignature builds a string signature for a function.
func buildSignature(fi FuncInfo) string {
	var sb strings.Builder

	sb.WriteString("func ")

	if fi.Receiver != "" {
		sb.WriteString("(")
		sb.WriteString(fi.Receiver)
		sb.WriteString(") ")
	}

	sb.WriteString(fi.Name)
	if fi.TypeParams != "" {
		sb.WriteString(fi.TypeParams)
	}
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

// FilterTestable removes functions that shouldn't be tested (e.g. init).
func FilterTestable(funcs []FuncInfo) []FuncInfo {
	var result []FuncInfo
	for _, fn := range funcs {
		if fn.Name == "init" {
			continue
		}
		result = append(result, fn)
	}
	return result
}

// DetectBuildTag checks if a Go source file has a build constraint.
// Returns the constraint string (e.g. "linux") or empty if none.
func DetectBuildTag(src string) string {
	lines := strings.Split(src, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "//go:build ") {
			return strings.TrimPrefix(trimmed, "//go:build ")
		}
		if strings.HasPrefix(trimmed, "// +build ") {
			return strings.TrimPrefix(trimmed, "// +build ")
		}
		if strings.HasPrefix(trimmed, "package ") {
			break
		}
	}
	return ""
}

// FindFunctionsByLines finds functions that overlap with the given line numbers.
// This is the key function: take changed lines from diff → find affected functions.
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

// exprToString converts an AST type expression to a string.
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
		return "func" + funcTypeToString(t)
	case *ast.ChanType:
		return "chan " + exprToString(t.Value)
	case *ast.IndexExpr:
		return exprToString(t.X) + "[" + exprToString(t.Index) + "]"
	case *ast.IndexListExpr:
		var parts []string
		for _, idx := range t.Indices {
			parts = append(parts, exprToString(idx))
		}
		return exprToString(t.X) + "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%T", expr)
	}
}
