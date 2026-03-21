// Package github implements GitHub API interaction:
// posting and updating reports in PR comments.
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const botMarker = "<!-- testgen-agent-report -->"

// FileReport holds the report for a single file.
type FileReport struct {
	File           string   // file path
	Functions      []string // tested functions
	TestsTotal     int      // total generated tests
	TestsPassed    int      // passed validation
	TestsPruned    int      // pruned (failing)
	DiffCoverage   float64  // diff coverage %
	MutationScore  float64  // mutation score % (0 = not run)
	MutationKilled int      // mutations killed
	MutationTotal  int      // total mutations
	Status         string   // "success", "partial", "failed"
}

// Report holds the full agent report.
type Report struct {
	Files          []FileReport
	TotalGenerated int
	TotalValidated int
	TotalCached    int
	TotalDiffCov   float64 // average diff coverage
	CoverageTarget float64 // target diff coverage %
	Model          string  // LLM model
	Duration       time.Duration
	BaseBranch     string
}

// Commenter posts reports to a GitHub PR.
type Commenter struct {
	token   string
	owner   string
	repo    string
	prNum   int
	apiBase string
}

// NewCommenter creates a new commenter.
func NewCommenter(token, owner, repo string, prNum int) *Commenter {
	return &Commenter{
		token:   token,
		owner:   owner,
		repo:    repo,
		prNum:   prNum,
		apiBase: "https://api.github.com",
	}
}

// PostReport posts (or updates) a report as a PR comment.
// If a previous testgen-agent comment exists, it will be updated in place.
func (c *Commenter) PostReport(report Report) error {
	body := formatReport(report)

	existingID, err := c.findBotComment()
	if err != nil {
		return fmt.Errorf("find existing comment: %w", err)
	}

	if existingID > 0 {
		return c.updateComment(existingID, body)
	}
	return c.postComment(body)
}

// formatReport builds the Markdown report text.
func formatReport(r Report) string {
	var sb strings.Builder

	sb.WriteString(botMarker + "\n")
	sb.WriteString("## 🤖 Testgen Agent Report\n\n")

	// Overall status badge
	overallStatus := "✅ All tests passed"
	if r.TotalGenerated > 0 && r.TotalValidated == 0 {
		overallStatus = "❌ No tests passed validation"
	} else if r.TotalValidated < r.TotalGenerated {
		overallStatus = "⚠️ Some tests failed validation"
	} else if r.TotalGenerated == 0 {
		overallStatus = "ℹ️ No tests generated"
	}
	sb.WriteString(fmt.Sprintf("> **Status:** %s\n\n", overallStatus))

	// Summary table
	sb.WriteString("### 📊 Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Model | `%s` |\n", r.Model))
	if r.BaseBranch != "" {
		sb.WriteString(fmt.Sprintf("| Base branch | `%s` |\n", r.BaseBranch))
	}
	sb.WriteString(fmt.Sprintf("| Files processed | %d |\n", len(r.Files)))
	sb.WriteString(fmt.Sprintf("| Tests generated | %d |\n", r.TotalGenerated))
	sb.WriteString(fmt.Sprintf("| Tests validated | %d |\n", r.TotalValidated))
	if r.TotalCached > 0 {
		sb.WriteString(fmt.Sprintf("| Functions cached (skipped) | %d |\n", r.TotalCached))
	}
	if r.TotalDiffCov > 0 {
		covEmoji := "🟢"
		if r.CoverageTarget > 0 && r.TotalDiffCov < r.CoverageTarget {
			covEmoji = "🟡"
		}
		target := ""
		if r.CoverageTarget > 0 {
			target = fmt.Sprintf(" / %.0f%% target", r.CoverageTarget)
		}
		sb.WriteString(fmt.Sprintf("| Avg diff coverage | %s %.1f%%%s |\n", covEmoji, r.TotalDiffCov, target))
	}

	hasMutation := false
	for _, f := range r.Files {
		if f.MutationTotal > 0 {
			hasMutation = true
			break
		}
	}
	if hasMutation {
		totalKilled, totalMut := 0, 0
		for _, f := range r.Files {
			totalKilled += f.MutationKilled
			totalMut += f.MutationTotal
		}
		score := float64(0)
		if totalMut > 0 {
			score = float64(totalKilled) / float64(totalMut) * 100
		}
		sb.WriteString(fmt.Sprintf("| Mutation score | 🧬 %.1f%% (%d/%d killed) |\n", score, totalKilled, totalMut))
	}

	sb.WriteString(fmt.Sprintf("| Duration | ⏱️ %s |\n", r.Duration.Round(time.Second)))
	sb.WriteString("\n")

	// Per-file details (collapsible)
	if len(r.Files) > 0 {
		sb.WriteString("### 📁 File Details\n\n")

		if len(r.Files) > 3 {
			sb.WriteString("<details>\n<summary>Click to expand file details</summary>\n\n")
		}

		sb.WriteString("| File | Functions | Generated | Validated | Diff Cov | Status |\n")
		sb.WriteString("|------|-----------|-----------|-----------|----------|--------|\n")

		for _, f := range r.Files {
			funcs := strings.Join(f.Functions, ", ")
			if len(funcs) > 40 {
				funcs = funcs[:37] + "..."
			}

			statusEmoji := "✅"
			switch f.Status {
			case "partial":
				statusEmoji = "⚠️"
			case "failed":
				statusEmoji = "❌"
			}

			covStr := "—"
			if f.DiffCoverage > 0 {
				covStr = fmt.Sprintf("%.1f%%", f.DiffCoverage)
			}

			sb.WriteString(fmt.Sprintf("| `%s` | %s | %d | %d | %s | %s |\n",
				f.File, funcs, f.TestsTotal, f.TestsPassed, covStr, statusEmoji))
		}

		if len(r.Files) > 3 {
			sb.WriteString("\n</details>\n")
		}

		sb.WriteString("\n")
	}

	// Mutation details (collapsible, only when available)
	if hasMutation {
		sb.WriteString("<details>\n<summary>🧬 Mutation Testing Details</summary>\n\n")
		sb.WriteString("| File | Score | Killed | Total | Survived |\n")
		sb.WriteString("|------|-------|--------|-------|----------|\n")
		for _, f := range r.Files {
			if f.MutationTotal == 0 {
				continue
			}
			survived := f.MutationTotal - f.MutationKilled
			scoreEmoji := "🟢"
			if f.MutationScore < 60 {
				scoreEmoji = "🔴"
			} else if f.MutationScore < 80 {
				scoreEmoji = "🟡"
			}
			sb.WriteString(fmt.Sprintf("| `%s` | %s %.1f%% | %d | %d | %d |\n",
				f.File, scoreEmoji, f.MutationScore, f.MutationKilled, f.MutationTotal, survived))
		}
		sb.WriteString("\n</details>\n\n")
	}

	// Warnings
	if r.TotalGenerated > 0 && r.TotalValidated == 0 {
		sb.WriteString("> ⚠️ **No tests passed validation.** Consider using a more capable model or reviewing the target code.\n\n")
	}

	sb.WriteString("---\n*Generated by [testgen-agent](https://github.com/Klagvar/testgen-agent)*\n")

	return sb.String()
}

type commentRequest struct {
	Body string `json:"body"`
}

type commentResponse struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

// findBotComment searches for an existing testgen-agent comment on the PR.
func (c *Commenter) findBotComment() (int, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=100",
		c.apiBase, c.owner, c.repo, c.prNum)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("list comments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, nil
	}

	var comments []commentResponse
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return 0, nil
	}

	for _, comment := range comments {
		if strings.Contains(comment.Body, botMarker) {
			return comment.ID, nil
		}
	}

	return 0, nil
}

// updateComment updates an existing PR comment by ID.
func (c *Commenter) updateComment(commentID int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d",
		c.apiBase, c.owner, c.repo, commentID)

	payload, err := json.Marshal(commentRequest{Body: body})
	if err != nil {
		return fmt.Errorf("marshal comment: %w", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("update comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// postComment sends a new comment to a PR via the GitHub API.
func (c *Commenter) postComment(body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.apiBase, c.owner, c.repo, c.prNum)

	payload, err := json.Marshal(commentRequest{Body: body})
	if err != nil {
		return fmt.Errorf("marshal comment: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// FormatReportMarkdown exports formatReport for testing.
func FormatReportMarkdown(r Report) string {
	return formatReport(r)
}
