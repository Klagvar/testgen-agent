package analyzer

import (
	"testing"
)

const sampleCode = `package calc

import (
	"errors"
	"math"
)

// Add sums two numbers. Supports overflow checking.
func Add(a, b int) (int, error) {
	result := a + b
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, errors.New("integer overflow")
	}
	return result, nil
}

// Subtract subtracts b from a.
func Subtract(a, b int) int {
	return a - b
}

// Divide divides a by b.
func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

// Multiply multiplies two numbers.
func Multiply(a, b int) int {
	return a * b
}

// Sqrt returns the square root of a number.
func Sqrt(x float64) (float64, error) {
	if x < 0 {
		return 0, errors.New("negative number")
	}
	return math.Sqrt(x), nil
}
`

func TestAnalyzeSource_Package(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource returned error: %v", err)
	}

	if analysis.Package != "calc" {
		t.Errorf("Package = %q, expected %q", analysis.Package, "calc")
	}
}

func TestAnalyzeSource_Imports(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource returned error: %v", err)
	}

	if len(analysis.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d: %v", len(analysis.Imports), analysis.Imports)
	}

	expected := map[string]bool{"errors": true, "math": true}
	for _, imp := range analysis.Imports {
		if !expected[imp] {
			t.Errorf("unexpected import: %q", imp)
		}
	}
}

func TestAnalyzeSource_Functions(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource returned error: %v", err)
	}

	if len(analysis.Functions) != 5 {
		t.Fatalf("expected 5 functions, got %d", len(analysis.Functions))
	}

	// Check names
	names := make([]string, len(analysis.Functions))
	for i, fn := range analysis.Functions {
		names[i] = fn.Name
	}
	t.Logf("Functions: %v", names)

	expectedNames := []string{"Add", "Subtract", "Divide", "Multiply", "Sqrt"}
	for i, expected := range expectedNames {
		if names[i] != expected {
			t.Errorf("function %d: name = %q, expected %q", i, names[i], expected)
		}
	}
}

func TestAnalyzeSource_FuncSignature(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource returned error: %v", err)
	}

	tests := []struct {
		name      string
		wantSig   string
		wantParams int
		wantReturns int
	}{
		{"Add", "func Add(a int, b int) (int, error)", 2, 2},
		{"Subtract", "func Subtract(a int, b int) int", 2, 1},
		{"Divide", "func Divide(a int, b int) (int, error)", 2, 2},
		{"Multiply", "func Multiply(a int, b int) int", 2, 1},
		{"Sqrt", "func Sqrt(x float64) (float64, error)", 1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fn *FuncInfo
			for i := range analysis.Functions {
				if analysis.Functions[i].Name == tt.name {
					fn = &analysis.Functions[i]
					break
				}
			}
			if fn == nil {
				t.Fatalf("function %q not found", tt.name)
			}

			if fn.Signature != tt.wantSig {
				t.Errorf("signature = %q, expected %q", fn.Signature, tt.wantSig)
			}
			if len(fn.Params) != tt.wantParams {
				t.Errorf("params = %d, expected %d", len(fn.Params), tt.wantParams)
			}
			if len(fn.Returns) != tt.wantReturns {
				t.Errorf("returns = %d, expected %d", len(fn.Returns), tt.wantReturns)
			}
		})
	}
}

func TestAnalyzeSource_DocComment(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource returned error: %v", err)
	}

	addFunc := analysis.Functions[0]
	if addFunc.DocComment == "" {
		t.Error("Add should have DocComment")
	}
	t.Logf("Add DocComment: %q", addFunc.DocComment)
}

func TestAnalyzeSource_LineNumbers(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource returned error: %v", err)
	}

	// Add starts around line 9 and ends ~15
	addFunc := analysis.Functions[0]
	t.Logf("Add: lines %d-%d", addFunc.StartLine, addFunc.EndLine)

	if addFunc.StartLine < 5 || addFunc.StartLine > 15 {
		t.Errorf("Add StartLine = %d, looks incorrect", addFunc.StartLine)
	}
	if addFunc.EndLine <= addFunc.StartLine {
		t.Errorf("Add EndLine (%d) <= StartLine (%d)", addFunc.EndLine, addFunc.StartLine)
	}
}

func TestFindFunctionsByLines(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource returned error: %v", err)
	}

	// Log all function positions
	for _, fn := range analysis.Functions {
		t.Logf("%s: lines %d-%d", fn.Name, fn.StartLine, fn.EndLine)
	}

	// Simulate changed lines from diff:
	// Lines of function Add + lines of new functions Multiply and Sqrt
	// We need the actual line numbers from the analysis
	addFunc := analysis.Functions[0]       // Add
	multiplyFunc := analysis.Functions[3]  // Multiply
	sqrtFunc := analysis.Functions[4]      // Sqrt

	changedLines := []int{
		addFunc.StartLine,     // affects Add
		addFunc.StartLine + 1, // also Add
		multiplyFunc.StartLine, // affects Multiply
		sqrtFunc.StartLine + 1, // affects Sqrt
	}

	found := FindFunctionsByLines(analysis, changedLines)

	if len(found) != 3 {
		names := make([]string, len(found))
		for i, fn := range found {
			names[i] = fn.Name
		}
		t.Fatalf("expected 3 functions, got %d: %v", len(found), names)
	}

	// Check that we found Add, Multiply and Sqrt (but NOT Subtract and Divide)
	foundNames := map[string]bool{}
	for _, fn := range found {
		foundNames[fn.Name] = true
	}

	for _, expected := range []string{"Add", "Multiply", "Sqrt"} {
		if !foundNames[expected] {
			t.Errorf("function %q not found among affected", expected)
		}
	}
	for _, notExpected := range []string{"Subtract", "Divide"} {
		if foundNames[notExpected] {
			t.Errorf("function %q should not be among affected", notExpected)
		}
	}
}

func TestAnalyzeSource_Body(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource returned error: %v", err)
	}

	multiplyFunc := analysis.Functions[3] // Multiply
	t.Logf("Multiply body:\n%s", multiplyFunc.Body)

	if multiplyFunc.Body == "" {
		t.Error("Multiply should have body")
	}

	// Body should contain return a * b
	if !contains(multiplyFunc.Body, "return a * b") {
		t.Errorf("Multiply body does not contain 'return a * b'")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstr(s, substr)
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ─── Tests for TypeInfo and PackageAnalysis ───

const sampleCodeWithTypes = `package service

import "errors"

// Config holds service settings.
type Config struct {
	Host    string
	Port    int
	Verbose bool
}

// Repository is the data access interface.
type Repository interface {
	Get(id string) (*Entity, error)
	Save(entity *Entity) error
	Delete(id string) error
}

// Entity is a domain entity.
type Entity struct {
	ID   string
	Name string
}

// Status is a string alias.
type Status string

// Service provides business logic.
type Service struct {
	config *Config
	repo   Repository
}

// NewService creates a new service.
func NewService(cfg *Config, repo Repository) *Service {
	return &Service{config: cfg, repo: repo}
}

// GetEntity retrieves an entity by ID.
func (s *Service) GetEntity(id string) (*Entity, error) {
	if id == "" {
		return nil, errors.New("empty id")
	}
	return s.repo.Get(id)
}

// helper is a utility function.
func helper(s string) string {
	return s + "_processed"
}

// ProcessEntity processes an entity.
func (s *Service) ProcessEntity(id string) (string, error) {
	entity, err := s.GetEntity(id)
	if err != nil {
		return "", err
	}
	return helper(entity.Name), nil
}
`

func TestAnalyzeSource_Types(t *testing.T) {
	analysis, err := AnalyzeSource("service.go", sampleCodeWithTypes)
	if err != nil {
		t.Fatalf("AnalyzeSource error: %v", err)
	}

	if len(analysis.Types) != 5 {
		names := make([]string, len(analysis.Types))
		for i, ti := range analysis.Types {
			names[i] = ti.Name
		}
		t.Fatalf("expected 5 types, got %d: %v", len(analysis.Types), names)
	}

	// Config = struct
	configType := findType(analysis.Types, "Config")
	if configType == nil {
		t.Fatal("Config type not found")
	}
	if configType.Kind != "struct" {
		t.Errorf("Config.Kind = %q, want struct", configType.Kind)
	}
	if len(configType.Fields) != 3 {
		t.Errorf("Config fields = %d, want 3", len(configType.Fields))
	}

	// Repository = interface
	repoType := findType(analysis.Types, "Repository")
	if repoType == nil {
		t.Fatal("Repository type not found")
	}
	if repoType.Kind != "interface" {
		t.Errorf("Repository.Kind = %q, want interface", repoType.Kind)
	}
	if len(repoType.Methods) != 3 {
		t.Errorf("Repository methods = %d, want 3", len(repoType.Methods))
	}
	t.Logf("Repository methods: %+v", repoType.Methods)

	// Status = alias
	statusType := findType(analysis.Types, "Status")
	if statusType == nil {
		t.Fatal("Status type not found")
	}
	if statusType.Kind != "alias" {
		t.Errorf("Status.Kind = %q, want alias", statusType.Kind)
	}
}

func TestAnalyzeSource_Receiver(t *testing.T) {
	analysis, err := AnalyzeSource("service.go", sampleCodeWithTypes)
	if err != nil {
		t.Fatalf("AnalyzeSource error: %v", err)
	}

	// GetEntity has receiver *Service
	getEntity := findFunc(analysis.Functions, "GetEntity")
	if getEntity == nil {
		t.Fatal("GetEntity not found")
	}
	if getEntity.Receiver != "*Service" {
		t.Errorf("GetEntity.Receiver = %q, want *Service", getEntity.Receiver)
	}
	t.Logf("GetEntity signature: %s", getEntity.Signature)

	// NewService has no receiver
	newService := findFunc(analysis.Functions, "NewService")
	if newService == nil {
		t.Fatal("NewService not found")
	}
	if newService.Receiver != "" {
		t.Errorf("NewService.Receiver = %q, want empty", newService.Receiver)
	}
}

func TestFindUsedTypes(t *testing.T) {
	analysis, err := AnalyzeSource("service.go", sampleCodeWithTypes)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// NewService takes *Config and Repository, returns *Service
	newService := findFunc(analysis.Functions, "NewService")
	if newService == nil {
		t.Fatal("NewService not found")
	}

	usedTypes := FindUsedTypes(*newService, analysis.Types)
	typeNames := make(map[string]bool)
	for _, ti := range usedTypes {
		typeNames[ti.Name] = true
	}

	t.Logf("Used types for NewService: %v", typeNames)

	if !typeNames["Config"] {
		t.Error("Config should be in used types")
	}
	if !typeNames["Repository"] {
		t.Error("Repository should be in used types")
	}
	if !typeNames["Service"] {
		t.Error("Service should be in used types")
	}
}

func TestFindCalledFunctions(t *testing.T) {
	analysis, err := AnalyzeSource("service.go", sampleCodeWithTypes)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	pkg := &PackageAnalysis{
		Package:   analysis.Package,
		FuncIndex: make(map[string]FuncInfo),
	}
	for _, fn := range analysis.Functions {
		pkg.FuncIndex[fn.Name] = fn
		if fn.Receiver != "" {
			recvName := fn.Receiver
			if len(recvName) > 0 && recvName[0] == '*' {
				recvName = recvName[1:]
			}
			pkg.FuncIndex[recvName+"."+fn.Name] = fn
		}
	}

	// ProcessEntity calls GetEntity and helper
	processEntity := findFunc(analysis.Functions, "ProcessEntity")
	if processEntity == nil {
		t.Fatal("ProcessEntity not found")
	}

	called := FindCalledFunctions(*processEntity, pkg)
	calledNames := make(map[string]bool)
	for _, fn := range called {
		calledNames[fn.Name] = true
	}

	t.Logf("ProcessEntity calls: %v", calledNames)

	if !calledNames["helper"] {
		t.Error("ProcessEntity should call helper")
	}
	if !calledNames["GetEntity"] {
		t.Error("ProcessEntity should call GetEntity")
	}
}

func findType(types []TypeInfo, name string) *TypeInfo {
	for i := range types {
		if types[i].Name == name {
			return &types[i]
		}
	}
	return nil
}

func findFunc(funcs []FuncInfo, name string) *FuncInfo {
	for i := range funcs {
		if funcs[i].Name == name {
			return &funcs[i]
		}
	}
	return nil
}
