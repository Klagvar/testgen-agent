package analyzer

import (
	"testing"
)

func TestDetectConcurrency_Goroutine(t *testing.T) {
	fn := FuncInfo{
		Name: "Process",
		Body: `func Process() {
	go func() {
		doWork()
	}()
}`,
	}

	info := DetectConcurrency(fn, nil)

	if !info.HasGoroutines {
		t.Error("should detect goroutine")
	}
	if !info.IsConcurrent {
		t.Error("should be concurrent")
	}
}

func TestDetectConcurrency_Mutex(t *testing.T) {
	fn := FuncInfo{
		Name:     "Increment",
		Receiver: "*Counter",
		Body: `func (c *Counter) Increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
}`,
	}

	types := []TypeInfo{
		{
			Name: "Counter",
			Kind: "struct",
			Fields: []FieldInfo{
				{Name: "mu", Type: "sync.Mutex"},
				{Name: "value", Type: "int"},
			},
		},
	}

	info := DetectConcurrency(fn, types)

	if !info.HasMutex {
		t.Error("should detect mutex from struct field")
	}
	if !info.IsConcurrent {
		t.Error("should be concurrent")
	}
}

func TestDetectConcurrency_Channel(t *testing.T) {
	fn := FuncInfo{
		Name: "Producer",
		Body: `func Producer(ch chan int) {
	ch <- 42
}`,
		Params: []Param{
			{Name: "ch", Type: "chan int"},
		},
	}

	info := DetectConcurrency(fn, nil)

	if !info.HasChannels {
		t.Error("should detect channel from parameter and send")
	}
	if !info.IsConcurrent {
		t.Error("should be concurrent")
	}
}

func TestDetectConcurrency_WaitGroup(t *testing.T) {
	fn := FuncInfo{
		Name: "RunWorkers",
		Body: `func RunWorkers(n int) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work()
		}()
	}
	wg.Wait()
}`,
	}

	info := DetectConcurrency(fn, nil)

	if !info.HasWaitGroup {
		t.Error("should detect WaitGroup")
	}
	if !info.HasGoroutines {
		t.Error("should detect goroutines")
	}
	if !info.IsConcurrent {
		t.Error("should be concurrent")
	}
	if len(info.Patterns) == 0 {
		t.Error("should have patterns")
	}
}

func TestDetectConcurrency_Atomic(t *testing.T) {
	fn := FuncInfo{
		Name: "Inc",
		Body: `func Inc(counter *int64) {
	atomic.AddInt64(counter, 1)
}`,
	}

	info := DetectConcurrency(fn, nil)

	if !info.HasAtomic {
		t.Error("should detect atomic")
	}
	if !info.IsConcurrent {
		t.Error("should be concurrent")
	}
}

func TestDetectConcurrency_SyncOnce(t *testing.T) {
	fn := FuncInfo{
		Name:     "Init",
		Receiver: "*Service",
		Body: `func (s *Service) Init() {
	s.once.Do(func() {
		s.db = connect()
	})
}`,
	}

	types := []TypeInfo{
		{
			Name: "Service",
			Kind: "struct",
			Fields: []FieldInfo{
				{Name: "once", Type: "sync.Once"},
				{Name: "db", Type: "*DB"},
			},
		},
	}

	info := DetectConcurrency(fn, types)

	if !info.HasOnce {
		t.Error("should detect sync.Once")
	}
}

func TestDetectConcurrency_NoConcurrency(t *testing.T) {
	fn := FuncInfo{
		Name: "Add",
		Body: `func Add(a, b int) int {
	return a + b
}`,
	}

	info := DetectConcurrency(fn, nil)

	if info.IsConcurrent {
		t.Error("simple function should not be concurrent")
	}
	if len(info.Patterns) != 0 {
		t.Errorf("should have 0 patterns, got %d: %v", len(info.Patterns), info.Patterns)
	}
}

func TestDetectConcurrency_ChannelMake(t *testing.T) {
	fn := FuncInfo{
		Name: "Pipeline",
		Body: `func Pipeline() {
	ch := make(chan int, 10)
	go func() {
		ch <- 42
	}()
	val := <-ch
	_ = val
}`,
	}

	info := DetectConcurrency(fn, nil)

	if !info.HasChannels {
		t.Error("should detect channel from make(chan)")
	}
	if !info.HasGoroutines {
		t.Error("should detect goroutine")
	}
}

func TestDetectConcurrency_ReceiverWithMutex(t *testing.T) {
	fn := FuncInfo{
		Name:     "Get",
		Receiver: "*SafeMap",
		Body: `func (m *SafeMap) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	return v, ok
}`,
	}

	types := []TypeInfo{
		{
			Name: "SafeMap",
			Kind: "struct",
			Fields: []FieldInfo{
				{Name: "mu", Type: "sync.RWMutex"},
				{Name: "data", Type: "map[string]string"},
			},
		},
	}

	info := DetectConcurrency(fn, types)

	if !info.HasMutex {
		t.Error("should detect RWMutex from receiver struct")
	}
}

func TestConcurrencyHint_Empty(t *testing.T) {
	info := ConcurrencyInfo{IsConcurrent: false}
	if info.ConcurrencyHint() != "" {
		t.Error("non-concurrent should have empty hint")
	}
}

func TestConcurrencyHint_WithPatterns(t *testing.T) {
	info := ConcurrencyInfo{
		IsConcurrent:  true,
		HasMutex:      true,
		HasGoroutines: true,
		Patterns:      []string{"sync.Mutex", "goroutine launch"},
	}

	hint := info.ConcurrencyHint()
	if hint == "" {
		t.Error("concurrent function should have non-empty hint")
	}
	if !containsStr(hint, "sync.Mutex") {
		t.Error("hint should mention mutex pattern")
	}
	if !containsStr(hint, "goroutine") {
		t.Error("hint should mention goroutines")
	}
	if !containsStr(hint, "WaitGroup") {
		t.Error("hint should mention WaitGroup as instruction")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
