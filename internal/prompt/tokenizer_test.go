package prompt

import (
	"strings"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

func TestEstimateTokens(t *testing.T) {
	b := DefaultBudget()
	text := strings.Repeat("abcd", 100) // 400 chars
	got := b.EstimateTokens(text)
	if got != 100 {
		t.Errorf("expected 100 tokens, got %d", got)
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	b := DefaultBudget()
	if b.EstimateTokens("") != 0 {
		t.Error("expected 0 for empty string")
	}
}

func TestTruncateBody_Short(t *testing.T) {
	body := "line1\nline2\nline3"
	got := TruncateBody(body, 10)
	if got != body {
		t.Error("short body should not be truncated")
	}
}

func TestTruncateBody_Long(t *testing.T) {
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "	x := 1")
	}
	body := strings.Join(lines, "\n")

	got := TruncateBody(body, 20)
	if !strings.Contains(got, "lines truncated") {
		t.Error("expected truncation comment")
	}
	gotLines := strings.Split(got, "\n")
	if len(gotLines) > 25 {
		t.Errorf("expected ~21 lines, got %d", len(gotLines))
	}
}

func TestEnforcePromptBudget_FitsWithin(t *testing.T) {
	req := TestGenRequest{
		PackageName: "test",
		FilePath:    "test.go",
		TargetFuncs: []analyzer.FuncInfo{
			{Name: "Add", Signature: "func Add(a, b int) int", Body: "return a+b"},
		},
	}
	b := DefaultBudget()
	warning := EnforcePromptBudget(&req, b)
	if warning != "" {
		t.Errorf("should not truncate small prompt, got: %s", warning)
	}
}

func TestEnforcePromptBudget_Truncates(t *testing.T) {
	var longBody strings.Builder
	for i := 0; i < 500; i++ {
		longBody.WriteString("	x := doSomethingVeryLongThatTakesUpLotsOfTokens(param1, param2, param3)\n")
	}

	req := TestGenRequest{
		PackageName: "test",
		FilePath:    "test.go",
		TargetFuncs: []analyzer.FuncInfo{
			{Name: "Big", Signature: "func Big()", Body: longBody.String()},
		},
		ExistingTests: strings.Repeat("func TestExisting(t *testing.T) { t.Log(\"test\") }\n", 500),
	}

	b := TokenBudget{MaxTokens: 6000, CharsPerToken: 4}
	warning := EnforcePromptBudget(&req, b)
	if warning == "" {
		t.Error("expected truncation warning")
	}
	if !strings.Contains(req.TargetFuncs[0].Body, "truncated") {
		t.Error("expected body to be truncated")
	}
}

func TestDefaultBudget(t *testing.T) {
	b := DefaultBudget()
	if b.MaxTokens != 32000 {
		t.Errorf("expected 32000, got %d", b.MaxTokens)
	}
	if b.CharsPerToken != 4 {
		t.Errorf("expected 4, got %d", b.CharsPerToken)
	}
}
