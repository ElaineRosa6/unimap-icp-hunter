package performance

import (
	"sync"
	"testing"
	"time"
)

func TestNewPerformanceOptimizer(t *testing.T) {
	po := NewPerformanceOptimizer()

	if po == nil {
		t.Fatal("NewPerformanceOptimizer() returned nil")
	}
	if po.cache == nil {
		t.Error("PerformanceOptimizer.cache is nil")
	}
	if po.concurrency == nil {
		t.Error("PerformanceOptimizer.concurrency is nil")
	}
	if po.metrics == nil {
		t.Error("PerformanceOptimizer.metrics is nil")
	}
}

func TestPerformanceOptimizer_Optimize(t *testing.T) {
	po := NewPerformanceOptimizer()

	content := "test content"
	result, err := po.Optimize("test-site", content)

	if err != nil {
		t.Errorf("Optimize() returned error: %v", err)
	}
	if result != content {
		t.Errorf("Optimize() result = %v, want %v", result, content)
	}
}

func TestPerformanceOptimizer_Optimize_CacheHit(t *testing.T) {
	po := NewPerformanceOptimizer()

	content := "cached content"
	// First call sets cache
	po.Optimize("cached-site", content)

	// Second call should hit cache
	result, err := po.Optimize("cached-site", content)

	if err != nil {
		t.Errorf("Optimize() returned error: %v", err)
	}
	if result != content {
		t.Errorf("Optimize() cached result = %v, want %v", result, content)
	}

	// Verify cache hit was recorded
	stats := po.GetPerformanceMetrics().GetStats()
	if stats["cache_hits"].(int) != 1 {
		t.Errorf("cache_hits = %v, want 1", stats["cache_hits"])
	}
}

func TestPerformanceOptimizer_GetCacheManager(t *testing.T) {
	po := NewPerformanceOptimizer()

	cm := po.GetCacheManager()
	if cm == nil {
		t.Error("GetCacheManager() returned nil")
	}
}

func TestPerformanceOptimizer_GetConcurrencyManager(t *testing.T) {
	po := NewPerformanceOptimizer()

	cm := po.GetConcurrencyManager()
	if cm == nil {
		t.Error("GetConcurrencyManager() returned nil")
	}
}

func TestPerformanceOptimizer_GetPerformanceMetrics(t *testing.T) {
	po := NewPerformanceOptimizer()

	pm := po.GetPerformanceMetrics()
	if pm == nil {
		t.Error("GetPerformanceMetrics() returned nil")
	}
}

func TestNewCacheManager(t *testing.T) {
	cm := NewCacheManager()

	if cm == nil {
		t.Fatal("NewCacheManager() returned nil")
	}
	if cm.cache == nil {
		t.Error("CacheManager.cache is nil")
	}
	if cm.maxSize != 1000 {
		t.Errorf("CacheManager.maxSize = %d, want 1000", cm.maxSize)
	}
}

func TestCacheManager_Get_Set(t *testing.T) {
	cm := NewCacheManager()

	cm.Set("test-key", "test-value", time.Minute)

	value, found := cm.Get("test-key")
	if !found {
		t.Error("CacheManager.Get() returned not found")
	}
	if value != "test-value" {
		t.Errorf("CacheManager.Get() value = %v, want test-value", value)
	}
}

func TestCacheManager_Get_NotFound(t *testing.T) {
	cm := NewCacheManager()

	value, found := cm.Get("non-existent-key")
	if found {
		t.Error("CacheManager.Get() should return false for non-existent key")
	}
	if value != "" {
		t.Errorf("CacheManager.Get() value = %v, want empty", value)
	}
}

func TestCacheManager_Delete(t *testing.T) {
	cm := NewCacheManager()

	cm.Set("delete-key", "value", time.Minute)
	cm.Delete("delete-key")

	_, found := cm.Get("delete-key")
	if found {
		t.Error("CacheManager.Delete() key should not exist")
	}
}

func TestCacheManager_Clear(t *testing.T) {
	cm := NewCacheManager()

	cm.Set("key1", "value1", time.Minute)
	cm.Set("key2", "value2", time.Minute)

	cm.Clear()

	if len(cm.cache) != 0 {
		t.Errorf("CacheManager.Clear() len = %d, want 0", len(cm.cache))
	}
}

func TestCacheManager_Eviction(t *testing.T) {
	cm := NewCacheManager()
	cm.maxSize = 3 // Small size for eviction test

	cm.Set("key1", "value1", time.Minute)
	time.Sleep(time.Millisecond) // Ensure different access times
	cm.Set("key2", "value2", time.Minute)
	time.Sleep(time.Millisecond)
	cm.Set("key3", "value3", time.Minute)
	time.Sleep(time.Millisecond)

	// This should trigger eviction
	cm.Set("key4", "value4", time.Minute)

	if len(cm.cache) > cm.maxSize {
		t.Errorf("CacheManager eviction failed, len = %d, max = %d", len(cm.cache), cm.maxSize)
	}
}

func TestCacheManager_GetStats(t *testing.T) {
	cm := NewCacheManager()

	cm.Set("key1", "value1", time.Minute)
	cm.Set("key2", "value2", time.Minute)

	// Access key1 multiple times
	cm.Get("key1")
	cm.Get("key1")

	stats := cm.GetStats()
	if stats["total_items"] != 2 {
		t.Errorf("GetStats() total_items = %v, want 2", stats["total_items"])
	}
}

func TestCacheManager_AccessCount(t *testing.T) {
	cm := NewCacheManager()

	cm.Set("count-key", "value", time.Minute)

	// Multiple accesses
	cm.Get("count-key")
	cm.Get("count-key")
	cm.Get("count-key")

	item := cm.cache["count-key"]
	if item.AccessCount != 4 { // 1 from Set + 3 from Get
		t.Errorf("AccessCount = %d, want 4", item.AccessCount)
	}
}

func TestNewConcurrencyManager(t *testing.T) {
	cm := NewConcurrencyManager()

	if cm == nil {
		t.Fatal("NewConcurrencyManager() returned nil")
	}
	if cm.maxConcurrent != 10 {
		t.Errorf("ConcurrencyManager.maxConcurrent = %d, want 10", cm.maxConcurrent)
	}
}

func TestConcurrencyManager_Acquire_Release(t *testing.T) {
	cm := NewConcurrencyManager()
	cm.SetMaxConcurrent(2)

	// Acquire should work
	cm.Acquire()
	if cm.GetCurrentConcurrent() != 1 {
		t.Errorf("GetCurrentConcurrent() = %d, want 1", cm.GetCurrentConcurrent())
	}

	cm.Acquire()
	if cm.GetCurrentConcurrent() != 2 {
		t.Errorf("GetCurrentConcurrent() = %d, want 2", cm.GetCurrentConcurrent())
	}

	cm.Release()
	if cm.GetCurrentConcurrent() != 1 {
		t.Errorf("GetCurrentConcurrent() after release = %d, want 1", cm.GetCurrentConcurrent())
	}

	cm.Release()
	if cm.GetCurrentConcurrent() != 0 {
		t.Errorf("GetCurrentConcurrent() after second release = %d, want 0", cm.GetCurrentConcurrent())
	}
}

func TestConcurrencyManager_SetMaxConcurrent(t *testing.T) {
	cm := NewConcurrencyManager()

	cm.SetMaxConcurrent(5)
	if cm.GetMaxConcurrent() != 5 {
		t.Errorf("GetMaxConcurrent() = %d, want 5", cm.GetMaxConcurrent())
	}

	// Invalid value should not change
	cm.SetMaxConcurrent(0)
	if cm.GetMaxConcurrent() != 5 {
		t.Errorf("SetMaxConcurrent(0) should not change, got %d", cm.GetMaxConcurrent())
	}

	cm.SetMaxConcurrent(-1)
	if cm.GetMaxConcurrent() != 5 {
		t.Errorf("SetMaxConcurrent(-1) should not change, got %d", cm.GetMaxConcurrent())
	}
}

func TestConcurrencyManager_Concurrent(t *testing.T) {
	cm := NewConcurrencyManager()
	cm.SetMaxConcurrent(5)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.Acquire()
			time.Sleep(time.Millisecond)
			cm.Release()
		}()
	}

	wg.Wait()

	// After all goroutines complete, concurrent count should be 0
	if cm.GetCurrentConcurrent() != 0 {
		t.Errorf("GetCurrentConcurrent() after concurrent test = %d, want 0", cm.GetCurrentConcurrent())
	}
}

func TestNewPerformanceMetrics(t *testing.T) {
	pm := NewPerformanceMetrics()

	if pm == nil {
		t.Fatal("NewPerformanceMetrics() returned nil")
	}
	if pm.optimizations == nil {
		t.Error("PerformanceMetrics.optimizations is nil")
	}
}

func TestPerformanceMetrics_RecordOptimization(t *testing.T) {
	pm := NewPerformanceMetrics()

	pm.RecordOptimization("test-site", 100*time.Millisecond)
	pm.RecordOptimization("test-site", 200*time.Millisecond)

	metrics := pm.optimizations["test-site"]
	if metrics.Count != 2 {
		t.Errorf("optimization Count = %d, want 2", metrics.Count)
	}
	if metrics.MinTime != 100*time.Millisecond {
		t.Errorf("MinTime = %v, want 100ms", metrics.MinTime)
	}
	if metrics.MaxTime != 200*time.Millisecond {
		t.Errorf("MaxTime = %v, want 200ms", metrics.MaxTime)
	}
}

func TestPerformanceMetrics_RecordCacheHit(t *testing.T) {
	pm := NewPerformanceMetrics()

	pm.RecordCacheHit("test-site", 50*time.Millisecond)
	pm.RecordCacheHit("test-site", 60*time.Millisecond)

	if pm.cacheHits != 2 {
		t.Errorf("cacheHits = %d, want 2", pm.cacheHits)
	}
}

func TestPerformanceMetrics_RecordCacheMiss(t *testing.T) {
	pm := NewPerformanceMetrics()

	pm.RecordCacheMiss("test-site")
	pm.RecordCacheMiss("test-site")

	if pm.cacheMisses != 2 {
		t.Errorf("cacheMisses = %d, want 2", pm.cacheMisses)
	}
}

func TestPerformanceMetrics_GetStats(t *testing.T) {
	pm := NewPerformanceMetrics()

	pm.RecordOptimization("site1", 100*time.Millisecond)
	pm.RecordOptimization("site2", 200*time.Millisecond)
	pm.RecordCacheHit("site1", 50*time.Millisecond)
	pm.RecordCacheMiss("site1")

	stats := pm.GetStats()

	if stats["total_requests"] != 3 {
		t.Errorf("total_requests = %v, want 3", stats["total_requests"])
	}
	if stats["cache_hits"] != 1 {
		t.Errorf("cache_hits = %v, want 1", stats["cache_hits"])
	}
	if stats["cache_misses"] != 1 {
		t.Errorf("cache_misses = %v, want 1", stats["cache_misses"])
	}
}

func TestPerformanceMetrics_GetStats_Empty(t *testing.T) {
	pm := NewPerformanceMetrics()

	stats := pm.GetStats()

	if stats["total_requests"] != 0 {
		t.Errorf("empty stats total_requests = %v, want 0", stats["total_requests"])
	}
	if stats["cache_hit_rate"] != 0.0 {
		t.Errorf("empty stats cache_hit_rate = %v, want 0", stats["cache_hit_rate"])
	}
}

func TestPerformanceMetrics_GetSiteStats(t *testing.T) {
	pm := NewPerformanceMetrics()

	pm.RecordOptimization("test-site", 100*time.Millisecond)
	pm.RecordOptimization("test-site", 200*time.Millisecond)

	stats := pm.GetSiteStats("test-site")
	if stats == nil {
		t.Fatal("GetSiteStats() returned nil")
	}
	if stats["count"] != 2 {
		t.Errorf("count = %v, want 2", stats["count"])
	}
}

func TestPerformanceMetrics_GetSiteStats_NotFound(t *testing.T) {
	pm := NewPerformanceMetrics()

	stats := pm.GetSiteStats("non-existent-site")
	if stats != nil {
		t.Error("GetSiteStats() should return nil for non-existent site")
	}
}

func TestPerformanceMetrics_ResetStats(t *testing.T) {
	pm := NewPerformanceMetrics()

	pm.RecordOptimization("site1", 100*time.Millisecond)
	pm.RecordCacheHit("site1", 50*time.Millisecond)
	pm.RecordCacheMiss("site1")

	pm.ResetStats()

	if pm.totalRequests != 0 {
		t.Errorf("ResetStats() totalRequests = %d, want 0", pm.totalRequests)
	}
	if pm.cacheHits != 0 {
		t.Errorf("ResetStats() cacheHits = %d, want 0", pm.cacheHits)
	}
	if len(pm.optimizations) != 0 {
		t.Errorf("ResetStats() optimizations len = %d, want 0", len(pm.optimizations))
	}
}