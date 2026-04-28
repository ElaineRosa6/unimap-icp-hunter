package utils

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewShutdownManager(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{
			name:    "default timeout",
			timeout: 0,
			want:    30 * time.Second,
		},
		{
			name:    "custom timeout",
			timeout: 10 * time.Second,
			want:    10 * time.Second,
		},
		{
			name:    "negative timeout uses default",
			timeout: -5 * time.Second,
			want:    30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewShutdownManager(tt.timeout)
			if sm.timeout != tt.want {
				t.Errorf("NewShutdownManager() timeout = %v, want %v", sm.timeout, tt.want)
			}
			if sm.ctx == nil {
				t.Error("NewShutdownManager() ctx is nil")
			}
			if sm.cancel == nil {
				t.Error("NewShutdownManager() cancel is nil")
			}
		})
	}
}

func TestShutdownManager_RegisterHandler(t *testing.T) {
	sm := NewShutdownManager(5 * time.Second)

	// Register handler
	handler := func(ctx context.Context) error {
		return nil
	}

	sm.RegisterHandler(handler)

	if len(sm.handlers) != 1 {
		t.Errorf("RegisterHandler() handlers count = %d, want 1", len(sm.handlers))
	}

	// Test multiple handlers
	sm.RegisterHandler(func(ctx context.Context) error { return nil })
	sm.RegisterHandler(func(ctx context.Context) error { return nil })

	if len(sm.handlers) != 3 {
		t.Errorf("RegisterHandler() handlers count = %d, want 3", len(sm.handlers))
	}
}

func TestShutdownManager_Shutdown(t *testing.T) {
	sm := NewShutdownManager(2 * time.Second)

	var mu sync.Mutex
	handlerResults := make([]bool, 0)

	// Register handlers
	sm.RegisterHandler(func(ctx context.Context) error {
		mu.Lock()
		handlerResults = append(handlerResults, true)
		mu.Unlock()
		return nil
	})

	sm.RegisterHandler(func(ctx context.Context) error {
		mu.Lock()
		handlerResults = append(handlerResults, true)
		mu.Unlock()
		return nil
	})

	// Execute shutdown
	sm.Shutdown()

	// Verify all handlers were called
	mu.Lock()
	count := len(handlerResults)
	mu.Unlock()

	if count != 2 {
		t.Errorf("Shutdown() handler count = %d, want 2", count)
	}

	// Verify context is cancelled
	select {
	case <-sm.ctx.Done():
		// Expected
	default:
		t.Error("Shutdown() context not cancelled")
	}
}

func TestShutdownManager_Shutdown_WithError(t *testing.T) {
	sm := NewShutdownManager(2 * time.Second)

	handlerErr := false
	sm.RegisterHandler(func(ctx context.Context) error {
		handlerErr = true
		return context.DeadlineExceeded
	})

	sm.Shutdown()

	if !handlerErr {
		t.Error("Shutdown() error handler not called")
	}
}

func TestShutdownManager_Wait(t *testing.T) {
	sm := NewShutdownManager(1 * time.Second)

	// Cancel context after delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		sm.cancel()
	}()

	// Wait should return after context is cancelled
	done := make(chan struct{})
	go func() {
		sm.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Expected
	case <-time.After(2 * time.Second):
		t.Error("Wait() blocked too long")
	}
}

func TestShutdownManager_Context(t *testing.T) {
	sm := NewShutdownManager(5 * time.Second)

	ctx := sm.Context()
	if ctx == nil {
		t.Error("Context() returned nil")
	}

	// Context should not be cancelled initially
	select {
	case <-ctx.Done():
		t.Error("Context() returned cancelled context")
	default:
		// Expected
	}
}

func TestShutdownManager_Stop(t *testing.T) {
	sm := NewShutdownManager(5 * time.Second)

	sm.Stop()

	// Context should be cancelled after Stop
	select {
	case <-sm.ctx.Done():
		// Expected
	default:
		t.Error("Stop() did not cancel context")
	}
}

func TestShutdownManager_ConcurrentHandlerRegistration(t *testing.T) {
	sm := NewShutdownManager(5 * time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.RegisterHandler(func(ctx context.Context) error { return nil })
		}()
	}

	wg.Wait()

	if len(sm.handlers) != 100 {
		t.Errorf("Concurrent registration handlers count = %d, want 100", len(sm.handlers))
	}
}

func TestGracefulShutdown(t *testing.T) {
	// This test verifies GracefulShutdown function structure
	// Note: It doesn't test signal handling (hard to test without os.Signal)

	handler1Called := false
	handler2Called := false

	// Create manager manually for testing
	sm := NewShutdownManager(1 * time.Second)
	sm.RegisterHandler(func(ctx context.Context) error {
		handler1Called = true
		return nil
	})
	sm.RegisterHandler(func(ctx context.Context) error {
		handler2Called = true
		return nil
	})

	// Trigger shutdown directly (bypass signal handling)
	sm.Shutdown()

	if !handler1Called || !handler2Called {
		t.Error("GracefulShutdown handlers not called")
	}
}