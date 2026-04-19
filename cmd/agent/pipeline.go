package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
	"github.com/gizatulin/testgen-agent/internal/branchcov"
	"github.com/gizatulin/testgen-agent/internal/cache"
	"github.com/gizatulin/testgen-agent/internal/config"
	"github.com/gizatulin/testgen-agent/internal/coverage"
	"github.com/gizatulin/testgen-agent/internal/dedup"
	"github.com/gizatulin/testgen-agent/internal/diff"
	"github.com/gizatulin/testgen-agent/internal/gitdiff"
	ghub "github.com/gizatulin/testgen-agent/internal/github"
	"github.com/gizatulin/testgen-agent/internal/llm"
	"github.com/gizatulin/testgen-agent/internal/merger"
	"github.com/gizatulin/testgen-agent/internal/mockgen"
	"github.com/gizatulin/testgen-agent/internal/mutation"
	"github.com/gizatulin/testgen-agent/internal/patterns"
	"github.com/gizatulin/testgen-agent/internal/prompt"
	"github.com/gizatulin/testgen-agent/internal/pruner"
	"github.com/gizatulin/testgen-agent/internal/typed"
	"github.com/gizatulin/testgen-agent/internal/validator"
)

type pipelineOpts struct {
	RepoPath          string
	BaseBranch        string
	OutDir            string
	APIKey            string
	BaseURL           string
	Model             string
	DryRun            bool
	NoValidate        bool
	NoCoverage        bool
	NoSmartDiff       bool
	RaceDetection     bool
	MutationTest      bool
	CoverageTarget    float64
	MaxRetries        int
	MaxCoverageIter   int
	TimeoutSeconds    int
	ProjectCfg        *config.Config
	FnCache           *cache.Cache
	// TypedPkgCache holds type-checked packages keyed by directory so that
	// repeated lookups within the same run do not pay the `go/packages.Load`
	// cost more than once per directory.
	TypedPkgCache *typedPkgCache
}

// typedPkgCache memoises typed.Load results across processFile invocations.
// A nil *typed.Package entry means a prior load failed and the caller should
// fall back to syntactic analysis.
type typedPkgCache struct {
	entries map[string]*typed.Package
}

func newTypedPkgCache() *typedPkgCache {
	return &typedPkgCache{entries: map[string]*typed.Package{}}
}

// get loads (or returns a cached) type-checked package for the given
// directory. A nil result means type-checking was not available for that
// directory; callers should gracefully fall back to syntactic heuristics.
func (c *typedPkgCache) get(dir string) *typed.Package {
	if c == nil {
		return nil
	}
	if p, ok := c.entries[dir]; ok {
		return p
	}
	res, err := typed.Load(dir)
	if err != nil || res == nil || res.Package == nil {
		c.entries[dir] = nil
		return nil
	}
	c.entries[dir] = res.Package
	return res.Package
}

type fileResult struct {
	Report           ghub.FileReport
	Generated        int
	Validated        int
	Cached           int
	Attempted        bool
	MutationScore    float64
	MutationKilled   int
	MutationTotal    int
	PromptTokens     int
	CompletionTokens int
}

// processFile runs the full test-generation pipeline for a single changed file.
// Returns a fileResult with counters and a report, or nil if the file was skipped.
func processFile(f diff.FileDiff, opts pipelineOpts) *fileResult {
	changedLines := f.ChangedLines()
	fmt.Printf("  📄 %s\n", f.NewPath)
	fmt.Printf("     Hunks: %d, Changed lines: %d\n", len(f.Hunks), len(changedLines))

	if !strings.HasSuffix(f.NewPath, ".go") || strings.HasSuffix(f.NewPath, "_test.go") {
		fmt.Printf("     ⏭️  Skipped (not .go or is _test.go)\n\n")
		return nil
	}

	if len(changedLines) == 0 {
		fmt.Printf("     ⏭️  Skipped (deleted or no changed lines)\n\n")
		return nil
	}

	if opts.ProjectCfg.ShouldExclude(f.NewPath) {
		fmt.Printf("     ⏭️  Skipped by config (exclude pattern)\n\n")
		return nil
	}

	fullPath := filepath.Join(opts.RepoPath, f.NewPath)

	analysis, err := analyzer.AnalyzeFile(fullPath)
	if err != nil {
		fmt.Printf("     ⚠️  AST analysis failed: %v\n\n", err)
		return nil
	}

	pkgDir := filepath.Dir(fullPath)
	pkgAnalysis, pkgErr := analyzer.AnalyzePackage(pkgDir)

	// Attempt to load a type-checked view of the same package. Used where
	// precise, type-aware analysis is essential (interface satisfaction and
	// call-graph resolution); syntactic fallbacks are used if it fails.
	typedPkg := opts.TypedPkgCache.get(pkgDir)

	if tag := analyzer.DetectBuildTag(readFileString(fullPath)); tag != "" {
		fmt.Printf("     ⚠️  Build constraint: %s (may not compile on all platforms)\n", tag)
	}

	affectedFuncs := analyzer.FindFunctionsByLines(analysis, changedLines)
	affectedFuncs = analyzer.FilterTestable(affectedFuncs)
	if len(affectedFuncs) == 0 {
		fmt.Printf("     ℹ️  Changes outside functions (or only init)\n\n")
		return nil
	}

	fmt.Printf("     🔍 Affected functions (%d):\n", len(affectedFuncs))
	for _, fn := range affectedFuncs {
		fmt.Printf("        • %s  (lines %d–%d)\n", fn.Signature, fn.StartLine, fn.EndLine)
	}

	res := &fileResult{Attempted: true}

	var usedTypes []analyzer.TypeInfo
	var calledFuncs []analyzer.FuncInfo

	if pkgErr == nil && pkgAnalysis != nil {
		usedTypes, calledFuncs = collectDependencies(affectedFuncs, pkgAnalysis, typedPkg)
	}

	implMap := make(map[string][]string)
	if pkgErr == nil && pkgAnalysis != nil {
		for _, ti := range usedTypes {
			if ti.Kind != "interface" || len(ti.Methods) == 0 {
				continue
			}
			names := resolveImplementors(ti, pkgAnalysis, typedPkg)
			if len(names) > 0 {
				implMap[ti.Name] = names
			}
		}
	}

	// Cache check
	if opts.FnCache != nil {
		affectedFuncs, res.Cached = filterCached(opts.FnCache, f.NewPath, affectedFuncs, usedTypes)
		if len(affectedFuncs) == 0 {
			fmt.Printf("     ✅ All functions cached — skipping LLM call\n\n")
			return res
		}
	}

	// Git-based comparison
	if !opts.NoSmartDiff && len(affectedFuncs) > 0 {
		affectedFuncs = filterBySmartDiff(affectedFuncs, opts.RepoPath, opts.BaseBranch, f.NewPath)
		if len(affectedFuncs) == 0 {
			return nil
		}
	}

	existingTests := readExistingTests(fullPath)
	existingTestNames := prompt.ExtractTestFuncNames(existingTests)

	useRace := opts.RaceDetection || opts.ProjectCfg.Race
	var concInfos map[string]analyzer.ConcurrencyInfo
	hasConcurrentFuncs := false

	if useRace {
		concInfos = make(map[string]analyzer.ConcurrencyInfo)
		for _, fn := range affectedFuncs {
			ci := analyzer.DetectConcurrency(fn, usedTypes)
			if ci.IsConcurrent {
				concInfos[fn.Name] = ci
				hasConcurrentFuncs = true
				fmt.Printf("     ⚡ %s: concurrent (%s)\n", fn.Name, strings.Join(ci.Patterns, ", "))
			}
		}
	}

	mockCode := mockgen.GenerateMockCode(usedTypes)
	if mockCode != "" {
		mockCount := 0
		for _, ti := range usedTypes {
			if ti.Kind == "interface" && len(ti.Methods) > 0 {
				mockCount++
			}
		}
		fmt.Printf("     🎭 Generated %d mock(s) for interfaces\n", mockCount)
	}

	patternHints := patterns.DetectAll(affectedFuncs, analysis.Imports, usedTypes)
	if len(patternHints) > 0 {
		for fnName, hints := range patternHints {
			kinds := make([]string, len(hints))
			for i, h := range hints {
				kinds[i] = string(h.Kind)
			}
			fmt.Printf("     🔍 %s: patterns (%s)\n", fnName, strings.Join(kinds, ", "))
		}
	}

	var pkgVars []analyzer.VarInfo
	if pkgErr == nil && pkgAnalysis != nil {
		pkgVars = pkgAnalysis.AllVars
	}

	req := prompt.TestGenRequest{
		PackageName:       analysis.Package,
		FilePath:          f.NewPath,
		Imports:           analysis.Imports,
		TargetFuncs:       affectedFuncs,
		ExistingTests:     existingTests,
		ExistingTestNames: existingTestNames,
		UsedTypes:         usedTypes,
		CalledFuncs:       calledFuncs,
		PackageVars:       pkgVars,
		CustomPrompt:      opts.ProjectCfg.CustomPrompt,
		ConcurrencyInfos:  concInfos,
		RaceDetection:     useRace,
		PatternHints:      patternHints,
		MockCode:          mockCode,
		Implementors:      implMap,
	}

	budget := prompt.DefaultBudget()
	if opts.ProjectCfg.MaxContextTokens > 0 {
		budget.MaxTokens = opts.ProjectCfg.MaxContextTokens
	}
	if warning := prompt.EnforcePromptBudget(&req, budget); warning != "" {
		fmt.Printf("     ⚠️  %s\n", warning)
	}

	messages := prompt.BuildMessages(req)

	if opts.DryRun {
		fmt.Printf("\n     📋 DRY RUN — Prompt:\n")
		fmt.Printf("     ── System (%d chars) ──\n", len(messages[0].Content))
		fmt.Printf("     ── User (%d chars) ──\n", len(messages[1].Content))
		fmt.Println(messages[1].Content)
		fmt.Println()
		return res
	}

	cfg := buildLLMConfig(opts.APIKey, opts.BaseURL, opts.Model)
	if cfg.APIKey == "" && cfg.BaseURL == "https://api.openai.com/v1" {
		fmt.Printf("     ⚠️  No API key. Use --api-key or TESTGEN_API_KEY env\n")
		fmt.Printf("     💡 Or set --api-url for local model (Ollama)\n\n")
		return res
	}

	client := llm.NewClient(cfg)
	testFilePath := buildTestFilePath(fullPath, opts.OutDir)

	genResult := runGenerationLoop(
		client, cfg, req, existingTests, testFilePath, opts, useRace, hasConcurrentFuncs,
		affectedFuncs, usedTypes, f.NewPath,
	)
	res.PromptTokens += genResult.PromptTokens
	res.CompletionTokens += genResult.CompletionTokens
	success := genResult.Success

	if success {
		res.Generated++
		res.Validated++
	}

	if !success {
		success = tryPrune(genResult.Code, genResult.TestOutput, testFilePath, opts,
			affectedFuncs, usedTypes, f.NewPath, cfg.Model)
		if success {
			res.Generated++
			res.Validated++
		} else {
			res.Report = buildFileReport(reportInputs{
				FilePath:         f.NewPath,
				Funcs:            affectedFuncs,
				Generated:        0,
				Validated:        0,
				DiffCov:          0,
				CoverageTarget:   opts.CoverageTarget,
				Success:          false,
				PromptTokens:     res.PromptTokens,
				CompletionTokens: res.CompletionTokens,
			})
			return res
		}
	}

	// Diff coverage
	var covLoop coverageLoopResult
	if !opts.NoValidate && !opts.NoCoverage && !opts.DryRun {
		covLoop = runCoverageLoop(
			client, cfg, req, testFilePath, fullPath, opts.RepoPath,
			changedLines, affectedFuncs, opts.CoverageTarget,
			opts.MaxCoverageIter, opts.TimeoutSeconds,
		)
		res.PromptTokens += covLoop.PromptTokens
		res.CompletionTokens += covLoop.CompletionTokens
	}

	// Branch / error-path coverage (derived from the same coverage profile)
	branchRes := calculateBranchMetrics(fullPath, affectedFuncs, covLoop.Blocks)

	// Mutation testing
	if opts.MutationTest && success && !opts.DryRun {
		mutScore, mutKilled, mutTotal := runMutationTesting(affectedFuncs, fullPath)
		res.MutationScore = mutScore
		res.MutationKilled = mutKilled
		res.MutationTotal = mutTotal
	}

	res.Report = buildFileReport(reportInputs{
		FilePath:         f.NewPath,
		Funcs:            affectedFuncs,
		Generated:        res.Generated,
		Validated:        res.Validated,
		DiffCov:          covLoop.Coverage,
		CoverageTarget:   opts.CoverageTarget,
		Success:          success,
		MutScore:         res.MutationScore,
		MutKilled:        res.MutationKilled,
		MutTotal:         res.MutationTotal,
		BranchResult:     branchRes,
		PromptTokens:     res.PromptTokens,
		CompletionTokens: res.CompletionTokens,
	})
	return res
}

// collectDependencies collects types and functions referenced by affectedFuncs.
//
// The type list is always gathered via the syntactic analyzer (signatures are
// already represented as strings in FuncInfo, so go/types adds no value
// here). For the call graph we prefer type-checked resolution through typedPkg
// when available: types.Info.Uses gives us the exact *types.Func referenced
// by each identifier, avoiding the false positives that plagued the
// name-based FindCalledFunctions helper (e.g. a local variable named the same
// as a package function).
func collectDependencies(affectedFuncs []analyzer.FuncInfo, pkgAnalysis *analyzer.PackageAnalysis, typedPkg *typed.Package) ([]analyzer.TypeInfo, []analyzer.FuncInfo) {
	var usedTypes []analyzer.TypeInfo
	var calledFuncs []analyzer.FuncInfo

	seenTypes := make(map[string]bool)
	for _, fn := range affectedFuncs {
		for _, ti := range analyzer.FindUsedTypes(fn, pkgAnalysis.AllTypes) {
			if !seenTypes[ti.Name] {
				usedTypes = append(usedTypes, ti)
				seenTypes[ti.Name] = true
			}
		}
	}

	seenFuncs := make(map[string]bool)
	for _, fn := range affectedFuncs {
		var calleeNames []string
		if typedPkg != nil {
			calleeNames = typedCalleeNames(typedPkg, fn)
		}
		if calleeNames == nil {
			// Fallback: name-based call graph via AST.
			for _, called := range analyzer.FindCalledFunctions(fn, pkgAnalysis) {
				calleeNames = append(calleeNames, called.Name)
			}
		}
		for _, name := range calleeNames {
			if seenFuncs[name] {
				continue
			}
			fi, ok := pkgAnalysis.FuncIndex[name]
			if !ok {
				continue
			}
			calledFuncs = append(calledFuncs, fi)
			seenFuncs[name] = true
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

	return usedTypes, calledFuncs
}

// typedCalleeNames returns the same-package callees of fn as resolved by the
// type checker. Returns nil when the function cannot be located in the
// type-checked AST (callers should treat nil as "fall back to syntactic").
func typedCalleeNames(pkg *typed.Package, fn analyzer.FuncInfo) []string {
	var callees []typed.Callee
	if fn.Receiver != "" {
		recv := strings.TrimPrefix(fn.Receiver, "*")
		callees = pkg.CalleesOfMethod(recv, fn.Name)
	} else {
		callees = pkg.Callees(fn.Name)
	}
	if callees == nil {
		return nil
	}
	names := make([]string, 0, len(callees))
	for _, c := range callees {
		if !c.SamePackage {
			continue
		}
		names = append(names, c.Name)
	}
	return names
}

// resolveImplementors returns the names of concrete types implementing the
// given interface, preferring the type-checker's answer to the name-based
// heuristic.
func resolveImplementors(iface analyzer.TypeInfo, pkgAnalysis *analyzer.PackageAnalysis, typedPkg *typed.Package) []string {
	if typedPkg != nil {
		if names := typedPkg.Implementors(iface.Name); len(names) > 0 {
			return names
		}
	}
	impls := analyzer.FindImplementors(iface, pkgAnalysis.AllTypes, pkgAnalysis.AllFuncs)
	if len(impls) == 0 {
		return nil
	}
	names := make([]string, len(impls))
	for i, imp := range impls {
		names[i] = imp.Name
	}
	return names
}

func filterCached(fnCache *cache.Cache, filePath string, funcs []analyzer.FuncInfo, usedTypes []analyzer.TypeInfo) ([]analyzer.FuncInfo, int) {
	var uncached []analyzer.FuncInfo
	cached := 0

	for _, fn := range funcs {
		key := cache.Key(filePath, fn.Name)
		hash := cache.ComputeHash(fn, usedTypes)

		if entry, hit := fnCache.Lookup(key, hash); hit {
			fmt.Printf("     ♻️  Cached: %s (tests: %s, model: %s)\n",
				fn.Name, strings.Join(entry.GeneratedFuncs, ", "), entry.Model)
			cached++
		} else {
			uncached = append(uncached, fn)
		}
	}

	if len(uncached) < len(funcs) && len(uncached) > 0 {
		fmt.Printf("     📝 %d/%d functions need generation\n", len(uncached), len(funcs))
	}

	return uncached, cached
}

func filterBySmartDiff(funcs []analyzer.FuncInfo, repoPath, baseBranch, filePath string) []analyzer.FuncInfo {
	cmpResult, cmpErr := gitdiff.FilterChanged(funcs, repoPath, baseBranch, filePath)
	if cmpErr != nil {
		fmt.Printf("     ⚠️  Git compare: %v (processing all functions)\n", cmpErr)
		return funcs
	}

	if len(cmpResult.Unchanged) > 0 {
		names := make([]string, len(cmpResult.Unchanged))
		for i, fn := range cmpResult.Unchanged {
			names[i] = fn.Name
		}
		fmt.Printf("     🔄 Unchanged vs base: %s (skipped)\n", strings.Join(names, ", "))
	}
	if len(cmpResult.New) > 0 {
		names := make([]string, len(cmpResult.New))
		for i, fn := range cmpResult.New {
			names[i] = fn.Name
		}
		fmt.Printf("     🆕 New functions: %s\n", strings.Join(names, ", "))
	}

	var needGeneration []analyzer.FuncInfo
	needGeneration = append(needGeneration, cmpResult.Changed...)
	needGeneration = append(needGeneration, cmpResult.New...)

	if len(needGeneration) == 0 {
		fmt.Printf("     ✅ All functions unchanged vs base — skipping LLM\n\n")
		return nil
	}

	if len(needGeneration) < len(funcs) {
		fmt.Printf("     📝 %d/%d functions actually changed\n", len(needGeneration), len(funcs))
	}

	return needGeneration
}

// generationLoopResult aggregates everything the generation loop produces so
// the caller can later compute quality metrics (token efficiency) and build
// the per-file report.
type generationLoopResult struct {
	Code             string // final test code written to disk
	TestOutput       string // last go test output (JSON stream)
	Success          bool
	PromptTokens     int
	CompletionTokens int
}

// runGenerationLoop attempts to generate and validate tests up to maxRetries times.
// LLM generates ONLY new test functions; they are merged with existing tests before validation.
func runGenerationLoop(
	client *llm.Client,
	cfg llm.Config,
	req prompt.TestGenRequest,
	existingTests string,
	testFilePath string,
	opts pipelineOpts,
	useRace bool,
	hasConcurrentFuncs bool,
	affectedFuncs []analyzer.FuncInfo,
	usedTypes []analyzer.TypeInfo,
	relPath string,
) generationLoopResult {
	var mergedCode string
	var rawNewCode string
	var lastTestOutput string
	var lastValError string
	var out generationLoopResult

	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		if attempt == 1 {
			fmt.Printf("     🤖 Generating tests via %s...\n", cfg.Model)
		} else {
			fmt.Printf("     🔄 Attempt %d/%d — fixing errors...\n", attempt, opts.MaxRetries)
		}

		var result *llm.GenerateResponse
		var err error

		if attempt == 1 {
			result, err = client.Generate(prompt.BuildMessages(req))
		} else {
			var failingNames []string
			var compactFB string
			if lastTestOutput != "" {
				testResults := pruner.ParseTestOutput(lastTestOutput)
				failingNames = pruner.FailingTopLevel(testResults)
				if len(failingNames) > 0 {
					fmt.Printf("     🎯 Focusing fix on %d failing test(s): %s\n",
						len(failingNames), strings.Join(failingNames, ", "))
				}
				fb := pruner.ParseStructuredFeedback(lastTestOutput)
				if len(fb) > 0 {
					compactFB = pruner.FormatCompactFeedback(fb)
				}
			}
			fixMessages := prompt.BuildFixMessages(req, rawNewCode, lastValError, attempt, failingNames, compactFB)
			result, err = client.Generate(fixMessages)
		}

		if err != nil {
			fmt.Printf("     ❌ LLM error: %v\n", err)
			break
		}

		rawNewCode = result.Content
		out.PromptTokens += result.PromptTokens
		out.CompletionTokens += result.CompletionTokens
		fmt.Printf("     ✅ Generated (%d prompt + %d completion tokens)\n",
			result.PromptTokens, result.CompletionTokens)

		rawNewCode = injectMockIfMissing(rawNewCode, req.MockCode, usedTypes)

		mergedCode = rawNewCode
		if existingTests != "" {
			mergeResult, mergeErr := merger.Merge(existingTests, rawNewCode)
			if mergeErr != nil {
				fmt.Printf("     ⚠️  AST merge failed, using raw LLM output: %v\n", mergeErr)
			} else {
				mergedCode = mergeResult.Code
				if len(mergeResult.Added) > 0 {
					fmt.Printf("     🔀 Merged: +%d new funcs", len(mergeResult.Added))
					if len(mergeResult.Skipped) > 0 {
						fmt.Printf(", %d duplicates skipped", len(mergeResult.Skipped))
					}
					fmt.Println()
				}
			}
		}

		dedupResult, dedupErr := dedup.Dedup(mergedCode)
		if dedupErr == nil && dedupResult.Removed > 0 {
			mergedCode = dedupResult.Code
			fmt.Printf("     🧹 Dedup: removed %d duplicate case(s)\n", dedupResult.Removed)
		}

		if err := os.MkdirAll(filepath.Dir(testFilePath), 0755); err != nil {
			fmt.Printf("     ❌ Cannot create directory: %v\n", err)
			break
		}

		if err := os.WriteFile(testFilePath, []byte(mergedCode), 0644); err != nil {
			fmt.Printf("     ❌ Cannot write file: %v\n", err)
			break
		}

		if opts.NoValidate {
			fmt.Printf("     💾 Tests saved: %s (validation disabled)\n\n", testFilePath)
			out.Code = mergedCode
			out.TestOutput = lastTestOutput
			out.Success = true
			return out
		}

		fmt.Printf("     🔬 Validating...\n")
		valResult := validator.Validate(opts.RepoPath, testFilePath, opts.TimeoutSeconds)

		if valResult.IsValid() {
			fmt.Printf("     %s\n", valResult.Summary())

			if useRace && hasConcurrentFuncs {
				fmt.Printf("     🏁 Running race detector...\n")
				raceResult := validator.ValidateWithRace(opts.RepoPath, testFilePath)
				if raceResult.TestError != "" && strings.Contains(raceResult.TestError, "race detector unavailable") {
					fmt.Printf("     ℹ️  Race detector skipped (CGO disabled)\n")
				} else if raceResult.HasRaces {
					fmt.Printf("     ❌ DATA RACE detected — tests rejected:\n%s\n", raceResult.RaceDetails)
					lastValError = "DATA RACE: " + raceResult.RaceDetails
					lastTestOutput = raceResult.TestOutput
					if attempt == opts.MaxRetries {
						fmt.Printf("     ⛔ Max retries reached (%d)\n", opts.MaxRetries)
					}
					continue
				} else if raceResult.IsValid() {
					fmt.Printf("     ✅ Race detector: no races found\n")
				} else {
					fmt.Printf("     ⚠️  Race test failed: %s\n", raceResult.TestError)
				}
			}

			fmt.Printf("     💾 Tests saved: %s\n\n", testFilePath)
			updateCache(opts.FnCache, relPath, affectedFuncs, usedTypes, testFilePath, cfg.Model)
			out.Code = mergedCode
			out.TestOutput = lastTestOutput
			out.Success = true
			return out
		}

		lastValError = valResult.Summary()
		lastTestOutput = valResult.TestOutput
		fmt.Printf("     %s\n", valResult.Summary())

		if attempt == opts.MaxRetries {
			fmt.Printf("     ⛔ Max retries reached (%d)\n", opts.MaxRetries)
		}
	}

	out.Code = mergedCode
	out.TestOutput = lastTestOutput
	out.Success = false
	return out
}

// tryPrune attempts to salvage passing tests from a failed generation by pruning failing ones.
func tryPrune(
	generatedCode, lastTestOutput, testFilePath string,
	opts pipelineOpts,
	affectedFuncs []analyzer.FuncInfo,
	usedTypes []analyzer.TypeInfo,
	relPath, model string,
) bool {
	if generatedCode == "" {
		fmt.Printf("     ❌ Failed to generate tests\n\n")
		return false
	}
	if lastTestOutput == "" {
		os.Remove(testFilePath)
		fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
		return false
	}

	fmt.Printf("     ✂️  Pruning failing tests...\n")
	pruneResult, pruneErr := pruner.Prune(generatedCode, lastTestOutput)

	if pruneErr != nil {
		fmt.Printf("     ⚠️  Prune failed: %v\n", pruneErr)
		os.Remove(testFilePath)
		fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
		return false
	}
	if pruneResult.KeptTests == 0 {
		fmt.Printf("     ⚠️  All tests failed, nothing to keep\n")
		os.Remove(testFilePath)
		fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
		return false
	}

	fmt.Printf("     ✂️  Removed %d functions, %d sub-test cases. Kept %d test functions.\n",
		len(pruneResult.RemovedFuncs), pruneResult.RemovedSubTests, pruneResult.KeptTests)

	if len(pruneResult.RemovedFuncs) > 0 {
		fmt.Printf("     🗑️  Removed: %s\n", strings.Join(pruneResult.RemovedFuncs, ", "))
	}

	if err := os.WriteFile(testFilePath, []byte(pruneResult.Code), 0644); err != nil {
		fmt.Printf("     ❌ Cannot write pruned file: %v\n", err)
		os.Remove(testFilePath)
		return false
	}

	fmt.Printf("     🔬 Re-validating pruned tests...\n")
	valResult := validator.Validate(opts.RepoPath, testFilePath, opts.TimeoutSeconds)

	if valResult.IsValid() {
		fmt.Printf("     %s\n", valResult.Summary())
		fmt.Printf("     💾 Pruned tests saved: %s\n\n", testFilePath)
		updateCache(opts.FnCache, relPath, affectedFuncs, usedTypes, testFilePath, model)
		return true
	}

	fmt.Printf("     ⚠️  Pruned tests still fail: %s\n", valResult.Summary())
	os.Remove(testFilePath)
	fmt.Printf("     🗑️  Invalid file deleted: %s\n\n", testFilePath)
	return false
}

func runMutationTesting(affectedFuncs []analyzer.FuncInfo, fullPath string) (score float64, killed, total int) {
	fmt.Printf("     🧬 Running mutation testing...\n")
	funcNames := make([]string, len(affectedFuncs))
	for i, fn := range affectedFuncs {
		funcNames[i] = fn.Name
	}

	moduleRoot := findModuleRoot(filepath.Dir(fullPath))
	if moduleRoot == "" {
		return 0, 0, 0
	}

	mutResult, mutErr := mutation.RunMutationTests(fullPath, funcNames, moduleRoot)
	if mutErr != nil {
		fmt.Printf("     ⚠️  Mutation testing failed: %v\n", mutErr)
		return 0, 0, 0
	}

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

	return mutResult.MutationScore, mutResult.Killed, mutResult.Total
}

func readFileString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// injectMockIfMissing checks if the LLM-generated code references mock types
// but doesn't define them. If so, appends pre-generated mock code.
func injectMockIfMissing(code, mockCode string, usedTypes []analyzer.TypeInfo) string {
	if mockCode == "" {
		return code
	}
	mocks := mockgen.GenerateMocks(usedTypes)
	injected := false
	for _, m := range mocks {
		if strings.Contains(code, m.MockName) && !strings.Contains(code, "type "+m.MockName+" struct") {
			code += "\n" + m.Code + "\n"
			injected = true
		}
	}
	if injected {
		fmt.Printf("     🎭 Auto-injected missing mock definition(s)\n")
	}
	return code
}

// reportInputs aggregates everything processFile collects about a file so the
// report builder stays a small pure helper with a single argument.
type reportInputs struct {
	FilePath         string
	Funcs            []analyzer.FuncInfo
	Generated        int
	Validated        int
	DiffCov          float64
	CoverageTarget   float64
	Success          bool
	MutScore         float64
	MutKilled        int
	MutTotal         int
	BranchResult     branchcov.Result
	PromptTokens     int
	CompletionTokens int
}

func buildFileReport(in reportInputs) ghub.FileReport {
	funcNames := make([]string, len(in.Funcs))
	for i, fn := range in.Funcs {
		funcNames[i] = fn.Name
	}

	status := "failed"
	if in.Success {
		status = "success"
		if in.DiffCov < in.CoverageTarget && in.DiffCov > 0 {
			status = "partial"
		}
	}

	tokenEff := 0.0
	totalTokens := in.PromptTokens + in.CompletionTokens
	if in.Validated > 0 && totalTokens > 0 {
		tokenEff = float64(totalTokens) / float64(in.Validated)
	}

	return ghub.FileReport{
		File:              in.FilePath,
		Functions:         funcNames,
		TestsTotal:        in.Generated,
		TestsPassed:       in.Validated,
		DiffCoverage:      in.DiffCov,
		MutationScore:     in.MutScore,
		MutationKilled:    in.MutKilled,
		MutationTotal:     in.MutTotal,
		BranchCoverage:    in.BranchResult.Coverage,
		BranchesTotal:     in.BranchResult.Total,
		BranchesCovered:   in.BranchResult.Covered,
		ErrorPathCoverage: in.BranchResult.ErrorPathCoverage,
		ErrorPathsTotal:   in.BranchResult.ErrorPathsTotal,
		ErrorPathsCovered: in.BranchResult.ErrorPathsCovered,
		PromptTokens:      in.PromptTokens,
		CompletionTokens:  in.CompletionTokens,
		TokenEfficiency:   tokenEff,
		Status:            status,
	}
}

// calculateBranchMetrics derives branch- and error-path coverage for the
// affected functions from the last coverage profile we observed. Returns a
// zero-valued Result if the inputs are insufficient (no blocks or no
// affected functions) — the reporter will simply omit those metrics.
func calculateBranchMetrics(sourcePath string, funcs []analyzer.FuncInfo, blocks []coverage.CoverageBlock) branchcov.Result {
	if len(blocks) == 0 || len(funcs) == 0 {
		return branchcov.Result{}
	}
	names := make(map[string]bool, len(funcs))
	for _, fn := range funcs {
		names[fn.Name] = true
	}
	branches, err := branchcov.Analyze(sourcePath, names)
	if err != nil || len(branches) == 0 {
		return branchcov.Result{}
	}
	return branchcov.Calculate(branches, blocks, filepath.Base(sourcePath))
}
