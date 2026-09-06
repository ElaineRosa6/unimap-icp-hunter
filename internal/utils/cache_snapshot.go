package utils

import (
	"encoding/json"
	"time"

	"github.com/unimap/project/internal/model"
)

// QuerySnapshotCache keeps assets and response metadata from the same write.
// Implementations must never assemble a snapshot from separate cache reads.
type QuerySnapshotCache interface {
	GetQuerySnapshot(key string) ([]model.UnifiedAsset, QueryCacheMetadata, bool)
	SetQuerySnapshot(key string, assets []model.UnifiedAsset, metadata QueryCacheMetadata, duration time.Duration)
}

func (c *MemoryCache) GetQuerySnapshot(key string) ([]model.UnifiedAsset, QueryCacheMetadata, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	item, exists := c.cache[key]
	if !exists || item.queryMetadata == nil {
		c.misses++
		return nil, QueryCacheMetadata{}, false
	}
	if !time.Now().Before(item.expiryTime) {
		delete(c.cache, key)
		delete(c.metadata, key)
		c.misses++
		return nil, QueryCacheMetadata{}, false
	}
	assets, valid := cloneCacheAssets(item.assets)
	if !valid {
		c.misses++
		return nil, QueryCacheMetadata{}, false
	}
	c.accessCounter++
	item.accessIdx = c.accessCounter
	item.accessFreq++
	item.lastAccess = time.Now()
	c.cache[key] = item
	c.hits++
	return assets, cloneQueryCacheMetadata(*item.queryMetadata), true
}

func (c *MemoryCache) SetQuerySnapshot(key string, assets []model.UnifiedAsset, metadata QueryCacheMetadata, duration time.Duration) {
	snapshot, valid := cloneCacheAssets(assets)
	meta := cloneQueryCacheMetadata(metadata)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !valid || duration <= 0 {
		delete(c.cache, key)
		delete(c.metadata, key)
		return
	}
	if _, exists := c.cache[key]; !exists && len(c.cache) >= c.maxSize {
		c.evictLFU()
	}
	c.accessCounter++
	now := time.Now()
	c.cache[key] = cacheItem{assets: snapshot, queryMetadata: &meta, expiryTime: now.Add(duration), lastAccess: now, accessIdx: c.accessCounter, accessFreq: 1}
	delete(c.metadata, key)
}

// Separate namespace makes legacy asset/metadata pairs unreachable to this
// protocol. One Redis SET publishes the entire response with one TTL.
func (c *RedisCache) querySnapshotKey(key string) string {
	return c.prefix + "query-snapshot:v1:" + key
}

type querySnapshotEnvelope struct {
	Version  int                  `json:"version"`
	Assets   []model.UnifiedAsset `json:"assets"`
	Metadata *QueryCacheMetadata  `json:"metadata"`
}

func (c *RedisCache) GetQuerySnapshot(key string) ([]model.UnifiedAsset, QueryCacheMetadata, bool) {
	data, err := c.client.Get(c.ctx, c.querySnapshotKey(key)).Bytes()
	var entry querySnapshotEnvelope
	hit := err == nil && json.Unmarshal(data, &entry) == nil && entry.Version == 1 && entry.Metadata != nil
	c.mutex.Lock()
	if hit {
		c.hits++
	} else {
		c.misses++
	}
	c.mutex.Unlock()
	if !hit {
		return nil, QueryCacheMetadata{}, false
	}
	return entry.Assets, *entry.Metadata, true
}

func (c *RedisCache) SetQuerySnapshot(key string, assets []model.UnifiedAsset, metadata QueryCacheMetadata, duration time.Duration) {
	fullKey := c.querySnapshotKey(key)
	data, err := json.Marshal(querySnapshotEnvelope{Version: 1, Assets: assets, Metadata: &metadata})
	if err != nil || duration <= 0 {
		c.client.Del(c.ctx, fullKey)
		return
	}
	c.client.Set(c.ctx, fullKey, data, duration)
}
