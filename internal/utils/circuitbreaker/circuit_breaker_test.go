package circuitbreaker

import (
	"testing"
	"time"
)

// --- Constructor Tests ---

func TestNewCircuitBreaker(t *testing.T) {
	t.Run("uses defaults for zero values", func(t *testing.T) {
		cb := NewCircuitBreaker(Config{})
		stats := cb.GetStats()
		if stats.CurrentState != StateClosed {
			t.Errorf("expected StateClosed, got %v", stats.CurrentState)
		}
		// Verify defaults exist on the private struct
		cb2 := cb.(*circuitBreaker)
		// Check that default Name is set
		if cb2.config.Name != "default" {
			t.Errorf("expected default Name 'default', got %q", cb2.config.Name)
		}
		// Check that MinRequests is set (needed for opening)
		if cb2.config.MinRequests != 10 {
			t.Errorf("expected default MinRequests 10, got %d", cb2.config.MinRequests)
		}
	})

	t.Run("respects provided values", func(t *testing.T) {
		cfg := Config{
			FailureThreshold:  80,
			RecoveryTimeout:   5 * time.Second,
			HalfOpenMaxRequests: 3,
			MinRequests:       5,
			Name:              "test-cb",
		}
		cb := NewCircuitBreaker(cfg).(*circuitBreaker)
		if cb.config.FailureThreshold != 80 {
			t.Errorf("expected FailureThreshold 80, got %d", cb.config.FailureThreshold)
		}
		if cb.config.RecoveryTimeout != 5*time.Second {
			t.Errorf("expected RecoveryTimeout 5s, got %v", cb.config.RecoveryTimeout)
		}
		if cb.config.HalfOpenMaxRequests != 3 {
			t.Errorf("expected HalfOpenMaxRequests 3, got %d", cb.config.HalfOpenMaxRequests)
		}
		if cb.config.MinRequests != 5 {
			t.Errorf("expected MinRequests 5, got %d", cb.config.MinRequests)
		}
		if cb.config.Name != "test-cb" {
			t.Errorf("expected Name 'test-cb', got %q", cb.config.Name)
		}
	})

	t.Run("uses defaults for negative threshold", func(t *testing.T) {
		cb := NewCircuitBreaker(Config{FailureThreshold: -1}).(*circuitBreaker)
		if cb.config.FailureThreshold != 50 {
			t.Errorf("expected default FailureThreshold 50 for negative value, got %d", cb.config.FailureThreshold)
		}
	})

	t.Run("uses defaults for threshold > 100", func(t *testing.T) {
		cb := NewCircuitBreaker(Config{FailureThreshold: 200}).(*circuitBreaker)
		if cb.config.FailureThreshold != 50 {
			t.Errorf("expected default FailureThreshold 50 for >100 value, got %d", cb.config.FailureThreshold)
		}
	})
}

// --- State Transition Tests ---

func TestCircuitBreaker_AllowInClosedState(t *testing.T) {
	cb := NewCircuitBreaker(Config{})
	if !cb.Allow() {
		t.Error("expected Allow() to return true in closed state")
	}
}

func TestCircuitBreaker_OpenAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 50,
		MinRequests:      5,
	}).(*circuitBreaker)

	// Need to meet MinRequests threshold
	for i := 0; i < 3; i++ {
		cb.Failure()
	}
	// Add successes to reach MinRequests but keep failures >= 50%
	cb.Success()
	cb.Failure() // Now 4 failures out of 5 = 80% > 50%

	if cb.state != StateOpen {
		t.Errorf("expected StateOpen after 80%% failure rate, got %v", cb.state)
	}
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 50,
		MinRequests:      5,
		RecoveryTimeout:  time.Hour, // long timeout so it stays open
	}).(*circuitBreaker)

	// Open the breaker
	for i := 0; i < 3; i++ {
		cb.Failure()
	}
	cb.Success()
	cb.Failure()

	if cb.state != StateOpen {
		t.Fatalf("expected StateOpen, got %v", cb.state)
	}

	if cb.Allow() {
		t.Error("expected Allow() to return false in open state")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 50,
		MinRequests:      5,
		RecoveryTimeout:  50 * time.Millisecond,
	}).(*circuitBreaker)

	// Open the breaker
	for i := 0; i < 3; i++ {
		cb.Failure()
	}
	cb.Success()
	cb.Failure()

	if cb.state != StateOpen {
		t.Fatalf("expected StateOpen, got %v", cb.state)
	}

	// Wait for recovery timeout
	time.Sleep(100 * time.Millisecond)

	// Should transition to half-open on next Allow()
	if !cb.Allow() {
		t.Error("expected Allow() to return true after recovery timeout")
	}
	if cb.state != StateHalfOpen {
		t.Errorf("expected StateHalfOpen, got %v", cb.state)
	}
}

func TestCircuitBreaker_HalfOpenClosesOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 50,
		MinRequests:      5,
		RecoveryTimeout:  50 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	}).(*circuitBreaker)

	// Open the breaker
	for i := 0; i < 3; i++ {
		cb.Failure()
	}
	cb.Success()
	cb.Failure()

	// Wait for recovery timeout
	time.Sleep(100 * time.Millisecond)
	cb.Allow() // transitions to half-open

	// Send successful requests to close
	cb.Success()
	cb.Success()

	if cb.state != StateClosed {
		t.Errorf("expected StateClosed after successful half-open requests, got %v", cb.state)
	}
}

func TestCircuitBreaker_HalfOpenOpensOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 50,
		MinRequests:      5,
		RecoveryTimeout:  50 * time.Millisecond,
	}).(*circuitBreaker)

	// Open the breaker
	for i := 0; i < 3; i++ {
		cb.Failure()
	}
	cb.Success()
	cb.Failure()

	// Wait for recovery timeout
	time.Sleep(100 * time.Millisecond)
	cb.Allow() // transitions to half-open

	// A single failure should open the breaker
	cb.Failure()

	if cb.state != StateOpen {
		t.Errorf("expected StateOpen after failure in half-open, got %v", cb.state)
	}
}

func TestCircuitBreaker_HalfOpenLimitsRequests(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold:    50,
		MinRequests:         5,
		RecoveryTimeout:     50 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	}).(*circuitBreaker)

	// Open the breaker
	for i := 0; i < 3; i++ {
		cb.Failure()
	}
	cb.Success()
	cb.Failure()

	time.Sleep(100 * time.Millisecond)
	cb.Allow() // transitions to half-open

	// Success/Failure increments halfOpenRequests in the HalfOpen state
	cb.Success() // halfOpenRequests = 1
	cb.Success() // halfOpenRequests = 2 -> transitions to Closed

	// After enough successes, should be closed again
	if cb.state != StateClosed {
		t.Fatalf("expected StateClosed after enough half-open successes, got %v", cb.state)
	}
}

// --- Stats Tests ---

func TestCircuitBreaker_GetStats(t *testing.T) {
	cb := NewCircuitBreaker(Config{}).(*circuitBreaker)

	cb.Success()
	cb.Success()
	cb.Failure()

	stats := cb.GetStats()
	if stats.TotalRequests != 3 {
		t.Errorf("expected TotalRequests=3, got %d", stats.TotalRequests)
	}
	if stats.SuccessRequests != 2 {
		t.Errorf("expected SuccessRequests=2, got %d", stats.SuccessRequests)
	}
	if stats.FailedRequests != 1 {
		t.Errorf("expected FailedRequests=1, got %d", stats.FailedRequests)
	}
	if stats.FailureRate < 30 || stats.FailureRate > 40 {
		t.Errorf("expected ~33%% failure rate, got %.1f", stats.FailureRate)
	}
}

func TestCircuitBreaker_GetState(t *testing.T) {
	cb := NewCircuitBreaker(Config{})
	if cb.GetState() != StateClosed {
		t.Errorf("expected StateClosed, got %v", cb.GetState())
	}
}

func TestStateString(t *testing.T) {
	// Verify state values
	if StateClosed != 0 {
		t.Errorf("expected StateClosed=0, got %d", StateClosed)
	}
	if StateOpen != 1 {
		t.Errorf("expected StateOpen=1, got %d", StateOpen)
	}
	if StateHalfOpen != 2 {
		t.Errorf("expected StateHalfOpen=2, got %d", StateHalfOpen)
	}
}

// --- Concurrency Tests ---

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		RecoveryTimeout: 10 * time.Millisecond,
	})
	done := make(chan bool)

	for i := 0; i < 20; i++ {
		go func() {
			cb.Allow()
			cb.Success()
			cb.Failure()
			cb.GetState()
			cb.GetStats()
			done <- true
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
