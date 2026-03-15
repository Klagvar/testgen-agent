// Package mockgen генерирует мок-реализации Go-интерфейсов.
// Моки создаются детерминистически через AST-анализ (без LLM),
// что гарантирует корректную компиляцию.
package mockgen

import (
	"fmt"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

// MockDef — определение одного мока.
type MockDef struct {
	InterfaceName string // имя исходного интерфейса
	MockName      string // имя мок-структуры (mockXxx)
	Code          string // полный Go-код мока
}

// GenerateMocks создаёт мок-реализации для переданных интерфейсов.
// Генерирует структуру с функциональными полями и методы-делегаты.
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

// GenerateMockCode возвращает полный Go-код всех моков, готовый для вставки в _test.go.
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

	// Структура с функциональными полями
	sb.WriteString(fmt.Sprintf("// %s is a mock implementation of %s for testing.\n", mockName, ti.Name))
	sb.WriteString(fmt.Sprintf("type %s struct {\n", mockName))

	for _, m := range ti.Methods {
		funcFieldName := m.Name + "Func"
		sb.WriteString(fmt.Sprintf("\t%s func%s\n", funcFieldName, m.Signature))
	}

	sb.WriteString("}\n\n")

	// Методы-делегаты
	for _, m := range ti.Methods {
		funcFieldName := m.Name + "Func"

		params, returns := parseMethodSignature(m.Signature)

		// Формируем список аргументов для вызова
		var callArgs []string
		for _, p := range params {
			callArgs = append(callArgs, p.name)
		}

		// Формируем сигнатуру метода
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

		// Тело: вызов функционального поля
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

// parseMethodSignature парсит "(id string, val int) (error)" -> params, returns.
func parseMethodSignature(sig string) ([]paramInfo, []string) {
	sig = strings.TrimSpace(sig)

	// Разбираем параметры — до первой закрывающей скобки
	if !strings.HasPrefix(sig, "(") {
		return nil, nil
	}

	// Находим закрывающую скобку для параметров (с учётом вложенности)
	paramEnd := findClosingParen(sig, 0)
	if paramEnd < 0 {
		return nil, nil
	}

	paramsStr := sig[1:paramEnd]
	returnsStr := strings.TrimSpace(sig[paramEnd+1:])

	// Парсим параметры
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
				// Только тип — генерируем имя
				paramCounter++
				params = append(params, paramInfo{
					name: fmt.Sprintf("arg%d", paramCounter),
					typ:  fields[0],
				})
			}
		}
	}

	// Парсим возвращаемые типы
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

// findClosingParen находит закрывающую скобку, учитывая вложенность.
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

// splitOutsideParens разделяет строку по запятым, не внутри скобок.
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
