package utils

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/unimap-icp-hunter/project/internal/model"
)

// QueryCache 查询缓存接口
type QueryCache interface {
	// Get 从缓存中获取查询结果
	Get(key string) ([]model.UnifiedAsset, bool)
	// Set 将查询结果存入缓存
	Set(key string, assets []model.UnifiedAsset, duration time.Duration)
	// Delete 从缓存中删除查询结果
	Delete(key string)
	// Clear 清空缓存
	Clear()
	// Size 获取缓存大小
	Size() int
	// GetStats 获取缓存统计信息
	GetStats() CacheStats
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Hits        int       // 缓存命中次数
	Misses      int       // 缓存未命中次数
	Size        int       // 当前缓存大小
	MaxSize     int       // 最大缓存大小
	HitRate     float64   // 缓存命中率
	LastCleanup time.Time // 上次清理时间
}

// MemoryCache 内存缓存实现
type MemoryCache struct {
	cache           map[string]cacheItem
	mutex           sync.RWMutex
	maxSize         int
	cleanupInterval time.Duration
	hits            int
	misses          int
	lastCleanup     time.Time
	accessCounter   uint64
}

// cacheItem 缓存项
type cacheItem struct {
	assets     []model.UnifiedAsset
	expiryTime time.Time
	lastAccess time.Time
	accessIdx  uint64
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache(maxSize int, cleanupInterval time.Duration) *MemoryCache {
	cache := &MemoryCache{
		cache:           make(map[string]cacheItem),
		maxSize:         maxSize,
		cleanupInterval: cleanupInterval,
		lastCleanup:     time.Now(),
	}

	// 启动定期清理过期缓存的 goroutine
	if cleanupInterval > 0 {
		go cache.startCleanupLoop()
	}

	return cache
}

// Get 从缓存中获取查询结果
func (c *MemoryCache) Get(key string) ([]model.UnifiedAsset, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, ok := c.cache[key]
	if !ok {
		c.misses++
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(item.expiryTime) {
		delete(c.cache, key)
		c.misses++
		return nil, false
	}

	// 更新最后访问时间/序号
	c.accessCounter++
	c.cache[key] = cacheItem{
		assets:     item.assets,
		expiryTime: item.expiryTime,
		lastAccess: time.Now(),
		accessIdx:  c.accessCounter,
	}

	c.hits++
	return item.assets, true
}

// Set 将查询结果存入缓存
func (c *MemoryCache) Set(key string, assets []model.UnifiedAsset, duration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	_, exists := c.cache[key]
	// 检查缓存大小是否超过限制（覆盖已有 key 不应触发驱逐）
	if !exists && len(c.cache) >= c.maxSize {
		// 使用LRU策略：删除最近最少使用的项
		c.evictLRU()
	}

	// 存入缓存
	c.accessCounter++
	c.cache[key] = cacheItem{
		assets:     assets,
		expiryTime: time.Now().Add(duration),
		lastAccess: time.Now(),
		accessIdx:  c.accessCounter,
	}
}

// Delete 从缓存中删除查询结果
func (c *MemoryCache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.cache, key)
}

// Clear 清空缓存
func (c *MemoryCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache = make(map[string]cacheItem)
	c.hits = 0
	c.misses = 0
}

// Size 获取缓存大小
func (c *MemoryCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.cache)
}

// GetStats 获取缓存统计信息
func (c *MemoryCache) GetStats() CacheStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Hits:        c.hits,
		Misses:      c.misses,
		Size:        len(c.cache),
		MaxSize:     c.maxSize,
		HitRate:     hitRate,
		LastCleanup: c.lastCleanup,
	}
}

// evictLRU 使用LRU策略删除缓存项
func (c *MemoryCache) evictLRU() {
	if len(c.cache) == 0 {
		return
	}

	lruKey := ""
	var lruIdx uint64
	hasLRU := false

	for key, item := range c.cache {
		if !hasLRU || item.accessIdx < lruIdx {
			lruKey = key
			lruIdx = item.accessIdx
			hasLRU = true
		}
	}

	if lruKey != "" {
		delete(c.cache, lruKey)
	}
}

// startCleanupLoop 启动定期清理过期缓存的循环
func (c *MemoryCache) startCleanupLoop() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanupExpired()
	}
}

// cleanupExpired 清理过期的缓存项
func (c *MemoryCache) cleanupExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	for key, item := range c.cache {
		if now.After(item.expiryTime) {
			delete(c.cache, key)
		}
	}

	c.lastCleanup = now
}

// GenerateCacheKey 生成缓存键
func GenerateCacheKey(engineName, query string, page, pageSize int) string {
	// 组合查询参数
	cacheKey := fmt.Sprintf("%s:%s:%d:%d", engineName, query, page, pageSize)

	// 使用MD5生成哈希值作为缓存键
	hash := md5.Sum([]byte(cacheKey))
	return hex.EncodeToString(hash[:])
}

// RedisCache Redis缓存实现
type RedisCache struct {
	client *redis.Client
	ctx    context.Context
	prefix string
	hits   int
	misses int
	mutex  sync.RWMutex
}

// NewRedisCache 创建Redis缓存
func NewRedisCache(addr, password string, db int, prefix string) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	cache := &RedisCache{
		client: client,
		ctx:    context.Background(),
		prefix: prefix,
	}

	// 测试连接
	if err := client.Ping(cache.ctx).Err(); err != nil {
		fmt.Printf("Redis connection failed: %v, falling back to memory cache\n", err)
		return nil
	}

	return cache
}

// Get 从缓存中获取查询结果
func (c *RedisCache) Get(key string) ([]model.UnifiedAsset, bool) {
	fullKey := c.prefix + key

	val, err := c.client.Get(c.ctx, fullKey).Result()
	if err != nil {
		if err == redis.Nil {
			c.mutex.Lock()
			c.misses++
			c.mutex.Unlock()
			return nil, false
		}
		// Redis错误，返回未命中
		c.mutex.Lock()
		c.misses++
		c.mutex.Unlock()
		return nil, false
	}

	// 解析JSON
	var assets []model.UnifiedAsset
	if err := json.Unmarshal([]byte(val), &assets); err != nil {
		c.mutex.Lock()
		c.misses++
		c.mutex.Unlock()
		return nil, false
	}

	c.mutex.Lock()
	c.hits++
	c.mutex.Unlock()
	return assets, true
}

// Set 将查询结果存入缓存
func (c *RedisCache) Set(key string, assets []model.UnifiedAsset, duration time.Duration) {
	fullKey := c.prefix + key

	// 序列化为JSON
	data, err := json.Marshal(assets)
	if err != nil {
		return
	}

	// 存入Redis
	c.client.Set(c.ctx, fullKey, data, duration)
}

// Delete 从缓存中删除查询结果
func (c *RedisCache) Delete(key string) {
	fullKey := c.prefix + key
	c.client.Del(c.ctx, fullKey)
}

// Clear 清空缓存
func (c *RedisCache) Clear() {
	// 使用前缀匹配删除所有键
	keys, err := c.client.Keys(c.ctx, c.prefix+"*").Result()
	if err == nil && len(keys) > 0 {
		c.client.Del(c.ctx, keys...)
	}

	c.mutex.Lock()
	c.hits = 0
	c.misses = 0
	c.mutex.Unlock()
}

// Size 获取缓存大小
func (c *RedisCache) Size() int {
	keys, err := c.client.Keys(c.ctx, c.prefix+"*").Result()
	if err != nil {
		return 0
	}
	return len(keys)
}

// GetStats 获取缓存统计信息
func (c *RedisCache) GetStats() CacheStats {
	c.mutex.RLock()
	hits := c.hits
	misses := c.misses
	c.mutex.RUnlock()

	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
		Hits:        hits,
		Misses:      misses,
		Size:        c.Size(),
		MaxSize:     -1, // Redis没有固定大小限制
		HitRate:     hitRate,
		LastCleanup: time.Now(),
	}
}

// NewCache 创建缓存实例，优先使用Redis，失败则使用内存缓存
func NewCache(useRedis bool, redisAddr, redisPassword string, redisDB int, redisPrefix string, memoryMaxSize int, cleanupInterval time.Duration) QueryCache {
	if useRedis {
		redisCache := NewRedisCache(redisAddr, redisPassword, redisDB, redisPrefix)
		if redisCache != nil {
			return redisCache
		}
	}

	// 回退到内存缓存
	return NewMemoryCache(memoryMaxSize, cleanupInterval)
}

// WarmupCache 预热缓存
func WarmupCache(cache QueryCache, queries []struct {
	EngineName string
	Query      string
	Page       int
	PageSize   int
}, duration time.Duration) {
	// 这里可以实现缓存预热逻辑
	// 例如，预先执行一些常见的查询并将结果存入缓存
	// 实际实现需要根据具体的查询逻辑来编写
}
