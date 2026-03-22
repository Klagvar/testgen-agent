package coverage

import (
	"os"
	"strings"
)

// FilterExecutableLines removes non-executable lines from the set of changed lines.
// Non-executable lines include: blank lines, comments, package declarations,
// import blocks, and lines containing only braces.
func FilterExecutableLines(filePath string, lines []int) []int {
	if len(lines) == 0 {
		return lines
	}

	src, err := os.ReadFile(filePath)
	if err != nil {
		return lines
	}

	return FilterExecutableLinesFromSource(string(src), lines)
}

// FilterExecutableLinesFromSource works like FilterExecutableLines but accepts
// source code directly (useful for testing without files).
func FilterExecutableLinesFromSource(src string, lines []int) []int {
	if len(lines) == 0 {
		return lines
	}

	srcLines := strings.Split(src, "\n")
	lineSet := make(map[int]bool, len(lines))
	for _, l := range lines {
		lineSet[l] = true
	}

	inBlockComment := false
	inImportBlock := false
	nonExec := make(map[int]bool)

	for i, line := range srcLines {
		lineNum := i + 1
		if !lineSet[lineNum] {
			continue
		}

		trimmed := strings.TrimSpace(line)

		if inBlockComment {
			nonExec[lineNum] = true
			if strings.Contains(trimmed, "*/") {
				inBlockComment = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "/*") {
			nonExec[lineNum] = true
			if !strings.Contains(trimmed, "*/") {
				inBlockComment = true
			}
			continue
		}

		if inImportBlock {
			nonExec[lineNum] = true
			if trimmed == ")" {
				inImportBlock = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "import (") || trimmed == "import (" {
			nonExec[lineNum] = true
			inImportBlock = true
			continue
		}

		if trimmed == "" ||
			strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "package ") ||
			strings.HasPrefix(trimmed, "import \"") ||
			trimmed == "{" ||
			trimmed == "}" ||
			trimmed == "})" ||
			trimmed == ")," ||
			trimmed == ")" {
			nonExec[lineNum] = true
			continue
		}
	}

	var result []int
	for _, l := range lines {
		if !nonExec[l] {
			result = append(result, l)
		}
	}
	return result
}
