package typed

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeModule writes a go.mod and the supplied source files to a temp dir and
// returns the directory path.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestLoad_BasicPackage(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": `package foo

type Greeter interface { Hello() string }

type English struct{}
func (English) Hello() string { return "hi" }
`,
	})
	res, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v (warnings: %v)", err, res)
	}
	if res.Package == nil {
		t.Fatal("expected Package to be populated")
	}
	if res.Package.Types.Scope().Lookup("Greeter") == nil {
		t.Error("Greeter should be in the package scope")
	}
}

func TestImplementors_StrictSignatureMatch(t *testing.T) {
	// Demonstrates that the type-based check rejects types whose method
	// name matches but whose signature doesn't.
	dir := writeModule(t, map[string]string{
		"greet.go": `package greet

type Greeter interface { Hello() string }

type English struct{}
func (English) Hello() string { return "hi" }

type Russian struct{}
func (*Russian) Hello() string { return "привет" }

type Wrong struct{}
func (Wrong) Hello(name string) string { return "hi " + name }

type NotGreeter struct{}
func (NotGreeter) Bye() string { return "bye" }
`,
	})
	res, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := res.Package.Implementors("Greeter")
	sort.Strings(got)
	want := []string{"English", "Russian"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("got[%d]=%s, want %s", i, got[i], name)
		}
	}
}

func TestImplementors_EmbeddedInterface(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"rw.go": `package rw

type Reader interface { Read() string }
type Writer interface { Write(s string) }
type ReadWriter interface { Reader; Writer }

type Impl struct{}
func (Impl) Read() string  { return "x" }
func (Impl) Write(s string) {}

type OnlyRead struct{}
func (OnlyRead) Read() string { return "x" }
`,
	})
	res, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := res.Package.Implementors("ReadWriter")
	sort.Strings(got)
	want := []string{"Impl"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCallees_FreeFunctions(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"calls.go": `package calls

func Helper() string { return "" }
func Other() int { return 0 }
func Entry() {
	_ = Helper()
	_ = Other()
	_ = Entry() // recursion, must be skipped
}
`,
	})
	res, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	callees := res.Package.Callees("Entry")
	if len(callees) != 2 {
		t.Fatalf("got %d callees, want 2: %+v", len(callees), callees)
	}
	names := map[string]bool{}
	for _, c := range callees {
		names[c.Name] = true
		if !c.SamePackage {
			t.Errorf("expected SamePackage=true for %s", c.Name)
		}
	}
	if !names["Helper"] || !names["Other"] {
		t.Errorf("missing expected callees: %+v", callees)
	}
	if names["Entry"] {
		t.Error("recursion should be skipped")
	}
}

func TestCallees_IgnoresFunctionTypedLocals(t *testing.T) {
	// A local variable of function type is *not* a real function call target
	// at the symbol level. Type-aware analysis must skip it, while the
	// name-based heuristic would happily include it.
	dir := writeModule(t, map[string]string{
		"calls.go": `package calls

func Real() string { return "r" }

func Entry() {
	var fn func() string = func() string { return "x" }
	_ = fn()    // callable but NOT a package function
	_ = Real()  // legitimate callee
}
`,
	})
	res, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	callees := res.Package.Callees("Entry")
	if len(callees) != 1 {
		t.Fatalf("got %d callees, want 1: %+v", len(callees), callees)
	}
	if callees[0].Name != "Real" {
		t.Errorf("expected Real, got %+v", callees[0])
	}
}

func TestCallees_MethodCallsReportReceiver(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"svc.go": `package svc

type Cache struct{}
func (*Cache) Get(key string) string { return "" }

func Use() {
	c := &Cache{}
	_ = c.Get("k")
}
`,
	})
	res, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	callees := res.Package.Callees("Use")
	if len(callees) != 1 {
		t.Fatalf("got %d callees, want 1: %+v", len(callees), callees)
	}
	if callees[0].Name != "Get" || callees[0].Receiver != "Cache" {
		t.Errorf("expected Cache.Get, got %+v", callees[0])
	}
}

func TestCallees_ExternalPackageIsReportedWithSamePackageFalse(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"x.go": `package x

import "strings"

func Run() {
	_ = strings.ToUpper("hi")
}
`,
	})
	res, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	callees := res.Package.Callees("Run")
	if len(callees) != 1 {
		t.Fatalf("got %d callees, want 1: %+v", len(callees), callees)
	}
	if callees[0].Name != "ToUpper" || callees[0].SamePackage {
		t.Errorf("expected ToUpper with SamePackage=false, got %+v", callees[0])
	}
}

func TestCalleesOfMethod(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"svc.go": `package svc

type Service struct{}

func helper() string { return "h" }

func (s *Service) Do() string {
	return helper()
}
`,
	})
	res, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	callees := res.Package.CalleesOfMethod("Service", "Do")
	if len(callees) != 1 || callees[0].Name != "helper" {
		t.Fatalf("expected callee 'helper', got %+v", callees)
	}
}
