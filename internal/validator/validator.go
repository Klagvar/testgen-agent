// Package validator проверяет сгенерированные тесты:
// компиляция, запуск, анализ ошибок.
package validator

import (
	"fmt"
	"os"
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

	// Race detector
	HasRaces    bool   // обнаружены data races
	RaceDetails string // детали data race

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
	if r.HasRaces {
		return fmt.Sprintf("⚠️  Tests passed but DATA RACE detected (%d passed, %s):\n%s",
			r.Passed, r.Duration, r.RaceDetails)
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

// findModuleRoot ищет ближайший go.mod вверх от директории.
// Возвращает директорию с go.mod или пустую строку.
func findModuleRoot(dir string) string {
	current := dir
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

// Validate проверяет сгенерированный тест-файл.
// repoDir — путь к репозиторию, testFile — путь к файлу тестов.
func Validate(repoDir string, testFile string) *Result {
	result := &Result{}
	start := time.Now()

	// Шаг 0: Автоформатирование (goimports фиксит импорты автоматически)
	_ = FormatFile(testFile)

	// Определяем директорию тест-файла
	testDir := filepath.Dir(testFile)

	// Ищем корень Go-модуля для этого тест-файла.
	// Если testdata/sample-project имеет свой go.mod — используем его,
	// а не go.mod основного проекта.
	moduleRoot := findModuleRoot(testDir)
	if moduleRoot == "" {
		moduleRoot = repoDir
	}

	// Шаг 1: Проверяем компиляцию
	compileErr := runGoCommand(moduleRoot, testDir, "build")
	if compileErr != "" {
		result.CompileOK = false
		result.CompileError = compileErr
		result.Duration = time.Since(start)
		return result
	}
	result.CompileOK = true

	// Шаг 2: Запускаем тесты
	testOutput, testErr := runGoTest(moduleRoot, testDir)
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

// ValidateWithRace проверяет тест-файл с включённым детектором гонок данных.
// Вызывает go test -race -v.
func ValidateWithRace(repoDir string, testFile string) *Result {
	result := &Result{}
	start := time.Now()

	_ = FormatFile(testFile)

	testDir := filepath.Dir(testFile)
	moduleRoot := findModuleRoot(testDir)
	if moduleRoot == "" {
		moduleRoot = repoDir
	}

	// Compile check
	compileErr := runGoCommand(moduleRoot, testDir, "build")
	if compileErr != "" {
		result.CompileOK = false
		result.CompileError = compileErr
		result.Duration = time.Since(start)
		return result
	}
	result.CompileOK = true

	// Run tests with -race
	testOutput, testErr := runGoTestRace(moduleRoot, testDir)
	result.TestOutput = testOutput
	result.Duration = time.Since(start)

	// Check for data races
	if strings.Contains(testOutput, "WARNING: DATA RACE") {
		result.HasRaces = true
		result.RaceDetails = extractRaceDetails(testOutput)
	}

	if testErr == "" {
		result.TestsOK = true
		result.Passed = countTests(testOutput, "PASS")
		return result
	}

	result.TestsOK = false
	result.TestError = testErr
	result.Passed = countTests(testOutput, "PASS")
	result.Failed = countTests(testOutput, "FAIL")

	return result
}

// runGoTestRace запускает go test -race и возвращает вывод.
func runGoTestRace(moduleRoot, pkgDir string) (output string, errMsg string) {
	relPkg, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		relPkg = "."
	}
	pkgPath := "./" + filepath.ToSlash(relPkg)
	if pkgPath == "./" {
		pkgPath = "."
	}

	cmd := exec.Command("go", "test", "-race", "-v", "-count=1", "-timeout", "60s", pkgPath)
	cmd.Dir = moduleRoot

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if err != nil {
		return outputStr, extractTestErrors(outputStr)
	}
	return outputStr, ""
}

// extractRaceDetails извлекает информацию о data race из вывода go test -race.
func extractRaceDetails(output string) string {
	var details []string
	lines := strings.Split(output, "\n")
	inRace := false

	for _, line := range lines {
		if strings.Contains(line, "WARNING: DATA RACE") {
			inRace = true
		}
		if inRace {
			details = append(details, line)
			// Race block ends with empty line or goroutine info
			if strings.TrimSpace(line) == "" && len(details) > 3 {
				inRace = false
			}
		}
	}

	if len(details) > 30 {
		details = details[:30]
		details = append(details, "... (truncated)")
	}

	return strings.Join(details, "\n")
}

// runGoCommand запускает go <command> в указанной директории.
func runGoCommand(moduleRoot, pkgDir, command string) string {
	// Определяем относительный путь пакета от корня модуля
	relPkg, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		relPkg = "."
	}
	// Преобразуем к формату Go-пакета: ./path/to/pkg
	pkgPath := "./" + filepath.ToSlash(relPkg)
	if pkgPath == "./" {
		pkgPath = "."
	}

	cmd := exec.Command("go", command, pkgPath)
	cmd.Dir = moduleRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output))
	}
	return ""
}

// runGoTest запускает go test и возвращает вывод и ошибку.
func runGoTest(moduleRoot, pkgDir string) (output string, errMsg string) {
	relPkg, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		relPkg = "."
	}
	pkgPath := "./" + filepath.ToSlash(relPkg)
	if pkgPath == "./" {
		pkgPath = "."
	}

	cmd := exec.Command("go", "test", "-v", "-count=1", "-timeout", "30s", pkgPath)
	cmd.Dir = moduleRoot

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
