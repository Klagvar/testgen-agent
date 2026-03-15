// Package cache реализует кэширование результатов генерации тестов.
// Для каждой функции хранится хэш (сигнатура + тело + зависимости).
// Если при следующем запуске хэш совпадает — функция пропускается,
// LLM не вызывается, экономятся токены и время.
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

// FuncEntry — запись кэша для одной функции.
type FuncEntry struct {
	Hash           string    `json:"hash"`            // SHA256 от (сигнатура + тело + типы)
	TestFile       string    `json:"test_file"`       // путь к файлу тестов
	GeneratedFuncs []string  `json:"generated_funcs"` // имена сгенерированных тест-функций
	Model          string    `json:"model"`           // модель, которая генерировала
	Timestamp      time.Time `json:"timestamp"`       // когда сгенерировано
}

// Cache — хранилище кэша. Ключ: "file.go::FuncName".
type Cache struct {
	Version string                `json:"version"`
	Entries map[string]FuncEntry  `json:"entries"`
	path    string                // путь к файлу кэша
}

// Load загружает кэш из файла. Если файла нет — возвращает пустой кэш.
func Load(repoDir string) *Cache {
	c := &Cache{
		Version: "1",
		Entries: make(map[string]FuncEntry),
		path:    filepath.Join(repoDir, cacheFileName),
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		return c // файла нет — пустой кэш
	}

	if err := json.Unmarshal(data, c); err != nil {
		return c // битый файл — пустой кэш
	}

	return c
}

// Save записывает кэш в файл.
func (c *Cache) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	return os.WriteFile(c.path, data, 0644)
}

// Key формирует ключ кэша для функции в файле.
func Key(filePath string, funcName string) string {
	return filepath.Base(filePath) + "::" + funcName
}

// ComputeHash вычисляет хэш функции на основе её содержимого и зависимостей.
// Учитывает: сигнатуру, тело, ресивер, типы параметров и возвратов.
func ComputeHash(fn analyzer.FuncInfo, usedTypes []analyzer.TypeInfo) string {
	h := sha256.New()

	// Сигнатура (включает ресивер, параметры, возвраты)
	h.Write([]byte(fn.Signature))

	// Тело функции
	h.Write([]byte(fn.Body))

	// Ресивер
	h.Write([]byte(fn.Receiver))

	// Типы-зависимости (сортируем для стабильности)
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

// Lookup проверяет, есть ли актуальная запись в кэше для функции.
// Возвращает entry и true если хэш совпадает (можно пропустить генерацию).
func (c *Cache) Lookup(key string, currentHash string) (FuncEntry, bool) {
	entry, exists := c.Entries[key]
	if !exists {
		return FuncEntry{}, false
	}

	if entry.Hash != currentHash {
		return FuncEntry{}, false // хэш изменился — нужна перегенерация
	}

	return entry, true
}

// Put записывает или обновляет запись в кэше.
func (c *Cache) Put(key string, entry FuncEntry) {
	c.Entries[key] = entry
}

// Remove удаляет запись из кэша.
func (c *Cache) Remove(key string) {
	delete(c.Entries, key)
}

// Invalidate удаляет все записи для указанного файла.
func (c *Cache) Invalidate(filePath string) {
	base := filepath.Base(filePath)
	for key := range c.Entries {
		if len(key) > len(base)+2 && key[:len(base)+2] == base+"::" {
			delete(c.Entries, key)
		}
	}
}

// Stats возвращает статистику кэша.
func (c *Cache) Stats() (total, expired int) {
	total = len(c.Entries)
	cutoff := time.Now().Add(-30 * 24 * time.Hour) // 30 дней
	for _, entry := range c.Entries {
		if entry.Timestamp.Before(cutoff) {
			expired++
		}
	}
	return
}

// Prune удаляет записи старше maxAge.
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

// FilePath возвращает путь к файлу кэша.
func (c *Cache) FilePath() string {
	return c.path
}
