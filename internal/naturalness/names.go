package naturalness

import (
	"go/ast"
	"strings"
	"unicode"
)

// buildFocalIndex lowercases and de-duplicates focalNames, returning the
// empty map when the caller did not provide any focal symbols.
func buildFocalIndex(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		out[strings.ToLower(n)] = struct{}{}
	}
	return out
}

// scoreTestName returns a 0..100 closeness score between the test's name
// and the most similar focal symbol. The test name is trimmed of the
// conventional `Test` prefix, split on `_` and camelCase, and compared to
// every focal name via normalised Levenshtein distance. The best match
// wins. Returns (score, false) when no focal names were supplied.
func scoreTestName(testName string, focal map[string]struct{}) (float64, bool) {
	if len(focal) == 0 {
		return 0, false
	}
	tokens := tokenizeTestName(testName)
	if len(tokens) == 0 {
		return 0, false
	}
	candidate := strings.ToLower(tokens[0])
	best := 0.0
	for f := range focal {
		if s := similarity(candidate, f); s > best {
			best = s
		}
	}
	// Give partial credit when later tokens contain the focal name,
	// which is common in table-driven tests named e.g. `TestUser_Create`.
	if len(tokens) > 1 {
		joined := strings.ToLower(strings.Join(tokens, ""))
		for f := range focal {
			if strings.Contains(joined, f) {
				if v := 0.9; v > best {
					best = v
				}
			}
		}
	}
	return best * 100, true
}

// scoreVarNames accumulates similarity scores between locally declared
// variables and their right-hand side types / constructors. The result is
// a (count, sum) pair so that callers can combine multiple functions
// into one mean value.
//
// Considered declarations:
//   - `foo := NewBar(...)`     → compare `foo` against `Bar`
//   - `foo := Bar{...}`        → compare `foo` against `Bar`
//   - `var foo Bar`            → compare `foo` against `Bar`
//   - `foo, err := …`          → skipped; `err` is standard
func scoreVarNames(fn *ast.FuncDecl) (count int, sum float64) {
	if fn.Body == nil {
		return 0, 0
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range s.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name == "_" || isStandardVar(ident.Name) {
					continue
				}
				if i >= len(s.Rhs) {
					continue
				}
				if label := rhsLabel(s.Rhs[i]); label != "" {
					sum += similarity(strings.ToLower(ident.Name), strings.ToLower(label))
					count++
				}
			}
		case *ast.DeclStmt:
			gd, ok := s.Decl.(*ast.GenDecl)
			if !ok {
				return true
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				label := typeLabel(vs.Type)
				if label == "" {
					continue
				}
				for _, name := range vs.Names {
					if name.Name == "_" || isStandardVar(name.Name) {
						continue
					}
					sum += similarity(strings.ToLower(name.Name), strings.ToLower(label))
					count++
				}
			}
		}
		return true
	})
	return count, sum
}

// rhsLabel extracts a "natural" label from an expression used as the RHS
// of an assignment. Returns the empty string when no informative label
// can be inferred (literals, arithmetic, unknown function call).
func rhsLabel(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.CompositeLit:
		return typeLabel(x.Type)
	case *ast.CallExpr:
		if id, ok := x.Fun.(*ast.Ident); ok {
			return strings.TrimPrefix(id.Name, "New")
		}
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			return strings.TrimPrefix(sel.Sel.Name, "New")
		}
	case *ast.UnaryExpr:
		return rhsLabel(x.X)
	}
	return ""
}

// typeLabel returns the rightmost identifier of a type expression, which
// is typically the human-meaningful part (`*pkg.Foo` → "Foo"). Unnamed
// types (maps, slices, funcs) return "" and are thus skipped.
func typeLabel(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return typeLabel(x.X)
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

// isStandardVar lists identifiers whose names are idiomatic in Go and
// shouldn't be penalised for being short. Skipping them keeps the metric
// focused on domain-specific names chosen by the generator.
func isStandardVar(name string) bool {
	switch name {
	case "err", "ok", "got", "want", "t", "b", "tb", "ctx", "cancel", "done", "r", "w":
		return true
	}
	return false
}

// tokenizeTestName splits a Go test function name into camelCase /
// underscore tokens, stripping the conventional `Test` prefix. Examples:
//
//	"TestFoo"          -> ["Foo"]
//	"TestFoo_Bar"      -> ["Foo", "Bar"]
//	"TestFooBar_Baz"   -> ["Foo", "Bar", "Baz"]
func tokenizeTestName(name string) []string {
	name = strings.TrimPrefix(name, "Test")
	// Split by underscore first, then split each segment by camelCase.
	var tokens []string
	for _, seg := range strings.Split(name, "_") {
		if seg == "" {
			continue
		}
		tokens = append(tokens, camelSplit(seg)...)
	}
	return tokens
}

// camelSplit splits an identifier at runs of lowercase→uppercase.
func camelSplit(s string) []string {
	var tokens []string
	start := 0
	for i := 1; i < len(s); i++ {
		if unicode.IsUpper(rune(s[i])) && !unicode.IsUpper(rune(s[i-1])) {
			tokens = append(tokens, s[start:i])
			start = i
		}
	}
	tokens = append(tokens, s[start:])
	return tokens
}

// similarity is a 0..1 normalised Levenshtein similarity on lowercased
// strings. It returns 1.0 for empty inputs (degenerate but defined).
func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0
	}
	d := levenshtein(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	return 1.0 - float64(d)/float64(maxLen)
}

// levenshtein computes the classic edit distance between a and b. O(n·m)
// time and O(min(n,m)) memory.
func levenshtein(a, b string) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
