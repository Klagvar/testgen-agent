package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

func TestComputeHash_Stable(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:      "Add",
		Signature: "func Add(a int, b int) int",
		Body:      "func Add(a, b int) int {\n\treturn a + b\n}",
		Receiver:  "",
	}

	hash1 := ComputeHash(fn, nil)
	hash2 := ComputeHash(fn, nil)

	if hash1 != hash2 {
		t.Errorf("same input should produce same hash: %s != %s", hash1, hash2)
	}

	if len(hash1) != 64 { // SHA256 hex = 64 chars
		t.Errorf("hash length = %d, want 64", len(hash1))
	}
}

func TestComputeHash_ChangesOnBodyEdit(t *testing.T) {
	fn1 := analyzer.FuncInfo{
		Name:      "Add",
		Signature: "func Add(a int, b int) int",
		Body:      "func Add(a, b int) int {\n\treturn a + b\n}",
	}

	fn2 := analyzer.FuncInfo{
		Name:      "Add",
		Signature: "func Add(a int, b int) int",
		Body:      "func Add(a, b int) int {\n\treturn a + b + 1\n}", // changed
	}

	hash1 := ComputeHash(fn1, nil)
	hash2 := ComputeHash(fn2, nil)

	if hash1 == hash2 {
		t.Error("different body should produce different hash")
	}
}

func TestComputeHash_ChangesOnTypeDep(t *testing.T) {
	fn := analyzer.FuncInfo{
		Name:      "NewService",
		Signature: "func NewService(cfg *Config) *Service",
		Body:      "func NewService(cfg *Config) *Service {\n\treturn &Service{cfg: cfg}\n}",
	}

	types1 := []analyzer.TypeInfo{
		{Name: "Config", Source: "type Config struct {\n\tHost string\n}"},
	}

	types2 := []analyzer.TypeInfo{
		{Name: "Config", Source: "type Config struct {\n\tHost string\n\tPort int\n}"}, // added field
	}

	hash1 := ComputeHash(fn, types1)
	hash2 := ComputeHash(fn, types2)

	if hash1 == hash2 {
		t.Error("different type definitions should produce different hash")
	}
}

func TestKey(t *testing.T) {
	key := Key("internal/calc/calc.go", "Add")
	if key != "internal/calc/calc.go::Add" {
		t.Errorf("Key = %q, want 'internal/calc/calc.go::Add'", key)
	}
}

func TestKey_NoDuplicateForSameBaseName(t *testing.T) {
	k1 := Key("pkg/a/utils.go", "Parse")
	k2 := Key("pkg/b/utils.go", "Parse")
	if k1 == k2 {
		t.Errorf("keys should differ for different paths: %q == %q", k1, k2)
	}
}

func TestCache_LoadSave(t *testing.T) {
	dir := t.TempDir()

	// Save
	c := Load(dir)
	c.Put("calc.go::Add", FuncEntry{
		Hash:           "abc123",
		TestFile:       "calc_test.go",
		GeneratedFuncs: []string{"TestAdd_HappyPath", "TestAdd_Overflow"},
		Model:          "qwen3-coder:30b",
		Timestamp:      time.Now(),
	})

	if err := c.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify file exists
	cachePath := filepath.Join(dir, cacheFileName)
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	// Load
	c2 := Load(dir)
	if len(c2.Entries) != 1 {
		t.Fatalf("loaded %d entries, want 1", len(c2.Entries))
	}

	entry, ok := c2.Entries["calc.go::Add"]
	if !ok {
		t.Fatal("calc.go::Add not found in loaded cache")
	}
	if entry.Hash != "abc123" {
		t.Errorf("Hash = %q, want abc123", entry.Hash)
	}
	if len(entry.GeneratedFuncs) != 2 {
		t.Errorf("GeneratedFuncs = %d, want 2", len(entry.GeneratedFuncs))
	}
}

func TestCache_Lookup(t *testing.T) {
	c := &Cache{Entries: make(map[string]FuncEntry)}

	c.Put("calc.go::Add", FuncEntry{
		Hash: "abc123",
	})

	// Matching hash
	entry, ok := c.Lookup("calc.go::Add", "abc123")
	if !ok {
		t.Error("expected cache hit")
	}
	if entry.Hash != "abc123" {
		t.Errorf("Hash = %q", entry.Hash)
	}

	// Different hash
	_, ok = c.Lookup("calc.go::Add", "different")
	if ok {
		t.Error("expected cache miss for different hash")
	}

	// Missing key
	_, ok = c.Lookup("calc.go::Sub", "abc123")
	if ok {
		t.Error("expected cache miss for missing key")
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := &Cache{Entries: make(map[string]FuncEntry)}

	c.Put("pkg/calc/calc.go::Add", FuncEntry{Hash: "a"})
	c.Put("pkg/calc/calc.go::Sub", FuncEntry{Hash: "b"})
	c.Put("pkg/utils/utils.go::Helper", FuncEntry{Hash: "c"})

	c.Invalidate("pkg/calc/calc.go")

	if len(c.Entries) != 1 {
		t.Errorf("after invalidate: %d entries, want 1", len(c.Entries))
	}

	if _, ok := c.Entries["pkg/utils/utils.go::Helper"]; !ok {
		t.Error("pkg/utils/utils.go::Helper should survive invalidation")
	}
}

func TestCache_Prune(t *testing.T) {
	c := &Cache{Entries: make(map[string]FuncEntry)}

	c.Put("old", FuncEntry{
		Hash:      "a",
		Timestamp: time.Now().Add(-60 * 24 * time.Hour), // 60 days ago
	})
	c.Put("new", FuncEntry{
		Hash:      "b",
		Timestamp: time.Now(),
	})

	removed := c.Prune(30 * 24 * time.Hour) // 30 days

	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if len(c.Entries) != 1 {
		t.Errorf("entries = %d, want 1", len(c.Entries))
	}
}

func TestCache_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	c := Load(dir)

	if len(c.Entries) != 0 {
		t.Errorf("fresh cache should be empty, got %d entries", len(c.Entries))
	}
}

func TestCache_Stats(t *testing.T) {
	c := &Cache{Entries: make(map[string]FuncEntry)}

	c.Put("fresh", FuncEntry{Timestamp: time.Now()})
	c.Put("old", FuncEntry{Timestamp: time.Now().Add(-60 * 24 * time.Hour)})

	total, expired := c.Stats()
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if expired != 1 {
		t.Errorf("expired = %d, want 1", expired)
	}
}
