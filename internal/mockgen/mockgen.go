// Package mockgen generates mock implementations of Go interfaces.
// Mocks are created deterministically via AST analysis (without LLM),
// guaranteeing correct compilation.
package mockgen

import (
	"fmt"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

// MockDef holds the definition of a single mock.
type MockDef struct {
	InterfaceName string // original interface name
	MockName      string // mock struct name (mockXxx)
	Code          string // full Go code of the mock
}

// GenerateMocks creates mock implementations for the given interfaces.
// Generates a struct with function fields and delegate methods.
func GenerateMocks(types []analyzer.TypeInfo) []MockDef {
	var mocks []MockDef

	for _, ti := range types {
		if ti.Kind != "interface" || len(ti.Methods) == 0 {
			continue
		}

		mock := generateMock(ti)
		mocks = append(mocks, mock)
	}

	return mocks
}

// GenerateMockCode returns the full Go code of all mocks, ready to insert into _test.go.
func GenerateMockCode(types []analyzer.TypeInfo) string {
	mocks := GenerateMocks(types)
	if len(mocks) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, mock := range mocks {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(mock.Code)
		sb.WriteString("\n")
	}

	return sb.String()
}

func generateMock(ti analyzer.TypeInfo) MockDef {
	mockName := "mock" + ti.Name

	var sb strings.Builder

	// Struct with function fields
	sb.WriteString(fmt.Sprintf("// %s is a mock implementation of %s for testing.\n", mockName, ti.Name))
	sb.WriteString(fmt.Sprintf("type %s struct {\n", mockName))

	for _, m := range ti.Methods {
		funcFieldName := m.Name + "Func"
		sb.WriteString(fmt.Sprintf("\t%s func%s\n", funcFieldName, m.Signature))
	}

	sb.WriteString("}\n\n")

	// Delegate methods
	for _, m := range ti.Methods {
		funcFieldName := m.Name + "Func"

		params, returns := parseMethodSignature(m.Signature)

		// Build argument list for the call
		var callArgs []string
		for _, p := range params {
			callArgs = append(callArgs, p.name)
		}

		// Build method signature
		var paramStrs []string
		for _, p := range params {
			paramStrs = append(paramStrs, p.name+" "+p.typ)
		}

		sb.WriteString(fmt.Sprintf("func (m *%s) %s(%s)",
			mockName, m.Name, strings.Join(paramStrs, ", ")))

		if len(returns) > 0 {
			if len(returns) == 1 {
				sb.WriteString(fmt.Sprintf(" %s", returns[0]))
			} else {
				sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(returns, ", ")))
			}
		}

		sb.WriteString(" {\n")

		// Body: call the function field
		callStr := fmt.Sprintf("m.%s(%s)", funcFieldName, strings.Join(callArgs, ", "))

		if len(returns) > 0 {
			sb.WriteString(fmt.Sprintf("\treturn %s\n", callStr))
		} else {
			sb.WriteString(fmt.Sprintf("\t%s\n", callStr))
		}

		sb.WriteString("}\n\n")
	}

	return MockDef{
		InterfaceName: ti.Name,
		MockName:      mockName,
		Code:          sb.String(),
	}
}

type paramInfo struct {
	name string
	typ  string
}

// parseMethodSignature parses "(id string, val int) (error)" -> params, returns.
func parseMethodSignature(sig string) ([]paramInfo, []string) {
	sig = strings.TrimSpace(sig)

	// Parse parameters — up to the first closing parenthesis
	if !strings.HasPrefix(sig, "(") {
		return nil, nil
	}

	// Find the closing parenthesis for parameters (accounting for nesting)
	paramEnd := findClosingParen(sig, 0)
	if paramEnd < 0 {
		return nil, nil
	}

	paramsStr := sig[1:paramEnd]
	returnsStr := strings.TrimSpace(sig[paramEnd+1:])

	// Parse parameters
	var params []paramInfo
	if paramsStr != "" {
		parts := splitOutsideParens(paramsStr)
		paramCounter := 0
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			fields := strings.Fields(part)
			if len(fields) >= 2 {
				// "id string" or "id *Type"
				name := fields[0]
				typ := strings.Join(fields[1:], " ")
				params = append(params, paramInfo{name: name, typ: typ})
			} else {
				// Type only — generate a name
				paramCounter++
				params = append(params, paramInfo{
					name: fmt.Sprintf("arg%d", paramCounter),
					typ:  fields[0],
				})
			}
		}
	}

	// Parse return types
	var returns []string
	if returnsStr != "" {
		if strings.HasPrefix(returnsStr, "(") {
			inner := returnsStr[1 : len(returnsStr)-1]
			for _, r := range splitOutsideParens(inner) {
				r = strings.TrimSpace(r)
				if r != "" {
					returns = append(returns, r)
				}
			}
		} else {
			returns = append(returns, returnsStr)
		}
	}

	return params, returns
}

// findClosingParen finds the closing parenthesis, accounting for nesting.
func findClosingParen(s string, openPos int) int {
	depth := 0
	for i := openPos; i < len(s); i++ {
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitOutsideParens splits a string by commas, not inside parentheses.
func splitOutsideParens(s string) []string {
	var parts []string
	depth := 0
	start := 0

	for i, c := range s {
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
		} else if c == ',' && depth == 0 {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}

	parts = append(parts, s[start:])
	return parts
}
