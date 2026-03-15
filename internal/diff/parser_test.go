package diff

import (
	"testing"
)

// Пример реального вывода git diff.
const sampleDiff = `diff --git a/calc/calc.go b/calc/calc.go
index 1234567..abcdefg 100644
--- a/calc/calc.go
+++ b/calc/calc.go
@@ -1,8 +1,12 @@
 package calc
 
-func Add(a, b int) int {
-	return a + b
+func Add(a, b int) (int, error) {
+	if a < 0 || b < 0 {
+		return 0, fmt.Errorf("negative numbers not allowed: %d, %d", a, b)
+	}
+	return a + b, nil
 }
 
 func Subtract(a, b int) int {
 	return a - b
 }
+
+func Multiply(a, b int) int {
+	return a * b
+}
`

func TestParse_SingleFile(t *testing.T) {
	files, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse вернул ошибку: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("ожидался 1 файл, получено %d", len(files))
	}

	file := files[0]

	// Проверяем пути
	if file.OldPath != "calc/calc.go" {
		t.Errorf("OldPath = %q, ожидалось %q", file.OldPath, "calc/calc.go")
	}
	if file.NewPath != "calc/calc.go" {
		t.Errorf("NewPath = %q, ожидалось %q", file.NewPath, "calc/calc.go")
	}

	// Проверяем количество хунков
	if len(file.Hunks) != 1 {
		t.Fatalf("ожидался 1 хунк, получено %d", len(file.Hunks))
	}

	hunk := file.Hunks[0]
	if hunk.OldStart != 1 || hunk.OldCount != 8 {
		t.Errorf("Old range = %d,%d, ожидалось 1,8", hunk.OldStart, hunk.OldCount)
	}
	if hunk.NewStart != 1 || hunk.NewCount != 12 {
		t.Errorf("New range = %d,%d, ожидалось 1,12", hunk.NewStart, hunk.NewCount)
	}
}

func TestParse_ChangedLines(t *testing.T) {
	files, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse вернул ошибку: %v", err)
	}

	changed := files[0].ChangedLines()

	// Добавленные строки: строки с + (новые номера в файле)
	// +func Add(a, b int) (int, error) {       → строка 3
	// +	if a < 0 || b < 0 {                   → строка 4
	// +		return 0, fmt.Errorf(...)           → строка 5
	// +	}                                      → строка 6
	// +	return a + b, nil                      → строка 7
	// (пустая строка +)                         → строка 12
	// +func Multiply(a, b int) int {             → строка 13  (но с пустой +)
	// и т.д.

	if len(changed) == 0 {
		t.Fatal("ChangedLines вернул пустой список")
	}

	t.Logf("Изменённые строки: %v", changed)

	// Проверяем что есть хотя бы несколько добавленных строк
	if len(changed) < 5 {
		t.Errorf("ожидалось >= 5 изменённых строк, получено %d", len(changed))
	}
}

func TestParse_LineTypes(t *testing.T) {
	files, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse вернул ошибку: %v", err)
	}

	hunk := files[0].Hunks[0]

	var added, deleted, context int
	for _, line := range hunk.Lines {
		switch line.Type {
		case LineAdded:
			added++
		case LineDeleted:
			deleted++
		case LineContext:
			context++
		}
	}

	t.Logf("Строки: added=%d, deleted=%d, context=%d", added, deleted, context)

	if added == 0 {
		t.Error("нет добавленных строк")
	}
	if deleted == 0 {
		t.Error("нет удалённых строк")
	}
	if context == 0 {
		t.Error("нет контекстных строк")
	}
}

const multiFileDiff = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -5,3 +5,4 @@
 func main() {
 	fmt.Println("hello")
+	fmt.Println("world")
 }
diff --git a/utils/helper.go b/utils/helper.go
index 3333333..4444444 100644
--- a/utils/helper.go
+++ b/utils/helper.go
@@ -1,4 +1,6 @@
 package utils
 
+import "strings"
+
 func ToUpper(s string) string {
-	return s
+	return strings.ToUpper(s)
 }
`

func TestParse_MultipleFiles(t *testing.T) {
	files, err := Parse(multiFileDiff)
	if err != nil {
		t.Fatalf("Parse вернул ошибку: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("ожидалось 2 файла, получено %d", len(files))
	}

	// Первый файл
	if files[0].NewPath != "main.go" {
		t.Errorf("файл 0: NewPath = %q, ожидалось %q", files[0].NewPath, "main.go")
	}
	if len(files[0].Hunks) != 1 {
		t.Errorf("файл 0: ожидался 1 хунк, получено %d", len(files[0].Hunks))
	}

	// Второй файл
	if files[1].NewPath != "utils/helper.go" {
		t.Errorf("файл 1: NewPath = %q, ожидалось %q", files[1].NewPath, "utils/helper.go")
	}
	if len(files[1].Hunks) != 1 {
		t.Errorf("файл 1: ожидался 1 хунк, получено %d", len(files[1].Hunks))
	}

	// Проверяем changed lines второго файла
	changed := files[1].ChangedLines()
	t.Logf("Изменённые строки в utils/helper.go: %v", changed)
	if len(changed) != 3 {
		t.Errorf("ожидалось 3 изменённых строки, получено %d", len(changed))
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		oldStart int
		oldCount int
		newStart int
		newCount int
	}{
		{
			name:     "обычный заголовок",
			header:   "@@ -1,5 +1,7 @@",
			oldStart: 1, oldCount: 5,
			newStart: 1, newCount: 7,
		},
		{
			name:     "с именем функции",
			header:   "@@ -10,3 +10,5 @@ func Calculate()",
			oldStart: 10, oldCount: 3,
			newStart: 10, newCount: 5,
		},
		{
			name:     "одна строка (без count)",
			header:   "@@ -1 +1 @@",
			oldStart: 1, oldCount: 1,
			newStart: 1, newCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStart, oldCount, newStart, newCount, err := parseHunkHeader(tt.header)
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if oldStart != tt.oldStart || oldCount != tt.oldCount {
				t.Errorf("old = %d,%d, ожидалось %d,%d", oldStart, oldCount, tt.oldStart, tt.oldCount)
			}
			if newStart != tt.newStart || newCount != tt.newCount {
				t.Errorf("new = %d,%d, ожидалось %d,%d", newStart, newCount, tt.newStart, tt.newCount)
			}
		})
	}
}
