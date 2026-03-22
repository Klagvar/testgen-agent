package analyzer

import "testing"

func TestFindImplementors_FullMatch(t *testing.T) {
	iface := TypeInfo{
		Name: "Storage",
		Kind: "interface",
		Methods: []MethodInfo{
			{Name: "Get", Signature: "(key string) (string, error)"},
			{Name: "Set", Signature: "(key string, value string) error"},
		},
	}

	allTypes := []TypeInfo{
		iface,
		{Name: "FileStorage", Kind: "struct"},
		{Name: "MemStorage", Kind: "struct"},
	}

	allFuncs := []FuncInfo{
		{Name: "Get", Receiver: "*FileStorage", Params: []Param{{Name: "key", Type: "string"}}, Returns: []string{"string", "error"}},
		{Name: "Set", Receiver: "*FileStorage", Params: []Param{{Name: "key", Type: "string"}, {Name: "value", Type: "string"}}, Returns: []string{"error"}},
		{Name: "Get", Receiver: "MemStorage", Params: []Param{{Name: "key", Type: "string"}}, Returns: []string{"string", "error"}},
		{Name: "Set", Receiver: "MemStorage", Params: []Param{{Name: "key", Type: "string"}, {Name: "value", Type: "string"}}, Returns: []string{"error"}},
	}

	impls := FindImplementors(iface, allTypes, allFuncs)
	if len(impls) != 2 {
		t.Fatalf("expected 2 implementors, got %d", len(impls))
	}

	names := map[string]bool{}
	for _, ti := range impls {
		names[ti.Name] = true
	}
	if !names["FileStorage"] {
		t.Error("expected FileStorage to be an implementor")
	}
	if !names["MemStorage"] {
		t.Error("expected MemStorage to be an implementor")
	}
}

func TestFindImplementors_MissingMethod(t *testing.T) {
	iface := TypeInfo{
		Name: "Storage",
		Kind: "interface",
		Methods: []MethodInfo{
			{Name: "Get", Signature: "(key string) (string, error)"},
			{Name: "Set", Signature: "(key string, value string) error"},
		},
	}

	allTypes := []TypeInfo{
		iface,
		{Name: "ReadOnly", Kind: "struct"},
	}

	allFuncs := []FuncInfo{
		{Name: "Get", Receiver: "*ReadOnly", Params: []Param{{Name: "key", Type: "string"}}, Returns: []string{"string", "error"}},
	}

	impls := FindImplementors(iface, allTypes, allFuncs)
	if len(impls) != 0 {
		t.Fatalf("expected 0 implementors for partial match, got %d", len(impls))
	}
}

func TestFindImplementors_EmptyInterface(t *testing.T) {
	iface := TypeInfo{
		Name:    "Any",
		Kind:    "interface",
		Methods: nil,
	}

	allTypes := []TypeInfo{
		iface,
		{Name: "Foo", Kind: "struct"},
	}

	allFuncs := []FuncInfo{
		{Name: "DoStuff", Receiver: "*Foo"},
	}

	impls := FindImplementors(iface, allTypes, allFuncs)
	if impls != nil {
		t.Fatalf("expected nil for empty interface, got %d implementors", len(impls))
	}
}

func TestFindImplementors_NoSelfReference(t *testing.T) {
	iface := TypeInfo{
		Name: "Doer",
		Kind: "interface",
		Methods: []MethodInfo{
			{Name: "Do", Signature: "()"},
		},
	}

	allTypes := []TypeInfo{
		iface,
		{Name: "RealDoer", Kind: "struct"},
	}

	allFuncs := []FuncInfo{
		{Name: "Do", Receiver: "*RealDoer"},
	}

	impls := FindImplementors(iface, allTypes, allFuncs)
	for _, ti := range impls {
		if ti.Name == "Doer" {
			t.Fatal("interface should not implement itself")
		}
	}
	if len(impls) != 1 || impls[0].Name != "RealDoer" {
		t.Fatalf("expected [RealDoer], got %v", impls)
	}
}
