// Package coverage implements diff coverage calculation —
// a metric for test coverage of only changed lines of code.
//
// Formula: DC = |covered ∩ changed| / |changed| × 100%
package coverage

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CoverageBlock represents a single coverage block from a Go coverage profile.
// Line format: file.go:startLine.startCol,endLine.endCol numStatements count
type CoverageBlock struct {
	File       string // file path (relative to module)
	StartLine  int
	StartCol   int
	EndLine    int
	EndCol     int
	NumStmt    int // number of statements
	Count      int // execution count (0 = not covered)
}

// DiffCoverageResult holds the diff coverage calculation result.
type DiffCoverageResult struct {
	FilePath       string  // file path
	ChangedLines   []int   // changed lines
	CoveredLines   []int   // lines covered by tests (from changed)
	UncoveredLines []int   // uncovered lines (from changed)
	Coverage       float64 // DC as percentage (0–100)
}

// TotalResult holds the aggregated result across all files.
type TotalResult struct {
	Files          []DiffCoverageResult
	TotalChanged   int
	TotalCovered   int
	TotalUncovered int
	Coverage       float64
}

// Summary returns a text description of the result.
func (r *TotalResult) Summary() string {
	return fmt.Sprintf("Diff coverage: %.1f%% (%d/%d changed lines covered)",
		r.Coverage, r.TotalCovered, r.TotalChanged)
}

// RunCoverage runs go test -coverprofile in the specified module
// and returns the profile file path.
func RunCoverage(moduleRoot string, pkgDir string) (string, string, error) {
	coverFile := filepath.Join(moduleRoot, "cover.out")

	// Determine relative package path
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

// ParseProfile parses a Go coverage profile file.
// Format: mode: set/count/atomic on the first line,
// followed by lines like: file.go:startLine.startCol,endLine.endCol numStatements count
func ParseProfile(content string) ([]CoverageBlock, error) {
	var blocks []CoverageBlock
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and mode: header
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

// parseCoverageLine parses a single coverage profile line.
// Format: path/file.go:startLine.startCol,endLine.endCol numStmt count
func parseCoverageLine(line string) (CoverageBlock, error) {
	var block CoverageBlock

	// Split: "file.go:10.5,20.3 1 1"
	// Find the last space-separated segment
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

	// Parse "file.go:10.5,20.3"
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

// parseLineCol parses a "line.col" string.
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

// CoveredLines returns a set of line numbers covered by tests
// for the specified file.
func CoveredLines(blocks []CoverageBlock, fileSuffix string) map[int]bool {
	covered := make(map[int]bool)

	for _, b := range blocks {
		// Match by path suffix (coverage profile contains full module path)
		if !strings.HasSuffix(b.File, fileSuffix) {
			continue
		}
		if b.Count == 0 {
			continue
		}

		// All lines from StartLine to EndLine are considered covered
		for line := b.StartLine; line <= b.EndLine; line++ {
			covered[line] = true
		}
	}

	return covered
}

// CalculateDiffCoverage computes diff coverage for a single file.
// changedLines are the changed line numbers from diff.
// blocks are all coverage blocks from go test -coverprofile.
// fileSuffix is the file path suffix for matching against the coverage profile.
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

// CalculateTotal aggregates results across all files.
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
