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
	"github.com/gizatulin/testgen-agent/internal/coverage"
	"github.com/gizatulin/testgen-agent/internal/diff"
	ghub "github.com/gizatulin/testgen-agent/internal/github"
	"github.com/gizatulin/testgen-agent/internal/llm"
	"github.com/gizatulin/testgen-agent/internal/merger"
	"github.com/gizatulin/testgen-agent/internal/mutation"
	"github.com/gizatulin/testgen-agent/internal/prompt"
	"github.com/gizatulin/testgen-agent/internal/pruner"
	"github.com/gizatulin/testgen-agent/internal/validator"
)

const (
	maxRetries         = 3   // максимум попыток исправления ошибок компиляции/тестов
	maxCoverageRetries = 2   // максимум итераций догенерации по coverage
	coverageThreshold  = 80.0 // минимальный diff coverage (%)
)

func main() {
	// CLI-флаги
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

	flag.Parse()

	// Поддержка позиционных аргументов для обратной совместимости
	if flag.NArg() > 0 && *repoPath == "." {
		*repoPath = flag.Arg(0)
	}
	if flag.NArg() > 1 && *baseBranch == "main" {
		*baseBranch = flag.Arg(1)
	}

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
	for _, f := range files {
		changedLines := f.ChangedLines()
		fmt.Printf("  📄 %s\n", f.NewPath)
		fmt.Printf("     Hunks: %d, Changed lines: %d\n", len(f.Hunks), len(changedLines))

		if !strings.HasSuffix(f.NewPath, ".go") || strings.HasSuffix(f.NewPath, "_test.go") {
			fmt.Printf("     ⏭️  Skipped (not .go or is _test.go)\n\n")
			continue
		}

		fullPath := filepath.Join(*repoPath, f.NewPath)

		// AST analysis — single file
		analysis, err := analyzer.AnalyzeFile(fullPath)
		if err != nil {
			fmt.Printf("     ⚠️  AST analysis failed: %v\n\n", err)
			continue
		}

		// Package-level analysis (types, cross-file functions)
		pkgDir := filepath.Dir(fullPath)
		pkgAnalysis, pkgErr := analyzer.AnalyzePackage(pkgDir)

		affectedFuncs := analyzer.FindFunctionsByLines(analysis, changedLines)
		if len(affectedFuncs) == 0 {
			fmt.Printf("     ℹ️  Changes outside functions\n\n")
			continue
		}

		fmt.Printf("     🔍 Affected functions (%d):\n", len(affectedFuncs))
		for _, fn := range affectedFuncs {
			fmt.Printf("        • %s  (lines %d–%d)\n", fn.Signature, fn.StartLine, fn.EndLine)
		}
		totalAttempted++

		// Collect used types and called functions
		var usedTypes []analyzer.TypeInfo
		var calledFuncs []analyzer.FuncInfo

		if pkgErr == nil && pkgAnalysis != nil {
			// Collect all types used by any affected function
			seenTypes := make(map[string]bool)
			for _, fn := range affectedFuncs {
				for _, ti := range analyzer.FindUsedTypes(fn, pkgAnalysis.AllTypes) {
					if !seenTypes[ti.Name] {
						usedTypes = append(usedTypes, ti)
						seenTypes[ti.Name] = true
					}
				}
			}

			// Collect cross-file called functions
			seenFuncs := make(map[string]bool)
			for _, fn := range affectedFuncs {
				for _, called := range analyzer.FindCalledFunctions(fn, pkgAnalysis) {
					if !seenFuncs[called.Name] {
						calledFuncs = append(calledFuncs, called)
						seenFuncs[called.Name] = true
					}
				}
			}

			if len(usedTypes) > 0 {
				typeNames := make([]string, len(usedTypes))
				for i, t := range usedTypes {
					typeNames[i] = t.Name
				}
				fmt.Printf("     📦 Types: %s\n", strings.Join(typeNames, ", "))
			}
			if len(calledFuncs) > 0 {
				funcNames := make([]string, len(calledFuncs))
				for i, f := range calledFuncs {
					funcNames[i] = f.Name
				}
				fmt.Printf("     🔗 Dependencies: %s\n", strings.Join(funcNames, ", "))
			}
		}

		// ─── Cache check: skip functions with matching hash ───
		if fnCache != nil {
			var uncachedFuncs []analyzer.FuncInfo
			for _, fn := range affectedFuncs {
				key := cache.Key(f.NewPath, fn.Name)
				hash := cache.ComputeHash(fn, usedTypes)

				if entry, hit := fnCache.Lookup(key, hash); hit {
					fmt.Printf("     ♻️  Cached: %s (tests: %s, model: %s)\n",
						fn.Name, strings.Join(entry.GeneratedFuncs, ", "), entry.Model)
					totalCached++
				} else {
					uncachedFuncs = append(uncachedFuncs, fn)
				}
			}

			if len(uncachedFuncs) == 0 {
				fmt.Printf("     ✅ All functions cached — skipping LLM call\n\n")
				continue
			}

			if len(uncachedFuncs) < len(affectedFuncs) {
				fmt.Printf("     📝 %d/%d functions need generation\n",
					len(uncachedFuncs), len(affectedFuncs))
			}

			affectedFuncs = uncachedFuncs
		}

		// Check for existing tests
		existingTests := readExistingTests(fullPath)

		// Build prompt
		req := prompt.TestGenRequest{
			PackageName:   analysis.Package,
			FilePath:      f.NewPath,
			Imports:       analysis.Imports,
			TargetFuncs:   affectedFuncs,
			ExistingTests: existingTests,
			UsedTypes:     usedTypes,
			CalledFuncs:   calledFuncs,
		}

		messages := prompt.BuildMessages(req)

		if *dryRun {
			fmt.Printf("\n     📋 DRY RUN — Prompt:\n")
			fmt.Printf("     ── System (%d chars) ──\n", len(messages[0].Content))
			fmt.Printf("     ── User (%d chars) ──\n", len(messages[1].Content))
			fmt.Println(messages[1].Content)
			fmt.Println()
			continue
		}

		// ─── Step 4: Call LLM ───
		cfg := buildLLMConfig(*apiKey, *baseURL, *model)
		if cfg.APIKey == "" && cfg.BaseURL == "https://api.openai.com/v1" {
			fmt.Printf("     ⚠️  No API key. Use --api-key or TESTGEN_API_KEY env\n")
			fmt.Printf("     💡 Or set --api-url for local model (Ollama)\n\n")
			continue
		}

		client := llm.NewClient(cfg)
		testFilePath := buildTestFilePath(fullPath, *outDir)

		// ─── Generation loop with validation ───
		var generatedCode string
		var lastTestOutput string // полный вывод go test -v для прунера
		success := false

		for attempt := 1; attempt <= maxRetries; attempt++ {
			if attempt == 1 {
				fmt.Printf("     🤖 Generating tests via %s...\n", cfg.Model)
			} else {
				fmt.Printf("     🔄 Attempt %d/%d — fixing errors...\n", attempt, maxRetries)
			}

			var result *llm.GenerateResponse

			if attempt == 1 {
				result, err = client.Generate(messages)
			} else {
				fixMessages := prompt.BuildFixMessages(req, generatedCode, lastValidationError, attempt)
				result, err = client.Generate(fixMessages)
			}

			if err != nil {
				fmt.Printf("     ❌ LLM error: %v\n", err)
				break
			}

			generatedCode = result.Content
			fmt.Printf("     ✅ Generated (%d prompt + %d completion tokens)\n",
				result.PromptTokens, result.CompletionTokens)

			// ─── AST Merge: preserve existing tests ───
			if existingTests != "" {
				mergeResult, mergeErr := merger.Merge(existingTests, generatedCode)
				if mergeErr != nil {
					fmt.Printf("     ⚠️  AST merge failed, using raw LLM output: %v\n", mergeErr)
				} else {
					generatedCode = mergeResult.Code
					if len(mergeResult.Added) > 0 {
						fmt.Printf("     🔀 Merged: +%d new funcs", len(mergeResult.Added))
						if len(mergeResult.Skipped) > 0 {
							fmt.Printf(", %d existing preserved", len(mergeResult.Skipped))
						}
						fmt.Println()
					}
				}
			}

			// Save file
			if err := os.MkdirAll(filepath.Dir(testFilePath), 0755); err != nil {
				fmt.Printf("     ❌ Cannot create directory: %v\n", err)
				break
			}

			if err := os.WriteFile(testFilePath, []byte(generatedCode), 0644); err != nil {
				fmt.Printf("     ❌ Cannot write file: %v\n", err)
				break
			}

			// ─── Step 5: Validation ───
			if *noValidate {
				fmt.Printf("     💾 Tests saved: %s (validation disabled)\n\n", testFilePath)
				totalGenerated++
				success = true
				break
			}

			fmt.Printf("     🔬 Validating...\n")
			valResult := validator.Validate(*repoPath, testFilePath)

			if valResult.IsValid() {
				fmt.Printf("     %s\n", valResult.Summary())
				fmt.Printf("     💾 Tests saved: %s\n\n", testFilePath)
				totalGenerated++
				totalValidated++
				success = true
				updateCache(fnCache, f.NewPath, affectedFuncs, usedTypes, testFilePath, cfg.Model)
				break
			}

			// Validation failed
			lastValidationError = valResult.Summary()
			lastTestOutput = valResult.TestOutput
			fmt.Printf("     %s\n", valResult.Summary())

			if attempt == maxRetries {
				fmt.Printf("     ⛔ Max retries reached (%d)\n", maxRetries)
			}
		}

		// If validation failed — try pruning failing tests
		if !success {
			if generatedCode != "" && lastTestOutput != "" {
				fmt.Printf("     ✂️  Pruning failing tests...\n")
				pruneResult, pruneErr := pruner.Prune(generatedCode, lastTestOutput)

				if pruneErr != nil {
					fmt.Printf("     ⚠️  Prune failed: %v\n", pruneErr)
					os.Remove(testFilePath)
					fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
				} else if pruneResult.KeptTests == 0 {
					fmt.Printf("     ⚠️  All tests failed, nothing to keep\n")
					os.Remove(testFilePath)
					fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
				} else {
					fmt.Printf("     ✂️  Removed %d functions, %d sub-test cases. Kept %d test functions.\n",
						len(pruneResult.RemovedFuncs), pruneResult.RemovedSubTests, pruneResult.KeptTests)

					if len(pruneResult.RemovedFuncs) > 0 {
						fmt.Printf("     🗑️  Removed: %s\n", strings.Join(pruneResult.RemovedFuncs, ", "))
					}

					// Save pruned code and re-validate
					if err := os.WriteFile(testFilePath, []byte(pruneResult.Code), 0644); err != nil {
						fmt.Printf("     ❌ Cannot write pruned file: %v\n", err)
						os.Remove(testFilePath)
					} else {
						fmt.Printf("     🔬 Re-validating pruned tests...\n")
						valResult := validator.Validate(*repoPath, testFilePath)

						if valResult.IsValid() {
							fmt.Printf("     %s\n", valResult.Summary())
							fmt.Printf("     💾 Pruned tests saved: %s\n\n", testFilePath)
							totalGenerated++
							totalValidated++
							success = true
							updateCache(fnCache, f.NewPath, affectedFuncs, usedTypes, testFilePath, cfg.Model)
						} else {
							fmt.Printf("     ⚠️  Pruned tests still fail: %s\n", valResult.Summary())
							os.Remove(testFilePath)
							fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
						}
					}
				}
			} else if generatedCode == "" {
				fmt.Printf("     ❌ Failed to generate tests\n\n")
			} else {
				os.Remove(testFilePath)
				fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
			}

			if !success {
				continue
			}
		}

		// ─── Step 6: Diff Coverage Analysis ───
		var fileDiffCov float64
		if !*noValidate && !*noCoverage && !*dryRun {
			fileDiffCov = runCoverageLoop(
				client, cfg, req, testFilePath, fullPath, *repoPath,
				changedLines, affectedFuncs, *coverageTarget,
			)
		}

		// ─── Step 7: Mutation Testing (optional) ───
		if *mutationTest && success && !*dryRun {
			fmt.Printf("     🧬 Running mutation testing...\n")
			funcNamesForMut := make([]string, len(affectedFuncs))
			for i, fn := range affectedFuncs {
				funcNamesForMut[i] = fn.Name
			}

			moduleRoot := findModuleRoot(filepath.Dir(fullPath))
			if moduleRoot != "" {
				mutResult, mutErr := mutation.RunMutationTests(fullPath, funcNamesForMut, moduleRoot)
				if mutErr != nil {
					fmt.Printf("     ⚠️  Mutation testing failed: %v\n", mutErr)
				} else {
					fmt.Printf("     🧬 Mutation Score: %.1f%% (%d killed / %d total, %d survived)\n",
						mutResult.MutationScore, mutResult.Killed, mutResult.Total, mutResult.Survived)
					if mutResult.Survived > 0 {
						for _, m := range mutResult.Mutants {
							if !m.Killed && m.Error == "" {
								fmt.Printf("        ⚠️  Survived: %s:%d  %s → %s (func %s)\n",
									filepath.Base(m.File), m.Line, m.Original, m.Replacement, m.FuncName)
							}
						}
					}
				}
			}
		}

		// Collect file report
		funcNames := make([]string, len(affectedFuncs))
		for i, fn := range affectedFuncs {
			funcNames[i] = fn.Name
		}

		status := "failed"
		if success {
			status = "success"
			if fileDiffCov < *coverageTarget && fileDiffCov > 0 {
				status = "partial"
			}
		}

		fileReports = append(fileReports, ghub.FileReport{
			File:         f.NewPath,
			Functions:    funcNames,
			TestsTotal:   totalGenerated,
			TestsPassed:  totalValidated,
			DiffCoverage: fileDiffCov,
			Status:       status,
		})
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

	if ghTokenVal != "" && ghRepoVal != "" && prNum > 0 {
		parts := strings.SplitN(ghRepoVal, "/", 2)
		if len(parts) == 2 {
			modelName := *model
			if modelName == "" {
				modelName = "gpt-4o-mini"
			}

			report := ghub.Report{
				Files:          fileReports,
				TotalGenerated: totalGenerated,
				TotalValidated: totalValidated,
				Model:          modelName,
				Duration:       time.Since(startTime),
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
				report.TotalDiffCov = totalCov / float64(covCount)
			}

			commenter := ghub.NewCommenter(ghTokenVal, parts[0], parts[1], prNum)
			if err := commenter.PostReport(report); err != nil {
				fmt.Printf("⚠️  Failed to post PR comment: %v\n", err)
			} else {
				fmt.Printf("💬 Report posted to PR #%d\n", prNum)
			}
		}
	}

	if totalAttempted > 0 && totalValidated == 0 {
		os.Exit(2)
	}
	if totalGenerated > totalValidated {
		os.Exit(1)
	}
}

// runCoverageLoop запускает итеративную догенерацию тестов на основе diff coverage.
// Возвращает итоговый diff coverage %.
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
	// Определяем модульный корень и директорию пакета
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
		if err != nil {
			fmt.Printf("     ⚠️  Cannot read coverage profile: %v\n", err)
			return lastCoverage
		}
		defer os.Remove(coverFile)

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

		// Save and validate
		if err := os.WriteFile(testFilePath, []byte(result.Content), 0644); err != nil {
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

// findModuleRoot ищет go.mod вверх по дереву каталогов.
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

// truncate обрезает строку до maxLen символов.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// lastValidationError хранит последнюю ошибку валидации для retry.
var lastValidationError string

// gitDiff получает diff из git-репозитория.
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

// readExistingTests пытается прочитать существующий файл тестов.
func readExistingTests(goFilePath string) string {
	ext := filepath.Ext(goFilePath)
	testPath := strings.TrimSuffix(goFilePath, ext) + "_test" + ext

	data, err := os.ReadFile(testPath)
	if err != nil {
		return ""
	}
	return string(data)
}

// buildTestFilePath определяет путь для файла тестов.
func buildTestFilePath(goFilePath, outDir string) string {
	ext := filepath.Ext(goFilePath)
	base := strings.TrimSuffix(filepath.Base(goFilePath), ext)
	testFileName := base + "_test" + ext

	if outDir != "" {
		return filepath.Join(outDir, testFileName)
	}

	return filepath.Join(filepath.Dir(goFilePath), testFileName)
}

// updateCache обновляет кэш для успешно сгенерированных функций.
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
			GeneratedFuncs: []string{"Test" + fn.Name}, // упрощённо
			Model:          model,
			Timestamp:      time.Now(),
		})
	}
}

// resolveEnv возвращает flagVal если не пусто, иначе os.Getenv(envKey).
func resolveEnv(flagVal, envKey string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envKey)
}

// resolvePRNumber извлекает номер PR из флага или переменных окружения.
func resolvePRNumber(flagVal int) int {
	if flagVal > 0 {
		return flagVal
	}
	// GitHub Actions: GITHUB_EVENT_PATH содержит JSON с номером PR
	// Но проще через env var, которую workflow передаёт
	prStr := os.Getenv("TESTGEN_PR_NUMBER")
	if prStr != "" {
		var n int
		fmt.Sscanf(prStr, "%d", &n)
		return n
	}
	return 0
}

// buildLLMConfig формирует конфигурацию LLM-клиента из CLI-флагов и env.
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
