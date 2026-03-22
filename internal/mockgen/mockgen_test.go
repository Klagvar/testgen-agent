package mockgen

import (
	"strings"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

func TestGenerateMocks_Interface(t *testing.T) {
	types := []analyzer.TypeInfo{
		{
			Name: "Repository",
			Kind: "interface",
			Methods: []analyzer.MethodInfo{
				{Name: "Get", Signature: "(id string) (*Entity, error)"},
				{Name: "Save", Signature: "(entity *Entity) error"},
				{Name: "Delete", Signature: "(id string) error"},
			},
		},
	}

	mocks := GenerateMocks(types)
	if len(mocks) != 1 {
		t.Fatalf("expected 1 mock, got %d", len(mocks))
	}

	mock := mocks[0]
	if mock.MockName != "mockRepository" {
		t.Errorf("MockName = %q, want mockRepository", mock.MockName)
	}

	t.Logf("Generated mock:\n%s", mock.Code)

	// Should have function fields
	if !strings.Contains(mock.Code, "GetFunc func") {
		t.Error("mock should contain GetFunc field")
	}
	if !strings.Contains(mock.Code, "SaveFunc func") {
		t.Error("mock should contain SaveFunc field")
	}
	if !strings.Contains(mock.Code, "DeleteFunc func") {
		t.Error("mock should contain DeleteFunc field")
	}

	// Should have delegate methods
	if !strings.Contains(mock.Code, "func (m *mockRepository) Get(") {
		t.Error("mock should contain Get method")
	}
	if !strings.Contains(mock.Code, "func (m *mockRepository) Save(") {
		t.Error("mock should contain Save method")
	}
	if !strings.Contains(mock.Code, "func (m *mockRepository) Delete(") {
		t.Error("mock should contain Delete method")
	}

	// Body should delegate to the function field
	if !strings.Contains(mock.Code, "return m.GetFunc(id)") {
		t.Error("Get method should delegate to GetFunc")
	}
}

func TestGenerateMocks_SkipsNonInterface(t *testing.T) {
	types := []analyzer.TypeInfo{
		{Name: "Config", Kind: "struct"},
		{Name: "Status", Kind: "alias"},
	}

	mocks := GenerateMocks(types)
	if len(mocks) != 0 {
		t.Errorf("expected 0 mocks for non-interfaces, got %d", len(mocks))
	}
}

func TestGenerateMocks_EmptyInterface(t *testing.T) {
	types := []analyzer.TypeInfo{
		{Name: "Empty", Kind: "interface", Methods: nil},
	}

	mocks := GenerateMocks(types)
	if len(mocks) != 0 {
		t.Errorf("expected 0 mocks for empty interface, got %d", len(mocks))
	}
}

func TestGenerateMockCode(t *testing.T) {
	types := []analyzer.TypeInfo{
		{
			Name: "Reader",
			Kind: "interface",
			Methods: []analyzer.MethodInfo{
				{Name: "Read", Signature: "(p []byte) (int, error)"},
			},
		},
	}

	code := GenerateMockCode(types)
	if code == "" {
		t.Fatal("expected non-empty mock code")
	}

	if !strings.Contains(code, "type mockReader struct") {
		t.Error("should contain mockReader struct")
	}
	if !strings.Contains(code, "ReadFunc func(p []byte) (int, error)") {
		t.Error("should contain ReadFunc field")
	}

	t.Logf("Generated:\n%s", code)
}

func TestParseMethodSignature(t *testing.T) {
	tests := []struct {
		sig         string
		wantParams  int
		wantReturns int
	}{
		{"(id string) (*Entity, error)", 1, 2},
		{"(entity *Entity) error", 1, 1},
		{"() error", 0, 1},
		{"(a int, b int) int", 2, 1},
		{"()", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.sig, func(t *testing.T) {
			params, returns := parseMethodSignature(tt.sig)
			if len(params) != tt.wantParams {
				t.Errorf("params = %d, want %d (%+v)", len(params), tt.wantParams, params)
			}
			if len(returns) != tt.wantReturns {
				t.Errorf("returns = %d, want %d (%v)", len(returns), tt.wantReturns, returns)
			}
		})
	}
}
