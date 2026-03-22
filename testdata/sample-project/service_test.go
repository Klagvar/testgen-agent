package sample

import (
	"errors"
	"testing"
)

func TestNewMemoryStorage(t *testing.T) {
	got := NewMemoryStorage()
	if got == nil {
		t.Fatal("NewMemoryStorage() returned nil")
	}
	if got.data == nil {
		t.Error("NewMemoryStorage() returned MemoryStorage with nil data map")
	}
}

func TestMemoryStorage_Get(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		data      map[string]string
		want      string
		wantErr   error
		wantErrIs error
	}{
		{
			name:      "happy path",
			key:       "key1",
			data:      map[string]string{"key1": "value1"},
			want:      "value1",
			wantErr:   nil,
			wantErrIs: nil,
		},
		{
			name:      "key not found",
			key:       "nonexistent",
			data:      map[string]string{"key1": "value1"},
			want:      "",
			wantErr:   nil,
			wantErrIs: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &MemoryStorage{data: tt.data}
			got, err := ms.Get(tt.key)
			if got != tt.want {
				t.Errorf("MemoryStorage.Get() got = %v, want %v", got, tt.want)
			}
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("MemoryStorage.Get() error = %v, wantErrIs %v", err, tt.wantErrIs)
				}
			} else if err != tt.wantErr {
				t.Errorf("MemoryStorage.Get() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestMemoryStorage_Set(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		data      map[string]string
		wantErr   error
		wantErrIs error
	}{
		{
			name:      "happy path",
			key:       "key1",
			value:     "value1",
			data:      map[string]string{},
			wantErr:   nil,
			wantErrIs: nil,
		},
		{
			name:      "empty key",
			key:       "",
			value:     "value1",
			data:      map[string]string{},
			wantErr:   nil,
			wantErrIs: ErrValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &MemoryStorage{data: tt.data}
			err := ms.Set(tt.key, tt.value)
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("MemoryStorage.Set() error = %v, wantErrIs %v", err, tt.wantErrIs)
				}
			} else if err != tt.wantErr {
				t.Errorf("MemoryStorage.Set() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestMemoryStorage_Delete(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		data      map[string]string
		wantErr   error
		wantErrIs error
	}{
		{
			name:      "happy path",
			key:       "key1",
			data:      map[string]string{"key1": "value1"},
			wantErr:   nil,
			wantErrIs: nil,
		},
		{
			name:      "key not found",
			key:       "nonexistent",
			data:      map[string]string{"key1": "value1"},
			wantErr:   nil,
			wantErrIs: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &MemoryStorage{data: tt.data}
			err := ms.Delete(tt.key)
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("MemoryStorage.Delete() error = %v, wantErrIs %v", err, tt.wantErrIs)
				}
			} else if err != tt.wantErr {
				t.Errorf("MemoryStorage.Delete() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestProcessWithStorage(t *testing.T) {
	tests := []struct {
		name      string
		storage   Storage
		key       string
		want      string
		wantErr   error
		wantErrIs error
	}{
		{
			name: "happy path",
			storage: &mockStorage{
				GetFunc: func(key string) (string, error) {
					return "value1", nil
				},
			},
			key:     "key1",
			want:    "processed:value1",
			wantErr: nil,
		},
		{
			name: "error from storage",
			storage: &mockStorage{
				GetFunc: func(key string) (string, error) {
					return "", ErrNotFound
				},
			},
			key:       "key1",
			want:      "",
			wantErr:   nil,
			wantErrIs: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProcessWithStorage(tt.storage, tt.key)
			if got != tt.want {
				t.Errorf("ProcessWithStorage() got = %v, want %v", got, tt.want)
			}
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("ProcessWithStorage() error = %v, wantErrIs %v", err, tt.wantErrIs)
				}
			} else if err != tt.wantErr {
				t.Errorf("ProcessWithStorage() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// mockStorage is a mock implementation of Storage for testing.
type mockStorage struct {
	GetFunc    func(key string) (string, error)
	SetFunc    func(key string, value string) error
	DeleteFunc func(key string) error
}

func (m *mockStorage) Get(key string) (string, error) {
	return m.GetFunc(key)
}

func (m *mockStorage) Set(key string, value string) error {
	return m.SetFunc(key, value)
}

func (m *mockStorage) Delete(key string) error {
	return m.DeleteFunc(key)
}
