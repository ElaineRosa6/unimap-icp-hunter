package utils

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/unimap-icp-hunter/project/internal/logger"
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
	// Close 关闭缓存，释放资源
	Close()
	// IsHealthy 检查缓存是否健康
	IsHealthy() bool
	// GetMulti 批量获取多个键的值
	GetMulti(keys []string) map[string][]model.UnifiedAsset
	// SetMulti 批量设置多个键值对
	SetMulti(keyAssets map[string][]model.UnifiedAsset, duration time.Duration)
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
	stopChan        chan struct{} // 用于停止清理 goroutine
	stopped         bool          // 标记是否已停止
}

// cacheItem 缓存项
type cacheItem struct {
	assets     []model.UnifiedAsset
	expiryTime time.Time
	lastAccess time.Time
	accessIdx  uint64
	accessFreq int // 访问频率计数
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache(maxSize int, cleanupInterval time.Duration) *MemoryCache {
	if maxSize <= 0 {
		maxSize = 10000 // Default max entries to prevent unbounded growth
	}
	cache := &MemoryCache{
		cache:           make(map[string]cacheItem),
		maxSize:         maxSize,
		cleanupInterval: cleanupInterval,
		lastCleanup:     time.Now(),
		stopChan:        make(chan struct{}),
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

	// 更新访问信息
	c.accessCounter++
	item.accessFreq++ // 增加访问频率计数
	c.cache[key] = cacheItem{
		assets:     item.assets,
		expiryTime: item.expiryTime,
		lastAccess: time.Now(),
		accessIdx:  c.accessCounter,
		accessFreq: item.accessFreq,
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
		// 使用LFU策略：删除访问频率最低的项
		c.evictLFU()
	}

	// 存入缓存
	c.accessCounter++
	c.cache[key] = cacheItem{
		assets:     assets,
		expiryTime: time.Now().Add(duration),
		lastAccess: time.Now(),
		accessIdx:  c.accessCounter,
		accessFreq: 1, // 初始访问频率为1
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

// GetMulti 批量获取多个键的值
func (c *MemoryCache) GetMulti(keys []string) map[string][]model.UnifiedAsset {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make(map[string][]model.UnifiedAsset)
	now := time.Now()

	for _, key := range keys {
		if item, exists := c.cache[key]; exists {
			// 检查是否过期
			if !item.expiryTime.IsZero() && now.After(item.expiryTime) {
				continue
			}
			result[key] = item.assets
		}
	}

	return result
}

// SetMulti 批量设置多个键值对
func (c *MemoryCache) SetMulti(keyAssets map[string][]model.UnifiedAsset, duration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	c.accessCounter++

	for key, assets := range keyAssets {
		// 检查缓存大小，如果超过限制，先删除一些项
		if len(c.cache) >= c.maxSize {
			c.evictLFU()
		}

		c.cache[key] = cacheItem{
			assets:     assets,
			expiryTime: now.Add(duration),
			lastAccess: now,
			accessIdx:  c.accessCounter,
			accessFreq: 1,
		}
	}
}

// evictLFU 使用LFU策略删除缓存项
func (c *MemoryCache) evictLFU() {
	if len(c.cache) == 0 {
		return
	}

	lfuKey := ""
	var lfuFreq int
	var lfuIdx uint64
	hasLFU := false

	for key, item := range c.cache {
		if !hasLFU || item.accessFreq < lfuFreq {
			lfuKey = key
			lfuFreq = item.accessFreq
			lfuIdx = item.accessIdx
			hasLFU = true
		} else if item.accessFreq == lfuFreq {
			// 如果访问频率相同，使用LRU作为tie-breaker
			if item.accessIdx < lfuIdx {
				lfuKey = key
				lfuIdx = item.accessIdx
			}
		}
	}

	if lfuKey != "" {
		delete(c.cache, lfuKey)
	}
}

// startCleanupLoop 启动定期清理过期缓存的循环
func (c *MemoryCache) startCleanupLoop() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopChan:
			return
		}
	}
}

// Close 关闭缓存，停止清理 goroutine
func (c *MemoryCache) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.stopped {
		return
	}
	c.stopped = true
	close(c.stopChan)
}

// IsHealthy 内存缓存始终健康
func (c *MemoryCache) IsHealthy() bool {
	return true
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
	// 规范化查询字符串
	normalizedQuery := normalizeQuery(query)

	// 组合查询参数
	cacheKey := fmt.Sprintf("%s:%s:%d:%d", engineName, normalizedQuery, page, pageSize)

	// 使用MD5生成哈希值作为缓存键
	hash := md5.Sum([]byte(cacheKey))
	return hex.EncodeToString(hash[:])
}

// normalizeQuery 规范化查询字符串
func normalizeQuery(query string) string {
	// 去除首尾空白
	query = strings.TrimSpace(query)

	// 转换为小写
	query = strings.ToLower(query)

	// 去除多余的空白
	query = strings.Join(strings.Fields(query), " ")

	return query
}

// RedisCache Redis缓存实现
type RedisCache struct {
	client    *redis.Client
	ctx       context.Context
	prefix    string
	hits      int
	misses    int
	mutex     sync.RWMutex
	healthy   bool
	lastCheck time.Time
}

// RedisConfig Redis配置
type RedisConfig struct {
	Addr            string
	Password        string
	DB              int
	Prefix          string
	PoolSize        int
	MinIdleConns    int
	MaxIdleConns    int
	MaxRetries      int
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	PoolTimeout     time.Duration
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// NewRedisCache 创建Redis缓存
func NewRedisCache(cfg RedisConfig) *RedisCache {
	opts := &redis.Options{
		Addr:            cfg.Addr,
		Password:        cfg.Password,
		DB:              cfg.DB,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		MaxRetries:      cfg.MaxRetries,
		DialTimeout:     cfg.DialTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		PoolTimeout:     cfg.PoolTimeout,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
	}

	// 设置默认值
	if opts.PoolSize == 0 {
		opts.PoolSize = 10
	}
	if opts.MinIdleConns == 0 {
		opts.MinIdleConns = 2
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = opts.PoolSize
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}
	if opts.DialTimeout == 0 {
		opts.DialTimeout = 5 * time.Second
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 3 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 3 * time.Second
	}
	if opts.PoolTimeout == 0 {
		opts.PoolTimeout = 4 * time.Second
	}
	if opts.ConnMaxIdleTime == 0 {
		opts.ConnMaxIdleTime = 5 * time.Minute
	}

	client := redis.NewClient(opts)

	cache := &RedisCache{
		client: client,
		ctx:    context.Background(),
		prefix: cfg.Prefix,
	}

	// 测试连接
	if err := client.Ping(cache.ctx).Err(); err != nil {
		logger.Errorf("Redis connection failed: %v, falling back to memory cache", err)
		return nil
	}

	cache.healthy = true
	cache.lastCheck = time.Now()
	logger.Infof("Redis cache initialized: addr=%s pool_size=%d min_idle=%d max_idle=%d prefix=%s",
		cfg.Addr, opts.PoolSize, opts.MinIdleConns, opts.MaxIdleConns, cfg.Prefix)
	return cache
}

// IsHealthy 检查Redis连接是否健康
func (c *RedisCache) IsHealthy() bool {
	c.mutex.RLock()
	healthy := c.healthy
	lastCheck := c.lastCheck
	c.mutex.RUnlock()

	// 每30秒检查一次
	if time.Since(lastCheck) < 30*time.Second {
		return healthy
	}

	// 执行健康检查
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
	defer cancel()

	err := c.client.Ping(ctx).Err()

	c.mutex.Lock()
	if err != nil {
		c.healthy = false
		logger.Warnf("Redis health check failed: %v", err)
	} else {
		c.healthy = true
	}
	c.lastCheck = time.Now()
	c.mutex.Unlock()

	return c.healthy
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
	// 使用SCAN命令替代KEYS命令，避免阻塞Redis
	var cursor uint64 = 0
	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(c.ctx, cursor, c.prefix+"*", 100).Result()
		if err != nil {
			logger.Errorf("Redis scan failed: %v", err)
			break
		}

		if len(keys) > 0 {
			c.client.Del(c.ctx, keys...)
		}

		if cursor == 0 {
			break
		}
	}

	c.mutex.Lock()
	c.hits = 0
	c.misses = 0
	c.mutex.Unlock()
}

// Size 获取缓存大小
func (c *RedisCache) Size() int {
	// 使用SCAN命令替代KEYS命令，避免阻塞Redis
	var cursor uint64 = 0
	var count int
	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(c.ctx, cursor, c.prefix+"*", 100).Result()
		if err != nil {
			logger.Errorf("Redis scan failed: %v", err)
			return 0
		}

		count += len(keys)

		if cursor == 0 {
			break
		}
	}
	return count
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

// GetMulti 批量获取多个键的值
func (c *RedisCache) GetMulti(keys []string) map[string][]model.UnifiedAsset {
	if len(keys) == 0 {
		return make(map[string][]model.UnifiedAsset)
	}

	// 构建完整的键名
	fullKeys := make([]string, len(keys))
	keyMap := make(map[string]string) // 完整键名到原始键名的映射
	for i, key := range keys {
		fullKey := c.prefix + key
		fullKeys[i] = fullKey
		keyMap[fullKey] = key
	}

	// 使用MGET批量获取
	pipe := c.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd)
	for _, fullKey := range fullKeys {
		cmds[fullKey] = pipe.Get(c.ctx, fullKey)
	}

	// 执行批量操作
	if _, err := pipe.Exec(c.ctx); err != nil && err != redis.Nil {
		logger.Errorf("Redis MGET failed: %v", err)
		return make(map[string][]model.UnifiedAsset)
	}

	// 处理结果
	result := make(map[string][]model.UnifiedAsset)
	for fullKey, cmd := range cmds {
		val, err := cmd.Result()
		if err != nil {
			if err != redis.Nil {
				logger.Errorf("Redis GET failed for key %s: %v", fullKey, err)
			}
			continue
		}

		// 解析JSON
		var assets []model.UnifiedAsset
		if err := json.Unmarshal([]byte(val), &assets); err != nil {
			logger.Errorf("Redis value unmarshal failed for key %s: %v", fullKey, err)
			continue
		}

		originalKey := keyMap[fullKey]
		result[originalKey] = assets
	}

	return result
}

// SetMulti 批量设置多个键值对
func (c *RedisCache) SetMulti(keyAssets map[string][]model.UnifiedAsset, duration time.Duration) {
	if len(keyAssets) == 0 {
		return
	}

	// 使用Pipeline批量设置
	pipe := c.client.Pipeline()
	for key, assets := range keyAssets {
		fullKey := c.prefix + key

		// 序列化为JSON
		data, err := json.Marshal(assets)
		if err != nil {
			logger.Errorf("JSON marshal failed for key %s: %v", key, err)
			continue
		}

		// 添加到Pipeline
		pipe.Set(c.ctx, fullKey, data, duration)
	}

	// 执行批量操作
	if _, err := pipe.Exec(c.ctx); err != nil {
		logger.Errorf("Redis MSET failed: %v", err)
	}
}

// Close 关闭Redis连接
func (c *RedisCache) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// NewCache 创建缓存实例，优先使用Redis，失败则使用内存缓存（简化版本）
func NewCache(useRedis bool, redisAddr, redisPassword string, redisDB int, redisPrefix string, memoryMaxSize int, cleanupInterval time.Duration) QueryCache {
	if useRedis {
		cfg := RedisConfig{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
			Prefix:   redisPrefix,
		}
		redisCache := NewRedisCache(cfg)
		if redisCache != nil {
			return redisCache
		}
	}

	// 回退到内存缓存
	return NewMemoryCache(memoryMaxSize, cleanupInterval)
}

// NewCacheWithConfig 创建缓存实例，支持完整的Redis配置
func NewCacheWithConfig(backend string, redisCfg RedisConfig, memoryMaxSize int, cleanupInterval time.Duration) QueryCache {
	if stringsEqualFold(backend, "redis") {
		redisCache := NewRedisCache(redisCfg)
		if redisCache != nil {
			return redisCache
		}
		logger.Warnf("Redis cache initialization failed, falling back to memory cache")
	}

	// 回退到内存缓存
	return NewMemoryCache(memoryMaxSize, cleanupInterval)
}

// stringsEqualFold 检查字符串是否相等（忽略大小写）
func stringsEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i]|32 != b[i]|32 {
			return false
		}
	}
	return true
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
