package sample

import (
	"testing"
)

func TestNewSet(t *testing.T) {
	tests := []struct {
		name  string
		items []int
		want  Set[int]
	}{
		{
			name:  "empty set",
			items: []int{},
			want:  Set[int]{items: map[int]struct{}{}},
		},
		{
			name:  "single item",
			items: []int{42},
			want:  Set[int]{items: map[int]struct{}{42: {}}},
		},
		{
			name:  "multiple items",
			items: []int{1, 2, 3, 2, 1},
			want:  Set[int]{items: map[int]struct{}{1: {}, 2: {}, 3: {}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSet(tt.items...)
			if len(got.items) != len(tt.want.items) {
				t.Errorf("NewSet() = %v, want %v", len(got.items), len(tt.want.items))
				return
			}
			for item := range got.items {
				if _, exists := tt.want.items[item]; !exists {
					t.Errorf("NewSet() = %v, want %v", got, tt.want)
					return
				}
			}
		})
	}
}

func TestSet_Add(t *testing.T) {
	tests := []struct {
		name string
		item int
		want bool
	}{
		{
			name: "add new item",
			item: 42,
			want: true,
		},
		{
			name: "add existing item",
			item: 1,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSet(1, 2, 3)
			got := s.Add(tt.item)
			if got != tt.want {
				t.Errorf("Set.Add() = %v, want %v", got, tt.want)
			}
			// Verify the set state
			if tt.want {
				if !s.Contains(tt.item) {
					t.Errorf("Set.Add() should have added item %v", tt.item)
				}
			} else {
				if !s.Contains(1) {
					t.Errorf("Set.Add() should not have modified existing item 1")
				}
			}
		})
	}
}

func TestSet_Contains(t *testing.T) {
	tests := []struct {
		name string
		item int
		want bool
	}{
		{
			name: "contains existing item",
			item: 2,
			want: true,
		},
		{
			name: "does not contain item",
			item: 42,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSet(1, 2, 3)
			got := s.Contains(tt.item)
			if got != tt.want {
				t.Errorf("Set.Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSet_Size(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "empty set",
			want: 0,
		},
		{
			name: "single item",
			want: 1,
		},
		{
			name: "multiple items",
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Set[int]
			switch tt.name {
			case "empty set":
				// s remains empty
			case "single item":
				s = NewSet(1)
			case "multiple items":
				s = NewSet(1, 2, 3)
			}
			got := s.Size()
			if got != tt.want {
				t.Errorf("Set.Size() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSet_Remove(t *testing.T) {
	tests := []struct {
		name string
		item int
		want bool
	}{
		{
			name: "remove existing item",
			item: 2,
			want: true,
		},
		{
			name: "remove non-existing item",
			item: 42,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSet(1, 2, 3)
			got := s.Remove(tt.item)
			if got != tt.want {
				t.Errorf("Set.Remove() = %v, want %v", got, tt.want)
			}
			// Verify the set state
			if tt.want {
				if s.Contains(tt.item) {
					t.Errorf("Set.Remove() should have removed item %v", tt.item)
				}
				if s.Size() != 2 {
					t.Errorf("Set.Remove() should have reduced size to 2, got %v", s.Size())
				}
			} else {
				if s.Size() != 3 {
					t.Errorf("Set.Remove() should not have changed size, got %v", s.Size())
				}
			}
		})
	}
}

func TestMapSlice(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		fn    func(int) string
		want  []string
	}{
		{
			name:  "empty slice",
			input: []int{},
			fn: func(i int) string {
				return "x"
			},
			want: []string{},
		},
		{
			name:  "mapping integers to strings",
			input: []int{1, 2, 3},
			fn: func(i int) string {
				return string(rune(i + 64))
			},
			want: []string{"A", "B", "C"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapSlice(tt.input, tt.fn)
			if len(got) != len(tt.want) {
				t.Errorf("MapSlice() = %v, want %v", got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("MapSlice()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		pred  func(int) bool
		want  []int
	}{
		{
			name:  "empty slice",
			input: []int{},
			pred:  func(int) bool { return true },
			want:  []int{},
		},
		{
			name:  "filter even numbers",
			input: []int{1, 2, 3, 4, 5, 6},
			pred:  func(i int) bool { return i%2 == 0 },
			want:  []int{2, 4, 6},
		},
		{
			name:  "filter no matches",
			input: []int{1, 3, 5},
			pred:  func(i int) bool { return i%2 == 0 },
			want:  []int{},
		},
		{
			name:  "filter all matches",
			input: []int{2, 4, 6},
			pred:  func(i int) bool { return i%2 == 0 },
			want:  []int{2, 4, 6},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Filter(tt.input, tt.pred)
			if len(got) != len(tt.want) {
				t.Errorf("Filter() = %v, want %v", got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("Filter()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestReduce(t *testing.T) {
	tests := []struct {
		name    string
		input   []int
		initial int
		fn      func(int, int) int
		want    int
	}{
		{
			name:    "empty slice",
			input:   []int{},
			initial: 0,
			fn:      func(acc, v int) int { return acc + v },
			want:    0,
		},
		{
			name:    "sum integers",
			input:   []int{1, 2, 3, 4},
			initial: 0,
			fn:      func(acc, v int) int { return acc + v },
			want:    10,
		},
		{
			name:    "multiply integers",
			input:   []int{2, 3, 4},
			initial: 1,
			fn:      func(acc, v int) int { return acc * v },
			want:    24,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Reduce(tt.input, tt.initial, tt.fn)
			if got != tt.want {
				t.Errorf("Reduce() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMinMax(t *testing.T) {
	tests := []struct {
		name    string
		input   []int
		want    int
		want2   int
		wantErr bool
	}{
		{
			name:    "empty slice",
			input:   []int{},
			want:    0,
			want2:   0,
			wantErr: true,
		},
		{
			name:    "single item",
			input:   []int{42},
			want:    42,
			want2:   42,
			wantErr: false,
		},
		{
			name:    "multiple items ascending",
			input:   []int{1, 2, 3, 4, 5},
			want:    1,
			want2:   5,
			wantErr: false,
		},
		{
			name:    "multiple items descending",
			input:   []int{5, 4, 3, 2, 1},
			want:    1,
			want2:   5,
			wantErr: false,
		},
		{
			name:    "multiple items unsorted",
			input:   []int{3, 1, 4, 1, 5},
			want:    1,
			want2:   5,
			wantErr: false,
		},
		{
			name:    "negative numbers",
			input:   []int{-3, -1, -4, -1, -5},
			want:    -5,
			want2:   -1,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got1, got2, err := MinMax(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("MinMax() should have returned error for empty slice")
				}
				return
			}
			if err != nil {
				t.Errorf("MinMax() = unexpected error: %v", err)
				return
			}
			if got1 != tt.want {
				t.Errorf("MinMax() min = %v, want %v", got1, tt.want)
			}
			if got2 != tt.want2 {
				t.Errorf("MinMax() max = %v, want %v", got2, tt.want2)
			}
		})
	}
}
