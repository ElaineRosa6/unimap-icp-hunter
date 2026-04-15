package degradation

import (
	"testing"
	"time"
)

// --- Constructor Tests ---

func TestNewDegradationManager(t *testing.T) {
	t.Run("uses defaults for zero values", func(t *testing.T) {
		dm := NewDegradationManager(Config{}).(*degradationManager)
		if dm.config.RecoveryInterval != 30*time.Second {
			t.Errorf("expected default RecoveryInterval 30s, got %v", dm.config.RecoveryInterval)
		}
		if dm.config.LoadThreshold != 0.8 {
			t.Errorf("expected default LoadThreshold 0.8, got %.2f", dm.config.LoadThreshold)
		}
		if dm.config.ErrorRateThreshold != 0.3 {
			t.Errorf("expected default ErrorRateThreshold 0.3, got %.2f", dm.config.ErrorRateThreshold)
		}
		if dm.config.ResponseTimeThreshold != 500*time.Millisecond {
			t.Errorf("expected default ResponseTimeThreshold 500ms, got %v", dm.config.ResponseTimeThreshold)
		}
		if dm.config.Name != "default" {
			t.Errorf("expected default Name 'default', got %q", dm.config.Name)
		}
	})

	t.Run("respects provided values", func(t *testing.T) {
		cfg := Config{
			ServiceLevel:        LevelImportant,
			Strategy:            StrategyErrorRateBased,
			LoadThreshold:       0.9,
			ErrorRateThreshold:  0.5,
			ResponseTimeThreshold: 1 * time.Second,
			RecoveryInterval:    10 * time.Second,
			Name:                "api-service",
		}
		dm := NewDegradationManager(cfg).(*degradationManager)
		if dm.config.ServiceLevel != LevelImportant {
			t.Errorf("expected ServiceLevel LevelImportant, got %v", dm.config.ServiceLevel)
		}
		if dm.config.Strategy != StrategyErrorRateBased {
			t.Errorf("expected StrategyErrorRateBased, got %v", dm.config.Strategy)
		}
		if dm.config.LoadThreshold != 0.9 {
			t.Errorf("expected LoadThreshold 0.9, got %.2f", dm.config.LoadThreshold)
		}
		if dm.config.ErrorRateThreshold != 0.5 {
			t.Errorf("expected ErrorRateThreshold 0.5, got %.2f", dm.config.ErrorRateThreshold)
		}
		if dm.config.ResponseTimeThreshold != 1*time.Second {
			t.Errorf("expected ResponseTimeThreshold 1s, got %v", dm.config.ResponseTimeThreshold)
		}
		if dm.config.Name != "api-service" {
			t.Errorf("expected Name 'api-service', got %q", dm.config.Name)
		}
	})
}

// --- ShouldDegrade Tests ---

func TestShouldDegrade_DefaultFalse(t *testing.T) {
	dm := NewDegradationManager(Config{Name: "test"})
	if dm.ShouldDegrade() {
		t.Error("expected ShouldDegrade() to return false by default")
	}
}

// --- Load-Based Degradation Tests ---

func TestLoadBasedDegradation(t *testing.T) {
	dm := NewDegradationManager(Config{
		Strategy:         StrategyLoadBased,
		LoadThreshold:    0.5,
		RecoveryInterval: time.Hour, // long recovery so it stays degraded
		Name:             "load-test",
	}).(*degradationManager)

	t.Run("below threshold does not degrade", func(t *testing.T) {
		dm2 := NewDegradationManager(Config{
			Strategy:         StrategyLoadBased,
			LoadThreshold:    0.9,
			RecoveryInterval: time.Hour,
			Name:             "load-test-2",
		})
		dm2.UpdateMetrics(0.5, 0.0, 0)
		if dm2.ShouldDegrade() {
			t.Error("expected no degradation at 50%% load with 90%% threshold")
		}
	})

	t.Run("above threshold degrades", func(t *testing.T) {
		dm.UpdateMetrics(0.8, 0.0, 0)
		if !dm.status.IsDegraded {
			t.Error("expected degradation at 80%% load with 50%% threshold")
		}
	})

	t.Run("degrades service level", func(t *testing.T) {
		if dm.status.CurrentLevel != LevelImportant {
			t.Errorf("expected degraded level LevelImportant, got %v", dm.status.CurrentLevel)
		}
	})
}

// --- ErrorRate-Based Degradation Tests ---

func TestErrorRateBasedDegradation(t *testing.T) {
	dm := NewDegradationManager(Config{
		Strategy:            StrategyErrorRateBased,
		ErrorRateThreshold: 0.2,
		RecoveryInterval:   time.Hour,
		Name:               "error-test",
	})

	dm.UpdateMetrics(0.0, 0.5, 0)
	if !dm.ShouldDegrade() {
		t.Error("expected degradation at 50%% error rate with 20%% threshold")
	}
}

// --- ResponseTime-Based Degradation Tests ---

func TestResponseTimeBasedDegradation(t *testing.T) {
	dm := NewDegradationManager(Config{
		Strategy:              StrategyResponseTimeBased,
		ResponseTimeThreshold: 100 * time.Millisecond,
		RecoveryInterval:      time.Hour,
		Name:                  "rt-test",
	})

	dm.UpdateMetrics(0.0, 0.0, 200*time.Millisecond)
	if !dm.ShouldDegrade() {
		t.Error("expected degradation at 200ms response time with 100ms threshold")
	}
}

// --- Recovery Tests ---

func TestRecoveryAfterInterval(t *testing.T) {
	dm := NewDegradationManager(Config{
		Strategy:         StrategyLoadBased,
		LoadThreshold:    0.5,
		RecoveryInterval: 50 * time.Millisecond,
		Name:             "recovery-test",
	})

	// Trigger degradation
	dm.UpdateMetrics(0.8, 0.0, 0)
	if !dm.ShouldDegrade() {
		t.Fatal("expected degradation to trigger")
	}

	// Wait for recovery interval
	time.Sleep(100 * time.Millisecond)

	// Update metrics with low values - should recover
	dm.UpdateMetrics(0.1, 0.0, 0)
	if dm.ShouldDegrade() {
		t.Error("expected recovery after interval with low load")
	}
}

// --- Reset Tests ---

func TestReset(t *testing.T) {
	dm := NewDegradationManager(Config{
		Strategy:         StrategyLoadBased,
		LoadThreshold:    0.5,
		RecoveryInterval: time.Hour,
		Name:             "reset-test",
	}).(*degradationManager)

	// Trigger degradation
	dm.UpdateMetrics(0.8, 0.0, 0)
	if !dm.ShouldDegrade() {
		t.Fatal("expected degradation to trigger")
	}

	// Reset
	dm.Reset()
	if dm.ShouldDegrade() {
		t.Error("expected no degradation after reset")
	}
	if dm.status.CurrentLevel != dm.config.ServiceLevel {
		t.Errorf("expected ServiceLevel after reset, got %v", dm.status.CurrentLevel)
	}
}

// --- GetStatus Tests ---

func TestGetStatus(t *testing.T) {
	dm := NewDegradationManager(Config{Name: "status-test"})

	status := dm.GetStatus()
	if status.CurrentLevel != LevelCritical {
		t.Errorf("expected LevelCritical, got %v", status.CurrentLevel)
	}
	if status.IsDegraded {
		t.Error("expected not degraded")
	}
}

// --- Service Level Constants ---

func TestServiceLevelValues(t *testing.T) {
	if LevelCritical != 0 {
		t.Errorf("expected LevelCritical=0, got %d", LevelCritical)
	}
	if LevelImportant != 1 {
		t.Errorf("expected LevelImportant=1, got %d", LevelImportant)
	}
	if LevelNormal != 2 {
		t.Errorf("expected LevelNormal=2, got %d", LevelNormal)
	}
	if LevelOptional != 3 {
		t.Errorf("expected LevelOptional=3, got %d", LevelOptional)
	}
}

func TestStrategyValues(t *testing.T) {
	if StrategyLoadBased != 0 {
		t.Errorf("expected StrategyLoadBased=0, got %d", StrategyLoadBased)
	}
	if StrategyErrorRateBased != 1 {
		t.Errorf("expected StrategyErrorRateBased=1, got %d", StrategyErrorRateBased)
	}
	if StrategyResponseTimeBased != 2 {
		t.Errorf("expected StrategyResponseTimeBased=2, got %d", StrategyResponseTimeBased)
	}
}

// --- Default Strategy Test ---

func TestDefaultStrategyBehavior(t *testing.T) {
	// Unknown strategy should default to load-based
	dm := NewDegradationManager(Config{
		Strategy:         DegradationStrategy(999), // unknown
		LoadThreshold:    0.5,
		RecoveryInterval: time.Hour,
		Name:             "default-strategy-test",
	})

	dm.UpdateMetrics(0.8, 0.0, 0)
	if !dm.ShouldDegrade() {
		t.Error("expected default strategy to behave like load-based")
	}
}
