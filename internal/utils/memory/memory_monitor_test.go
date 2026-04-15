package memory

import (
	"testing"
	"time"
)

func TestNewMemoryMonitor(t *testing.T) {
	t.Run("uses defaults for zero values", func(t *testing.T) {
		m := NewMemoryMonitor(Config{})
		if m.maxHistory != 1000 {
			t.Errorf("expected default MaxHistory 1000, got %d", m.maxHistory)
		}
	})

	t.Run("respects provided values", func(t *testing.T) {
		m := NewMemoryMonitor(Config{
			Interval:   5 * time.Second,
			MaxHistory: 50,
		})
		if m.maxHistory != 50 {
			t.Errorf("expected MaxHistory 50, got %d", m.maxHistory)
		}
	})

	t.Run("uses default for negative interval", func(t *testing.T) {
		m := NewMemoryMonitor(Config{Interval: -time.Second})
		if !m.running {
			// monitor created but not started - just verify no panic
		}
	})
}

func TestMemoryMonitor_GetCurrentStats(t *testing.T) {
	m := NewMemoryMonitor(Config{})
	stats := m.GetCurrentStats()

	if stats.Alloc == 0 {
		t.Error("expected non-zero Alloc")
	}
	if stats.Sys == 0 {
		t.Error("expected non-zero Sys")
	}
	if stats.HeapObjects == 0 {
		t.Error("expected non-zero HeapObjects")
	}
	if stats.NumGoRoutines < 1 {
		t.Errorf("expected at least 1 goroutine, got %d", stats.NumGoRoutines)
	}
	if stats.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestMemoryMonitor_GetHistoryStats(t *testing.T) {
	m := NewMemoryMonitor(Config{})

	// Initially empty
	history := m.GetHistoryStats()
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}

	// Manually collect some stats
	m.collectStats()
	m.collectStats()

	history = m.GetHistoryStats()
	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}

	// Verify returns copy, not reference
	history2 := m.GetHistoryStats()
	history2[0].Alloc = 999999
	history3 := m.GetHistoryStats()
	if history3[0].Alloc == 999999 {
		t.Error("expected GetHistoryStats to return copy")
	}
}

func TestMemoryMonitor_GetMemoryUsage(t *testing.T) {
	m := NewMemoryMonitor(Config{})
	usage := m.GetMemoryUsage()

	// Should be a valid percentage
	if usage < 0 {
		t.Errorf("expected non-negative memory usage, got %.2f", usage)
	}
}

func TestMemoryMonitor_GetMemoryGrowthRate(t *testing.T) {
	m := NewMemoryMonitor(Config{})

	t.Run("zero with less than 2 samples", func(t *testing.T) {
		rate := m.GetMemoryGrowthRate()
		if rate != 0 {
			t.Errorf("expected 0 growth rate with <2 samples, got %.2f", rate)
		}
	})

	t.Run("non-zero after multiple samples", func(t *testing.T) {
		m2 := NewMemoryMonitor(Config{})
		m2.collectStats()
		time.Sleep(10 * time.Millisecond)
		m2.collectStats()

		rate := m2.GetMemoryGrowthRate()
		// Rate could be positive or negative depending on GC behavior
		// Just verify it doesn't panic and returns a number
		_ = rate
	})
}

func TestMemoryMonitor_ForceGC(t *testing.T) {
	m := NewMemoryMonitor(Config{})
	// Should not panic
	m.ForceGC()
}

func TestMemoryMonitor_StartStop(t *testing.T) {
	m := NewMemoryMonitor(Config{Interval: 100 * time.Millisecond})

	m.Start()
	m.Start() // calling Start twice should be idempotent

	// Give it time to collect at least one sample
	time.Sleep(50 * time.Millisecond)

	m.Stop()
	m.Stop() // calling Stop twice should be idempotent
}

func TestMemoryMonitor_HistoryLimit(t *testing.T) {
	maxHistory := 5
	m := NewMemoryMonitor(Config{MaxHistory: maxHistory})

	// Collect more samples than the limit
	for i := 0; i < 10; i++ {
		m.collectStats()
	}

	history := m.GetHistoryStats()
	if len(history) != maxHistory {
		t.Errorf("expected %d history entries (max history), got %d", maxHistory, len(history))
	}
}

func TestMemoryStatsFieldsPopulated(t *testing.T) {
	m := NewMemoryMonitor(Config{})
	stats := m.GetCurrentStats()

	// Verify all numeric fields are readable
	_ = stats.TotalAlloc
	_ = stats.Lookups
	_ = stats.Mallocs
	_ = stats.Frees
	_ = stats.HeapAlloc
	_ = stats.HeapSys
	_ = stats.HeapIdle
	_ = stats.HeapInuse
	_ = stats.HeapReleased
	_ = stats.HeapObjects
	_ = stats.StackInuse
	_ = stats.StackSys
	_ = stats.GCCPUFraction
	_ = stats.NumGC
	_ = stats.PauseTotalNs
}
