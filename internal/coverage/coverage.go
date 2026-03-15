// Package coverage реализует расчёт diff coverage —
// метрики покрытия тестами только изменённых строк кода.
//
// Формула: DC = |covered ∩ changed| / |changed| × 100%
package coverage

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CoverageBlock — один блок покрытия из Go coverage profile.
// Формат строки: file.go:startLine.startCol,endLine.endCol numStatements count
type CoverageBlock struct {
	File       string // путь к файлу (относительно модуля)
	StartLine  int
	StartCol   int
	EndLine    int
	EndCol     int
	NumStmt    int // количество statements
	Count      int // сколько раз выполнялся (0 = не покрыт)
}

// DiffCoverageResult — результат расчёта diff coverage.
type DiffCoverageResult struct {
	FilePath       string  // путь к файлу
	ChangedLines   []int   // изменённые строки
	CoveredLines   []int   // покрытые тестами строки (из changed)
	UncoveredLines []int   // непокрытые строки (из changed)
	Coverage       float64 // DC в процентах (0–100)
}

// TotalResult — агрегированный результат по всем файлам.
type TotalResult struct {
	Files          []DiffCoverageResult
	TotalChanged   int
	TotalCovered   int
	TotalUncovered int
	Coverage       float64
}

// Summary возвращает текстовое описание результата.
func (r *TotalResult) Summary() string {
	return fmt.Sprintf("Diff coverage: %.1f%% (%d/%d changed lines covered)",
		r.Coverage, r.TotalCovered, r.TotalChanged)
}

// RunCoverage запускает go test -coverprofile в указанном модуле
// и возвращает путь к файлу профиля.
func RunCoverage(moduleRoot string, pkgDir string) (string, string, error) {
	coverFile := filepath.Join(moduleRoot, "cover.out")

	// Определяем относительный путь пакета
	relPkg, err := filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		relPkg = "."
	}
	if relPkg == "." {
		relPkg = "./..."
	} else {
		relPkg = "./" + filepath.ToSlash(relPkg) + "/..."
	}

	cmd := exec.Command("go", "test",
		"-coverprofile="+coverFile,
		"-covermode=set",
		"-count=1",
		relPkg,
	)
	cmd.Dir = moduleRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", string(output), fmt.Errorf("go test -coverprofile failed: %w\n%s", err, output)
	}

	return coverFile, string(output), nil
}

// ParseProfile парсит Go coverage profile файл.
// Формат: mode: set/count/atomic на первой строке,
// далее строки вида: file.go:startLine.startCol,endLine.endCol numStatements count
func ParseProfile(content string) ([]CoverageBlock, error) {
	var blocks []CoverageBlock
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Пропускаем пустые строки и заголовок mode:
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		block, err := parseCoverageLine(line)
		if err != nil {
			return nil, fmt.Errorf("parse line %q: %w", line, err)
		}
		blocks = append(blocks, block)
	}

	return blocks, scanner.Err()
}

// parseCoverageLine парсит одну строку coverage profile.
// Формат: path/file.go:startLine.startCol,endLine.endCol numStmt count
func parseCoverageLine(line string) (CoverageBlock, error) {
	var block CoverageBlock

	// Разделяем: "file.go:10.5,20.3 1 1"
	// Находим последний пробел-разделённый участок
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return block, fmt.Errorf("expected 3 fields, got %d", len(parts))
	}

	// parts[0] = "file.go:10.5,20.3"
	// parts[1] = numStmt
	// parts[2] = count

	numStmt, err := strconv.Atoi(parts[1])
	if err != nil {
		return block, fmt.Errorf("parse numStmt %q: %w", parts[1], err)
	}
	block.NumStmt = numStmt

	count, err := strconv.Atoi(parts[2])
	if err != nil {
		return block, fmt.Errorf("parse count %q: %w", parts[2], err)
	}
	block.Count = count

	// Парсим "file.go:10.5,20.3"
	fileRange := parts[0]
	colonIdx := strings.LastIndex(fileRange, ":")
	if colonIdx < 0 {
		return block, fmt.Errorf("no colon in %q", fileRange)
	}

	block.File = fileRange[:colonIdx]
	rangeStr := fileRange[colonIdx+1:] // "10.5,20.3"

	commaIdx := strings.Index(rangeStr, ",")
	if commaIdx < 0 {
		return block, fmt.Errorf("no comma in range %q", rangeStr)
	}

	startStr := rangeStr[:commaIdx]  // "10.5"
	endStr := rangeStr[commaIdx+1:]  // "20.3"

	block.StartLine, block.StartCol, err = parseLineCol(startStr)
	if err != nil {
		return block, fmt.Errorf("parse start %q: %w", startStr, err)
	}

	block.EndLine, block.EndCol, err = parseLineCol(endStr)
	if err != nil {
		return block, fmt.Errorf("parse end %q: %w", endStr, err)
	}

	return block, nil
}

// parseLineCol парсит "line.col" строку.
func parseLineCol(s string) (int, int, error) {
	dotIdx := strings.Index(s, ".")
	if dotIdx < 0 {
		return 0, 0, fmt.Errorf("no dot in %q", s)
	}

	line, err := strconv.Atoi(s[:dotIdx])
	if err != nil {
		return 0, 0, err
	}

	col, err := strconv.Atoi(s[dotIdx+1:])
	if err != nil {
		return 0, 0, err
	}

	return line, col, nil
}

// CoveredLines возвращает множество номеров строк, покрытых тестами,
// для указанного файла.
func CoveredLines(blocks []CoverageBlock, fileSuffix string) map[int]bool {
	covered := make(map[int]bool)

	for _, b := range blocks {
		// Сопоставляем по суффиксу пути (coverage profile содержит полный module path)
		if !strings.HasSuffix(b.File, fileSuffix) {
			continue
		}
		if b.Count == 0 {
			continue
		}

		// Все строки от StartLine до EndLine считаются покрытыми
		for line := b.StartLine; line <= b.EndLine; line++ {
			covered[line] = true
		}
	}

	return covered
}

// CalculateDiffCoverage вычисляет diff coverage для одного файла.
// changedLines — номера изменённых строк из diff.
// blocks — все блоки покрытия из go test -coverprofile.
// fileSuffix — суффикс пути файла для сопоставления с coverage profile.
func CalculateDiffCoverage(fileSuffix string, changedLines []int, blocks []CoverageBlock) DiffCoverageResult {
	result := DiffCoverageResult{
		FilePath:     fileSuffix,
		ChangedLines: changedLines,
	}

	if len(changedLines) == 0 {
		result.Coverage = 100.0
		return result
	}

	covered := CoveredLines(blocks, fileSuffix)

	for _, line := range changedLines {
		if covered[line] {
			result.CoveredLines = append(result.CoveredLines, line)
		} else {
			result.UncoveredLines = append(result.UncoveredLines, line)
		}
	}

	result.Coverage = float64(len(result.CoveredLines)) / float64(len(changedLines)) * 100.0

	return result
}

// CalculateTotal агрегирует результаты по всем файлам.
func CalculateTotal(results []DiffCoverageResult) TotalResult {
	total := TotalResult{
		Files: results,
	}

	for _, r := range results {
		total.TotalChanged += len(r.ChangedLines)
		total.TotalCovered += len(r.CoveredLines)
		total.TotalUncovered += len(r.UncoveredLines)
	}

	if total.TotalChanged > 0 {
		total.Coverage = float64(total.TotalCovered) / float64(total.TotalChanged) * 100.0
	} else {
		total.Coverage = 100.0
	}

	return total
}
