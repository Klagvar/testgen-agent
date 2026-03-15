// Package validator проверяет сгенерированные тесты:
// компиляция, запуск, анализ ошибок.
package validator

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Result — результат валидации тестов.
type Result struct {
	// Компиляция
	CompileOK    bool   // скомпилировалось ли
	CompileError string // ошибка компиляции (если есть)

	// Запуск тестов
	TestsOK    bool   // все тесты прошли
	TestOutput string // вывод go test
	TestError  string // ошибки при запуске тестов

	// Статистика
	Passed   int           // количество прошедших тестов
	Failed   int           // количество упавших тестов
	Duration time.Duration // время выполнения
}

// IsValid возвращает true если тесты компилируются и проходят.
func (r *Result) IsValid() bool {
	return r.CompileOK && r.TestsOK
}

// Summary возвращает краткое описание результата.
func (r *Result) Summary() string {
	if !r.CompileOK {
		return fmt.Sprintf("❌ Ошибка компиляции:\n%s", r.CompileError)
	}
	if !r.TestsOK {
		return fmt.Sprintf("⚠️  Тесты упали (%d passed, %d failed):\n%s", r.Passed, r.Failed, r.TestError)
	}
	return fmt.Sprintf("✅ Все тесты прошли (%d passed, %s)", r.Passed, r.Duration)
}

// FormatFile запускает goimports на файле для автоматического исправления импортов.
// Если goimports не установлен, использует go fmt.
func FormatFile(filePath string) error {
	// Пробуем goimports (фиксит неиспользуемые и недостающие импорты)
	cmd := exec.Command("goimports", "-w", filePath)
	if err := cmd.Run(); err != nil {
		// Fallback на go fmt (хотя бы отформатирует)
		cmd = exec.Command("go", "fmt", filePath)
		return cmd.Run()
	}
	return nil
}

// Validate проверяет сгенерированный тест-файл.
// repoDir — путь к репозиторию, testFile — путь к файлу тестов.
func Validate(repoDir string, testFile string) *Result {
	result := &Result{}
	start := time.Now()

	// Шаг 0: Автоформатирование (goimports фиксит импорты автоматически)
	_ = FormatFile(testFile)

	// Определяем пакет (директорию) тест-файла относительно репозитория
	testDir := filepath.Dir(testFile)

	// Шаг 1: Проверяем компиляцию
	compileErr := runGoCommand(repoDir, testDir, "build")
	if compileErr != "" {
		result.CompileOK = false
		result.CompileError = compileErr
		result.Duration = time.Since(start)
		return result
	}
	result.CompileOK = true

	// Шаг 2: Запускаем тесты
	testOutput, testErr := runGoTest(repoDir, testDir)
	result.TestOutput = testOutput
	result.Duration = time.Since(start)

	if testErr == "" {
		result.TestsOK = true
		result.Passed = countTests(testOutput, "PASS")
		return result
	}

	// Разбираем результаты упавших тестов
	result.TestsOK = false
	result.TestError = testErr
	result.Passed = countTests(testOutput, "PASS")
	result.Failed = countTests(testOutput, "FAIL")

	return result
}

// runGoCommand запускает go <command> в указанной директории.
func runGoCommand(repoDir, pkgDir, command string) string {
	// Определяем относительный путь пакета
	relPkg, err := filepath.Rel(repoDir, pkgDir)
	if err != nil {
		relPkg = "."
	}
	// Преобразуем к формату Go-пакета: ./path/to/pkg
	pkgPath := "./" + filepath.ToSlash(relPkg)

	cmd := exec.Command("go", command, pkgPath)
	cmd.Dir = repoDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output))
	}
	return ""
}

// runGoTest запускает go test и возвращает вывод и ошибку.
func runGoTest(repoDir, pkgDir string) (output string, errMsg string) {
	relPkg, err := filepath.Rel(repoDir, pkgDir)
	if err != nil {
		relPkg = "."
	}
	pkgPath := "./" + filepath.ToSlash(relPkg)

	cmd := exec.Command("go", "test", "-v", "-count=1", "-timeout", "30s", pkgPath)
	cmd.Dir = repoDir

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if err != nil {
		return outputStr, extractTestErrors(outputStr)
	}
	return outputStr, ""
}

// extractTestErrors извлекает сообщения об ошибках из вывода go test.
func extractTestErrors(output string) string {
	var errors []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Ошибки компиляции
		if strings.Contains(trimmed, ": ") && (strings.Contains(trimmed, ".go:") || strings.Contains(trimmed, "cannot") || strings.Contains(trimmed, "undefined")) {
			errors = append(errors, trimmed)
		}
		// Упавшие тесты
		if strings.HasPrefix(trimmed, "--- FAIL:") {
			errors = append(errors, trimmed)
		}
		// t.Errorf / t.Fatalf вывод
		if strings.Contains(trimmed, "Error Trace:") || strings.Contains(trimmed, "Error:") {
			errors = append(errors, trimmed)
		}
		// Прямые ошибки тестов
		if strings.HasPrefix(trimmed, "FAIL") {
			errors = append(errors, trimmed)
		}
	}

	if len(errors) == 0 {
		return output // Возвращаем весь вывод если не смогли распарсить
	}

	return strings.Join(errors, "\n")
}

// countTests подсчитывает количество тестов с данным статусом в выводе go test -v.
func countTests(output, status string) int {
	count := 0
	prefix := "--- " + status + ":"
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(strings.TrimSpace(line), prefix) {
			count++
		}
	}
	return count
}
