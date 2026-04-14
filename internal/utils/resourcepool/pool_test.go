package resourcepool

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockResource is a simple mock Resource for testing
type mockResource struct {
	id       string
	valid    bool
	lastUsed time.Time
	closed   bool
}

func (m *mockResource) ID() string                { return m.id }
func (m *mockResource) Validate() bool            { return m.valid }
func (m *mockResource) Close() error {
	m.closed = true
	return nil
}
func (m *mockResource) LastUsed() time.Time       { return m.lastUsed }
func (m *mockResource) SetLastUsed(t time.Time)   { m.lastUsed = t }

// mockFactory creates mock resources
type mockFactory struct {
	counter int
	mu      sync.Mutex
	valid   bool
}

func (f *mockFactory) Create() (Resource, error) {
	f.mu.Lock()
	f.counter++
	id := fmt.Sprintf("mock-%d", f.counter)
	f.mu.Unlock()

	return &mockResource{
		id:       id,
		valid:    f.valid,
		lastUsed: time.Now(),
	}, nil
}

func (f *mockFactory) Validate(r Resource) bool {
	mr, ok := r.(*mockResource)
	if !ok {
		return false
	}
	return mr.valid
}

func TestNewPool_Defaults(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{}, f)
	defer p.Close()

	if p.config.MaxSize != 10 {
		t.Fatalf("expected MaxSize 10, got %d", p.config.MaxSize)
	}
	if p.config.MinSize != 1 {
		t.Fatalf("expected MinSize 1, got %d", p.config.MinSize)
	}
}

func TestNewPool_CustomConfig(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{
		MaxSize: 5,
		MinSize: 2,
		Name:    "test-pool",
	}, f)
	defer p.Close()

	if p.config.MaxSize != 5 {
		t.Fatalf("expected MaxSize 5, got %d", p.config.MaxSize)
	}
	if p.config.MinSize != 2 {
		t.Fatalf("expected MinSize 2, got %d", p.config.MinSize)
	}
	if p.config.Name != "test-pool" {
		t.Fatalf("expected Name test-pool, got %s", p.config.Name)
	}
}

func TestPool_AcquireRelease(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3}, f)
	defer p.Close()

	r, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil resource")
	}

	if err := p.Release(r); err != nil {
		t.Fatalf("unexpected release error: %v", err)
	}
}

func TestPool_AcquireNewResource(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 0, MaxSize: 3}, f)
	defer p.Close()

	// 池中没有预创建资源，应创建新资源
	r, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
	if r.ID() != "mock-1" {
		t.Fatalf("expected mock-1, got %s", r.ID())
	}
}

func TestPool_Close(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3}, f)

	p.Close()

	// 关闭后再次关闭不应 panic
	p.Close()
}

func TestPool_AcquireAfterClose(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3}, f)
	p.Close()

	_, err := p.Acquire()
	if err == nil {
		t.Fatal("expected error after close")
	}
}

func TestPool_ReleaseNotInUse(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3}, f)
	defer p.Close()

	// 释放未使用的资源应报错
	r := &mockResource{id: "not-in-use", valid: true, lastUsed: time.Now()}
	err := p.Release(r)
	if err == nil {
		t.Fatal("expected error for resource not in use")
	}
}

func TestPool_GetMetrics(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3}, f)
	defer p.Close()

	// 获取资源并释放
	r, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p.Release(r)

	m := p.GetMetrics()
	if m.TotalAcquired < 1 {
		t.Fatalf("expected at least 1 acquired, got %d", m.TotalAcquired)
	}
	if m.TotalReleased < 1 {
		t.Fatalf("expected at least 1 released, got %d", m.TotalReleased)
	}
}

func TestPool_GetStats(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3, Name: "test"}, f)
	defer p.Close()

	stats := p.GetStats()
	if stats["name"] != "test" {
		t.Fatalf("expected name test, got %v", stats["name"])
	}
	if stats["closed"] != false {
		t.Fatal("expected not closed")
	}
}

func TestPool_MaxCapacity(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 0, MaxSize: 2}, f)
	defer p.Close()

	r1, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r2, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 达到最大容量，尝试获取第三个会阻塞超时
	_, err = p.Acquire()
	if err == nil {
		t.Fatal("expected error at max capacity")
	}

	p.Release(r1)
	p.Release(r2)
}

func TestPool_InvalidResource(t *testing.T) {
	f := &mockFactory{valid: false}
	p := NewPool(PoolConfig{MinSize: 0, MaxSize: 2}, f)
	defer p.Close()

	// 创建无效资源时 Acquire 会失败
	_, err := p.Acquire()
	if err == nil {
		t.Fatal("expected error for invalid resource")
	}
}

func TestPool_ConcurrentAcquireRelease(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 2, MaxSize: 10}, f)
	defer p.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []string
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := p.Acquire()
			if err != nil {
				mu.Lock()
				errs = append(errs, err.Error())
				mu.Unlock()
				return
			}
			time.Sleep(10 * time.Millisecond)
			if err := p.Release(r); err != nil {
				mu.Lock()
				errs = append(errs, err.Error())
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("concurrent acquire/release errors: %v", errs)
	}
}
