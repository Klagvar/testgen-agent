package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
	"github.com/gizatulin/testgen-agent/internal/coverage"
	"github.com/gizatulin/testgen-agent/internal/diff"
	"github.com/gizatulin/testgen-agent/internal/llm"
	"github.com/gizatulin/testgen-agent/internal/prompt"
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

		// AST analysis
		analysis, err := analyzer.AnalyzeFile(fullPath)
		if err != nil {
			fmt.Printf("     ⚠️  AST analysis failed: %v\n\n", err)
			continue
		}

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

		// Check for existing tests
		existingTests := readExistingTests(fullPath)

		// Build prompt
		req := prompt.TestGenRequest{
			PackageName:   analysis.Package,
			FilePath:      f.NewPath,
			Imports:       analysis.Imports,
			TargetFuncs:   affectedFuncs,
			ExistingTests: existingTests,
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
				break
			}

			// Validation failed
			lastValidationError = valResult.Summary()
			fmt.Printf("     %s\n", valResult.Summary())

			if attempt == maxRetries {
				fmt.Printf("     ⛔ Max retries reached (%d)\n", maxRetries)
			}
		}

		// If validation failed — delete invalid file
		if !success {
			if generatedCode != "" {
				os.Remove(testFilePath)
				fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
			} else {
				fmt.Printf("     ❌ Failed to generate tests\n\n")
			}
			continue
		}

		// ─── Step 6: Diff Coverage Analysis ───
		if *noValidate || *noCoverage || *dryRun {
			continue
		}

		// Запускаем coverage-guided iteration
		runCoverageLoop(
			client, cfg, req, testFilePath, fullPath, *repoPath,
			changedLines, affectedFuncs, *coverageTarget,
		)
	}

	// Summary
	fmt.Println("═══════════════════════════════════")
	fmt.Printf("📊 Total: generated %d, validated %d\n", totalGenerated, totalValidated)

	if totalAttempted > 0 && totalValidated == 0 {
		os.Exit(2)
	}
	if totalGenerated > totalValidated {
		os.Exit(1)
	}
}

// runCoverageLoop запускает итеративную догенерацию тестов на основе diff coverage.
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
) {
	// Определяем модульный корень и директорию пакета
	pkgDir := filepath.Dir(sourceFilePath)
	moduleRoot := findModuleRoot(pkgDir)

	if moduleRoot == "" {
		fmt.Printf("     ⚠️  Cannot find go.mod for coverage analysis\n")
		return
	}

	for iter := 1; iter <= maxCoverageRetries; iter++ {
		fmt.Printf("\n     📊 Coverage analysis (iteration %d/%d)...\n", iter, maxCoverageRetries)

		// Run go test -coverprofile
		coverFile, testOutput, err := coverage.RunCoverage(moduleRoot, pkgDir)
		if err != nil {
			fmt.Printf("     ⚠️  Coverage run failed: %v\n", err)
			if testOutput != "" {
				fmt.Printf("     📋 Output: %s\n", truncate(testOutput, 200))
			}
			return
		}

		// Parse coverage profile
		profileData, err := os.ReadFile(coverFile)
		if err != nil {
			fmt.Printf("     ⚠️  Cannot read coverage profile: %v\n", err)
			return
		}
		defer os.Remove(coverFile)

		blocks, err := coverage.ParseProfile(string(profileData))
		if err != nil {
			fmt.Printf("     ⚠️  Cannot parse coverage profile: %v\n", err)
			return
		}

		// Calculate diff coverage
		sourceFile := filepath.Base(sourceFilePath)
		dcResult := coverage.CalculateDiffCoverage(sourceFile, changedLines, blocks)

		fmt.Printf("     📈 Diff coverage: %.1f%% (%d/%d changed lines covered)\n",
			dcResult.Coverage, len(dcResult.CoveredLines), len(dcResult.ChangedLines))

		if dcResult.Coverage >= target {
			fmt.Printf("     ✅ Coverage target reached (%.1f%% >= %.1f%%)\n", dcResult.Coverage, target)
			return
		}

		fmt.Printf("     📉 Below target (%.1f%% < %.1f%%), uncovered lines: %v\n",
			dcResult.Coverage, target, dcResult.UncoveredLines)

		if len(dcResult.UncoveredLines) == 0 {
			fmt.Printf("     ℹ️  No specific uncovered lines to target\n")
			return
		}

		// Read current test file
		currentTests, err := os.ReadFile(testFilePath)
		if err != nil {
			fmt.Printf("     ⚠️  Cannot read test file: %v\n", err)
			return
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
			return
		}

		fmt.Printf("     ✅ Generated (%d prompt + %d completion tokens)\n",
			result.PromptTokens, result.CompletionTokens)

		// Save and validate
		if err := os.WriteFile(testFilePath, []byte(result.Content), 0644); err != nil {
			fmt.Printf("     ❌ Cannot write file: %v\n", err)
			return
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
		return
	}
	profileData, err := os.ReadFile(coverFile)
	if err != nil {
		return
	}
	os.Remove(coverFile)

	blocks, err := coverage.ParseProfile(string(profileData))
	if err != nil {
		return
	}

	sourceFile := filepath.Base(sourceFilePath)
	dcResult := coverage.CalculateDiffCoverage(sourceFile, changedLines, blocks)
	fmt.Printf("     📈 Final diff coverage: %.1f%% (%d/%d lines)\n",
		dcResult.Coverage, len(dcResult.CoveredLines), len(dcResult.ChangedLines))
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
