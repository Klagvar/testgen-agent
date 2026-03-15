// Package prompt формирует структурированные промпты для LLM
// на основе результатов diff-парсинга и AST-анализа.
package prompt

import (
	"fmt"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

// TestGenRequest — входные данные для генерации промпта.
type TestGenRequest struct {
	PackageName   string              // имя пакета
	FilePath      string              // путь к файлу
	Imports       []string            // импорты файла
	TargetFuncs   []analyzer.FuncInfo // затронутые функции
	ExistingTests string              // существующие тесты (если есть)
}

// BuildSystemPrompt формирует системный промпт — инструкции для LLM.
func BuildSystemPrompt() string {
	return `You are an expert Go developer specializing in writing unit tests.

Your task is to generate high-quality unit tests for the provided Go functions.

## Test Requirements

1. **Format**: Use the standard "testing" package. Prefer table-driven tests.
2. **Coverage**: Cover all execution branches:
   - Happy path (normal cases)
   - Boundary values (0, empty strings, nil, max values)
   - Error cases (invalid input, division by zero, etc.)
3. **Naming**: Test names must be descriptive, format: Test<FuncName>_<Scenario>
4. **Isolation**: Each test must be independent.
5. **Assertions**: Use t.Errorf / t.Fatalf for checks. Do NOT use external assertion libraries (e.g. testify).
6. **Output format**: Return ONLY Go code — a single _test.go file, ready to compile.

## Common Mistakes (AVOID THESE)

- Do NOT use compile-time overflow expressions like math.MaxInt64+1 or -math.MinInt64.
  For overflow tests, use variables: a := math.MaxInt64, then pass them to the function.
- Do NOT import unused packages — Go will not compile.
- Do NOT declare unused variables — Go will not compile. Use _ for unused values.
  Example: _, err := SomeFunc() if you only need the error.
- Verify that function call signatures match definitions (number and types of args/returns).
- The % operator in Go preserves the sign of the dividend: -7 % 3 == -1 (NOT 2 like in Python).
- Do NOT use invalid escape sequences in Go string literals. Valid ones: \n \t \r \\ \" \' \a \b \f \v \0 \x \u \U.
  For backslashes use: "\\" or raw strings.
- Do NOT use filepath.Join with hardcoded OS-specific paths.
- When testing functions using os/exec, remember that commands are platform-dependent.
- Do NOT redeclare functions that already exist in the existing test file.

## Existing Tests Policy

If existing tests are provided, you MUST include them UNCHANGED in your output.
Add new tests AFTER the existing ones. Do NOT modify, rename, or remove existing test functions.

## Response Format

Return ONLY the Go test file code — no explanations, no markdown wrappers.
Code must start with "package ..." and be valid, compilable Go code.`
}

// BuildUserPrompt формирует пользовательский промпт с контекстом функций.
func BuildUserPrompt(req TestGenRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Generate unit tests for package `%s` (file `%s`).\n\n", req.PackageName, req.FilePath))

	// Импорты файла
	if len(req.Imports) > 0 {
		sb.WriteString("## Source File Imports\n\n```go\nimport (\n")
		for _, imp := range req.Imports {
			sb.WriteString(fmt.Sprintf("\t\"%s\"\n", imp))
		}
		sb.WriteString(")\n```\n\n")
	}

	// Функции для тестирования
	sb.WriteString(fmt.Sprintf("## Functions to Test (%d)\n\n", len(req.TargetFuncs)))

	for i, fn := range req.TargetFuncs {
		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, fn.Name))

		// Сигнатура
		sb.WriteString(fmt.Sprintf("**Signature:** `%s`\n\n", fn.Signature))

		// Документация
		if fn.DocComment != "" {
			sb.WriteString(fmt.Sprintf("**Documentation:** %s\n", strings.TrimSpace(fn.DocComment)))
		}

		// Параметры
		if len(fn.Params) > 0 {
			sb.WriteString("**Parameters:**\n")
			for _, p := range fn.Params {
				sb.WriteString(fmt.Sprintf("- `%s` — type `%s`\n", p.Name, p.Type))
			}
			sb.WriteString("\n")
		}

		// Возвращаемые типы
		if len(fn.Returns) > 0 {
			sb.WriteString(fmt.Sprintf("**Returns:** `%s`\n\n", strings.Join(fn.Returns, ", ")))
		}

		// Тело функции
		sb.WriteString("**Implementation:**\n\n```go\n")
		sb.WriteString(fn.Body)
		sb.WriteString("\n```\n\n")

		// Анализ ветвлений
		branches := analyzeBranches(fn.Body)
		if len(branches) > 0 {
			sb.WriteString("**Code branches:**\n")
			for _, b := range branches {
				sb.WriteString(fmt.Sprintf("- %s\n", b))
			}
			sb.WriteString("\n")
		}

		sb.WriteString("---\n\n")
	}

	// Существующие тесты
	if req.ExistingTests != "" {
		sb.WriteString("## Existing Tests (MUST PRESERVE)\n\n")
		sb.WriteString("The test file already contains tests. You MUST include ALL of them in your output UNCHANGED.\n")
		sb.WriteString("Add new tests AFTER the existing ones. Do NOT modify or remove any existing test functions.\n\n")
		sb.WriteString("```go\n")
		sb.WriteString(req.ExistingTests)
		sb.WriteString("\n```\n\n")
		sb.WriteString("Generate the complete _test.go file: first ALL existing tests (unchanged), then NEW tests for the listed functions.\n")
	} else {
		sb.WriteString("Generate a complete _test.go file with tests for all listed functions.\n")
	}

	return sb.String()
}

// analyzeBranches — простой анализ ветвлений в теле функции для подсказки LLM.
func analyzeBranches(body string) []string {
	var branches []string

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "if ") {
			cond := strings.TrimPrefix(trimmed, "if ")
			cond = strings.TrimSuffix(cond, " {")
			branches = append(branches, fmt.Sprintf("Condition: `%s`", cond))
		} else if strings.HasPrefix(trimmed, "} else if ") {
			cond := strings.TrimPrefix(trimmed, "} else if ")
			cond = strings.TrimSuffix(cond, " {")
			branches = append(branches, fmt.Sprintf("Else-if: `%s`", cond))
		} else if trimmed == "} else {" {
			branches = append(branches, "Else branch")
		} else if strings.HasPrefix(trimmed, "switch ") {
			branches = append(branches, "Switch statement")
		} else if strings.HasPrefix(trimmed, "case ") {
			caseVal := strings.TrimPrefix(trimmed, "case ")
			caseVal = strings.TrimSuffix(caseVal, ":")
			branches = append(branches, fmt.Sprintf("Case: `%s`", caseVal))
		} else if strings.Contains(trimmed, "err != nil") {
			branches = append(branches, "Error check (err != nil)")
		}
	}

	return branches
}

// BuildMessages формирует массив сообщений для LLM API.
func BuildMessages(req TestGenRequest) []Message {
	return []Message{
		{Role: "system", Content: BuildSystemPrompt()},
		{Role: "user", Content: BuildUserPrompt(req)},
	}
}

// BuildFixMessages формирует сообщения для повторной попытки —
// отправляем LLM предыдущий код + ошибки и просим исправить.
func BuildFixMessages(req TestGenRequest, previousCode string, errors string, attempt int) []Message {
	fixPrompt := buildFixPrompt(previousCode, errors, attempt)

	return []Message{
		{Role: "system", Content: BuildSystemPrompt()},
		{Role: "user", Content: BuildUserPrompt(req)},
		{Role: "assistant", Content: previousCode},
		{Role: "user", Content: fixPrompt},
	}
}

// buildFixPrompt формирует промпт с описанием ошибки для исправления.
func buildFixPrompt(previousCode string, errors string, attempt int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Error in Generated Tests (attempt %d)\n\n", attempt))
	sb.WriteString("The previous test code failed validation. Here are the errors:\n\n")
	sb.WriteString("```\n")
	sb.WriteString(errors)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Fix Instructions\n\n")
	sb.WriteString("1. Carefully read the errors above.\n")
	sb.WriteString("2. Fix ONLY the problematic parts of the test code.\n")
	sb.WriteString("3. Make sure that:\n")
	sb.WriteString("   - All types are correct (no overflow, no type mismatches)\n")
	sb.WriteString("   - All imports are used and present\n")
	sb.WriteString("   - All called functions exist with correct signatures\n")
	sb.WriteString("   - Tests correctly verify expected behavior\n")
	sb.WriteString("   - Existing tests are preserved unchanged\n")
	sb.WriteString("4. Return the COMPLETE fixed _test.go file (not a fragment, the entire file).\n")
	sb.WriteString("5. Return ONLY code — no explanations, no markdown wrappers.\n")

	return sb.String()
}

// Message — одно сообщение для LLM API.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
