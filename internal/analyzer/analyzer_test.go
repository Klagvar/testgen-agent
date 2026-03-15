package analyzer

import (
	"testing"
)

const sampleCode = `package calc

import (
	"errors"
	"math"
)

// Add складывает два числа. Поддерживает проверку на переполнение.
func Add(a, b int) (int, error) {
	result := a + b
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, errors.New("integer overflow")
	}
	return result, nil
}

// Subtract вычитает b из a.
func Subtract(a, b int) int {
	return a - b
}

// Divide делит a на b.
func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

// Multiply умножает два числа.
func Multiply(a, b int) int {
	return a * b
}

// Sqrt возвращает квадратный корень числа.
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
		t.Fatalf("AnalyzeSource вернул ошибку: %v", err)
	}

	if analysis.Package != "calc" {
		t.Errorf("Package = %q, ожидалось %q", analysis.Package, "calc")
	}
}

func TestAnalyzeSource_Imports(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource вернул ошибку: %v", err)
	}

	if len(analysis.Imports) != 2 {
		t.Fatalf("ожидалось 2 импорта, получено %d: %v", len(analysis.Imports), analysis.Imports)
	}

	expected := map[string]bool{"errors": true, "math": true}
	for _, imp := range analysis.Imports {
		if !expected[imp] {
			t.Errorf("неожиданный импорт: %q", imp)
		}
	}
}

func TestAnalyzeSource_Functions(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource вернул ошибку: %v", err)
	}

	if len(analysis.Functions) != 5 {
		t.Fatalf("ожидалось 5 функций, получено %d", len(analysis.Functions))
	}

	// Проверяем имена
	names := make([]string, len(analysis.Functions))
	for i, fn := range analysis.Functions {
		names[i] = fn.Name
	}
	t.Logf("Функции: %v", names)

	expectedNames := []string{"Add", "Subtract", "Divide", "Multiply", "Sqrt"}
	for i, expected := range expectedNames {
		if names[i] != expected {
			t.Errorf("функция %d: имя = %q, ожидалось %q", i, names[i], expected)
		}
	}
}

func TestAnalyzeSource_FuncSignature(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource вернул ошибку: %v", err)
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
				t.Fatalf("функция %q не найдена", tt.name)
			}

			if fn.Signature != tt.wantSig {
				t.Errorf("сигнатура = %q, ожидалось %q", fn.Signature, tt.wantSig)
			}
			if len(fn.Params) != tt.wantParams {
				t.Errorf("параметров = %d, ожидалось %d", len(fn.Params), tt.wantParams)
			}
			if len(fn.Returns) != tt.wantReturns {
				t.Errorf("возвратов = %d, ожидалось %d", len(fn.Returns), tt.wantReturns)
			}
		})
	}
}

func TestAnalyzeSource_DocComment(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource вернул ошибку: %v", err)
	}

	addFunc := analysis.Functions[0]
	if addFunc.DocComment == "" {
		t.Error("Add должна иметь DocComment")
	}
	t.Logf("Add DocComment: %q", addFunc.DocComment)
}

func TestAnalyzeSource_LineNumbers(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource вернул ошибку: %v", err)
	}

	// Add начинается где-то на строке 9 и заканчивается ~15
	addFunc := analysis.Functions[0]
	t.Logf("Add: строки %d-%d", addFunc.StartLine, addFunc.EndLine)

	if addFunc.StartLine < 5 || addFunc.StartLine > 15 {
		t.Errorf("Add StartLine = %d, выглядит некорректно", addFunc.StartLine)
	}
	if addFunc.EndLine <= addFunc.StartLine {
		t.Errorf("Add EndLine (%d) <= StartLine (%d)", addFunc.EndLine, addFunc.StartLine)
	}
}

func TestFindFunctionsByLines(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource вернул ошибку: %v", err)
	}

	// Логируем позиции всех функций
	for _, fn := range analysis.Functions {
		t.Logf("%s: строки %d-%d", fn.Name, fn.StartLine, fn.EndLine)
	}

	// Имитируем изменённые строки из diff:
	// Строки функции Add + строки новых функций Multiply и Sqrt
	// Нам нужно узнать реальные номера строк из анализа
	addFunc := analysis.Functions[0]       // Add
	multiplyFunc := analysis.Functions[3]  // Multiply
	sqrtFunc := analysis.Functions[4]      // Sqrt

	changedLines := []int{
		addFunc.StartLine,     // затрагивает Add
		addFunc.StartLine + 1, // тоже Add
		multiplyFunc.StartLine, // затрагивает Multiply
		sqrtFunc.StartLine + 1, // затрагивает Sqrt
	}

	found := FindFunctionsByLines(analysis, changedLines)

	if len(found) != 3 {
		names := make([]string, len(found))
		for i, fn := range found {
			names[i] = fn.Name
		}
		t.Fatalf("ожидалось 3 функции, получено %d: %v", len(found), names)
	}

	// Проверяем что нашли Add, Multiply и Sqrt (но НЕ Subtract и Divide)
	foundNames := map[string]bool{}
	for _, fn := range found {
		foundNames[fn.Name] = true
	}

	for _, expected := range []string{"Add", "Multiply", "Sqrt"} {
		if !foundNames[expected] {
			t.Errorf("функция %q не найдена среди затронутых", expected)
		}
	}
	for _, notExpected := range []string{"Subtract", "Divide"} {
		if foundNames[notExpected] {
			t.Errorf("функция %q не должна быть среди затронутых", notExpected)
		}
	}
}

func TestAnalyzeSource_Body(t *testing.T) {
	analysis, err := AnalyzeSource("calc.go", sampleCode)
	if err != nil {
		t.Fatalf("AnalyzeSource вернул ошибку: %v", err)
	}

	multiplyFunc := analysis.Functions[3] // Multiply
	t.Logf("Multiply body:\n%s", multiplyFunc.Body)

	if multiplyFunc.Body == "" {
		t.Error("Multiply должна иметь тело")
	}

	// Тело должно содержать return a * b
	if !contains(multiplyFunc.Body, "return a * b") {
		t.Errorf("тело Multiply не содержит 'return a * b'")
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

// ─── Тесты для TypeInfo и PackageAnalysis ───

const sampleCodeWithTypes = `package service

import "errors"

// Config содержит настройки сервиса.
type Config struct {
	Host    string
	Port    int
	Verbose bool
}

// Repository — интерфейс доступа к данным.
type Repository interface {
	Get(id string) (*Entity, error)
	Save(entity *Entity) error
	Delete(id string) error
}

// Entity — доменная сущность.
type Entity struct {
	ID   string
	Name string
}

// Status — алиас для строки.
type Status string

// Service предоставляет бизнес-логику.
type Service struct {
	config *Config
	repo   Repository
}

// NewService создаёт новый сервис.
func NewService(cfg *Config, repo Repository) *Service {
	return &Service{config: cfg, repo: repo}
}

// GetEntity получает сущность по ID.
func (s *Service) GetEntity(id string) (*Entity, error) {
	if id == "" {
		return nil, errors.New("empty id")
	}
	return s.repo.Get(id)
}

// helper — вспомогательная функция.
func helper(s string) string {
	return s + "_processed"
}

// ProcessEntity обрабатывает сущность.
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
