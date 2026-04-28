package utils

import (
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestNewMemoryCache(t *testing.T) {
	cache := NewMemoryCache(100, 60*time.Second)

	if cache == nil {
		t.Fatal("NewMemoryCache() returned nil")
	}
	if cache.maxSize != 100 {
		t.Errorf("NewMemoryCache() maxSize = %d, want 100", cache.maxSize)
	}
	if cache.cleanupInterval != 60*time.Second {
		t.Errorf("NewMemoryCache() cleanupInterval = %v, want 60s", cache.cleanupInterval)
	}
	if cache.cache == nil {
		t.Error("NewMemoryCache() cache map is nil")
	}
}

func TestMemoryCache_Get_Set(t *testing.T) {
	cache := NewMemoryCache(10, 0) // No cleanup for test
	defer cache.Close()

	assets := []model.UnifiedAsset{
		{IP: "192.168.1.1", Port: 80},
		{IP: "192.168.1.2", Port: 443},
	}

	// Set and Get
	cache.Set("test-key", assets, 10*time.Minute)
	got, ok := cache.Get("test-key")

	if !ok {
		t.Error("MemoryCache.Get() returned not ok")
	}
	if len(got) != 2 {
		t.Errorf("MemoryCache.Get() len = %d, want 2", len(got))
	}
}

func TestMemoryCache_Get_Miss(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	_, ok := cache.Get("non-existent-key")
	if ok {
		t.Error("MemoryCache.Get() should return false for non-existent key")
	}
}

func TestMemoryCache_Get_Expired(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	assets := []model.UnifiedAsset{{IP: "1.1.1.1"}}
	cache.Set("expire-key", assets, 1*time.Millisecond)

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	_, ok := cache.Get("expire-key")
	if ok {
		t.Error("MemoryCache.Get() should return false for expired key")
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	assets := []model.UnifiedAsset{{IP: "1.1.1.1"}}
	cache.Set("delete-key", assets, 10*time.Minute)

	cache.Delete("delete-key")

	_, ok := cache.Get("delete-key")
	if ok {
		t.Error("MemoryCache.Delete() key should not exist")
	}
}

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	// Add multiple items
	for i := 0; i < 5; i++ {
		cache.Set("key-"+string(rune(i)), []model.UnifiedAsset{{IP: "1.1.1.1"}}, 10*time.Minute)
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("MemoryCache.Clear() size = %d, want 0", cache.Size())
	}
	if cache.hits != 0 || cache.misses != 0 {
		t.Error("MemoryCache.Clear() should reset hits/misses")
	}
}

func TestMemoryCache_Size(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	if cache.Size() != 0 {
		t.Errorf("MemoryCache.Size() initial = %d, want 0", cache.Size())
	}

	cache.Set("key1", []model.UnifiedAsset{{IP: "1.1.1.1"}}, 10*time.Minute)
	cache.Set("key2", []model.UnifiedAsset{{IP: "2.2.2.2"}}, 10*time.Minute)

	if cache.Size() != 2 {
		t.Errorf("MemoryCache.Size() after adds = %d, want 2", cache.Size())
	}
}

func TestMemoryCache_GetStats(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	// Initial stats
	stats := cache.GetStats()
	if stats.Size != 0 {
		t.Errorf("GetStats() initial Size = %d, want 0", stats.Size)
	}

	// Add and get to create hits/misses
	cache.Set("key1", []model.UnifiedAsset{{IP: "1.1.1.1"}}, 10*time.Minute)
	cache.Get("key1") // hit
	cache.Get("key2") // miss

	stats = cache.GetStats()
	if stats.Hits != 1 {
		t.Errorf("GetStats() Hits = %d, want 1", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("GetStats() Misses = %d, want 1", stats.Misses)
	}
	expectedRate := 0.5
	if stats.HitRate != expectedRate {
		t.Errorf("GetStats() HitRate = %v, want %v", stats.HitRate, expectedRate)
	}
}

func TestMemoryCache_Eviction(t *testing.T) {
	cache := NewMemoryCache(3, 0) // Small cache for eviction test
	defer cache.Close()

	// Fill cache
	cache.Set("key1", []model.UnifiedAsset{{IP: "1.1.1.1"}}, 10*time.Minute)
	cache.Set("key2", []model.UnifiedAsset{{IP: "2.2.2.2"}}, 10*time.Minute)
	cache.Set("key3", []model.UnifiedAsset{{IP: "3.3.3.3"}}, 10*time.Minute)

	// Access key1 multiple times to increase frequency
	cache.Get("key1")
	cache.Get("key1")

	// Add new key - should evict least frequently used
	cache.Set("key4", []model.UnifiedAsset{{IP: "4.4.4.4"}}, 10*time.Minute)

	// key1 should still exist (higher frequency)
	_, ok1 := cache.Get("key1")
	if !ok1 {
		t.Error("Eviction should not remove most frequently accessed key")
	}
}

func TestMemoryCache_GetMulti(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	// Set multiple keys
	cache.Set("key1", []model.UnifiedAsset{{IP: "1.1.1.1"}}, 10*time.Minute)
	cache.Set("key2", []model.UnifiedAsset{{IP: "2.2.2.2"}}, 10*time.Minute)

	result := cache.GetMulti([]string{"key1", "key2", "key3"})
	if len(result) != 2 {
		t.Errorf("GetMulti() result count = %d, want 2", len(result))
	}
	if _, ok := result["key1"]; !ok {
		t.Error("GetMulti() should contain key1")
	}
	if _, ok := result["key3"]; ok {
		t.Error("GetMulti() should not contain non-existent key3")
	}
}

func TestMemoryCache_SetMulti(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	keyAssets := map[string][]model.UnifiedAsset{
		"key1": {{IP: "1.1.1.1"}},
		"key2": {{IP: "2.2.2.2"}},
	}

	cache.SetMulti(keyAssets, 10*time.Minute)

	if cache.Size() != 2 {
		t.Errorf("SetMulti() size = %d, want 2", cache.Size())
	}
}

func TestMemoryCache_IsHealthy(t *testing.T) {
	cache := NewMemoryCache(10, 0)
	defer cache.Close()

	if !cache.IsHealthy() {
		t.Error("MemoryCache.IsHealthy() should return true")
	}
}

func TestMemoryCache_Close(t *testing.T) {
	cache := NewMemoryCache(10, 100*time.Millisecond)

	cache.Close()

	// Double close should be safe
	cache.Close()

	if !cache.stopped {
		t.Error("MemoryCache.Close() should set stopped=true")
	}
}

func TestGenerateCacheKey(t *testing.T) {
	tests := []struct {
		engine   string
		query    string
		page     int
		pageSize int
	}{
		{"fofa", "domain=\"test.com\"", 1, 10},
		{"hunter", "ip=\"1.1.1.1\"", 2, 20},
		{"zoomeye", "port:80", 1, 100},
	}

	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			key1 := GenerateCacheKey(tt.engine, tt.query, tt.page, tt.pageSize)
			key2 := GenerateCacheKey(tt.engine, tt.query, tt.page, tt.pageSize)

			if key1 != key2 {
				t.Error("GenerateCacheKey() should produce consistent keys")
			}
			if len(key1) != 32 { // MD5 hex length
				t.Errorf("GenerateCacheKey() len = %d, want 32", len(key1))
			}
		})
	}
}

func TestGenerateCacheKey_DifferentInputs(t *testing.T) {
	key1 := GenerateCacheKey("fofa", "test", 1, 10)
	key2 := GenerateCacheKey("hunter", "test", 1, 10)
	key3 := GenerateCacheKey("fofa", "different", 1, 10)
	key4 := GenerateCacheKey("fofa", "test", 2, 10)

	if key1 == key2 {
		t.Error("Different engines should produce different keys")
	}
	if key1 == key3 {
		t.Error("Different queries should produce different keys")
	}
	if key1 == key4 {
		t.Error("Different pages should produce different keys")
	}
}

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		input  string
		want   string
	}{
		{"  Domain=\"test\"  ", "domain=\"test\""},
		{"DOMAIN=\"TEST\"", "domain=\"test\""},
		{"ip=\"1.1.1.1\"  AND  port=\"80\"", "ip=\"1.1.1.1\" and port=\"80\""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeQuery(tt.input); got != tt.want {
				t.Errorf("normalizeQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}