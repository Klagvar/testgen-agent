// Package cache implements caching of test generation results.
// For each function, a hash (signature + body + dependencies) is stored.
// If the hash matches on the next run, the function is skipped,
// saving LLM tokens and time.
package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gizatulin/testgen-agent/internal/analyzer"
)

const cacheFileName = ".testgen-cache.json"

// FuncEntry is a cache entry for a single function.
type FuncEntry struct {
	Hash           string    `json:"hash"`            // SHA256 of (signature + body + types)
	TestFile       string    `json:"test_file"`       // path to the test file
	GeneratedFuncs []string  `json:"generated_funcs"` // names of generated test functions
	Model          string    `json:"model"`           // model that generated the tests
	Timestamp      time.Time `json:"timestamp"`       // when generated
}

// Cache is the cache store. Key: "file.go::FuncName".
type Cache struct {
	Version string                `json:"version"`
	Entries map[string]FuncEntry  `json:"entries"`
	path    string                // path to the cache file
}

// Load reads the cache from a file. Returns an empty cache if file is missing.
func Load(repoDir string) *Cache {
	c := &Cache{
		Version: "1",
		Entries: make(map[string]FuncEntry),
		path:    filepath.Join(repoDir, cacheFileName),
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		return c // no file — empty cache
	}

	if err := json.Unmarshal(data, c); err != nil {
		return c // corrupted file — empty cache
	}

	return c
}

// Save writes the cache to a file.
func (c *Cache) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	return os.WriteFile(c.path, data, 0644)
}

// Key builds a cache key for a function in a file.
func Key(filePath string, funcName string) string {
	return filepath.Base(filePath) + "::" + funcName
}

// ComputeHash computes a hash for a function based on its content and dependencies.
// Considers: signature, body, receiver, parameter and return types.
func ComputeHash(fn analyzer.FuncInfo, usedTypes []analyzer.TypeInfo) string {
	h := sha256.New()

	// Signature (includes receiver, parameters, returns)
	h.Write([]byte(fn.Signature))

	// Function body
	h.Write([]byte(fn.Body))

	// Receiver
	h.Write([]byte(fn.Receiver))

	// Type dependencies (sorted for stability)
	typeHashes := make([]string, 0, len(usedTypes))
	for _, t := range usedTypes {
		typeHashes = append(typeHashes, t.Name+":"+t.Source)
	}
	sort.Strings(typeHashes)
	for _, th := range typeHashes {
		h.Write([]byte(th))
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// Lookup checks whether there is a valid cache entry for the function.
// Returns the entry and true if the hash matches (generation can be skipped).
func (c *Cache) Lookup(key string, currentHash string) (FuncEntry, bool) {
	entry, exists := c.Entries[key]
	if !exists {
		return FuncEntry{}, false
	}

	if entry.Hash != currentHash {
		return FuncEntry{}, false // hash changed — regeneration needed
	}

	return entry, true
}

// Put writes or updates a cache entry.
func (c *Cache) Put(key string, entry FuncEntry) {
	c.Entries[key] = entry
}

// Remove removes an entry from the cache.
func (c *Cache) Remove(key string) {
	delete(c.Entries, key)
}

// Invalidate removes all entries for the given file.
func (c *Cache) Invalidate(filePath string) {
	base := filepath.Base(filePath)
	for key := range c.Entries {
		if len(key) > len(base)+2 && key[:len(base)+2] == base+"::" {
			delete(c.Entries, key)
		}
	}
}

// Stats returns cache statistics.
func (c *Cache) Stats() (total, expired int) {
	total = len(c.Entries)
	cutoff := time.Now().Add(-30 * 24 * time.Hour) // 30 days
	for _, entry := range c.Entries {
		if entry.Timestamp.Before(cutoff) {
			expired++
		}
	}
	return
}

// Prune removes entries older than maxAge.
func (c *Cache) Prune(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for key, entry := range c.Entries {
		if entry.Timestamp.Before(cutoff) {
			delete(c.Entries, key)
			removed++
		}
	}
	return removed
}

// FilePath returns the path to the cache file.
func (c *Cache) FilePath() string {
	return c.path
}
