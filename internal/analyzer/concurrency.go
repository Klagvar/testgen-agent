package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// ConcurrencyInfo describes concurrency patterns detected in a function.
type ConcurrencyInfo struct {
	HasGoroutines  bool     // go func() or go identifier()
	HasMutex       bool     // sync.Mutex, sync.RWMutex
	HasChannels    bool     // chan, <-
	HasWaitGroup   bool     // sync.WaitGroup
	HasAtomic      bool     // sync/atomic
	HasOnce        bool     // sync.Once
	Patterns       []string // human-readable descriptions of detected patterns
	IsConcurrent   bool     // true if at least one pattern is detected
}

// DetectConcurrency analyzes a function for concurrency patterns.
// Checks the function body, parameters, and used types.
func DetectConcurrency(fn FuncInfo, usedTypes []TypeInfo) ConcurrencyInfo {
	info := ConcurrencyInfo{}

	// 1. Analyze function body via AST
	if fn.Body != "" {
		detectInBody(fn.Body, &info)
	}

	// 2. Analyze parameters
	for _, p := range fn.Params {
		detectInType(p.Type, &info)
	}

	// 3. Analyze return types
	for _, r := range fn.Returns {
		detectInType(r, &info)
	}

	// 4. Analyze receiver type
	if fn.Receiver != "" {
		for _, ti := range usedTypes {
			recvName := strings.TrimPrefix(fn.Receiver, "*")
			if ti.Name == recvName {
				detectInTypeInfo(ti, &info)
			}
		}
	}

	info.IsConcurrent = info.HasGoroutines || info.HasMutex || info.HasChannels ||
		info.HasWaitGroup || info.HasAtomic || info.HasOnce

	return info
}

// detectInBody analyzes the function body via AST.
func detectInBody(body string, info *ConcurrencyInfo) {
	// Wrap body in package for parsing
	wrapped := "package tmp\n" + body
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		// If AST parsing fails, fall back to text search
		detectInString(body, info)
		return
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GoStmt:
			// go func() { ... }() or go someFunc()
			info.HasGoroutines = true
			addPattern(info, "goroutine launch (go statement)")

		case *ast.ChanType:
			// chan T, <-chan T, chan<- T
			info.HasChannels = true
			addPattern(info, "channel type")

		case *ast.SendStmt:
			// ch <- value
			info.HasChannels = true
			addPattern(info, "channel send")

		case *ast.UnaryExpr:
			if node.Op.String() == "<-" {
				// <-ch
				info.HasChannels = true
				addPattern(info, "channel receive")
			}

		case *ast.SelectorExpr:
			if ident, ok := node.X.(*ast.Ident); ok {
				detectSelectorPattern(ident.Name, node.Sel.Name, info)
			}

		case *ast.CallExpr:
			// Detect make(chan ...)
			if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "make" {
				if len(node.Args) > 0 {
					if _, ok := node.Args[0].(*ast.ChanType); ok {
						info.HasChannels = true
						addPattern(info, "channel creation (make)")
					}
				}
			}
		}
		return true
	})

	// Additionally: text search for import-dependent patterns
	detectInString(body, info)
}

// detectSelectorPattern detects patterns like sync.Mutex, atomic.AddInt64, etc.
func detectSelectorPattern(pkg, sel string, info *ConcurrencyInfo) {
	switch pkg {
	case "sync":
		switch sel {
		case "Mutex", "RWMutex":
			info.HasMutex = true
			addPattern(info, "sync."+sel)
		case "WaitGroup":
			info.HasWaitGroup = true
			addPattern(info, "sync.WaitGroup")
		case "Once":
			info.HasOnce = true
			addPattern(info, "sync.Once")
		}
	case "atomic":
		info.HasAtomic = true
		addPattern(info, "sync/atomic."+sel)
	}
}

// detectInType analyzes the string representation of a type.
func detectInType(typeStr string, info *ConcurrencyInfo) {
	if strings.HasPrefix(typeStr, "chan ") || typeStr == "chan" ||
		strings.HasPrefix(typeStr, "<-chan") || strings.HasPrefix(typeStr, "chan<-") {
		info.HasChannels = true
		addPattern(info, "channel parameter/return")
	}
}

// detectInTypeInfo analyzes TypeInfo (receiver struct).
func detectInTypeInfo(ti TypeInfo, info *ConcurrencyInfo) {
	for _, field := range ti.Fields {
		fieldType := field.Type

		// sync.Mutex, sync.RWMutex
		if strings.Contains(fieldType, "Mutex") || strings.Contains(fieldType, "RWMutex") {
			info.HasMutex = true
			addPattern(info, "struct field: "+fieldType)
		}

		// sync.WaitGroup
		if strings.Contains(fieldType, "WaitGroup") {
			info.HasWaitGroup = true
			addPattern(info, "struct field: "+fieldType)
		}

		// sync.Once
		if strings.Contains(fieldType, "Once") {
			info.HasOnce = true
			addPattern(info, "struct field: "+fieldType)
		}

		// channel fields
		if strings.HasPrefix(fieldType, "chan ") || fieldType == "chan" {
			info.HasChannels = true
			addPattern(info, "struct field: channel")
		}
	}
}

// detectInString performs text-based pattern search (fallback and supplement to AST).
func detectInString(body string, info *ConcurrencyInfo) {
	if strings.Contains(body, "sync.Mutex") || strings.Contains(body, "sync.RWMutex") {
		if !info.HasMutex {
			info.HasMutex = true
			addPattern(info, "sync.Mutex (text match)")
		}
	}
	if strings.Contains(body, "sync.WaitGroup") {
		if !info.HasWaitGroup {
			info.HasWaitGroup = true
			addPattern(info, "sync.WaitGroup (text match)")
		}
	}
	if strings.Contains(body, "sync.Once") {
		if !info.HasOnce {
			info.HasOnce = true
			addPattern(info, "sync.Once (text match)")
		}
	}
	if strings.Contains(body, "atomic.") {
		if !info.HasAtomic {
			info.HasAtomic = true
			addPattern(info, "sync/atomic (text match)")
		}
	}
}

// addPattern adds a pattern, avoiding duplicates.
func addPattern(info *ConcurrencyInfo, pattern string) {
	for _, p := range info.Patterns {
		if p == pattern {
			return
		}
	}
	info.Patterns = append(info.Patterns, pattern)
}

// ConcurrencyHint returns a hint string for the LLM prompt.
func (ci ConcurrencyInfo) ConcurrencyHint() string {
	if !ci.IsConcurrent {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("This function uses concurrency primitives:\n")

	for _, p := range ci.Patterns {
		sb.WriteString("  - " + p + "\n")
	}

	sb.WriteString("\nGenerate ADDITIONAL concurrent tests:\n")
	sb.WriteString("1. Launch N goroutines (e.g., 100) calling this function simultaneously\n")
	sb.WriteString("2. Use sync.WaitGroup to synchronize goroutines\n")
	sb.WriteString("3. Verify the final state is correct after all goroutines finish\n")
	sb.WriteString("4. Name concurrent tests with '_Concurrent' suffix, e.g., TestCounter_Concurrent\n")

	if ci.HasChannels {
		sb.WriteString("5. Test channel operations: sending, receiving, closing, select with timeout\n")
	}
	if ci.HasMutex {
		sb.WriteString("5. Test for data race safety: concurrent reads and writes must be consistent\n")
	}

	return sb.String()
}
