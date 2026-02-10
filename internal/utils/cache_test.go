package utils

import (
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestNewMemoryCache(t *testing.T) {
	cache := NewMemoryCache(100, 5*time.Minute)
	if cache == nil {
		t.Error("NewMemoryCache() returned nil")
	}
	if cache.Size() != 0 {
		t.Errorf("NewMemoryCache() initial size should be 0, got %d", cache.Size())
	}
}

func TestMemoryCache_GetSet(t *testing.T) {
	cache := NewMemoryCache(100, 5*time.Minute)

	key := "test:key"
	assets := []model.UnifiedAsset{
		{
			IP:     "192.168.1.1",
			Port:   80,
			Source: "fofa",
		},
	}

	// 测试设置缓存
	cache.Set(key, assets, 1*time.Minute)
	if cache.Size() != 1 {
		t.Errorf("MemoryCache.Set() size should be 1, got %d", cache.Size())
	}

	// 测试获取缓存
	cachedAssets, found := cache.Get(key)
	if !found {
		t.Error("MemoryCache.Get() should find the key")
	}
	if len(cachedAssets) != len(assets) {
		t.Errorf("MemoryCache.Get() should return %d assets, got %d", len(assets), len(cachedAssets))
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	cache := NewMemoryCache(100, 5*time.Minute)

	key := "test:key"
	assets := []model.UnifiedAsset{
		{
			IP:     "192.168.1.1",
			Port:   80,
			Source: "fofa",
		},
	}

	// 设置缓存
	cache.Set(key, assets, 1*time.Minute)
	if cache.Size() != 1 {
		t.Errorf("MemoryCache.Set() size should be 1, got %d", cache.Size())
	}

	// 删除缓存
	cache.Delete(key)
	if cache.Size() != 0 {
		t.Errorf("MemoryCache.Delete() size should be 0, got %d", cache.Size())
	}

	// 检查是否删除成功
	_, found := cache.Get(key)
	if found {
		t.Error("MemoryCache.Get() should not find the key after deletion")
	}
}

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache(100, 5*time.Minute)

	// 设置多个缓存
	for i := 0; i < 5; i++ {
		key := "test:key:" + string(rune('0'+i))
		assets := []model.UnifiedAsset{
			{
				IP:     "192.168.1." + string(rune('1'+i)),
				Port:   80,
				Source: "fofa",
			},
		}
		cache.Set(key, assets, 1*time.Minute)
	}

	if cache.Size() != 5 {
		t.Errorf("MemoryCache size should be 5, got %d", cache.Size())
	}

	// 清空缓存
	cache.Clear()
	if cache.Size() != 0 {
		t.Errorf("MemoryCache.Clear() size should be 0, got %d", cache.Size())
	}
}

func TestMemoryCache_LRU(t *testing.T) {
	// 创建一个容量为2的缓存
	cache := NewMemoryCache(2, 5*time.Minute)

	key1 := "test:key:1"
	key2 := "test:key:2"
	key3 := "test:key:3"

	// 设置第一个缓存
	cache.Set(key1, []model.UnifiedAsset{{IP: "192.168.1.1", Port: 80, Source: "fofa"}}, 1*time.Minute)
	if cache.Size() != 1 {
		t.Errorf("MemoryCache size should be 1, got %d", cache.Size())
	}

	// 设置第二个缓存
	cache.Set(key2, []model.UnifiedAsset{{IP: "192.168.1.2", Port: 80, Source: "hunter"}}, 1*time.Minute)
	if cache.Size() != 2 {
		t.Errorf("MemoryCache size should be 2, got %d", cache.Size())
	}

	// 访问第一个缓存，使其成为最近使用的
	_, found := cache.Get(key1)
	if !found {
		t.Error("MemoryCache should find key1")
	}

	// 设置第三个缓存，应该淘汰key2（最久未使用的）
	cache.Set(key3, []model.UnifiedAsset{{IP: "192.168.1.3", Port: 80, Source: "zoomeye"}}, 1*time.Minute)
	if cache.Size() != 2 {
		t.Errorf("MemoryCache size should be 2, got %d", cache.Size())
	}

	// 检查key2是否被淘汰
	_, found = cache.Get(key2)
	if found {
		t.Error("MemoryCache should not find key2 after LRU eviction")
	}

	// 检查key1和key3是否存在
	_, found = cache.Get(key1)
	if !found {
		t.Error("MemoryCache should find key1")
	}

	_, found = cache.Get(key3)
	if !found {
		t.Error("MemoryCache should find key3")
	}
}

func TestMemoryCache_Expiry(t *testing.T) {
	// 创建一个清理间隔为100ms的缓存
	cache := NewMemoryCache(10, 100*time.Millisecond)

	key := "test:key"
	assets := []model.UnifiedAsset{{IP: "192.168.1.1", Port: 80, Source: "fofa"}}

	// 设置一个过期时间为50ms的缓存
	cache.Set(key, assets, 50*time.Millisecond)
	if cache.Size() != 1 {
		t.Errorf("MemoryCache size should be 1, got %d", cache.Size())
	}

	// 等待100ms，让缓存过期
	time.Sleep(100 * time.Millisecond)

	// 检查缓存是否过期
	_, found := cache.Get(key)
	if found {
		t.Error("MemoryCache should not find expired key")
	}
}

func TestGenerateCacheKey(t *testing.T) {
	engineName := "fofa"
	query := "ip=192.168.1.1"
	page := 1
	pageSize := 100

	key := GenerateCacheKey(engineName, query, page, pageSize)
	if key == "" {
		t.Error("GenerateCacheKey() returned empty string")
	}

	// 测试不同参数生成不同的key
	differentKey := GenerateCacheKey("hunter", query, page, pageSize)
	if key == differentKey {
		t.Error("GenerateCacheKey() should return different keys for different engine names")
	}

	differentKey = GenerateCacheKey(engineName, "ip=192.168.1.2", page, pageSize)
	if key == differentKey {
		t.Error("GenerateCacheKey() should return different keys for different queries")
	}

	differentKey = GenerateCacheKey(engineName, query, 2, pageSize)
	if key == differentKey {
		t.Error("GenerateCacheKey() should return different keys for different pages")
	}

	differentKey = GenerateCacheKey(engineName, query, page, 200)
	if key == differentKey {
		t.Error("GenerateCacheKey() should return different keys for different page sizes")
	}
}

func TestNewCache(t *testing.T) {
	// 测试创建内存缓存
	cache := NewCache(false, "localhost:6379", "", 0, "unimap:", 100, 5*time.Minute)
	if cache == nil {
		t.Error("NewCache() returned nil for memory cache")
	}

	// 测试创建Redis缓存（如果Redis不可用，应该回退到内存缓存）
	cache = NewCache(true, "localhost:6379", "", 0, "unimap:", 100, 5*time.Minute)
	if cache == nil {
		t.Error("NewCache() returned nil for Redis cache (should fall back to memory cache)")
	}
}

func TestCacheStats(t *testing.T) {
	cache := NewMemoryCache(100, 5*time.Minute)

	// 获取初始统计信息
	stats := cache.GetStats()
	if stats.Hits != 0 {
		t.Errorf("Initial cache hits should be 0, got %d", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("Initial cache misses should be 0, got %d", stats.Misses)
	}
	if stats.Size != 0 {
		t.Errorf("Initial cache size should be 0, got %d", stats.Size)
	}
	if stats.MaxSize != 100 {
		t.Errorf("Cache max size should be 100, got %d", stats.MaxSize)
	}

	// 测试缓存未命中
	_, found := cache.Get("non-existent-key")
	if found {
		t.Error("Cache should not find non-existent key")
	}

	stats = cache.GetStats()
	if stats.Misses != 1 {
		t.Errorf("Cache misses should be 1, got %d", stats.Misses)
	}

	// 测试缓存命中
	key := "test:key"
	assets := []model.UnifiedAsset{{IP: "192.168.1.1", Port: 80, Source: "fofa"}}
	cache.Set(key, assets, 1*time.Minute)

	_, found = cache.Get(key)
	if !found {
		t.Error("Cache should find key after setting")
	}

	stats = cache.GetStats()
	if stats.Hits != 1 {
		t.Errorf("Cache hits should be 1, got %d", stats.Hits)
	}
	if stats.Size != 1 {
		t.Errorf("Cache size should be 1, got %d", stats.Size)
	}
}
