package resourcepool

import (
	"net/http"
	"testing"
	"time"
)

// --- http_pool.go tests ---

func TestHTTPResource_Validate(t *testing.T) {
	h := &HTTPResource{
		client:    &http.Client{},
		id:        "test-1",
		lastUsed:  time.Now(),
		createdAt: time.Now(),
	}
	if !h.Validate() {
		t.Fatal("expected valid")
	}
}

func TestHTTPResource_Validate_NilClient(t *testing.T) {
	h := &HTTPResource{
		client:    nil,
		id:        "test-2",
		lastUsed:  time.Now(),
		createdAt: time.Now(),
	}
	if h.Validate() {
		t.Fatal("expected invalid for nil client")
	}
}

func TestHTTPResource_Validate_Expired(t *testing.T) {
	h := &HTTPResource{
		client:    &http.Client{},
		id:        "test-3",
		lastUsed:  time.Now(),
		createdAt: time.Now().Add(-25 * time.Hour),
	}
	if h.Validate() {
		t.Fatal("expected invalid for expired resource")
	}
}

func TestHTTPResource_Close(t *testing.T) {
	h := &HTTPResource{id: "test-4"}
	if err := h.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPResource_ID(t *testing.T) {
	h := &HTTPResource{id: "unique-id"}
	if h.ID() != "unique-id" {
		t.Fatalf("expected unique-id, got %s", h.ID())
	}
}

func TestHTTPResource_LastUsed(t *testing.T) {
	now := time.Now()
	h := &HTTPResource{lastUsed: now}
	if !h.LastUsed().Equal(now) {
		t.Fatal("expected matching lastUsed time")
	}
}

func TestHTTPResource_SetLastUsed(t *testing.T) {
	h := &HTTPResource{}
	tm := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	h.SetLastUsed(tm)
	if !h.LastUsed().Equal(tm) {
		t.Fatal("expected SetLastUsed to update")
	}
}

func TestHTTPResource_Client(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	h := &HTTPResource{client: client}
	if h.Client() != client {
		t.Fatal("expected matching client")
	}
}

func TestNewHTTPClientFactory_Defaults(t *testing.T) {
	f := NewHTTPClientFactory(0)
	if f.timeout != 30*time.Second {
		t.Fatalf("expected 30s default timeout, got %v", f.timeout)
	}
}

func TestNewHTTPClientFactory_Negative(t *testing.T) {
	f := NewHTTPClientFactory(-1)
	if f.timeout != 30*time.Second {
		t.Fatalf("expected 30s default for negative, got %v", f.timeout)
	}
}

func TestNewHTTPClientFactory_Custom(t *testing.T) {
	f := NewHTTPClientFactory(10 * time.Second)
	if f.timeout != 10*time.Second {
		t.Fatalf("expected 10s timeout, got %v", f.timeout)
	}
}

func TestHTTPClientFactory_Create(t *testing.T) {
	f := NewHTTPClientFactory(5 * time.Second)
	r, err := f.Create()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
	hr, ok := r.(*HTTPResource)
	if !ok {
		t.Fatal("expected *HTTPResource")
	}
	if hr.Client() == nil {
		t.Fatal("expected non-nil HTTP client")
	}
	if hr.id == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestHTTPClientFactory_Validate(t *testing.T) {
	f := &HTTPClientFactory{}
	h := &HTTPResource{
		client:    &http.Client{},
		createdAt: time.Now(),
	}
	if !f.Validate(h) {
		t.Fatal("expected valid")
	}
}

func TestHTTPClientFactory_Validate_WrongType(t *testing.T) {
	f := &HTTPClientFactory{}
	mr := &mockResource{id: "x", valid: true, lastUsed: time.Now()}
	if f.Validate(mr) {
		t.Fatal("expected false for wrong resource type")
	}
}

func TestNewHTTPPoolManager(t *testing.T) {
	m := NewHTTPPoolManager(PoolConfig{MinSize: 1, MaxSize: 3})
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.pool == nil {
		t.Fatal("expected non-nil pool")
	}
	if m.clientMapping == nil {
		t.Fatal("expected non-nil clientMapping")
	}
	m.Close()
}

func TestNewHTTPPoolManager_DefaultName(t *testing.T) {
	m := NewHTTPPoolManager(PoolConfig{})
	defer m.Close()
	if m.pool.config.Name != "http-pool" {
		t.Fatalf("expected http-pool name, got %s", m.pool.config.Name)
	}
}

func TestHTTPPoolManager_AcquireRelease(t *testing.T) {
	m := NewHTTPPoolManager(PoolConfig{MinSize: 1, MaxSize: 3})
	defer m.Close()

	client, err := m.AcquireHTTPClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if err := m.ReleaseHTTPClient(client); err != nil {
		t.Fatalf("unexpected release error: %v", err)
	}
}

func TestHTTPPoolManager_ReleaseUnknownClient(t *testing.T) {
	m := NewHTTPPoolManager(PoolConfig{MinSize: 1, MaxSize: 3})
	defer m.Close()

	unknown := &http.Client{}
	err := m.ReleaseHTTPClient(unknown)
	if err == nil {
		t.Fatal("expected error for unknown client")
	}
}

func TestHTTPPoolManager_GetPool(t *testing.T) {
	m := NewHTTPPoolManager(PoolConfig{MinSize: 1})
	defer m.Close()
	if m.GetPool() != m.pool {
		t.Fatal("expected GetPool to return the pool")
	}
}

func TestHTTPPoolManager_Close(t *testing.T) {
	m := NewHTTPPoolManager(PoolConfig{MinSize: 1})
	if err := m.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
	// Verify mapping cleared
	if len(m.clientMapping) != 0 {
		t.Fatal("expected empty mapping after close")
	}
}

func TestNewHTTPPool(t *testing.T) {
	p := NewHTTPPool(PoolConfig{MinSize: 1, MaxSize: 3, Name: "test-http"})
	defer p.Close()
	if p.config.Name != "test-http" {
		t.Fatalf("expected test-http name, got %s", p.config.Name)
	}
}

func TestNewHTTPPool_DefaultName(t *testing.T) {
	p := NewHTTPPool(PoolConfig{})
	defer p.Close()
	if p.config.Name != "http-pool" {
		t.Fatalf("expected http-pool name, got %s", p.config.Name)
	}
}

func TestAcquireHTTPClient_Success(t *testing.T) {
	p := NewHTTPPool(PoolConfig{MinSize: 1, MaxSize: 3})
	defer p.Close()

	client, err := AcquireHTTPClient(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestAcquireHTTPClient_InvalidResource(t *testing.T) {
	// Create a pool with a non-HTTP factory to test type assertion failure
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3}, f)
	defer p.Close()

	_, err := AcquireHTTPClient(p)
	if err != ErrInvalidResourceType {
		t.Fatalf("expected ErrInvalidResourceType, got %v", err)
	}
}

func TestReleaseHTTPClient_NoOp(t *testing.T) {
	// ReleaseHTTPClient is a no-op for the legacy interface
	p := NewHTTPPool(PoolConfig{MinSize: 1})
	defer p.Close()

	err := ReleaseHTTPClient(p, &http.Client{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- utils.go tests ---

func TestGenerateID_NonEmpty(t *testing.T) {
	id := generateID()
	if len(id) == 0 {
		t.Fatal("expected non-empty ID")
	}
}

func TestGenerateID_Unique(t *testing.T) {
	id1 := generateID()
	id2 := generateID()
	if id1 == id2 {
		t.Fatal("expected unique IDs")
	}
}

func TestIsValidResource_Nil(t *testing.T) {
	if IsValidResource(nil) {
		t.Fatal("expected false for nil")
	}
}

func TestIsValidResource_Valid(t *testing.T) {
	r := &mockResource{id: "v1", valid: true, lastUsed: time.Now()}
	if !IsValidResource(r) {
		t.Fatal("expected true for valid resource")
	}
}

func TestIsValidResource_Invalid(t *testing.T) {
	r := &mockResource{id: "v2", valid: false, lastUsed: time.Now()}
	if IsValidResource(r) {
		t.Fatal("expected false for invalid resource")
	}
}

func TestSafeClose_Nil(t *testing.T) {
	SafeClose(nil) // should not panic
}

func TestSafeClose_NonNil(t *testing.T) {
	r := &mockResource{id: "close-1", valid: true, lastUsed: time.Now()}
	SafeClose(r)
	if !r.closed {
		t.Fatal("expected resource to be closed")
	}
}

func TestWithTimeout_Success(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3}, f)
	defer p.Close()

	r, err := WithTimeout(p, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
}

func TestWithTimeout_InvalidResource(t *testing.T) {
	f := &mockFactory{valid: false}
	p := NewPool(PoolConfig{MinSize: 0, MaxSize: 3}, f)
	defer p.Close()

	_, err := WithTimeout(p, 2*time.Second)
	if err == nil {
		t.Fatal("expected error for invalid resource")
	}
}

func TestNewResourceWrapper(t *testing.T) {
	r := &mockResource{id: "wrap-1", valid: true, lastUsed: time.Now()}
	w := NewResourceWrapper(r)
	if w == nil {
		t.Fatal("expected non-nil wrapper")
	}
	if w.Resource != r {
		t.Fatal("expected wrapping resource to match")
	}
}

func TestResourceWrapper_AcquireTime(t *testing.T) {
	r := &mockResource{id: "wrap-2", valid: true, lastUsed: time.Now()}
	w := NewResourceWrapper(r)
	before := time.Now().Add(-time.Second)
	after := time.Now().Add(time.Second)
	acquireTime := w.AcquireTime()
	if acquireTime.Before(before) || acquireTime.After(after) {
		t.Fatalf("expected acquireTime near now, got %v", acquireTime)
	}
}

func TestResourceWrapper_UsageDuration(t *testing.T) {
	r := &mockResource{id: "wrap-3", valid: true, lastUsed: time.Now()}
	w := NewResourceWrapper(r)
	time.Sleep(50 * time.Millisecond)
	dur := w.UsageDuration()
	if dur < 40*time.Millisecond {
		t.Fatalf("expected duration >= 40ms, got %v", dur)
	}
}

// --- pool.go edge cases ---

func TestPool_CleanupIdleResources(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{
		MinSize:     0,
		MaxSize:     5,
		MaxIdleTime: 50 * time.Millisecond,
	}, f)
	defer p.Close()

	// Acquire and release a resource to mark it as "recently used"
	r, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected acquire error: %v", err)
	}
	if err := p.Release(r); err != nil {
		t.Fatalf("unexpected release error: %v", err)
	}

	// Wait for idle time to pass, then trigger cleanup
	time.Sleep(100 * time.Millisecond)
	p.cleanupIdleResources()

	// Should not panic, metrics should reflect any destroyed resources
	m := p.GetMetrics()
	_ = m // just verify we can read metrics
}

func TestPool_ValidateResources(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 2, MaxSize: 5}, f)
	defer p.Close()

	// Run validation — should not panic or error
	p.validateResources()

	m := p.GetMetrics()
	// Some resources may have been destroyed if they became invalid
	_ = m
}

func TestPool_ValidateResources_InvalidDestroys(t *testing.T) {
	// Create pool with valid factory, then manually invalidate resources
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 2, MaxSize: 5}, f)
	defer p.Close()

	// Acquire both resources and release them back
	r1, _ := p.Acquire()
	r2, _ := p.Acquire()

	// Make them invalid before releasing
	mr1 := r1.(*mockResource)
	mr2 := r2.(*mockResource)
	mr1.valid = false
	mr2.valid = false

	p.Release(r1)
	p.Release(r2)

	// Validation should destroy both invalid resources
	p.validateResources()

	m := p.GetMetrics()
	if m.TotalDestroyed < 2 {
		t.Fatalf("expected at least 2 destroyed, got %d", m.TotalDestroyed)
	}
}

func TestPool_Acquire_PoolFull_WaitTimeout(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 2, MaxSize: 2}, f)
	defer p.Close()

	// Hold both resources
	r1, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r2, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Set a short create timeout so the wait times out
	p.config.CreateTimeout = 50 * time.Millisecond

	_, err = p.Acquire()
	if err == nil {
		t.Fatal("expected timeout error")
	}

	p.Release(r1)
	p.Release(r2)
}

func TestPool_Acquire_InvalidResourceFromPool(t *testing.T) {
	// Pre-create an invalid resource in the pool
	f := &mockFactory{valid: true, counter: 0}
	p := NewPool(PoolConfig{MinSize: 0, MaxSize: 3}, f)
	defer p.Close()

	// Manually inject an invalid resource
	p.resources <- &mockResource{id: "invalid-1", valid: false, lastUsed: time.Now()}

	// Acquire should find it invalid, close it, and try to create a new one
	r, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
}

func TestPool_Acquire_WaitAndInvalid(t *testing.T) {
	// Fill the pool, then when waiting the returned resource is invalid
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 1}, f)
	defer p.Close()

	r, _ := p.Acquire()

	// Put an invalid resource into the pool (simulating the case where
	// the pool was seeded with a bad resource)
	p.resources <- &mockResource{id: "bad-wait", valid: false, lastUsed: time.Now()}

	// Release the held resource so the wait can proceed
	p.Release(r)

	// Now acquire — it will get the invalid resource from the wait path
	// or create a new one. Either way it should not hang.
	p.config.CreateTimeout = 100 * time.Millisecond
	_, _ = p.Acquire()
}

func TestPool_Release_ToFullPool(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 1}, f)
	defer p.Close()

	r, err := p.Acquire()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Pool channel is empty (resource is in-use), so release should succeed
	if err := p.Release(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPool_GetMetrics_Closed(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1}, f)
	p.Close()

	m := p.GetMetrics()
	if m.TotalCreated < 1 {
		t.Fatalf("expected at least 1 created, got %d", m.TotalCreated)
	}
}

func TestPool_GetStats_Closed(t *testing.T) {
	f := &mockFactory{valid: true}
	p := NewPool(PoolConfig{MinSize: 1}, f)
	p.Close()

	stats := p.GetStats()
	if stats["closed"] != true {
		t.Fatal("expected closed=true")
	}
}

func TestPool_CreateFactoryError(t *testing.T) {
	f := &errorFactory{fail: true}
	p := NewPool(PoolConfig{MinSize: 1, MaxSize: 3}, f)
	defer p.Close()

	// Pre-creation silently continues on error, so pool may be empty
	// Acquire should try to create and get the factory error
	_, err := p.Acquire()
	if err == nil {
		t.Fatal("expected factory error")
	}
}

// errorFactory is a factory that always fails
type errorFactory struct {
	fail bool
}

func (e *errorFactory) Create() (Resource, error) {
	return nil, ErrResourceNotFound
}

func (e *errorFactory) Validate(r Resource) bool {
	return false
}
