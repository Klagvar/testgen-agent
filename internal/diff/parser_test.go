package diff

import (
	"testing"
)

// Example of real git diff output.
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
		t.Fatalf("Parse returned error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	file := files[0]

	// Check paths
	if file.OldPath != "calc/calc.go" {
		t.Errorf("OldPath = %q, expected %q", file.OldPath, "calc/calc.go")
	}
	if file.NewPath != "calc/calc.go" {
		t.Errorf("NewPath = %q, expected %q", file.NewPath, "calc/calc.go")
	}

	// Check number of hunks
	if len(file.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(file.Hunks))
	}

	hunk := file.Hunks[0]
	if hunk.OldStart != 1 || hunk.OldCount != 8 {
		t.Errorf("Old range = %d,%d, expected 1,8", hunk.OldStart, hunk.OldCount)
	}
	if hunk.NewStart != 1 || hunk.NewCount != 12 {
		t.Errorf("New range = %d,%d, expected 1,12", hunk.NewStart, hunk.NewCount)
	}
}

func TestParse_ChangedLines(t *testing.T) {
	files, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	changed := files[0].ChangedLines()

	// Added lines: lines with + (new line numbers in the file)
	// +func Add(a, b int) (int, error) {       → line 3
	// +	if a < 0 || b < 0 {                   → line 4
	// +		return 0, fmt.Errorf(...)           → line 5
	// +	}                                      → line 6
	// +	return a + b, nil                      → line 7
	// (empty line +)                           → line 12
	// +func Multiply(a, b int) int {             → line 13  (but with blank +)
	// etc.

	if len(changed) == 0 {
		t.Fatal("ChangedLines returned empty list")
	}

	t.Logf("Changed lines: %v", changed)

	// Check that there are at least a few added lines
	if len(changed) < 5 {
		t.Errorf("expected >= 5 changed lines, got %d", len(changed))
	}
}

func TestParse_LineTypes(t *testing.T) {
	files, err := Parse(sampleDiff)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
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

	t.Logf("Lines: added=%d, deleted=%d, context=%d", added, deleted, context)

	if added == 0 {
		t.Error("no added lines")
	}
	if deleted == 0 {
		t.Error("no deleted lines")
	}
	if context == 0 {
		t.Error("no context lines")
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
		t.Fatalf("Parse returned error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	// First file
	if files[0].NewPath != "main.go" {
		t.Errorf("file 0: NewPath = %q, expected %q", files[0].NewPath, "main.go")
	}
	if len(files[0].Hunks) != 1 {
		t.Errorf("file 0: expected 1 hunk, got %d", len(files[0].Hunks))
	}

	// Second file
	if files[1].NewPath != "utils/helper.go" {
		t.Errorf("file 1: NewPath = %q, expected %q", files[1].NewPath, "utils/helper.go")
	}
	if len(files[1].Hunks) != 1 {
		t.Errorf("file 1: expected 1 hunk, got %d", len(files[1].Hunks))
	}

	// Check changed lines of second file
	changed := files[1].ChangedLines()
	t.Logf("Changed lines in utils/helper.go: %v", changed)
	if len(changed) != 3 {
		t.Errorf("expected 3 changed lines, got %d", len(changed))
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
			name:     "normal header",
			header:   "@@ -1,5 +1,7 @@",
			oldStart: 1, oldCount: 5,
			newStart: 1, newCount: 7,
		},
		{
			name:     "with function name",
			header:   "@@ -10,3 +10,5 @@ func Calculate()",
			oldStart: 10, oldCount: 3,
			newStart: 10, newCount: 5,
		},
		{
			name:     "single line (no count)",
			header:   "@@ -1 +1 @@",
			oldStart: 1, oldCount: 1,
			newStart: 1, newCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStart, oldCount, newStart, newCount, err := parseHunkHeader(tt.header)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if oldStart != tt.oldStart || oldCount != tt.oldCount {
				t.Errorf("old = %d,%d, expected %d,%d", oldStart, oldCount, tt.oldStart, tt.oldCount)
			}
			if newStart != tt.newStart || newCount != tt.newCount {
				t.Errorf("new = %d,%d, expected %d,%d", newStart, newCount, tt.newStart, tt.newCount)
			}
		})
	}
}
