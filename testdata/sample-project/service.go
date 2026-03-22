package sample

import "fmt"

type Storage interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

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

func ProcessWithStorage(s Storage, key string) (string, error) {
	val, err := s.Get(key)
	if err != nil {
		return "", fmt.Errorf("process failed: %w", err)
	}
	return "processed:" + val, nil
}
