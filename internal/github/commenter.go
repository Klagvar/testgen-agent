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
	File              string   // file path
	Functions         []string // tested functions
	TestsTotal        int      // total generated tests
	TestsPassed       int      // passed validation
	TestsPruned       int      // pruned (failing)
	DiffCoverage      float64  // diff coverage %
	BranchCoverage    float64  // branch coverage % within changed functions (−1 if not computed)
	BranchesTotal     int      // total branches discovered
	BranchesCovered   int      // branches whose body executed
	ErrorPathCoverage float64  // error-path coverage % (−1 if not computed)
	ErrorPathsTotal   int      // total `if err != nil` branches
	ErrorPathsCovered int      // covered error paths
	MutationScore     float64  // mutation score % (0 = not run)
	MutationKilled    int      // mutations killed
	MutationTotal     int      // total mutations
	PromptTokens      int      // cumulative prompt tokens consumed for this file
	CompletionTokens  int      // cumulative completion tokens consumed for this file
	TokenEfficiency   float64  // (prompt+completion) / max(TestsPassed,1); 0 if no tokens recorded
	Naturalness       *Naturalness // optional, populated only when computed
	Status            string   // "success", "partial", "failed"
}

// Naturalness mirrors naturalness.Result but avoids cross-package imports
// from the reporting layer. Populated per-file by the pipeline when
// naturalness analysis is enabled.
type Naturalness struct {
	TestCount              int
	AssertionRatio         float64
	NoAssertionsPct        float64
	DuplicateAssertionsPct float64
	NilOnlyAssertionsPct   float64
	ErrorAssertionsPct     float64
	TestNameScore          float64
	VarNameScore           float64
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
	RunID          string // GitHub Actions run ID for artifact links
	RepoFullName   string // owner/repo for building URLs
	CommitSHA      string // trigger commit SHA
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

// PostReport posts a new report as a PR comment.
// Each workflow run creates a separate comment for full history.
func (c *Commenter) PostReport(report Report) error {
	body := formatReport(report)
	return c.postComment(body)
}

// formatReport builds the Markdown report text.
func formatReport(r Report) string {
	var sb strings.Builder

	sb.WriteString(botMarker + "\n")
	sb.WriteString("## 🤖 Testgen Agent Report\n\n")
	if r.CommitSHA != "" {
		short := r.CommitSHA
		if len(short) > 7 {
			short = short[:7]
		}
		if r.RepoFullName != "" {
			sb.WriteString(fmt.Sprintf("> **Commit:** [`%s`](https://github.com/%s/commit/%s)\n", short, r.RepoFullName, r.CommitSHA))
		} else {
			sb.WriteString(fmt.Sprintf("> **Commit:** `%s`\n", short))
		}
	}

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

	// Aggregated quality metrics (branch / error-path / token efficiency).
	bTotal, bCov, ePTotal, ePCov := 0, 0, 0, 0
	totalPromptTok, totalCompTok := 0, 0
	for _, f := range r.Files {
		bTotal += f.BranchesTotal
		bCov += f.BranchesCovered
		ePTotal += f.ErrorPathsTotal
		ePCov += f.ErrorPathsCovered
		totalPromptTok += f.PromptTokens
		totalCompTok += f.CompletionTokens
	}
	if bTotal > 0 {
		pct := float64(bCov) / float64(bTotal) * 100
		sb.WriteString(fmt.Sprintf("| Branch coverage | 🌳 %.1f%% (%d/%d) |\n", pct, bCov, bTotal))
	}
	if ePTotal > 0 {
		pct := float64(ePCov) / float64(ePTotal) * 100
		sb.WriteString(fmt.Sprintf("| Error-path coverage | 🛡️ %.1f%% (%d/%d) |\n", pct, ePCov, ePTotal))
	}
	if r.TotalValidated > 0 && (totalPromptTok+totalCompTok) > 0 {
		eff := float64(totalPromptTok+totalCompTok) / float64(r.TotalValidated)
		sb.WriteString(fmt.Sprintf("| Token efficiency | 🪙 %.0f tokens / passing test |\n", eff))
	}

	sb.WriteString(fmt.Sprintf("| Duration | ⏱️ %s |\n", r.Duration.Round(time.Second)))
	sb.WriteString("\n")

	// Naturalness summary: averaged across files that reported metrics.
	var natAgg struct {
		N     int
		Ratio float64
		NoA   float64
		Dup   float64
		Nil   float64
		Err   float64
		TN    float64
		VN    float64
	}
	for _, f := range r.Files {
		if f.Naturalness == nil || f.Naturalness.TestCount == 0 {
			continue
		}
		natAgg.N++
		natAgg.Ratio += f.Naturalness.AssertionRatio
		natAgg.NoA += f.Naturalness.NoAssertionsPct
		natAgg.Dup += f.Naturalness.DuplicateAssertionsPct
		natAgg.Nil += f.Naturalness.NilOnlyAssertionsPct
		natAgg.Err += f.Naturalness.ErrorAssertionsPct
		natAgg.TN += f.Naturalness.TestNameScore
		natAgg.VN += f.Naturalness.VarNameScore
	}
	if natAgg.N > 0 {
		d := float64(natAgg.N)
		sb.WriteString("### 🧾 Naturalness\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Assertions per test | %.2f |\n", natAgg.Ratio/d))
		sb.WriteString(fmt.Sprintf("| Tests without assertions | %.1f%% |\n", natAgg.NoA/d))
		sb.WriteString(fmt.Sprintf("| Tests with duplicate assertions | %.1f%% |\n", natAgg.Dup/d))
		sb.WriteString(fmt.Sprintf("| Nil-only assertions | %.1f%% |\n", natAgg.Nil/d))
		sb.WriteString(fmt.Sprintf("| Error assertions | %.1f%% |\n", natAgg.Err/d))
		sb.WriteString(fmt.Sprintf("| Test-name closeness | %.1f / 100 |\n", natAgg.TN/d))
		sb.WriteString(fmt.Sprintf("| Variable-name closeness | %.1f / 100 |\n", natAgg.VN/d))
		sb.WriteString("\n")
	}

	// Per-file details (collapsible)
	if len(r.Files) > 0 {
		sb.WriteString("### 📁 File Details\n\n")

		if len(r.Files) > 3 {
			sb.WriteString("<details>\n<summary>Click to expand file details</summary>\n\n")
		}

		sb.WriteString("| File | Functions | Generated | Validated | Diff Cov | Branch Cov | Err-path Cov | Status |\n")
		sb.WriteString("|------|-----------|-----------|-----------|----------|------------|--------------|--------|\n")

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
			branchStr := "—"
			if f.BranchesTotal > 0 {
				branchStr = fmt.Sprintf("%.1f%%", f.BranchCoverage)
			}
			errPathStr := "—"
			if f.ErrorPathsTotal > 0 {
				errPathStr = fmt.Sprintf("%.1f%%", f.ErrorPathCoverage)
			}

			sb.WriteString(fmt.Sprintf("| `%s` | %s | %d | %d | %s | %s | %s | %s |\n",
				f.File, funcs, f.TestsTotal, f.TestsPassed, covStr, branchStr, errPathStr, statusEmoji))
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

	if r.RunID != "" && r.RepoFullName != "" {
		sb.WriteString(fmt.Sprintf(
			"📊 [Full HTML report (workflow artifact)](https://github.com/%s/actions/runs/%s)\n\n",
			r.RepoFullName, r.RunID,
		))
	}

	sb.WriteString("---\n*Generated by [testgen-agent](https://github.com/Klagvar/testgen-agent)*\n")

	return sb.String()
}

type commentRequest struct {
	Body string `json:"body"`
}

// postComment sends a new comment to a PR via the GitHub API with retry.
func (c *Commenter) postComment(body string) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		lastErr = c.doPostComment(body)
		if lastErr == nil {
			return nil
		}
		if attempt < maxRetries {
			backoff := time.Duration(attempt) * 2 * time.Second
			time.Sleep(backoff)
		}
	}
	return lastErr
}

func (c *Commenter) doPostComment(body string) error {
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
