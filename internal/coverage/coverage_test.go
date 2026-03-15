package coverage

import (
	"testing"
)

const sampleProfile = `mode: set
example.com/sample/calc.go:9.35,10.17 1 1
example.com/sample/calc.go:10.17,12.3 1 1
example.com/sample/calc.go:13.2,13.18 1 1
example.com/sample/calc.go:17.28,18.16 1 1
example.com/sample/calc.go:22.39,23.14 1 0
example.com/sample/calc.go:23.14,25.3 1 0
example.com/sample/calc.go:26.2,26.18 1 1
`

func TestParseProfile(t *testing.T) {
	blocks, err := ParseProfile(sampleProfile)
	if err != nil {
		t.Fatalf("ParseProfile error: %v", err)
	}

	if len(blocks) != 7 {
		t.Fatalf("expected 7 blocks, got %d", len(blocks))
	}

	// Check first block
	b := blocks[0]
	if b.File != "example.com/sample/calc.go" {
		t.Errorf("File = %q, want %q", b.File, "example.com/sample/calc.go")
	}
	if b.StartLine != 9 || b.StartCol != 35 {
		t.Errorf("Start = %d.%d, want 9.35", b.StartLine, b.StartCol)
	}
	if b.EndLine != 10 || b.EndCol != 17 {
		t.Errorf("End = %d.%d, want 10.17", b.EndLine, b.EndCol)
	}
	if b.NumStmt != 1 {
		t.Errorf("NumStmt = %d, want 1", b.NumStmt)
	}
	if b.Count != 1 {
		t.Errorf("Count = %d, want 1", b.Count)
	}
}

func TestParseProfile_Empty(t *testing.T) {
	blocks, err := ParseProfile("mode: set\n")
	if err != nil {
		t.Fatalf("ParseProfile error: %v", err)
	}
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestParseProfile_Invalid(t *testing.T) {
	_, err := ParseProfile("mode: set\ninvalid line\n")
	if err == nil {
		t.Error("expected error for invalid line, got nil")
	}
}

func TestCoveredLines(t *testing.T) {
	blocks, _ := ParseProfile(sampleProfile)

	covered := CoveredLines(blocks, "calc.go")

	// Lines 9-10, 10-12, 13, 17-18, 26 should be covered (count > 0)
	// Lines 22-23, 23-25 should NOT be covered (count = 0)

	expectedCovered := []int{9, 10, 11, 12, 13, 17, 18, 26}
	for _, line := range expectedCovered {
		if !covered[line] {
			t.Errorf("line %d should be covered", line)
		}
	}

	expectedNotCovered := []int{22, 23, 24, 25}
	for _, line := range expectedNotCovered {
		if covered[line] {
			t.Errorf("line %d should NOT be covered", line)
		}
	}
}

func TestCoveredLines_NoMatch(t *testing.T) {
	blocks, _ := ParseProfile(sampleProfile)

	covered := CoveredLines(blocks, "nonexistent.go")
	if len(covered) != 0 {
		t.Errorf("expected 0 covered lines for nonexistent file, got %d", len(covered))
	}
}

func TestCalculateDiffCoverage(t *testing.T) {
	blocks, _ := ParseProfile(sampleProfile)

	// Changed lines: 9, 10, 13, 22, 23 (3 covered + 2 not covered)
	changedLines := []int{9, 10, 13, 22, 23}

	result := CalculateDiffCoverage("calc.go", changedLines, blocks)

	if len(result.CoveredLines) != 3 {
		t.Errorf("CoveredLines = %d, want 3", len(result.CoveredLines))
	}
	if len(result.UncoveredLines) != 2 {
		t.Errorf("UncoveredLines = %d, want 2", len(result.UncoveredLines))
	}

	expectedCoverage := 60.0
	if result.Coverage != expectedCoverage {
		t.Errorf("Coverage = %.1f%%, want %.1f%%", result.Coverage, expectedCoverage)
	}
}

func TestCalculateDiffCoverage_AllCovered(t *testing.T) {
	blocks, _ := ParseProfile(sampleProfile)

	changedLines := []int{9, 10, 13}

	result := CalculateDiffCoverage("calc.go", changedLines, blocks)

	if result.Coverage != 100.0 {
		t.Errorf("Coverage = %.1f%%, want 100.0%%", result.Coverage)
	}
	if len(result.UncoveredLines) != 0 {
		t.Errorf("UncoveredLines = %d, want 0", len(result.UncoveredLines))
	}
}

func TestCalculateDiffCoverage_NoneChanged(t *testing.T) {
	blocks, _ := ParseProfile(sampleProfile)

	result := CalculateDiffCoverage("calc.go", []int{}, blocks)

	if result.Coverage != 100.0 {
		t.Errorf("Coverage = %.1f%%, want 100.0%%", result.Coverage)
	}
}

func TestCalculateTotal(t *testing.T) {
	results := []DiffCoverageResult{
		{
			FilePath:       "a.go",
			ChangedLines:   []int{1, 2, 3, 4},
			CoveredLines:   []int{1, 2, 3},
			UncoveredLines: []int{4},
			Coverage:       75.0,
		},
		{
			FilePath:       "b.go",
			ChangedLines:   []int{10, 11},
			CoveredLines:   []int{10, 11},
			UncoveredLines: nil,
			Coverage:       100.0,
		},
	}

	total := CalculateTotal(results)

	if total.TotalChanged != 6 {
		t.Errorf("TotalChanged = %d, want 6", total.TotalChanged)
	}
	if total.TotalCovered != 5 {
		t.Errorf("TotalCovered = %d, want 5", total.TotalCovered)
	}
	if total.TotalUncovered != 1 {
		t.Errorf("TotalUncovered = %d, want 1", total.TotalUncovered)
	}

	expectedCoverage := 5.0 / 6.0 * 100.0
	if total.Coverage < expectedCoverage-0.1 || total.Coverage > expectedCoverage+0.1 {
		t.Errorf("Coverage = %.1f%%, want %.1f%%", total.Coverage, expectedCoverage)
	}
}

func TestCalculateTotal_Empty(t *testing.T) {
	total := CalculateTotal(nil)

	if total.Coverage != 100.0 {
		t.Errorf("Coverage = %.1f%%, want 100.0%%", total.Coverage)
	}
}

func TestTotalResult_Summary(t *testing.T) {
	total := TotalResult{
		TotalChanged: 10,
		TotalCovered: 7,
		Coverage:     70.0,
	}

	summary := total.Summary()
	if summary == "" {
		t.Error("Summary should not be empty")
	}
	t.Logf("Summary: %s", summary)
}

func TestParseCoverageLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{"valid", "pkg/file.go:10.5,20.3 1 1", false},
		{"too few fields", "pkg/file.go:10.5,20.3", true},
		{"no colon", "pkgfile.go 1 1", true},
		{"no comma", "pkg/file.go:10.5 1 1", true},
		{"bad count", "pkg/file.go:10.5,20.3 1 abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCoverageLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCoverageLine(%q) error = %v, wantErr %v", tt.line, err, tt.wantErr)
			}
		})
	}
}
