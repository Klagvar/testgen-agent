package branchcov

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/coverage"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestAnalyze_IfElseIfElse(t *testing.T) {
	src := `package p

func Classify(x int) string {
	if x > 0 {
		return "pos"
	} else if x < 0 {
		return "neg"
	} else {
		return "zero"
	}
}
`
	p := writeFile(t, src)
	branches, err := Analyze(p, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(branches) != 3 {
		t.Fatalf("got %d branches, want 3: %+v", len(branches), branches)
	}
	kinds := []Kind{branches[0].Kind, branches[1].Kind, branches[2].Kind}
	expected := []Kind{KindIf, KindElseIf, KindElse}
	for i, k := range expected {
		if kinds[i] != k {
			t.Errorf("branches[%d].Kind = %s, want %s", i, kinds[i], k)
		}
	}
	for _, b := range branches {
		if b.IsErrorPath {
			t.Errorf("none of these branches are error paths: %+v", b)
		}
	}
}

func TestAnalyze_DetectsErrorPath(t *testing.T) {
	src := `package p

import "errors"

func Do() error {
	err := errors.New("x")
	if err != nil {
		return err
	}
	return nil
}
`
	p := writeFile(t, src)
	branches, err := Analyze(p, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("got %d branches, want 1: %+v", len(branches), branches)
	}
	if !branches[0].IsErrorPath {
		t.Errorf("expected IsErrorPath=true, got %+v", branches[0])
	}
}

func TestAnalyze_Switch(t *testing.T) {
	src := `package p

func Color(s string) int {
	switch s {
	case "red":
		return 1
	case "green":
		return 2
	default:
		return 0
	}
}
`
	p := writeFile(t, src)
	branches, err := Analyze(p, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(branches) != 3 {
		t.Fatalf("got %d branches, want 3: %+v", len(branches), branches)
	}
	seen := map[Kind]int{}
	for _, b := range branches {
		seen[b.Kind]++
	}
	if seen[KindSwitchCase] != 2 {
		t.Errorf("expected 2 switch cases, got %d", seen[KindSwitchCase])
	}
	if seen[KindDefault] != 1 {
		t.Errorf("expected 1 default, got %d", seen[KindDefault])
	}
}

func TestAnalyze_TypeSwitch(t *testing.T) {
	src := `package p

func TypeOf(x interface{}) string {
	switch x.(type) {
	case int:
		return "int"
	case string:
		return "str"
	default:
		return "other"
	}
}
`
	p := writeFile(t, src)
	branches, err := Analyze(p, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	// 2 type-cases + 1 default
	if len(branches) != 3 {
		t.Fatalf("got %d branches, want 3: %+v", len(branches), branches)
	}
}

func TestAnalyze_Select(t *testing.T) {
	src := `package p

func Race(a, b chan int) int {
	select {
	case v := <-a:
		return v
	case v := <-b:
		return v
	default:
		return -1
	}
}
`
	p := writeFile(t, src)
	branches, err := Analyze(p, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(branches) != 3 {
		t.Fatalf("got %d branches, want 3: %+v", len(branches), branches)
	}
	seen := map[Kind]int{}
	for _, b := range branches {
		seen[b.Kind]++
	}
	if seen[KindSelectCase] != 2 || seen[KindDefault] != 1 {
		t.Errorf("unexpected kinds: %+v", seen)
	}
}

func TestAnalyze_FuncNameFilter(t *testing.T) {
	src := `package p

func A() {
	if true {
		_ = 1
	}
}

func B() {
	if true {
		_ = 2
	}
}
`
	p := writeFile(t, src)
	branches, err := Analyze(p, map[string]bool{"B": true})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(branches) != 1 || branches[0].Function != "B" {
		t.Fatalf("expected 1 branch in B, got %+v", branches)
	}
}

func TestCalculate_MixedCoverage(t *testing.T) {
	// Three branches at lines 5, 10, 15. Only line 5 and 15 are covered.
	branches := []Branch{
		{Kind: KindIf, Line: 5, IsErrorPath: true},
		{Kind: KindElse, Line: 10, IsErrorPath: false},
		{Kind: KindSwitchCase, Line: 15, IsErrorPath: false},
	}
	blocks := []coverage.CoverageBlock{
		{File: "pkg/foo.go", StartLine: 5, EndLine: 5, Count: 1},
		{File: "pkg/foo.go", StartLine: 10, EndLine: 10, Count: 0},
		{File: "pkg/foo.go", StartLine: 15, EndLine: 15, Count: 3},
	}
	res := Calculate(branches, blocks, "pkg/foo.go")
	if res.Total != 3 || res.Covered != 2 {
		t.Errorf("coverage counts wrong: %+v", res)
	}
	if res.Coverage < 66.0 || res.Coverage > 67.0 {
		t.Errorf("coverage percent = %f, want ~66.67", res.Coverage)
	}
	if res.ErrorPathsTotal != 1 || res.ErrorPathsCovered != 1 {
		t.Errorf("error path counts wrong: %+v", res)
	}
	if res.ErrorPathCoverage != 100.0 {
		t.Errorf("error path coverage = %f, want 100", res.ErrorPathCoverage)
	}
}

func TestCalculate_NoBranches(t *testing.T) {
	res := Calculate(nil, nil, "foo.go")
	if res.Coverage != 100 || res.ErrorPathCoverage != 100 {
		t.Errorf("empty branches must yield 100%%: %+v", res)
	}
}

func TestCalculate_NoErrorPaths(t *testing.T) {
	branches := []Branch{{Kind: KindIf, Line: 5, IsErrorPath: false}}
	blocks := []coverage.CoverageBlock{{File: "a.go", StartLine: 5, EndLine: 5, Count: 0}}
	res := Calculate(branches, blocks, "a.go")
	if res.ErrorPathCoverage != 100 {
		t.Errorf("no error paths must yield 100%% err-coverage, got %f", res.ErrorPathCoverage)
	}
}
