package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
	"github.com/gizatulin/testgen-agent/internal/diff"
	"github.com/gizatulin/testgen-agent/internal/llm"
	"github.com/gizatulin/testgen-agent/internal/prompt"
	"github.com/gizatulin/testgen-agent/internal/validator"
)

const maxRetries = 3

func main() {
	// CLI-флаги
	repoPath := flag.String("repo", ".", "Путь к Git-репозиторию")
	baseBranch := flag.String("base", "main", "Базовая ветка для сравнения")
	apiKey := flag.String("api-key", "", "API-ключ LLM (или переменная TESTGEN_API_KEY)")
	baseURL := flag.String("api-url", "", "URL LLM API (по умолчанию OpenAI)")
	model := flag.String("model", "", "Модель LLM (по умолчанию gpt-4o-mini)")
	outDir := flag.String("out", "", "Директория для сохранения тестов (по умолчанию рядом с файлом)")
	dryRun := flag.Bool("dry-run", false, "Только показать промпт, не вызывать LLM")
	noValidate := flag.Bool("no-validate", false, "Не запускать валидацию тестов")

	flag.Parse()

	// Поддержка позиционных аргументов для обратной совместимости
	if flag.NArg() > 0 && *repoPath == "." {
		*repoPath = flag.Arg(0)
	}
	if flag.NArg() > 1 && *baseBranch == "main" {
		*baseBranch = flag.Arg(1)
	}

	fmt.Printf("📂 Репозиторий: %s\n", *repoPath)
	fmt.Printf("🔀 Base branch: %s\n\n", *baseBranch)

	// ─── Шаг 1: Получаем diff ───
	diffOutput, err := gitDiff(*repoPath, *baseBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Ошибка git diff: %v\n", err)
		os.Exit(1)
	}

	if len(strings.TrimSpace(diffOutput)) == 0 {
		fmt.Println("✅ Нет изменений (diff пустой)")
		return
	}

	// ─── Шаг 2: Парсим diff ───
	files, err := diff.Parse(diffOutput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Ошибка парсинга diff: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📝 Изменено файлов: %d\n\n", len(files))

	totalAttempted := 0
	totalGenerated := 0
	totalValidated := 0

	// ─── Шаг 3: Для каждого .go файла — AST-анализ + генерация тестов ───
	for _, f := range files {
		changedLines := f.ChangedLines()
		fmt.Printf("  📄 %s\n", f.NewPath)
		fmt.Printf("     Хунков: %d, Изменённых строк: %d\n", len(f.Hunks), len(changedLines))

		if !strings.HasSuffix(f.NewPath, ".go") || strings.HasSuffix(f.NewPath, "_test.go") {
			fmt.Printf("     ⏭️  Пропущен (не .go или _test.go)\n\n")
			continue
		}

		fullPath := filepath.Join(*repoPath, f.NewPath)

		// AST-анализ
		analysis, err := analyzer.AnalyzeFile(fullPath)
		if err != nil {
			fmt.Printf("     ⚠️  AST-анализ не удался: %v\n\n", err)
			continue
		}

		affectedFuncs := analyzer.FindFunctionsByLines(analysis, changedLines)
		if len(affectedFuncs) == 0 {
			fmt.Printf("     ℹ️  Изменения вне функций\n\n")
			continue
		}

		fmt.Printf("     🔍 Затронутые функции (%d):\n", len(affectedFuncs))
		for _, fn := range affectedFuncs {
			fmt.Printf("        • %s  (строки %d–%d)\n", fn.Signature, fn.StartLine, fn.EndLine)
		}
		totalAttempted++

		// Проверяем, есть ли уже тесты
		existingTests := readExistingTests(fullPath)

		// Формируем промпт
		req := prompt.TestGenRequest{
			PackageName:   analysis.Package,
			FilePath:      f.NewPath,
			Imports:       analysis.Imports,
			TargetFuncs:   affectedFuncs,
			ExistingTests: existingTests,
		}

		messages := prompt.BuildMessages(req)

		if *dryRun {
			fmt.Printf("\n     📋 DRY RUN — Промпт:\n")
			fmt.Printf("     ── System (%d символов) ──\n", len(messages[0].Content))
			fmt.Printf("     ── User (%d символов) ──\n", len(messages[1].Content))
			fmt.Println(messages[1].Content)
			fmt.Println()
			continue
		}

		// ─── Шаг 4: Вызов LLM ───
		cfg := buildLLMConfig(*apiKey, *baseURL, *model)
		if cfg.APIKey == "" && cfg.BaseURL == "https://api.openai.com/v1" {
			fmt.Printf("     ⚠️  API-ключ не задан. Используйте --api-key или TESTGEN_API_KEY\n")
			fmt.Printf("     💡 Или укажите --api-url для локальной модели (Ollama)\n\n")
			continue
		}

		client := llm.NewClient(cfg)
		testFilePath := buildTestFilePath(fullPath, *outDir)

		// ─── Цикл генерации с валидацией ───
		var generatedCode string
		success := false

		for attempt := 1; attempt <= maxRetries; attempt++ {
			if attempt == 1 {
				fmt.Printf("     🤖 Генерация тестов через %s...\n", cfg.Model)
			} else {
				fmt.Printf("     🔄 Попытка %d/%d — исправление ошибок...\n", attempt, maxRetries)
			}

			var result *llm.GenerateResponse

			if attempt == 1 {
				result, err = client.Generate(messages)
			} else {
				// Отправляем предыдущий код + ошибки для фикса
				fixMessages := prompt.BuildFixMessages(req, generatedCode, lastValidationError, attempt)
				result, err = client.Generate(fixMessages)
			}

			if err != nil {
				fmt.Printf("     ❌ Ошибка LLM: %v\n", err)
				break
			}

			generatedCode = result.Content
			fmt.Printf("     ✅ Сгенерировано (токенов: %d prompt + %d completion)\n",
				result.PromptTokens, result.CompletionTokens)

			// Сохраняем файл
			if err := os.MkdirAll(filepath.Dir(testFilePath), 0755); err != nil {
				fmt.Printf("     ❌ Ошибка создания директории: %v\n", err)
				break
			}

			if err := os.WriteFile(testFilePath, []byte(generatedCode), 0644); err != nil {
				fmt.Printf("     ❌ Ошибка записи файла: %v\n", err)
				break
			}

			// ─── Шаг 5: Валидация ───
			if *noValidate {
				fmt.Printf("     💾 Тесты сохранены: %s (валидация отключена)\n\n", testFilePath)
				totalGenerated++
				success = true
				break
			}

			fmt.Printf("     🔬 Валидация...\n")
			valResult := validator.Validate(*repoPath, testFilePath)

			if valResult.IsValid() {
				fmt.Printf("     %s\n", valResult.Summary())
				fmt.Printf("     💾 Тесты сохранены: %s\n\n", testFilePath)
				totalGenerated++
				totalValidated++
				success = true
				break
			}

			// Валидация не прошла
			lastValidationError = valResult.Summary()
			fmt.Printf("     %s\n", valResult.Summary())

			if attempt == maxRetries {
				fmt.Printf("     ⛔ Превышено максимальное количество попыток (%d)\n", maxRetries)
			}
		}

		// Если валидация не прошла — удаляем невалидный файл с диска,
		// чтобы он не попал в коммит.
		if !success {
			if generatedCode != "" {
				os.Remove(testFilePath)
				fmt.Printf("     🗑️  Невалидный файл удалён: %s\n\n", testFilePath)
			} else {
				fmt.Printf("     ❌ Не удалось сгенерировать тесты\n\n")
			}
		}
	}

	// Итоги
	fmt.Println("═══════════════════════════════════")
	fmt.Printf("📊 Итого: сгенерировано %d, валидировано %d\n", totalGenerated, totalValidated)

	// Выходим с кодом ошибки если были попытки генерации,
	// но ни один файл не прошёл валидацию.
	if totalAttempted > 0 && totalValidated == 0 {
		os.Exit(2)
	}
	// Частичный успех — часть файлов валидна, часть нет.
	if totalGenerated > totalValidated {
		os.Exit(1)
	}
}

// lastValidationError хранит последнюю ошибку валидации для retry.
var lastValidationError string

// gitDiff получает diff из git-репозитория.
func gitDiff(repoPath, baseBranch string) (string, error) {
	cmd := exec.Command("git", "diff", baseBranch)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// Если нет base branch, пробуем unstaged diff
		cmd = exec.Command("git", "diff")
		cmd.Dir = repoPath
		output, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return string(output), nil
}

// readExistingTests пытается прочитать существующий файл тестов.
func readExistingTests(goFilePath string) string {
	ext := filepath.Ext(goFilePath)
	testPath := strings.TrimSuffix(goFilePath, ext) + "_test" + ext

	data, err := os.ReadFile(testPath)
	if err != nil {
		return ""
	}
	return string(data)
}

// buildTestFilePath определяет путь для файла тестов.
func buildTestFilePath(goFilePath, outDir string) string {
	ext := filepath.Ext(goFilePath)
	base := strings.TrimSuffix(filepath.Base(goFilePath), ext)
	testFileName := base + "_test" + ext

	if outDir != "" {
		return filepath.Join(outDir, testFileName)
	}

	return filepath.Join(filepath.Dir(goFilePath), testFileName)
}

// buildLLMConfig формирует конфигурацию LLM-клиента из CLI-флагов и env.
func buildLLMConfig(apiKey, baseURL, model string) llm.Config {
	cfg := llm.DefaultConfig()

	if apiKey != "" {
		cfg.APIKey = apiKey
	} else if envKey := os.Getenv("TESTGEN_API_KEY"); envKey != "" {
		cfg.APIKey = envKey
	}

	if baseURL != "" {
		cfg.BaseURL = baseURL
	} else if envURL := os.Getenv("TESTGEN_API_URL"); envURL != "" {
		cfg.BaseURL = envURL
	}

	if model != "" {
		cfg.Model = model
	} else if envModel := os.Getenv("TESTGEN_MODEL"); envModel != "" {
		cfg.Model = envModel
	}

	return cfg
}
