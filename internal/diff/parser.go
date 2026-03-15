// Package diff парсит вывод git diff (unified diff format) и возвращает
// структурированные данные: какие файлы изменены, какие строки добавлены/изменены.
package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// FileDiff представляет изменения в одном файле.
type FileDiff struct {
	OldPath string // путь к файлу до изменений (a/...)
	NewPath string // путь к файлу после изменений (b/...)
	Hunks   []Hunk // отдельные блоки изменений
}

// Hunk — один блок изменений внутри файла (то, что между @@...@@).
type Hunk struct {
	OldStart int    // начальная строка в старом файле
	OldCount int    // количество строк в старом файле
	NewStart int    // начальная строка в новом файле
	NewCount int    // количество строк в новом файле
	Header   string // полная строка заголовка @@ ... @@

	Lines []Line // строки внутри хунка
}

// LineType — тип строки в diff.
type LineType int

const (
	LineContext  LineType = iota // строка без изменений (контекст)
	LineAdded                   // добавленная строка (+)
	LineDeleted                 // удалённая строка (-)
)

// Line — одна строка внутри хунка.
type Line struct {
	Type    LineType
	Content string // содержимое строки (без префикса +/-/ )
	OldNo   int    // номер строки в старом файле (0 если добавленная)
	NewNo   int    // номер строки в новом файле (0 если удалённая)
}

// Parse парсит unified diff (вывод git diff) и возвращает список изменённых файлов.
func Parse(diffText string) ([]FileDiff, error) {
	var files []FileDiff
	lines := strings.Split(diffText, "\n")

	i := 0
	for i < len(lines) {
		// Ищем начало нового файла: "diff --git a/... b/..."
		if !strings.HasPrefix(lines[i], "diff --git ") {
			i++
			continue
		}

		file, nextIdx, err := parseFileDiff(lines, i)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга файла на строке %d: %w", i, err)
		}

		files = append(files, file)
		i = nextIdx
	}

	return files, nil
}

// parseFileDiff парсит один файл из diff, начиная с позиции start.
// Возвращает FileDiff и индекс следующей строки после этого файла.
func parseFileDiff(lines []string, start int) (FileDiff, int, error) {
	var file FileDiff

	// Парсим "diff --git a/path b/path"
	parts := strings.SplitN(lines[start], " ", 4)
	if len(parts) >= 4 {
		file.OldPath = strings.TrimPrefix(parts[2], "a/")
		file.NewPath = strings.TrimPrefix(parts[3], "b/")
	}

	i := start + 1

	// Пропускаем метаданные (index, ---, +++)
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "@@") {
			break
		}
		// Обновляем пути из --- и +++ если есть
		if strings.HasPrefix(line, "--- a/") {
			file.OldPath = strings.TrimPrefix(line, "--- a/")
		} else if strings.HasPrefix(line, "+++ b/") {
			file.NewPath = strings.TrimPrefix(line, "+++ b/")
		}
		i++

		// Если встретили начало следующего файла — выходим
		if i < len(lines) && strings.HasPrefix(lines[i], "diff --git ") {
			return file, i, nil
		}
	}

	// Парсим хунки
	for i < len(lines) {
		if strings.HasPrefix(lines[i], "diff --git ") {
			break // начался другой файл
		}

		if strings.HasPrefix(lines[i], "@@") {
			hunk, nextIdx, err := parseHunk(lines, i)
			if err != nil {
				return file, 0, err
			}
			file.Hunks = append(file.Hunks, hunk)
			i = nextIdx
		} else {
			i++
		}
	}

	return file, i, nil
}

// parseHunk парсит один хунк, начиная со строки @@ ... @@.
func parseHunk(lines []string, start int) (Hunk, int, error) {
	var hunk Hunk
	hunk.Header = lines[start]

	// Парсим заголовок: @@ -oldStart,oldCount +newStart,newCount @@
	oldStart, oldCount, newStart, newCount, err := parseHunkHeader(lines[start])
	if err != nil {
		return hunk, 0, fmt.Errorf("ошибка парсинга заголовка хунка %q: %w", lines[start], err)
	}

	hunk.OldStart = oldStart
	hunk.OldCount = oldCount
	hunk.NewStart = newStart
	hunk.NewCount = newCount

	// Парсим строки хунка
	i := start + 1
	oldLine := oldStart
	newLine := newStart

	for i < len(lines) {
		line := lines[i]

		// Конец хунка: начался новый хунк, новый файл, или конец diff
		if strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "diff --git ") {
			break
		}

		if len(line) == 0 {
			// Пустая строка в конце diff — пропускаем
			i++
			continue
		}

		switch line[0] {
		case '+':
			hunk.Lines = append(hunk.Lines, Line{
				Type:    LineAdded,
				Content: line[1:],
				OldNo:   0,
				NewNo:   newLine,
			})
			newLine++
		case '-':
			hunk.Lines = append(hunk.Lines, Line{
				Type:    LineDeleted,
				Content: line[1:],
				OldNo:   oldLine,
				NewNo:   0,
			})
			oldLine++
		default:
			// Контекстная строка (пробел в начале)
			content := line
			if len(content) > 0 && content[0] == ' ' {
				content = content[1:]
			}
			hunk.Lines = append(hunk.Lines, Line{
				Type:    LineContext,
				Content: content,
				OldNo:   oldLine,
				NewNo:   newLine,
			})
			oldLine++
			newLine++
		}

		i++
	}

	return hunk, i, nil
}

// parseHunkHeader парсит строку вида "@@ -1,5 +1,7 @@" или "@@ -1,5 +1,7 @@ funcName".
func parseHunkHeader(header string) (oldStart, oldCount, newStart, newCount int, err error) {
	// Убираем @@ в начале и всё после @@ в конце
	header = strings.TrimPrefix(header, "@@")
	atIdx := strings.Index(header, "@@")
	if atIdx == -1 {
		return 0, 0, 0, 0, fmt.Errorf("некорректный заголовок хунка: нет закрывающего @@")
	}
	header = strings.TrimSpace(header[:atIdx])

	// Теперь имеем "-1,5 +1,7"
	parts := strings.Fields(header)
	if len(parts) != 2 {
		return 0, 0, 0, 0, fmt.Errorf("ожидалось 2 части, получено %d: %q", len(parts), header)
	}

	oldStart, oldCount, err = parseRange(parts[0], "-")
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("ошибка парсинга старого диапазона: %w", err)
	}

	newStart, newCount, err = parseRange(parts[1], "+")
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("ошибка парсинга нового диапазона: %w", err)
	}

	return oldStart, oldCount, newStart, newCount, nil
}

// parseRange парсит "-1,5" или "+1,7" или "-1" (count=1 по умолчанию).
func parseRange(s, prefix string) (start, count int, err error) {
	s = strings.TrimPrefix(s, prefix)

	parts := strings.SplitN(s, ",", 2)
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("не удалось распарсить start: %w", err)
	}

	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("не удалось распарсить count: %w", err)
		}
	} else {
		count = 1 // если count не указан, значит 1 строка
	}

	return start, count, nil
}

// ChangedLines возвращает номера добавленных/изменённых строк в новом файле
// для данного FileDiff. Это строки, которые нужно покрыть тестами.
func (f *FileDiff) ChangedLines() []int {
	var lines []int
	for _, hunk := range f.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == LineAdded && line.NewNo > 0 {
				lines = append(lines, line.NewNo)
			}
		}
	}
	return lines
}
