package sample

import (
	"sync"
	"sync/atomic"
)

// SafeCounter is a thread-safe counter.
type SafeCounter struct {
	mu    sync.Mutex
	value int
}

// Increment safely increments the counter.
func (c *SafeCounter) Increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
}

// Decrement safely decrements the counter.
func (c *SafeCounter) Decrement() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value--
}

// Value returns the current counter value.
func (c *SafeCounter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// FanOut runs fn concurrently n times and collects results.
func FanOut(n int, fn func(int) int) []int {
	results := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = fn(idx)
		}(i)
	}
	wg.Wait()
	return results
}

// ParallelSum sums elements of a slice using goroutines.
func ParallelSum(nums []int, workers int) int64 {
	if workers <= 0 {
		workers = 1
	}
	if len(nums) == 0 {
		return 0
	}

	var total atomic.Int64
	chunkSize := (len(nums) + workers - 1) / workers

	var wg sync.WaitGroup
	for i := 0; i < len(nums); i += chunkSize {
		end := i + chunkSize
		if end > len(nums) {
			end = len(nums)
		}
		wg.Add(1)
		go func(chunk []int) {
			defer wg.Done()
			var sum int64
			for _, v := range chunk {
				sum += int64(v)
			}
			total.Add(sum)
		}(nums[i:end])
	}
	wg.Wait()
	return total.Load()
}
