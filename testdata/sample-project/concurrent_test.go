package sample

import (
	"sync"
	"testing"
)

func TestSafeCounter_Increment(t *testing.T) {
	counter := &SafeCounter{}
	counter.Increment()
	if got := counter.Value(); got != 1 {
		t.Errorf("Expected value 1, got %d", got)
	}
}

func TestSafeCounter_Decrement(t *testing.T) {
	counter := &SafeCounter{}
	counter.Decrement()
	if got := counter.Value(); got != -1 {
		t.Errorf("Expected value -1, got %d", got)
	}
}

func TestSafeCounter_Value(t *testing.T) {
	counter := &SafeCounter{}
	if got := counter.Value(); got != 0 {
		t.Errorf("Expected value 0, got %d", got)
	}
}

func TestSafeCounter_IncrementConcurrent(t *testing.T) {
	counter := &SafeCounter{}
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.Increment()
		}()
	}

	wg.Wait()
	if got := counter.Value(); got != numGoroutines {
		t.Errorf("Expected value %d, got %d", numGoroutines, got)
	}
}

func TestSafeCounter_DecrementConcurrent(t *testing.T) {
	counter := &SafeCounter{}
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.Decrement()
		}()
	}

	wg.Wait()
	if got := counter.Value(); got != -numGoroutines {
		t.Errorf("Expected value %d, got %d", -numGoroutines, got)
	}
}

func TestSafeCounter_ValueConcurrent(t *testing.T) {
	counter := &SafeCounter{}
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.Increment()
		}()
	}

	wg.Wait()
	if got := counter.Value(); got != numGoroutines {
		t.Errorf("Expected value %d, got %d", numGoroutines, got)
	}
}

func TestFanOut(t *testing.T) {
	tests := []struct {
		name string
		n    int
		fn   func(int) int
		want []int
	}{
		{
			name: "n=0",
			n:    0,
			fn:   func(i int) int { return i * 2 },
			want: []int{},
		},
		{
			name: "n=3",
			n:    3,
			fn:   func(i int) int { return i * 2 },
			want: []int{0, 2, 4},
		},
		{
			name: "n=5",
			n:    5,
			fn:   func(i int) int { return i + 10 },
			want: []int{10, 11, 12, 13, 14},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FanOut(tt.n, tt.fn)
			if len(got) != len(tt.want) {
				t.Errorf("FanOut() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("FanOut()[%d] = %d, want %d", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestFanOutConcurrent(t *testing.T) {
	n := 100
	fn := func(i int) int { return i * i }
	results := FanOut(n, fn)

	if len(results) != n {
		t.Errorf("Expected %d results, got %d", n, len(results))
	}

	for i, v := range results {
		if v != i*i {
			t.Errorf("FanOut()[%d] = %d, want %d", i, v, i*i)
		}
	}
}

func TestParallelSum(t *testing.T) {
	tests := []struct {
		name    string
		nums    []int
		workers int
		want    int64
	}{
		{
			name:    "empty slice",
			nums:    []int{},
			workers: 4,
			want:    0,
		},
		{
			name:    "single element",
			nums:    []int{5},
			workers: 1,
			want:    5,
		},
		{
			name:    "multiple elements",
			nums:    []int{1, 2, 3, 4, 5},
			workers: 2,
			want:    15,
		},
		{
			name:    "workers greater than elements",
			nums:    []int{1, 2, 3},
			workers: 10,
			want:    6,
		},
		{
			name:    "zero workers",
			nums:    []int{1, 2, 3, 4},
			workers: 0,
			want:    10,
		},
		{
			name:    "negative elements",
			nums:    []int{-1, -2, -3},
			workers: 2,
			want:    -6,
		},
		{
			name:    "mixed positive and negative",
			nums:    []int{1, -2, 3, -4, 5},
			workers: 3,
			want:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParallelSum(tt.nums, tt.workers)
			if got != tt.want {
				t.Errorf("ParallelSum() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParallelSumConcurrent(t *testing.T) {
	nums := make([]int, 1000)
	for i := range nums {
		nums[i] = i + 1
	}
	workers := 100

	result := ParallelSum(nums, workers)
	expected := int64(0)
	for _, v := range nums {
		expected += int64(v)
	}

	if result != expected {
		t.Errorf("ParallelSum() = %d, want %d", result, expected)
	}
}
