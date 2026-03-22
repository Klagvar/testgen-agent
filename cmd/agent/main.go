package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
	"github.com/gizatulin/testgen-agent/internal/cache"
	"github.com/gizatulin/testgen-agent/internal/config"
	"github.com/gizatulin/testgen-agent/internal/coverage"
	"github.com/gizatulin/testgen-agent/internal/dedup"
	"github.com/gizatulin/testgen-agent/internal/diff"
	ghub "github.com/gizatulin/testgen-agent/internal/github"
	"github.com/gizatulin/testgen-agent/internal/llm"
	"github.com/gizatulin/testgen-agent/internal/merger"
	"github.com/gizatulin/testgen-agent/internal/prompt"
	"github.com/gizatulin/testgen-agent/internal/report"
	"github.com/gizatulin/testgen-agent/internal/validator"
)

const (
	maxRetries         = 3   // max retry attempts for fixing compilation/test errors
	maxCoverageRetries = 2   // max coverage re-generation iterations
	coverageThreshold  = 80.0 // minimum diff coverage (%)
)

func main() {
	// Load .env file (before flag parsing so env vars are available for defaults)
	if err := config.LoadEnvFile(".env"); err != nil {
		fmt.Printf("⚠️  .env: %v\n", err)
	}

	// CLI flags
	repoPath := flag.String("repo", ".", "Path to Git repository")
	baseBranch := flag.String("base", "main", "Base branch for comparison")
	apiKey := flag.String("api-key", "", "LLM API key (or TESTGEN_API_KEY env)")
	baseURL := flag.String("api-url", "", "LLM API URL (default: OpenAI)")
	model := flag.String("model", "", "LLM model (default: gpt-4o-mini)")
	outDir := flag.String("out", "", "Output directory for tests (default: next to source)")
	dryRun := flag.Bool("dry-run", false, "Preview prompt without calling LLM")
	noValidate := flag.Bool("no-validate", false, "Skip test validation")
	coverageTarget := flag.Float64("coverage", coverageThreshold, "Target diff coverage (%)")
	noCoverage := flag.Bool("no-coverage", false, "Skip diff coverage analysis")
	ghToken := flag.String("github-token", "", "GitHub token for PR comments (or GITHUB_TOKEN env)")
	ghRepo := flag.String("github-repo", "", "GitHub repo (owner/repo)")
	prNumber := flag.Int("pr-number", 0, "Pull request number for comment")
	mutationTest := flag.Bool("mutation", false, "Run mutation testing after test generation")
	noCache := flag.Bool("no-cache", false, "Disable function-level caching")
	noSmartDiff := flag.Bool("no-smart-diff", false, "Disable git-based function comparison")
	raceDetection := flag.Bool("race", false, "Enable race detection for concurrent tests")
	reportFormat := flag.String("report", "", "Generate report: html, json (empty = no report)")

	flag.Parse()

	// Support positional args for backward compatibility
	if flag.NArg() > 0 && *repoPath == "." {
		*repoPath = flag.Arg(0)
	}
	if flag.NArg() > 1 && *baseBranch == "main" {
		*baseBranch = flag.Arg(1)
	}

	// ─── Load project config (.testgen.yml) ───
	projectCfg, cfgErr := config.Load(*repoPath)
	if cfgErr != nil {
		fmt.Printf("⚠️  Config: %v\n", cfgErr)
	}
	if errs := projectCfg.Validate(); len(errs) > 0 {
		for _, e := range errs {
			fmt.Printf("⚠️  Config error: %s\n", e)
		}
	}

	// Apply config defaults (CLI flags take priority)
	applyConfigDefaults(projectCfg, model, baseURL, apiKey, outDir,
		coverageTarget, noValidate, noCoverage, noCache, noSmartDiff, mutationTest, raceDetection)

	fmt.Printf("📂 Repository: %s\n", *repoPath)
	fmt.Printf("🔀 Base branch: %s\n\n", *baseBranch)

	// ─── Step 1: Get diff ───
	diffOutput, err := gitDiff(*repoPath, *baseBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ git diff error: %v\n", err)
		os.Exit(1)
	}

	if len(strings.TrimSpace(diffOutput)) == 0 {
		fmt.Println("✅ No changes (diff is empty)")
		return
	}

	// ─── Step 2: Parse diff ───
	files, err := diff.Parse(diffOutput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Diff parse error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📝 Changed files: %d\n\n", len(files))

	totalAttempted := 0
	totalGenerated := 0
	totalValidated := 0
	totalCached := 0
	startTime := time.Now()

	var fileReports []ghub.FileReport

	// Load function-level cache
	var fnCache *cache.Cache
	if !*noCache {
		fnCache = cache.Load(*repoPath)
		total, _ := fnCache.Stats()
		if total > 0 {
			fmt.Printf("📦 Cache loaded: %d entries\n\n", total)
		}
	}

	// ─── Step 3: For each .go file — AST analysis + test generation ───
	opts := pipelineOpts{
		RepoPath:       *repoPath,
		BaseBranch:     *baseBranch,
		OutDir:         *outDir,
		APIKey:         *apiKey,
		BaseURL:        *baseURL,
		Model:          *model,
		DryRun:         *dryRun,
		NoValidate:     *noValidate,
		NoCoverage:     *noCoverage,
		NoSmartDiff:    *noSmartDiff,
		RaceDetection:  *raceDetection,
		MutationTest:   *mutationTest,
		CoverageTarget: *coverageTarget,
		ProjectCfg:     projectCfg,
		FnCache:        fnCache,
	}

	for _, f := range files {
		res := processFile(f, opts)
		if res == nil {
			continue
		}
		if res.Attempted {
			totalAttempted++
		}
		totalGenerated += res.Generated
		totalValidated += res.Validated
		totalCached += res.Cached
		fileReports = append(fileReports, res.Report)
	}

	// ─── Save cache ───
	if fnCache != nil {
		if err := fnCache.Save(); err != nil {
			fmt.Printf("⚠️  Cannot save cache: %v\n", err)
		} else {
			total, _ := fnCache.Stats()
			fmt.Printf("📦 Cache saved: %d entries\n", total)
		}
	}

	// Summary
	fmt.Println("═══════════════════════════════════")
	fmt.Printf("📊 Total: generated %d, validated %d", totalGenerated, totalValidated)
	if totalCached > 0 {
		fmt.Printf(", cached %d", totalCached)
	}
	fmt.Println()

	// ─── Post PR Comment ───
	ghTokenVal := resolveEnv(*ghToken, "GITHUB_TOKEN")
	ghRepoVal := resolveEnv(*ghRepo, "GITHUB_REPOSITORY")
	prNum := resolvePRNumber(*prNumber)

	tokenPreview := "(empty)"
	if ghTokenVal != "" {
		tokenPreview = ghTokenVal[:min(8, len(ghTokenVal))] + "***"
	}
	fmt.Printf("🔑 GitHub: token=%s repo=%q pr=#%d\n", tokenPreview, ghRepoVal, prNum)

	if ghTokenVal == "" || ghRepoVal == "" || prNum == 0 {
		var missing []string
		if ghTokenVal == "" {
			missing = append(missing, "--github-token or GITHUB_TOKEN")
		}
		if ghRepoVal == "" {
			missing = append(missing, "--github-repo or GITHUB_REPOSITORY")
		}
		if prNum == 0 {
			missing = append(missing, "--pr-number or TESTGEN_PR_NUMBER")
		}
		fmt.Printf("ℹ️  PR comment skipped (missing: %s)\n", strings.Join(missing, ", "))
	} else {
		parts := strings.SplitN(ghRepoVal, "/", 2)
		if len(parts) != 2 {
			fmt.Printf("⚠️  Invalid --github-repo format %q (expected owner/repo)\n", ghRepoVal)
		} else {
			modelName := *model
			if modelName == "" {
				modelName = "gpt-4o-mini"
			}

			ghReport := ghub.Report{
				Files:          fileReports,
				TotalGenerated: totalGenerated,
				TotalValidated: totalValidated,
				TotalCached:    totalCached,
				CoverageTarget: *coverageTarget,
				Model:          modelName,
				Duration:       time.Since(startTime),
				BaseBranch:     *baseBranch,
			}

			// Calculate average diff coverage
			var totalCov float64
			covCount := 0
			for _, fr := range fileReports {
				if fr.DiffCoverage > 0 {
					totalCov += fr.DiffCoverage
					covCount++
				}
			}
			if covCount > 0 {
				ghReport.TotalDiffCov = totalCov / float64(covCount)
			}

			commenter := ghub.NewCommenter(ghTokenVal, parts[0], parts[1], prNum)
			if err := commenter.PostReport(ghReport); err != nil {
				fmt.Printf("⚠️  Failed to post PR comment: %v\n", err)
			} else {
				fmt.Printf("💬 Report posted to PR #%d\n", prNum)
			}
		}
	}

	// ─── Generate report ───
	reportFmt := *reportFormat
	if reportFmt == "" && projectCfg.ReportFormat != "" && projectCfg.ReportFormat != "text" {
		reportFmt = projectCfg.ReportFormat
	}

	if reportFmt == "html" {
		modelName := *model
		if modelName == "" {
			modelName = "gpt-4o-mini"
		}

		reportData := report.ReportData{
			ProjectName:    filepath.Base(*repoPath),
			Branch:         *baseBranch,
			Model:          modelName,
			Timestamp:      time.Now(),
			Duration:       time.Since(startTime),
			TotalGenerated: totalGenerated,
			TotalValidated: totalValidated,
			TotalCached:    totalCached,
		}

		// Convert fileReports to report.FileResult
		for _, fr := range fileReports {
			reportData.Files = append(reportData.Files, report.FileResult{
				File:         fr.File,
				Functions:    fr.Functions,
				TestsTotal:   fr.TestsTotal,
				TestsPassed:  fr.TestsPassed,
				DiffCoverage: fr.DiffCoverage,
				Status:       fr.Status,
			})
		}

		// Compute average diff coverage
		var totalCov float64
		covN := 0
		for _, fr := range reportData.Files {
			if fr.DiffCoverage > 0 {
				totalCov += fr.DiffCoverage
				covN++
			}
		}
		if covN > 0 {
			reportData.TotalDiffCov = totalCov / float64(covN)
		}

		reportPath, reportErr := report.GenerateHTML(reportData, *repoPath)
		if reportErr != nil {
			fmt.Printf("⚠️  Report generation failed: %v\n", reportErr)
		} else {
			fmt.Printf("📄 HTML report: %s\n", reportPath)
		}
	}

	if totalAttempted > 0 && totalValidated == 0 {
		os.Exit(2)
	}
	if totalGenerated > totalValidated {
		os.Exit(1)
	}
}

// runCoverageLoop runs iterative test re-generation based on diff coverage.
// Returns the final diff coverage %.
func runCoverageLoop(
	client *llm.Client,
	cfg llm.Config,
	baseReq prompt.TestGenRequest,
	testFilePath string,
	sourceFilePath string,
	repoPath string,
	changedLines []int,
	affectedFuncs []analyzer.FuncInfo,
	target float64,
) float64 {
	// Determine module root and package directory
	pkgDir := filepath.Dir(sourceFilePath)
	moduleRoot := findModuleRoot(pkgDir)

	if moduleRoot == "" {
		fmt.Printf("     ⚠️  Cannot find go.mod for coverage analysis\n")
		return 0
	}

	var lastCoverage float64

	for iter := 1; iter <= maxCoverageRetries; iter++ {
		fmt.Printf("\n     📊 Coverage analysis (iteration %d/%d)...\n", iter, maxCoverageRetries)

		// Run go test -coverprofile
		coverFile, testOutput, err := coverage.RunCoverage(moduleRoot, pkgDir)
		if err != nil {
			fmt.Printf("     ⚠️  Coverage run failed: %v\n", err)
			if testOutput != "" {
				fmt.Printf("     📋 Output: %s\n", truncate(testOutput, 200))
			}
			return lastCoverage
		}

		// Parse coverage profile
		profileData, err := os.ReadFile(coverFile)
		os.Remove(coverFile)
		if err != nil {
			fmt.Printf("     ⚠️  Cannot read coverage profile: %v\n", err)
			return lastCoverage
		}

		blocks, err := coverage.ParseProfile(string(profileData))
		if err != nil {
			fmt.Printf("     ⚠️  Cannot parse coverage profile: %v\n", err)
			return lastCoverage
		}

		// Calculate diff coverage
		sourceFile := filepath.Base(sourceFilePath)
		dcResult := coverage.CalculateDiffCoverage(sourceFile, changedLines, blocks)
		lastCoverage = dcResult.Coverage

		fmt.Printf("     📈 Diff coverage: %.1f%% (%d/%d changed lines covered)\n",
			dcResult.Coverage, len(dcResult.CoveredLines), len(dcResult.ChangedLines))

		if dcResult.Coverage >= target {
			fmt.Printf("     ✅ Coverage target reached (%.1f%% >= %.1f%%)\n", dcResult.Coverage, target)
			return lastCoverage
		}

		fmt.Printf("     📉 Below target (%.1f%% < %.1f%%), uncovered lines: %v\n",
			dcResult.Coverage, target, dcResult.UncoveredLines)

		if len(dcResult.UncoveredLines) == 0 {
			fmt.Printf("     ℹ️  No specific uncovered lines to target\n")
			return lastCoverage
		}

		// Read current test file
		currentTests, err := os.ReadFile(testFilePath)
		if err != nil {
			fmt.Printf("     ⚠️  Cannot read test file: %v\n", err)
			return lastCoverage
		}

		// Build coverage gap prompt
		gapReq := prompt.CoverageGapRequest{
			TestGenRequest:   baseReq,
			ExistingTestCode: string(currentTests),
			UncoveredLines:   dcResult.UncoveredLines,
			CurrentCoverage:  dcResult.Coverage,
			Iteration:        iter,
		}

		gapMessages := prompt.BuildCoverageGapMessages(gapReq)

		fmt.Printf("     🤖 Generating additional tests for uncovered lines...\n")

		result, err := client.Generate(gapMessages)
		if err != nil {
			fmt.Printf("     ❌ LLM error during coverage iteration: %v\n", err)
			return lastCoverage
		}

		fmt.Printf("     ✅ Generated (%d prompt + %d completion tokens)\n",
			result.PromptTokens, result.CompletionTokens)

		newCode := result.Content

		// Merge new tests with existing ones
		mergeResult, mergeErr := merger.Merge(string(currentTests), newCode)
		if mergeErr != nil {
			fmt.Printf("     ⚠️  AST merge failed in coverage loop, using raw output: %v\n", mergeErr)
		} else {
			newCode = mergeResult.Code
			if len(mergeResult.Added) > 0 {
				fmt.Printf("     🔀 Coverage merge: +%d new funcs\n", len(mergeResult.Added))
			}
		}

		// Deduplicate
		dedupResult, dedupErr := dedup.Dedup(newCode)
		if dedupErr == nil && dedupResult.Removed > 0 {
			newCode = dedupResult.Code
			fmt.Printf("     🧹 Coverage dedup: removed %d duplicate(s)\n", dedupResult.Removed)
		}

		// Save and validate
		if err := os.WriteFile(testFilePath, []byte(newCode), 0644); err != nil {
			fmt.Printf("     ❌ Cannot write file: %v\n", err)
			return lastCoverage
		}

		fmt.Printf("     🔬 Validating...\n")
		valResult := validator.Validate(repoPath, testFilePath)

		if !valResult.IsValid() {
			fmt.Printf("     ⚠️  Coverage iteration %d failed validation: %s\n", iter, valResult.Summary())

			// Restore previous version
			if err := os.WriteFile(testFilePath, currentTests, 0644); err != nil {
				fmt.Printf("     ❌ Cannot restore previous test file: %v\n", err)
			} else {
				fmt.Printf("     ↩️  Restored previous test version\n")
			}
			continue
		}

		fmt.Printf("     ✅ Coverage iteration %d: tests validated\n", iter)
	}

	// Final coverage measurement
	fmt.Printf("\n     📊 Final coverage measurement...\n")
	coverFile, _, err := coverage.RunCoverage(moduleRoot, pkgDir)
	if err != nil {
		fmt.Printf("     ⚠️  Final coverage run failed: %v\n", err)
		return lastCoverage
	}
	profileData, err := os.ReadFile(coverFile)
	if err != nil {
		return lastCoverage
	}
	os.Remove(coverFile)

	blocks, err := coverage.ParseProfile(string(profileData))
	if err != nil {
		return lastCoverage
	}

	sourceFile := filepath.Base(sourceFilePath)
	dcResult := coverage.CalculateDiffCoverage(sourceFile, changedLines, blocks)
	lastCoverage = dcResult.Coverage
	fmt.Printf("     📈 Final diff coverage: %.1f%% (%d/%d lines)\n",
		dcResult.Coverage, len(dcResult.CoveredLines), len(dcResult.ChangedLines))
	return lastCoverage
}

// findModuleRoot searches for go.mod up the directory tree.
func findModuleRoot(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	for {
		if _, err := os.Stat(filepath.Join(absDir, "go.mod")); err == nil {
			return absDir
		}
		parent := filepath.Dir(absDir)
		if parent == absDir {
			return ""
		}
		absDir = parent
	}
}

// truncate trims a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// gitDiff retrieves the diff from the git repository.
func gitDiff(repoPath, baseBranch string) (string, error) {
	cmd := exec.Command("git", "diff", baseBranch)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("git", "diff")
		cmd.Dir = repoPath
		output, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return string(output), nil
}

// readExistingTests attempts to read an existing test file.
func readExistingTests(goFilePath string) string {
	ext := filepath.Ext(goFilePath)
	testPath := strings.TrimSuffix(goFilePath, ext) + "_test" + ext

	data, err := os.ReadFile(testPath)
	if err != nil {
		return ""
	}
	return string(data)
}

// buildTestFilePath determines the output path for the test file.
func buildTestFilePath(goFilePath, outDir string) string {
	ext := filepath.Ext(goFilePath)
	base := strings.TrimSuffix(filepath.Base(goFilePath), ext)
	testFileName := base + "_test" + ext

	if outDir != "" {
		return filepath.Join(outDir, testFileName)
	}

	return filepath.Join(filepath.Dir(goFilePath), testFileName)
}

// updateCache updates the cache for successfully generated functions.
func updateCache(fnCache *cache.Cache, filePath string, funcs []analyzer.FuncInfo, usedTypes []analyzer.TypeInfo, testFile string, model string) {
	if fnCache == nil {
		return
	}

	for _, fn := range funcs {
		key := cache.Key(filePath, fn.Name)
		hash := cache.ComputeHash(fn, usedTypes)

		fnCache.Put(key, cache.FuncEntry{
			Hash:           hash,
			TestFile:       filepath.Base(testFile),
			GeneratedFuncs: []string{"Test" + fn.Name}, // simplified
			Model:          model,
			Timestamp:      time.Now(),
		})
	}
}

// resolveEnv returns flagVal if non-empty, otherwise os.Getenv(envKey).
func resolveEnv(flagVal, envKey string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envKey)
}

// resolvePRNumber extracts the PR number from a flag or environment variables.
func resolvePRNumber(flagVal int) int {
	if flagVal > 0 {
		return flagVal
	}
	// GitHub Actions: GITHUB_EVENT_PATH contains JSON with the PR number
	// Simpler via env var passed by the workflow
	prStr := os.Getenv("TESTGEN_PR_NUMBER")
	if prStr != "" {
		var n int
		fmt.Sscanf(prStr, "%d", &n)
		return n
	}
	return 0
}

// buildLLMConfig builds the LLM client configuration from CLI flags and env.
func buildLLMConfig(apiKey, baseURL, model string) llm.Config {
	cfg := llm.DefaultConfig()

	if apiKey != "" {
		cfg.APIKey = apiKey
	} else if envKey := os.Getenv("TESTGEN_API_KEY"); envKey != "" {
		cfg.APIKey = envKey
	}

	if baseURL != "" {
		cfg.BaseURL = baseURL
	} else if envURL := os.Getenv("TESTGEN_API_URL"); envURL != "" {
		cfg.BaseURL = envURL
	}

	if model != "" {
		cfg.Model = model
	} else if envModel := os.Getenv("TESTGEN_MODEL"); envModel != "" {
		cfg.Model = envModel
	}

	return cfg
}

// applyConfigDefaults applies values from .testgen.yml for CLI flags
// that were not set by the user. CLI flags take priority.
func applyConfigDefaults(
	cfg *config.Config,
	model, baseURL, apiKey, outDir *string,
	coverageTarget *float64,
	noValidate, noCoverage, noCache, noSmartDiff, mutationTest, raceDetection *bool,
) {
	// Only if the flag is not set — use config value
	if *model == "" && cfg.Model != "" {
		*model = cfg.Model
	}
	if *baseURL == "" && cfg.APIURL != "" && cfg.APIURL != "https://api.openai.com/v1" {
		*baseURL = cfg.APIURL
	}
	if *apiKey == "" && cfg.APIKey != "" {
		*apiKey = cfg.APIKey
	}
	if *outDir == "" && cfg.OutDir != "" {
		*outDir = cfg.OutDir
	}

	// Boolean flags: config enables them (but CLI --no-* can override)
	if cfg.Mutation && !*mutationTest {
		*mutationTest = true
	}
	if cfg.NoValidate && !*noValidate {
		*noValidate = true
	}
	if cfg.NoCoverage && !*noCoverage {
		*noCoverage = true
	}
	if cfg.NoCache && !*noCache {
		*noCache = true
	}
	if cfg.NoSmartDiff && !*noSmartDiff {
		*noSmartDiff = true
	}
	if cfg.Race && !*raceDetection {
		*raceDetection = true
	}

	// Coverage threshold: use config if CLI uses default value
	if *coverageTarget == coverageThreshold && cfg.CoverageThreshold > 0 {
		*coverageTarget = cfg.CoverageThreshold
	}
}
