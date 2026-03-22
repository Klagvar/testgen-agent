// Package prompt builds structured prompts for the LLM
// based on diff parsing and AST analysis results.
package prompt

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
	"github.com/gizatulin/testgen-agent/internal/patterns"
)

// ExtractFuncSource extracts specific functions by name from Go source code
// and returns them as a valid Go file (with package + imports).
// If funcNames is empty or none are found, returns the original source.
func ExtractFuncSource(src string, funcNames []string) string {
	if len(funcNames) == 0 || strings.TrimSpace(src) == "" {
		return src
	}

	nameSet := make(map[string]bool, len(funcNames))
	for _, n := range funcNames {
		nameSet[n] = true
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "extract.go", src, parser.ParseComments)
	if err != nil {
		return src
	}

	lines := strings.Split(src, "\n")
	var extracted []string

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !nameSet[fn.Name.Name] {
			continue
		}
		startLine := fset.Position(fn.Pos()).Line
		if fn.Doc != nil {
			docStart := fset.Position(fn.Doc.Pos()).Line
			if docStart < startLine {
				startLine = docStart
			}
		}
		endLine := fset.Position(fn.End()).Line
		if startLine >= 1 && endLine <= len(lines) {
			extracted = append(extracted, strings.Join(lines[startLine-1:endLine], "\n"))
		}
	}

	if len(extracted) == 0 {
		return src
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("package %s\n\n", f.Name.Name))
	if len(f.Imports) > 0 {
		sb.WriteString("import (\n")
		for _, imp := range f.Imports {
			if imp.Name != nil {
				sb.WriteString(fmt.Sprintf("\t%s %s\n", imp.Name.Name, imp.Path.Value))
			} else {
				sb.WriteString(fmt.Sprintf("\t%s\n", imp.Path.Value))
			}
		}
		sb.WriteString(")\n\n")
	}
	sb.WriteString(strings.Join(extracted, "\n\n"))
	sb.WriteString("\n")
	return sb.String()
}

// ExtractTestFuncNames parses Go test source and returns top-level function names.
func ExtractTestFuncNames(src string) []string {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		return nil
	}
	var names []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		names = append(names, fn.Name.Name)
	}
	return names
}

// TestGenRequest holds input data for prompt generation.
type TestGenRequest struct {
	PackageName      string                       // package name
	FilePath         string                       // file path
	Imports          []string                     // file imports
	TargetFuncs      []analyzer.FuncInfo          // affected functions
	ExistingTests    string                       // existing tests full code (used by coverage gap)
	ExistingTestNames []string                    // existing test function names (to avoid duplication)
	UsedTypes        []analyzer.TypeInfo          // package types used by functions
	CalledFuncs      []analyzer.FuncInfo          // called functions from the package (cross-file)
	CustomPrompt     string                       // additional instructions from .testgen.yml
	ConcurrencyInfos map[string]analyzer.ConcurrencyInfo // funcName → concurrency info
	RaceDetection    bool                         // run with -race flag
	PatternHints     map[string][]patterns.PatternHint  // funcName → detected patterns
	MockCode         string                       // pre-generated mock code for interfaces
}

// BuildSystemPrompt builds the system prompt — instructions for the LLM.
func BuildSystemPrompt() string {
	return `You are an expert Go developer specializing in writing unit tests.

Your task is to generate high-quality unit tests for the provided Go functions.

## Test Requirements

1. **Format**: Use the standard "testing" package. Prefer table-driven tests.
2. **Coverage**: Cover all execution branches:
   - Happy path (normal cases)
   - Boundary values (0, empty strings, nil, max values)
   - Error cases (invalid input, division by zero, etc.)
3. **Naming**: Test names must be descriptive, format: Test<FuncName>_<Scenario>
4. **Isolation**: Each test must be independent.
5. **Assertions**: Use t.Errorf / t.Fatalf for checks. Do NOT use external assertion libraries (e.g. testify).
6. **Output format**: Return ONLY Go code — ready to compile.

## Common Mistakes (AVOID THESE)

- Do NOT use compile-time overflow expressions like math.MaxInt64+1 or -math.MinInt64.
  For overflow tests, use variables: a := math.MaxInt64, then pass them to the function.
- Do NOT import unused packages — Go will not compile.
- Do NOT declare unused variables — Go will not compile. Use _ for unused values.
  Example: _, err := SomeFunc() if you only need the error.
- Verify that function call signatures match definitions (number and types of args/returns).
- The % operator in Go preserves the sign of the dividend: -7 % 3 == -1 (NOT 2 like in Python).
- Do NOT use invalid escape sequences in Go string literals. Valid ones: \n \t \r \\ \" \' \a \b \f \v \0 \x \u \U.
  For backslashes use: "\\" or raw strings.
- Do NOT use filepath.Join with hardcoded OS-specific paths.
- When testing functions using os/exec, remember that commands are platform-dependent.

## CRITICAL: Output Only NEW Tests

Generate ONLY new test functions. Do NOT include existing test functions in your output.
Your code will be automatically merged with the existing test file.

## Response Format

Return a valid Go file with:
1. package declaration
2. import block (only imports needed by YOUR new tests)
3. ONLY new test functions (and any new helper types/vars they need)

No explanations, no markdown wrappers. Code must start with "package ..." and be valid Go.`
}

// BuildUserPrompt builds the user prompt with function context.
func BuildUserPrompt(req TestGenRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Generate unit tests for package `%s` (file `%s`).\n\n", req.PackageName, req.FilePath))

	// File imports
	if len(req.Imports) > 0 {
		sb.WriteString("## Source File Imports\n\n```go\nimport (\n")
		for _, imp := range req.Imports {
			sb.WriteString(fmt.Sprintf("\t\"%s\"\n", imp))
		}
		sb.WriteString(")\n```\n\n")
	}

	// Package types (if any)
	if len(req.UsedTypes) > 0 {
		sb.WriteString("## Type Definitions (from the same package)\n\n")
		sb.WriteString("These types are used by the functions under test. Use them to correctly construct test data.\n\n")
		for _, ti := range req.UsedTypes {
			sb.WriteString(fmt.Sprintf("### %s (%s)\n\n", ti.Name, ti.Kind))
			if ti.Source != "" {
				sb.WriteString("```go\n")
				sb.WriteString(ti.Source)
				sb.WriteString("\n```\n\n")
			}
			if ti.Kind == "interface" && len(ti.Methods) > 0 {
				sb.WriteString("**Methods:**\n")
				for _, m := range ti.Methods {
					sb.WriteString(fmt.Sprintf("- `%s%s`\n", m.Name, m.Signature))
				}
				sb.WriteString("\n")
				sb.WriteString("⚠️ To test functions that use this interface, create a **mock struct** implementing it.\n")
				sb.WriteString("Example pattern:\n```go\ntype mock" + ti.Name + " struct {\n")
				for _, m := range ti.Methods {
					sb.WriteString(fmt.Sprintf("\t%sFunc func%s\n", m.Name, m.Signature))
				}
				sb.WriteString("}\n")
				for _, m := range ti.Methods {
					sb.WriteString(fmt.Sprintf("func (m *mock%s) %s%s { return m.%sFunc%s }\n",
						ti.Name, m.Name, m.Signature,
						m.Name, extractCallArgs(m.Signature)))
				}
				sb.WriteString("```\n\n")
			}
		}
	}

	// Pre-generated mock implementations
	if req.MockCode != "" {
		sb.WriteString("## Pre-generated Mock Implementations\n\n")
		sb.WriteString("Use these mock structs in your tests. They are guaranteed to compile correctly.\n\n")
		sb.WriteString("```go\n")
		sb.WriteString(req.MockCode)
		sb.WriteString("```\n\n")
	}

	// Called functions from the package (cross-file context)
	if len(req.CalledFuncs) > 0 {
		sb.WriteString("## Helper Functions (called by functions under test)\n\n")
		sb.WriteString("These functions are called internally. You do NOT need to test them, but knowing their signatures helps write correct tests.\n\n")
		for _, fn := range req.CalledFuncs {
			sb.WriteString(fmt.Sprintf("- `%s`", fn.Signature))
			if fn.DocComment != "" {
				sb.WriteString(fmt.Sprintf(" — %s", strings.TrimSpace(fn.DocComment)))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Functions to test
	sb.WriteString(fmt.Sprintf("## Functions to Test (%d)\n\n", len(req.TargetFuncs)))

	for i, fn := range req.TargetFuncs {
		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, fn.Name))

		// Receiver
		if fn.Receiver != "" {
			sb.WriteString(fmt.Sprintf("**Receiver:** `%s` — this is a method. Create an instance of the receiver type in the test.\n\n", fn.Receiver))
		}

		// Signature
		sb.WriteString(fmt.Sprintf("**Signature:** `%s`\n\n", fn.Signature))

		// Documentation
		if fn.DocComment != "" {
			sb.WriteString(fmt.Sprintf("**Documentation:** %s\n", strings.TrimSpace(fn.DocComment)))
		}

		// Parameters
		if len(fn.Params) > 0 {
			sb.WriteString("**Parameters:**\n")
			for _, p := range fn.Params {
				sb.WriteString(fmt.Sprintf("- `%s` — type `%s`\n", p.Name, p.Type))
			}
			sb.WriteString("\n")
		}

		// Return types
		if len(fn.Returns) > 0 {
			sb.WriteString(fmt.Sprintf("**Returns:** `%s`\n\n", strings.Join(fn.Returns, ", ")))
		}

		// Function body
		sb.WriteString("**Implementation:**\n\n```go\n")
		sb.WriteString(fn.Body)
		sb.WriteString("\n```\n\n")

		// Branch analysis
		branches := analyzeBranches(fn.Body)
		if len(branches) > 0 {
			sb.WriteString("**Code branches:**\n")
			for _, b := range branches {
				sb.WriteString(fmt.Sprintf("- %s\n", b))
			}
			sb.WriteString("\n")
		}

		// Concurrency hints
		if req.ConcurrencyInfos != nil {
			if ci, ok := req.ConcurrencyInfos[fn.Name]; ok && ci.IsConcurrent {
				sb.WriteString("**⚡ Concurrency Analysis:**\n\n")
				sb.WriteString(ci.ConcurrencyHint())
				sb.WriteString("\n")
				if req.RaceDetection {
					sb.WriteString("⚠️ Tests will be run with `-race` flag. Ensure all concurrent accesses are properly synchronized.\n\n")
				}
			}
		}

		// Pattern-specific hints
		if req.PatternHints != nil {
			if hints, ok := req.PatternHints[fn.Name]; ok && len(hints) > 0 {
				sb.WriteString("**🔍 Pattern-Specific Guidance:**\n\n")
				for _, h := range hints {
					sb.WriteString(fmt.Sprintf("_%s_\n\n", h.Summary))
					sb.WriteString(h.Guide)
					sb.WriteString("\n\n")
				}
			}
		}

		sb.WriteString("---\n\n")
	}

	// Existing tests — show only names to prevent duplication
	if len(req.ExistingTestNames) > 0 {
		sb.WriteString("## Existing Tests (DO NOT REDECLARE)\n\n")
		sb.WriteString("The test file already contains these test functions:\n")
		for _, name := range req.ExistingTestNames {
			sb.WriteString(fmt.Sprintf("- `%s`\n", name))
		}
		sb.WriteString("\n**Do NOT redeclare any of these.** Your output will be automatically merged with the existing file.\n")
		sb.WriteString("Generate ONLY new test functions for the listed functions under test.\n")
	} else {
		sb.WriteString("Generate test functions for all listed functions.\n")
	}

	return sb.String()
}

// extractCallArgs extracts call arguments from a function signature.
// "(id string) (*Entity, error)" → "(id)"
func extractCallArgs(sig string) string {
	parenEnd := strings.Index(sig, ")")
	if parenEnd < 0 {
		return "()"
	}
	params := sig[1:parenEnd]
	if params == "" {
		return "()"
	}

	parts := strings.Split(params, ",")
	var args []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		fields := strings.Fields(p)
		if len(fields) > 0 {
			args = append(args, fields[0])
		}
	}
	return "(" + strings.Join(args, ", ") + ")"
}

// analyzeBranches performs a simple branch analysis of the function body for LLM hints.
func analyzeBranches(body string) []string {
	var branches []string

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "if ") {
			cond := strings.TrimPrefix(trimmed, "if ")
			cond = strings.TrimSuffix(cond, " {")
			branches = append(branches, fmt.Sprintf("Condition: `%s`", cond))
		} else if strings.HasPrefix(trimmed, "} else if ") {
			cond := strings.TrimPrefix(trimmed, "} else if ")
			cond = strings.TrimSuffix(cond, " {")
			branches = append(branches, fmt.Sprintf("Else-if: `%s`", cond))
		} else if trimmed == "} else {" {
			branches = append(branches, "Else branch")
		} else if strings.HasPrefix(trimmed, "switch ") {
			branches = append(branches, "Switch statement")
		} else if strings.HasPrefix(trimmed, "case ") {
			caseVal := strings.TrimPrefix(trimmed, "case ")
			caseVal = strings.TrimSuffix(caseVal, ":")
			branches = append(branches, fmt.Sprintf("Case: `%s`", caseVal))
		} else if strings.Contains(trimmed, "err != nil") {
			branches = append(branches, "Error check (err != nil)")
		}
	}

	return branches
}

// BuildMessages builds a message array for the LLM API.
func BuildMessages(req TestGenRequest) []Message {
	systemPrompt := BuildSystemPrompt()
	if req.CustomPrompt != "" {
		systemPrompt += "\n\n## Additional project-specific instructions:\n" + req.CustomPrompt
	}
	return []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: BuildUserPrompt(req)},
	}
}

// BuildFixMessages builds messages for a retry attempt.
// If failingTestNames is provided, only those functions are extracted from previousNewCode
// to focus the LLM on just the failing tests. Otherwise, all new tests are sent.
func BuildFixMessages(req TestGenRequest, previousNewCode string, errors string, attempt int, failingTestNames []string) []Message {
	codeToFix := previousNewCode
	if len(failingTestNames) > 0 {
		codeToFix = ExtractFuncSource(previousNewCode, failingTestNames)
	}

	fixPrompt := buildFixPrompt(errors, attempt)

	return []Message{
		{Role: "system", Content: BuildSystemPrompt()},
		{Role: "user", Content: BuildUserPrompt(req)},
		{Role: "assistant", Content: codeToFix},
		{Role: "user", Content: fixPrompt},
	}
}

// buildFixPrompt builds a prompt with the error description for fixing.
func buildFixPrompt(errors string, attempt int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Error in Generated Tests (attempt %d)\n\n", attempt))
	sb.WriteString("The tests you generated failed validation. Here are the errors:\n\n")
	sb.WriteString("```\n")
	sb.WriteString(errors)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Fix Instructions\n\n")
	sb.WriteString("1. Carefully read the errors above.\n")
	sb.WriteString("2. Fix ONLY the problematic parts of your test code.\n")
	sb.WriteString("3. Make sure that:\n")
	sb.WriteString("   - All types are correct (no overflow, no type mismatches)\n")
	sb.WriteString("   - All imports are used and present\n")
	sb.WriteString("   - All called functions exist with correct signatures\n")
	sb.WriteString("   - Tests correctly verify expected behavior\n")
	sb.WriteString("4. Return ONLY the fixed new test functions (same format as before — valid Go file with package, imports, and ONLY the new test functions).\n")
	sb.WriteString("5. Return ONLY code — no explanations, no markdown wrappers.\n")

	return sb.String()
}

// CoverageGapRequest holds data for re-generating tests for uncovered lines.
type CoverageGapRequest struct {
	TestGenRequest                  // base request with functions
	ExistingTestCode string        // current test code
	UncoveredLines   []int         // uncovered lines
	CurrentCoverage  float64       // current diff coverage (%)
	Iteration        int           // re-generation iteration number
}

// BuildCoverageGapMessages builds a prompt for re-generating tests
// that cover uncovered lines of code.
func BuildCoverageGapMessages(req CoverageGapRequest) []Message {
	gapPrompt := buildCoverageGapPrompt(req)

	return []Message{
		{Role: "system", Content: BuildSystemPrompt()},
		{Role: "user", Content: gapPrompt},
	}
}

// buildCoverageGapPrompt builds a prompt for covering uncovered lines.
func buildCoverageGapPrompt(req CoverageGapRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Improve Test Coverage (iteration %d)\n\n", req.Iteration))
	sb.WriteString(fmt.Sprintf("Package: `%s`, file: `%s`\n\n", req.PackageName, req.FilePath))
	sb.WriteString(fmt.Sprintf("Current diff coverage: **%.1f%%**. Need to improve it.\n\n", req.CurrentCoverage))

	// Functions with uncovered lines
	sb.WriteString("## Functions with Uncovered Lines\n\n")
	for _, fn := range req.TargetFuncs {
		// Determine uncovered lines within this function
		var uncovInFunc []int
		for _, line := range req.UncoveredLines {
			if line >= fn.StartLine && line <= fn.EndLine {
				uncovInFunc = append(uncovInFunc, line)
			}
		}
		if len(uncovInFunc) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", fn.Name))
		sb.WriteString(fmt.Sprintf("**Signature:** `%s`\n\n", fn.Signature))
		sb.WriteString("**Implementation:**\n\n```go\n")
		sb.WriteString(fn.Body)
		sb.WriteString("\n```\n\n")

		sb.WriteString(fmt.Sprintf("**Uncovered lines:** %v (relative to file)\n\n", uncovInFunc))

		// Branch analysis
		branches := analyzeBranches(fn.Body)
		if len(branches) > 0 {
			sb.WriteString("**Code branches (focus on uncovered ones):**\n")
			for _, b := range branches {
				sb.WriteString(fmt.Sprintf("- %s\n", b))
			}
			sb.WriteString("\n")
		}

		sb.WriteString("---\n\n")
	}

	// Existing test names (to avoid duplication)
	existingNames := ExtractTestFuncNames(req.ExistingTestCode)
	if len(existingNames) > 0 {
		sb.WriteString("## Existing Tests (DO NOT REDECLARE)\n\n")
		sb.WriteString("The test file already contains these functions:\n")
		for _, name := range existingNames {
			sb.WriteString(fmt.Sprintf("- `%s`\n", name))
		}
		sb.WriteString("\nDo NOT redeclare any of these. Your output will be merged automatically.\n\n")
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. Analyze which execution paths lead to the uncovered lines.\n")
	sb.WriteString("2. Write NEW test cases that exercise those specific paths.\n")
	sb.WriteString("3. Focus on edge cases, error conditions, and boundary values that weren't tested.\n")
	sb.WriteString("4. Return ONLY new test functions (valid Go file: package + imports + new test functions).\n")
	sb.WriteString("5. Return ONLY code — no explanations, no markdown wrappers.\n")

	return sb.String()
}

// Message represents a single message for the LLM API.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
