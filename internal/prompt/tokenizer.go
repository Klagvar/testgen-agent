package prompt

import (
	"fmt"
	"strings"
)

const defaultCharsPerToken = 4

// TokenBudget controls prompt size limits.
type TokenBudget struct {
	MaxTokens     int // total context window
	CharsPerToken int // heuristic: chars per token (default 4)
}

// DefaultBudget returns a budget for typical models (32k context).
func DefaultBudget() TokenBudget {
	return TokenBudget{
		MaxTokens:     32000,
		CharsPerToken: defaultCharsPerToken,
	}
}

// EstimateTokens estimates token count from a string.
func (b TokenBudget) EstimateTokens(s string) int {
	if b.CharsPerToken <= 0 {
		return len(s) / defaultCharsPerToken
	}
	return len(s) / b.CharsPerToken
}

// TruncateBody truncates a function body keeping the first and last N lines.
// Inserts a comment indicating how many lines were omitted.
func TruncateBody(body string, maxLines int) string {
	lines := strings.Split(body, "\n")
	if len(lines) <= maxLines {
		return body
	}

	keepHead := maxLines * 2 / 3
	keepTail := maxLines - keepHead
	if keepTail < 3 {
		keepTail = 3
		keepHead = maxLines - keepTail
	}

	omitted := len(lines) - keepHead - keepTail
	var result []string
	result = append(result, lines[:keepHead]...)
	result = append(result, fmt.Sprintf("\t// ... %d lines truncated ...", omitted))
	result = append(result, lines[len(lines)-keepTail:]...)
	return strings.Join(result, "\n")
}

// EnforcePromptBudget adjusts the request to fit within the token budget.
// Returns a warning message if truncation was needed, empty string otherwise.
func EnforcePromptBudget(req *TestGenRequest, budget TokenBudget) string {
	const (
		systemOverhead  = 900
		responseReserve = 4000
		maxBodyLines    = 60
	)

	available := budget.MaxTokens - systemOverhead - responseReserve
	if available <= 0 {
		return ""
	}

	prompt := BuildUserPrompt(*req)
	estimated := budget.EstimateTokens(prompt)

	if estimated <= available {
		return ""
	}

	for i := range req.TargetFuncs {
		req.TargetFuncs[i].Body = TruncateBody(req.TargetFuncs[i].Body, maxBodyLines)
	}

	if len(req.CalledFuncs) > 5 {
		req.CalledFuncs = req.CalledFuncs[:5]
	}

	if len(req.ExistingTestNames) > 20 {
		req.ExistingTestNames = req.ExistingTestNames[:20]
	}

	newPrompt := BuildUserPrompt(*req)
	newEstimate := budget.EstimateTokens(newPrompt)
	return fmt.Sprintf("Prompt truncated (estimated %d tokens, limit %d, after truncation %d)",
		estimated, available, newEstimate)
}
