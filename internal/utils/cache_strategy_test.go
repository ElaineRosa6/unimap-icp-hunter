package utils

import (
	"testing"
	"time"
)

func TestCacheStrategyManager_RegisterAndGet(t *testing.T) {
	m := NewCacheStrategyManager()
	strategy := NewDefaultCacheStrategy(30 * time.Minute)
	m.RegisterStrategy("test", strategy)

	got := m.GetStrategy("test")
	if got != strategy {
		t.Fatal("expected registered strategy")
	}
}

func TestCacheStrategyManager_GetUnknown_ReturnsDefault(t *testing.T) {
	m := NewCacheStrategyManager()
	got := m.GetStrategy("nonexistent")
	if got == nil {
		t.Fatal("expected default strategy for unknown name")
	}
	if got.GetStats().StrategyName != "DefaultCacheStrategy" {
		t.Fatalf("expected DefaultCacheStrategy, got %s", got.GetStats().StrategyName)
	}
}

func TestCacheStrategyManager_GetCacheDuration(t *testing.T) {
	m := NewCacheStrategyManager()
	// 使用 default strategy，返回固定 TTL
	duration := m.GetCacheDuration("nonexistent", "quake", "query", 1, 50)
	if duration != 30*time.Minute {
		t.Fatalf("expected 30m, got %v", duration)
	}
}

func TestCacheStrategyManager_RecordQuery(t *testing.T) {
	m := NewCacheStrategyManager()
	// 不应 panic
	m.RecordQuery("nonexistent", "quake", "query", 1, 50, 100*time.Millisecond, true)
}

func TestCacheStrategyManager_GetStats(t *testing.T) {
	m := NewCacheStrategyManager()
	stats := m.GetStats("nonexistent")
	if stats.StrategyName != "DefaultCacheStrategy" {
		t.Fatalf("expected DefaultCacheStrategy, got %s", stats.StrategyName)
	}
}

func TestDynamicCacheStrategy_New(t *testing.T) {
	s := NewDynamicCacheStrategy(5*time.Minute, 1*time.Minute, 2*time.Hour)
	if s.baseDuration != 5*time.Minute {
		t.Fatalf("expected 5m base, got %v", s.baseDuration)
	}
}

func TestDynamicCacheStrategy_GetCacheDuration_FirstQuery(t *testing.T) {
	s := NewDynamicCacheStrategy(5*time.Minute, 1*time.Minute, 2*time.Hour)
	// 第一次查询应返回 baseDuration
	d := s.GetCacheDuration("quake", "country=\"CN\"", 1, 50)
	if d != 5*time.Minute {
		t.Fatalf("expected 5m for first query, got %v", d)
	}
}

func TestDynamicCacheStrategy_RecordQuery(t *testing.T) {
	s := NewDynamicCacheStrategy(5*time.Minute, 1*time.Minute, 2*time.Hour)
	// 记录查询不应 panic
	s.RecordQuery("quake", "country=\"CN\"", 1, 50, 100*time.Millisecond, true)
}

func TestDynamicCacheStrategy_GetStats(t *testing.T) {
	s := NewDynamicCacheStrategy(5*time.Minute, 1*time.Minute, 2*time.Hour)
	stats := s.GetStats()
	if stats.StrategyName != "DynamicCacheStrategy" {
		t.Fatalf("expected DynamicCacheStrategy, got %s", stats.StrategyName)
	}
}

func TestDefaultCacheStrategy_New(t *testing.T) {
	s := NewDefaultCacheStrategy(30 * time.Minute)
	// NewDefaultCacheStrategy sets baseDuration, verify via GetCacheDuration
	d := s.GetCacheDuration("", "", 0, 0)
	if d != 30*time.Minute {
		t.Fatalf("expected 30m, got %v", d)
	}
}

func TestDefaultCacheStrategy_GetCacheDuration(t *testing.T) {
	s := NewDefaultCacheStrategy(30 * time.Minute)
	d := s.GetCacheDuration("quake", "query", 1, 50)
	if d != 30*time.Minute {
		t.Fatalf("expected 30m, got %v", d)
	}
}

func TestDefaultCacheStrategy_RecordQuery(t *testing.T) {
	s := NewDefaultCacheStrategy(30 * time.Minute)
	s.RecordQuery("quake", "query", 1, 50, 100*time.Millisecond, true)
	stats := s.GetStats()
	if stats.TotalQueries != 1 {
		t.Fatalf("expected 1 query, got %d", stats.TotalQueries)
	}
}

func TestDefaultCacheStrategy_GetStats(t *testing.T) {
	s := NewDefaultCacheStrategy(30 * time.Minute)
	stats := s.GetStats()
	if stats.StrategyName != "DefaultCacheStrategy" {
		t.Fatalf("expected DefaultCacheStrategy, got %s", stats.StrategyName)
	}
	if stats.TotalQueries != 0 {
		t.Fatalf("expected 0 queries, got %d", stats.TotalQueries)
	}
}

func TestConfigBasedCacheStrategy_New(t *testing.T) {
	s := NewConfigBasedCacheStrategy(15 * time.Minute)
	if s.defaultTTL != 15*time.Minute {
		t.Fatalf("expected 15m default, got %v", s.defaultTTL)
	}
}

func TestConfigBasedCacheStrategy_GetCacheDuration_NoConfig(t *testing.T) {
	s := NewConfigBasedCacheStrategy(15 * time.Minute)
	d := s.GetCacheDuration("quake", "query", 1, 50)
	if d != 15*time.Minute {
		t.Fatalf("expected 15m, got %v", d)
	}
}

func TestConfigBasedCacheStrategy_GetCacheDuration_WithConfig(t *testing.T) {
	s := NewConfigBasedCacheStrategy(15 * time.Minute)
	s.SetEngineConfig("quake", &SimpleEngineCacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
	})
	d := s.GetCacheDuration("quake", "query", 1, 50)
	if d != 1*time.Hour {
		t.Fatalf("expected 1h, got %v", d)
	}
}

func TestConfigBasedCacheStrategy_GetCacheDuration_DisabledEngine(t *testing.T) {
	s := NewConfigBasedCacheStrategy(15 * time.Minute)
	s.SetEngineConfig("quake", &SimpleEngineCacheConfig{
		Enabled: false,
		TTL:     1 * time.Hour,
	})
	d := s.GetCacheDuration("quake", "query", 1, 50)
	if d != 0 {
		t.Fatalf("expected 0 for disabled engine, got %v", d)
	}
}

func TestConfigBasedCacheStrategy_RecordQuery(t *testing.T) {
	s := NewConfigBasedCacheStrategy(15 * time.Minute)
	s.RecordQuery("quake", "query", 1, 50, 100*time.Millisecond, true)
	stats := s.GetStats()
	if stats.TotalQueries != 1 {
		t.Fatalf("expected 1 query, got %d", stats.TotalQueries)
	}
}

func TestConfigBasedCacheStrategy_SetEngineConfigFromMap(t *testing.T) {
	s := NewConfigBasedCacheStrategy(15 * time.Minute)
	s.SetEngineConfigFromMap(map[string]struct {
		Enabled bool
		TTL     int
		MaxSize int
	}{
		"quake": {Enabled: true, TTL: 3600, MaxSize: 1000},
	})
	d := s.GetCacheDuration("quake", "query", 1, 50)
	if d != time.Hour {
		t.Fatalf("expected 1h, got %v", d)
	}
}

func TestSimpleEngineCacheConfig(t *testing.T) {
	cfg := &SimpleEngineCacheConfig{
		Enabled: true,
		TTL:     30 * time.Minute,
		MaxSize: 500,
	}
	if !cfg.IsEnabled() {
		t.Fatal("expected enabled")
	}
	if cfg.GetTTL() != 30*time.Minute {
		t.Fatalf("expected 30m TTL, got %v", cfg.GetTTL())
	}
	if cfg.GetMaxSize() != 500 {
		t.Fatalf("expected 500 max size, got %d", cfg.GetMaxSize())
	}
}
