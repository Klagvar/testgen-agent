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
	PackageName    string               // имя пакета
	FilePath       string               // путь к файлу
	Imports        []string             // импорты файла
	TargetFuncs    []analyzer.FuncInfo  // затронутые функции
	ExistingTests  string               // существующие тесты (если есть)
}

// BuildSystemPrompt формирует системный промпт — инструкции для LLM.
func BuildSystemPrompt() string {
	return `Ты — опытный Go-разработчик, специализирующийся на написании юнит-тестов.

Твоя задача — сгенерировать качественные юнит-тесты для предоставленных Go-функций.

## Требования к тестам:

1. **Формат**: используй стандартный пакет "testing". Предпочитай table-driven tests.
2. **Покрытие**: покрой все ветви выполнения:
   - Нормальные случаи (happy path)
   - Граничные значения (0, пустые строки, nil, максимальные значения)
   - Ошибочные случаи (невалидный ввод, деление на ноль, и т.д.)
3. **Именование**: имена тестов должны быть описательными, формат: Test<FuncName>_<Scenario>
4. **Изоляция**: каждый тест должен быть независимым.
5. **Assertions**: используй t.Errorf / t.Fatalf для проверок. НЕ используй внешние assertion-библиотеки.
6. **Выходной формат**: верни ТОЛЬКО Go-код — один файл _test.go, готовый к компиляции.

## Частые ошибки (ИЗБЕГАЙ ИХ):

- НЕ используй константы вроде math.MaxInt64+1 или -math.MinInt64 — они вызывают overflow при компиляции.
  Для тестов переполнения используй переменные: a := math.MaxInt64 и передавай их в функцию.
- НЕ импортируй пакеты, которые не используешь в тестах — Go не скомпилирует.
- Если переменная объявлена но не используется, Go не скомпилирует. Используй _ для неиспользуемых значений.
  Пример: _, err := SomeFunc() если тебе нужен только err.
- Проверяй, что сигнатура вызова совпадает с определением функции (количество и типы аргументов/возвратов).
- Оператор % в Go сохраняет знак делимого: -7 % 3 == -1 (НЕ 2 как в Python). Учитывай это в expected-значениях.
- В строковых литералах Go НЕ используй невалидные escape-последовательности. Допустимые: \n \t \r \\ \" \' \a \b \f \v \0 \x \u \U.
  Если в строке нужен обратный слэш — экранируй его: "\\" или используй raw string: ` + "`" + `C:\path` + "`" + `.
- НЕ используй filepath.Join с хардкод путями — пути должны быть платформо-независимыми.
- При тестировании функций с os/exec (exec.Command), помни что команды платформо-зависимы.

## Структура ответа:

Верни ТОЛЬКО код Go-файла с тестами — без пояснений, без markdown-обёрток.
Код должен начинаться с "package ..." и быть валидным Go-кодом.`
}

// BuildUserPrompt формирует пользовательский промпт с контекстом функций.
func BuildUserPrompt(req TestGenRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Сгенерируй юнит-тесты для пакета `%s` (файл `%s`).\n\n", req.PackageName, req.FilePath))

	// Импорты файла
	if len(req.Imports) > 0 {
		sb.WriteString("## Импорты исходного файла\n\n```go\nimport (\n")
		for _, imp := range req.Imports {
			sb.WriteString(fmt.Sprintf("\t\"%s\"\n", imp))
		}
		sb.WriteString(")\n```\n\n")
	}

	// Функции для тестирования
	sb.WriteString(fmt.Sprintf("## Функции для тестирования (%d шт.)\n\n", len(req.TargetFuncs)))

	for i, fn := range req.TargetFuncs {
		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, fn.Name))

		// Сигнатура
		sb.WriteString(fmt.Sprintf("**Сигнатура:** `%s`\n\n", fn.Signature))

		// Документация
		if fn.DocComment != "" {
			sb.WriteString(fmt.Sprintf("**Документация:** %s\n", strings.TrimSpace(fn.DocComment)))
		}

		// Параметры
		if len(fn.Params) > 0 {
			sb.WriteString("**Параметры:**\n")
			for _, p := range fn.Params {
				sb.WriteString(fmt.Sprintf("- `%s` — тип `%s`\n", p.Name, p.Type))
			}
			sb.WriteString("\n")
		}

		// Возвращаемые типы
		if len(fn.Returns) > 0 {
			sb.WriteString(fmt.Sprintf("**Возвращает:** `%s`\n\n", strings.Join(fn.Returns, ", ")))
		}

		// Тело функции
		sb.WriteString("**Реализация:**\n\n```go\n")
		sb.WriteString(fn.Body)
		sb.WriteString("\n```\n\n")

		// Анализ ветвлений
		branches := analyzeBranches(fn.Body)
		if len(branches) > 0 {
			sb.WriteString("**Ветвления в коде:**\n")
			for _, b := range branches {
				sb.WriteString(fmt.Sprintf("- %s\n", b))
			}
			sb.WriteString("\n")
		}

		sb.WriteString("---\n\n")
	}

	// Существующие тесты
	if req.ExistingTests != "" {
		sb.WriteString("## Существующие тесты (не дублируй их)\n\n```go\n")
		sb.WriteString(req.ExistingTests)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("Сгенерируй полный файл _test.go с тестами для всех перечисленных функций.\n")

	return sb.String()
}

// analyzeBranches простой анализ ветвлений в теле функции для подсказки LLM.
func analyzeBranches(body string) []string {
	var branches []string

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "if ") {
			// Извлекаем условие
			cond := strings.TrimPrefix(trimmed, "if ")
			cond = strings.TrimSuffix(cond, " {")
			branches = append(branches, fmt.Sprintf("Условие: `%s`", cond))
		} else if strings.HasPrefix(trimmed, "} else if ") {
			cond := strings.TrimPrefix(trimmed, "} else if ")
			cond = strings.TrimSuffix(cond, " {")
			branches = append(branches, fmt.Sprintf("Иначе-если: `%s`", cond))
		} else if trimmed == "} else {" {
			branches = append(branches, "Ветка else")
		} else if strings.HasPrefix(trimmed, "switch ") {
			branches = append(branches, "Switch-выражение")
		} else if strings.HasPrefix(trimmed, "case ") {
			caseVal := strings.TrimPrefix(trimmed, "case ")
			caseVal = strings.TrimSuffix(caseVal, ":")
			branches = append(branches, fmt.Sprintf("Case: `%s`", caseVal))
		} else if strings.Contains(trimmed, "err != nil") {
			branches = append(branches, "Проверка ошибки (err != nil)")
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

	sb.WriteString(fmt.Sprintf("## Ошибка в сгенерированных тестах (попытка %d)\n\n", attempt))
	sb.WriteString("Предыдущий код тестов не прошёл валидацию. Вот ошибки:\n\n")
	sb.WriteString("```\n")
	sb.WriteString(errors)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Инструкции по исправлению\n\n")
	sb.WriteString("1. Внимательно прочитай ошибки выше.\n")
	sb.WriteString("2. Исправь **только** проблемные места в коде тестов.\n")
	sb.WriteString("3. Убедись, что:\n")
	sb.WriteString("   - Все типы корректны (нет overflow, нет несовпадений типов)\n")
	sb.WriteString("   - Все импорты используются и присутствуют\n")
	sb.WriteString("   - Все вызываемые функции существуют с правильными сигнатурами\n")
	sb.WriteString("   - Тесты корректно проверяют ожидаемое поведение\n")
	sb.WriteString("4. Верни ПОЛНЫЙ исправленный файл _test.go (не фрагмент, а весь файл).\n")
	sb.WriteString("5. Верни ТОЛЬКО код — без пояснений, без markdown-обёрток.\n")

	return sb.String()
}

// Message — одно сообщение для LLM API.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
