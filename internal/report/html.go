// Package report generates HTML reports with test generation results.
// The report is a single self-contained HTML file with inline CSS and SVG charts.
package report

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileResult — результат генерации тестов для одного файла.
type FileResult struct {
	File          string
	Functions     []string
	TestsTotal    int
	TestsPassed   int
	TestsPruned   int
	DiffCoverage  float64
	MutationScore float64
	MutantsTotal  int
	MutantsKilled int
	Status        string // "success", "partial", "failed"
	Cached        int    // количество функций из кэша
}

// ReportData — данные для HTML-отчёта.
type ReportData struct {
	ProjectName    string
	Branch         string
	Model          string
	Timestamp      time.Time
	Duration       time.Duration
	Files          []FileResult
	TotalGenerated int
	TotalValidated int
	TotalCached    int
	TotalDiffCov   float64

	// Aggregated mutation data
	MutationEnabled bool
	MutationScore   float64
	MutantsTotal    int
	MutantsKilled   int
}

// GenerateHTML creates an HTML report file and returns its path.
func GenerateHTML(data ReportData, outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = "."
	}

	fileName := fmt.Sprintf("testgen-report-%s.html", data.Timestamp.Format("2006-01-02-150405"))
	filePath := filepath.Join(outputDir, fileName)

	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"statusEmoji": statusEmoji,
		"statusColor": statusColor,
		"barWidth":    barWidth,
		"percent":     func(v float64) string { return fmt.Sprintf("%.1f", v) },
		"join":        func(s []string) string { return strings.Join(s, ", ") },
		"formatDur":   formatDuration,
		"sub":         func(a, b int) int { return a - b },
	}).Parse(htmlTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	if err := tmpl.Execute(f, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return filePath, nil
}

func statusEmoji(s string) string {
	switch s {
	case "success":
		return "✅"
	case "partial":
		return "⚠️"
	case "failed":
		return "❌"
	default:
		return "❓"
	}
}

func statusColor(s string) string {
	switch s {
	case "success":
		return "#22c55e"
	case "partial":
		return "#f59e0b"
	case "failed":
		return "#ef4444"
	default:
		return "#6b7280"
	}
}

func barWidth(val float64) string {
	if val > 100 {
		val = 100
	}
	if val < 0 {
		val = 0
	}
	return fmt.Sprintf("%.1f%%", val)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Testgen Agent Report — {{.ProjectName}}</title>
<style>
  :root {
    --bg: #0f172a;
    --surface: #1e293b;
    --surface2: #334155;
    --text: #f1f5f9;
    --text2: #94a3b8;
    --accent: #3b82f6;
    --green: #22c55e;
    --yellow: #f59e0b;
    --red: #ef4444;
    --border: #475569;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
    background: var(--bg);
    color: var(--text);
    line-height: 1.6;
    padding: 2rem;
  }
  .container { max-width: 1100px; margin: 0 auto; }

  /* Header */
  .header {
    background: linear-gradient(135deg, #1e293b 0%, #0f172a 100%);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 2rem;
    margin-bottom: 1.5rem;
  }
  .header h1 { font-size: 1.8rem; margin-bottom: 0.5rem; }
  .header h1 span { color: var(--accent); }
  .header-meta { display: flex; gap: 2rem; flex-wrap: wrap; color: var(--text2); font-size: 0.9rem; }
  .header-meta span { display: flex; align-items: center; gap: 0.3rem; }

  /* Stats cards */
  .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 1.5rem; }
  .stat-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 1.2rem;
    text-align: center;
  }
  .stat-card .value { font-size: 2rem; font-weight: 700; }
  .stat-card .label { color: var(--text2); font-size: 0.85rem; margin-top: 0.2rem; }
  .stat-card.green .value { color: var(--green); }
  .stat-card.yellow .value { color: var(--yellow); }
  .stat-card.blue .value { color: var(--accent); }

  /* Table */
  .table-wrap {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    overflow: hidden;
    margin-bottom: 1.5rem;
  }
  .table-wrap h2 { padding: 1rem 1.5rem 0.5rem; font-size: 1.2rem; }
  table { width: 100%; border-collapse: collapse; }
  thead { background: var(--surface2); }
  th, td { padding: 0.75rem 1rem; text-align: left; border-bottom: 1px solid var(--border); }
  th { font-weight: 600; color: var(--text2); font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.05em; }
  td { font-size: 0.95rem; }
  tr:last-child td { border-bottom: none; }
  tr:hover { background: rgba(59, 130, 246, 0.05); }

  /* Coverage bar */
  .bar-container {
    width: 100%;
    height: 8px;
    background: var(--surface2);
    border-radius: 4px;
    overflow: hidden;
    margin-top: 0.3rem;
  }
  .bar-fill {
    height: 100%;
    border-radius: 4px;
    transition: width 0.5s ease;
  }
  .bar-fill.green { background: var(--green); }
  .bar-fill.yellow { background: var(--yellow); }
  .bar-fill.red { background: var(--red); }

  /* Chart */
  .chart-wrap {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 1.5rem;
    margin-bottom: 1.5rem;
  }
  .chart-wrap h2 { font-size: 1.2rem; margin-bottom: 1rem; }
  .chart { display: flex; align-items: flex-end; gap: 6px; height: 200px; padding-bottom: 1.5rem; position: relative; }
  .chart-col {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    position: relative;
    height: 100%;
    justify-content: flex-end;
  }
  .chart-bar {
    width: 100%;
    max-width: 60px;
    border-radius: 4px 4px 0 0;
    position: relative;
    min-height: 2px;
    transition: height 0.5s ease;
  }
  .chart-label {
    font-size: 0.7rem;
    color: var(--text2);
    text-align: center;
    position: absolute;
    bottom: -1.2rem;
    left: 50%;
    transform: translateX(-50%);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 80px;
  }
  .chart-value {
    font-size: 0.75rem;
    color: var(--text);
    margin-bottom: 0.2rem;
    font-weight: 600;
  }

  /* Status badge */
  .badge {
    display: inline-block;
    padding: 0.15rem 0.6rem;
    border-radius: 9999px;
    font-size: 0.8rem;
    font-weight: 600;
  }

  /* Functions list */
  .func-list { color: var(--text2); font-size: 0.85rem; }
  .func-list code { background: var(--surface2); padding: 0.1rem 0.4rem; border-radius: 3px; font-size: 0.8rem; }

  /* Footer */
  .footer {
    text-align: center;
    color: var(--text2);
    font-size: 0.8rem;
    padding: 1rem;
    border-top: 1px solid var(--border);
  }
</style>
</head>
<body>
<div class="container">

<!-- Header -->
<div class="header">
  <h1>🤖 <span>Testgen Agent</span> Report</h1>
  <div class="header-meta">
    <span>📂 {{.ProjectName}}</span>
    <span>🔀 {{.Branch}}</span>
    <span>🧠 {{.Model}}</span>
    <span>🕐 {{.Timestamp.Format "2006-01-02 15:04:05"}}</span>
    <span>⏱️ {{formatDur .Duration}}</span>
  </div>
</div>

<!-- Stats -->
<div class="stats">
  <div class="stat-card blue">
    <div class="value">{{.TotalGenerated}}</div>
    <div class="label">Tests Generated</div>
  </div>
  <div class="stat-card green">
    <div class="value">{{.TotalValidated}}</div>
    <div class="label">Tests Validated</div>
  </div>
  <div class="stat-card{{if gt .TotalCached 0}} yellow{{end}}">
    <div class="value">{{.TotalCached}}</div>
    <div class="label">Cached (skipped)</div>
  </div>
  <div class="stat-card{{if ge .TotalDiffCov 80.0}} green{{else if ge .TotalDiffCov 50.0}} yellow{{else}} stat-card{{end}}">
    <div class="value">{{percent .TotalDiffCov}}%</div>
    <div class="label">Avg Diff Coverage</div>
  </div>
  {{if .MutationEnabled}}
  <div class="stat-card{{if ge .MutationScore 70.0}} green{{else if ge .MutationScore 40.0}} yellow{{else}} stat-card{{end}}">
    <div class="value">{{percent .MutationScore}}%</div>
    <div class="label">Mutation Score</div>
  </div>
  {{end}}
</div>

<!-- Coverage Chart -->
{{if gt (len .Files) 0}}
<div class="chart-wrap">
  <h2>📊 Diff Coverage by File</h2>
  <div class="chart">
    {{range .Files}}
    <div class="chart-col">
      <div class="chart-value">{{percent .DiffCoverage}}%</div>
      <div class="chart-bar {{if ge .DiffCoverage 80.0}}green{{else if ge .DiffCoverage 50.0}}yellow{{else}}red{{end}}" style="height: {{barWidth .DiffCoverage}}; background: {{statusColor .Status}};"></div>
      <div class="chart-label" title="{{.File}}">{{.File}}</div>
    </div>
    {{end}}
  </div>
</div>
{{end}}

<!-- File Details Table -->
<div class="table-wrap">
  <h2>📝 File Details</h2>
  <table>
    <thead>
      <tr>
        <th>Status</th>
        <th>File</th>
        <th>Functions</th>
        <th>Tests</th>
        <th>Diff Coverage</th>
        {{if $.MutationEnabled}}<th>Mutation</th>{{end}}
      </tr>
    </thead>
    <tbody>
      {{range .Files}}
      <tr>
        <td>
          <span class="badge" style="background: {{statusColor .Status}}22; color: {{statusColor .Status}};">
            {{statusEmoji .Status}} {{.Status}}
          </span>
        </td>
        <td><code>{{.File}}</code></td>
        <td>
          <div class="func-list">
            {{range .Functions}}<code>{{.}}</code> {{end}}
          </div>
        </td>
        <td>
          {{.TestsPassed}}{{if gt .TestsPruned 0}} <span style="color: var(--yellow);">({{.TestsPruned}} pruned)</span>{{end}}
          {{if gt .Cached 0}}<span style="color: var(--text2);">+{{.Cached}} cached</span>{{end}}
        </td>
        <td>
          <strong>{{percent .DiffCoverage}}%</strong>
          <div class="bar-container">
            <div class="bar-fill {{if ge .DiffCoverage 80.0}}green{{else if ge .DiffCoverage 50.0}}yellow{{else}}red{{end}}" style="width: {{barWidth .DiffCoverage}};"></div>
          </div>
        </td>
        {{if $.MutationEnabled}}
        <td>
          {{if gt .MutantsTotal 0}}
          <strong>{{percent .MutationScore}}%</strong>
          <div style="color: var(--text2); font-size: 0.8rem;">{{.MutantsKilled}}/{{.MutantsTotal}} killed</div>
          {{else}}
          <span style="color: var(--text2);">—</span>
          {{end}}
        </td>
        {{end}}
      </tr>
      {{end}}
    </tbody>
  </table>
</div>

<!-- Footer -->
<div class="footer">
  Generated by <strong>testgen-agent</strong> · {{.Timestamp.Format "Mon Jan 02 15:04:05 MST 2006"}}
</div>

</div>
</body>
</html>`
