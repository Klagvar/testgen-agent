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

const genericsCode = `package generic

type Set[T comparable] struct {
	items map[T]struct{}
}

func Map[T, U any](items []T, fn func(T) U) []U {
	result := make([]U, len(items))
	for i, item := range items {
		result[i] = fn(item)
	}
	return result
}

func Filter[T any](items []T, predicate func(T) bool) []T {
	var result []T
	for _, item := range items {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

func (s *Set[T]) Add(item T) {
	s.items[item] = struct{}{}
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{items: make(map[T]struct{})}
}
`

func TestAnalyzeSource_Generics_Functions(t *testing.T) {
	a, err := AnalyzeSource("generic.go", genericsCode)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	mapFn := findFunc(a.Functions, "Map")
	if mapFn == nil {
		t.Fatal("Map function not found")
	}
	if mapFn.TypeParams != "[T any, U any]" {
		t.Errorf("Map type params: got %q, want %q", mapFn.TypeParams, "[T any, U any]")
	}
	expected := "func Map[T any, U any](items []T, fn func(T) U) []U"
	if mapFn.Signature != expected {
		t.Errorf("Map signature:\n  got  %q\n  want %q", mapFn.Signature, expected)
	}

	filterFn := findFunc(a.Functions, "Filter")
	if filterFn == nil {
		t.Fatal("Filter function not found")
	}
	if filterFn.TypeParams != "[T any]" {
		t.Errorf("Filter type params: got %q, want %q", filterFn.TypeParams, "[T any]")
	}
}

func TestAnalyzeSource_Generics_Types(t *testing.T) {
	a, err := AnalyzeSource("generic.go", genericsCode)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	var setType *TypeInfo
	for i := range a.Types {
		if a.Types[i].Name == "Set" {
			setType = &a.Types[i]
			break
		}
	}
	if setType == nil {
		t.Fatal("Set type not found")
	}
	if setType.TypeParams != "[T comparable]" {
		t.Errorf("Set type params: got %q, want %q", setType.TypeParams, "[T comparable]")
	}
	if setType.Kind != "struct" {
		t.Errorf("Set kind: got %q, want struct", setType.Kind)
	}
}

func TestAnalyzeSource_Generics_MethodWithTypeParam(t *testing.T) {
	a, err := AnalyzeSource("generic.go", genericsCode)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	addFn := findFunc(a.Functions, "Add")
	if addFn == nil {
		t.Fatal("Add method not found")
	}
	if addFn.Receiver != "*Set[T]" {
		t.Errorf("Add receiver: got %q, want %q", addFn.Receiver, "*Set[T]")
	}
}

func TestAnalyzeSource_Generics_Constructor(t *testing.T) {
	a, err := AnalyzeSource("generic.go", genericsCode)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := findFunc(a.Functions, "NewSet")
	if fn == nil {
		t.Fatal("NewSet not found")
	}
	if fn.TypeParams != "[T comparable]" {
		t.Errorf("NewSet type params: got %q, want %q", fn.TypeParams, "[T comparable]")
	}
	expected := "func NewSet[T comparable]() *Set[T]"
	if fn.Signature != expected {
		t.Errorf("NewSet signature:\n  got  %q\n  want %q", fn.Signature, expected)
	}
}

func TestFilterTestable(t *testing.T) {
	funcs := []FuncInfo{
		{Name: "init"},
		{Name: "Add"},
		{Name: "init"},
		{Name: "Process"},
	}
	result := FilterTestable(funcs)
	if len(result) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(result))
	}
	if result[0].Name != "Add" || result[1].Name != "Process" {
		t.Errorf("unexpected functions: %v", result)
	}
}

func TestFilterTestable_NoInit(t *testing.T) {
	funcs := []FuncInfo{{Name: "Add"}, {Name: "Sub"}}
	result := FilterTestable(funcs)
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestDetectBuildTag_GoBuild(t *testing.T) {
	src := "//go:build linux\n\npackage main\n"
	tag := DetectBuildTag(src)
	if tag != "linux" {
		t.Errorf("expected linux, got %q", tag)
	}
}

func TestDetectBuildTag_PlusBuild(t *testing.T) {
	src := "// +build windows\n\npackage main\n"
	tag := DetectBuildTag(src)
	if tag != "windows" {
		t.Errorf("expected windows, got %q", tag)
	}
}

func TestDetectBuildTag_None(t *testing.T) {
	src := "package main\n\nfunc main() {}\n"
	tag := DetectBuildTag(src)
	if tag != "" {
		t.Errorf("expected empty, got %q", tag)
	}
}

func TestExprToString_IndexExpr(t *testing.T) {
	a, err := AnalyzeSource("generic.go", genericsCode)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	fn := findFunc(a.Functions, "NewSet")
	if fn == nil {
		t.Fatal("NewSet not found")
	}
	if len(fn.Returns) != 1 || fn.Returns[0] != "*Set[T]" {
		t.Errorf("NewSet returns: got %v, want [*Set[T]]", fn.Returns)
	}
}
