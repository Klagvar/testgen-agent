// Package diff parses git diff output (unified diff format) and returns
// structured data: which files changed, which lines were added/modified.
package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// FileDiff represents changes in a single file.
type FileDiff struct {
	OldPath string // file path before changes (a/...)
	NewPath string // file path after changes (b/...)
	Hunks   []Hunk // individual change blocks
}

// Hunk represents a single change block within a file (between @@...@@).
type Hunk struct {
	OldStart int    // start line in old file
	OldCount int    // line count in old file
	NewStart int    // start line in new file
	NewCount int    // line count in new file
	Header   string // full header line @@ ... @@

	Lines []Line // lines within the hunk
}

// LineType represents a line type in diff.
type LineType int

const (
	LineContext  LineType = iota // unchanged line (context)
	LineAdded                   // added line (+)
	LineDeleted                 // deleted line (-)
)

// Line represents a single line within a hunk.
type Line struct {
	Type    LineType
	Content string // line content (without +/-/ prefix)
	OldNo   int    // line number in old file (0 if added)
	NewNo   int    // line number in new file (0 if deleted)
}

// Parse parses a unified diff (git diff output) and returns a list of changed files.
func Parse(diffText string) ([]FileDiff, error) {
	var files []FileDiff
	lines := strings.Split(diffText, "\n")

	i := 0
	for i < len(lines) {
		// Look for start of a new file: "diff --git a/... b/..."
		if !strings.HasPrefix(lines[i], "diff --git ") {
			i++
			continue
		}

		file, nextIdx, err := parseFileDiff(lines, i)
		if err != nil {
			return nil, fmt.Errorf("error parsing file at line %d: %w", i, err)
		}

		files = append(files, file)
		i = nextIdx
	}

	return files, nil
}

// parseFileDiff parses a single file from the diff starting at position start.
// Returns FileDiff and the index of the next line after this file.
func parseFileDiff(lines []string, start int) (FileDiff, int, error) {
	var file FileDiff

	// Parse "diff --git a/path b/path"
	parts := strings.SplitN(lines[start], " ", 4)
	if len(parts) >= 4 {
		file.OldPath = strings.TrimPrefix(parts[2], "a/")
		file.NewPath = strings.TrimPrefix(parts[3], "b/")
	}

	i := start + 1

	// Skip metadata (index, ---, +++)
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "@@") {
			break
		}
		// Update paths from --- and +++ if present
		if strings.HasPrefix(line, "--- a/") {
			file.OldPath = strings.TrimPrefix(line, "--- a/")
		} else if strings.HasPrefix(line, "+++ b/") {
			file.NewPath = strings.TrimPrefix(line, "+++ b/")
		}
		i++

		// If we hit the start of the next file — exit
		if i < len(lines) && strings.HasPrefix(lines[i], "diff --git ") {
			return file, i, nil
		}
	}

	// Parse hunks
	for i < len(lines) {
		if strings.HasPrefix(lines[i], "diff --git ") {
			break // another file started
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

// parseHunk parses a single hunk starting from the @@ ... @@ line.
func parseHunk(lines []string, start int) (Hunk, int, error) {
	var hunk Hunk
	hunk.Header = lines[start]

	// Parse header: @@ -oldStart,oldCount +newStart,newCount @@
	oldStart, oldCount, newStart, newCount, err := parseHunkHeader(lines[start])
	if err != nil {
		return hunk, 0, fmt.Errorf("error parsing hunk header %q: %w", lines[start], err)
	}

	hunk.OldStart = oldStart
	hunk.OldCount = oldCount
	hunk.NewStart = newStart
	hunk.NewCount = newCount

	// Parse hunk lines
	i := start + 1
	oldLine := oldStart
	newLine := newStart

	for i < len(lines) {
		line := lines[i]

		// End of hunk: new hunk started, new file, or end of diff
		if strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "diff --git ") {
			break
		}

		if len(line) == 0 {
			// Empty line at end of diff — skip
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
			// Context line (space prefix)
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

// parseHunkHeader parses a line like "@@ -1,5 +1,7 @@" or "@@ -1,5 +1,7 @@ funcName".
func parseHunkHeader(header string) (oldStart, oldCount, newStart, newCount int, err error) {
	// Strip @@ at start and everything after @@ at end
	header = strings.TrimPrefix(header, "@@")
	atIdx := strings.Index(header, "@@")
	if atIdx == -1 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header: no closing @@")
	}
	header = strings.TrimSpace(header[:atIdx])

	// Now we have "-1,5 +1,7"
	parts := strings.Fields(header)
	if len(parts) != 2 {
		return 0, 0, 0, 0, fmt.Errorf("expected 2 parts, got %d: %q", len(parts), header)
	}

	oldStart, oldCount, err = parseRange(parts[0], "-")
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("error parsing old range: %w", err)
	}

	newStart, newCount, err = parseRange(parts[1], "+")
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("error parsing new range: %w", err)
	}

	return oldStart, oldCount, newStart, newCount, nil
}

// parseRange parses "-1,5" or "+1,7" or "-1" (count=1 by default).
func parseRange(s, prefix string) (start, count int, err error) {
	s = strings.TrimPrefix(s, prefix)

	parts := strings.SplitN(s, ",", 2)
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse start: %w", err)
	}

	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse count: %w", err)
		}
	} else {
		count = 1 // if count omitted, one line
	}

	return start, count, nil
}

// ChangedLines returns the added/modified line numbers in the new file
// for this FileDiff. These are the lines that need test coverage.
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
