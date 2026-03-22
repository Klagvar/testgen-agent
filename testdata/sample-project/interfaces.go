// Tests: embedded interface resolution, interface satisfaction analysis
package sample

import "fmt"

// Reader is a basic read interface.
type Reader interface {
	Read(data []byte) (int, error)
}

// Writer is a basic write interface.
type Writer interface {
	Write(data []byte) (int, error)
}

// ReadWriter embeds Reader and Writer (tests embedded resolution).
type ReadWriter interface {
	Reader
	Writer
}

// Closer adds a Close method.
type Closer interface {
	Close() error
}

// ReadWriteCloser embeds ReadWriter and Closer (tests recursive embedding).
type ReadWriteCloser interface {
	ReadWriter
	Closer
}

// Storage is a key-value interface (tests interface satisfaction).
type Storage interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// MemoryStorage implements Storage using a map.
type MemoryStorage struct {
	data map[string]string
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{data: make(map[string]string)}
}

func (m *MemoryStorage) Get(key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", fmt.Errorf("key %q: %w", key, ErrNotFound)
	}
	return v, nil
}

func (m *MemoryStorage) Set(key, value string) error {
	if key == "" {
		return fmt.Errorf("empty key: %w", ErrValidation)
	}
	m.data[key] = value
	return nil
}

func (m *MemoryStorage) Delete(key string) error {
	if _, ok := m.data[key]; !ok {
		return fmt.Errorf("key %q: %w", key, ErrNotFound)
	}
	delete(m.data, key)
	return nil
}

// ProcessStorage exercises a Storage interface (LLM should see MemoryStorage as implementor).
func ProcessStorage(s Storage, key, value string) error {
	if err := s.Set(key, value); err != nil {
		return err
	}
	got, err := s.Get(key)
	if err != nil {
		return err
	}
	if got != value {
		return fmt.Errorf("mismatch: got %q, want %q", got, value)
	}
	return nil
}
