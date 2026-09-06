package service

import (
	"context"
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/utils"
)

// A completed writer after the snapshot read must not change its generation.
type generationAuditCache struct {
	*utils.MemoryCache
	interleave     bool
	snapshotWrites int
	legacyWrites   int
	snapshotReads  int
	legacyReads    int
}

func (c *generationAuditCache) Get(key string) ([]model.UnifiedAsset, bool) {
	c.legacyReads++
	return c.MemoryCache.Get(key)
}
func (c *generationAuditCache) GetQuerySnapshot(key string) ([]model.UnifiedAsset, utils.QueryCacheMetadata, bool) {
	c.snapshotReads++
	assets, metadata, hit := c.MemoryCache.GetQuerySnapshot(key)
	if hit && c.interleave {
		c.MemoryCache.SetQuerySnapshot(key, []model.UnifiedAsset{{Title: "generation-B"}}, utils.QueryCacheMetadata{EngineStats: map[string]int{"generation-B": 1}, Errors: []string{"generation-B"}}, time.Hour)
	}
	return assets, metadata, hit
}

func TestAuditCachedResponseGeneration(t *testing.T) {
	for _, interleave := range []bool{false, true} {
		name := "control"
		if interleave {
			name = "writer_between_reads"
		}
		t.Run(name, func(t *testing.T) {
			svc := NewUnifiedServiceWithConfig(nil)
			defer svc.Shutdown()
			svc.cache.Close()
			c := &generationAuditCache{MemoryCache: utils.NewMemoryCache(2, 0), interleave: interleave}
			svc.cache = c
			req := QueryRequest{Query: `port="443"`, Engines: []string{"fofa"}, PageSize: 10}
			key := svc.buildQueryCacheKey(req)
			c.SetQuerySnapshot(key, []model.UnifiedAsset{{Title: "generation-A"}}, utils.QueryCacheMetadata{EngineStats: map[string]int{"generation-A": 1}, Errors: []string{"generation-A"}}, time.Hour)
			response, hit := svc.handleCachedQueryResult(context.Background(), req, key)
			if c.snapshotReads != 1 || c.legacyReads != 0 {
				t.Fatalf("read path snapshot=%d legacy=%d", c.snapshotReads, c.legacyReads)
			}
			if !hit {
				t.Fatal("expected complete cached response")
			}
			if len(response.Assets) != 1 || len(response.Errors) != 1 {
				t.Fatalf("unexpected response: %+v", response)
			}
			generation := response.Assets[0].Title
			if response.EngineStats[generation] != 1 || response.Errors[0] != generation {
				t.Fatalf("mixed generation: asset=%q stats=%v errors=%v", generation, response.EngineStats, response.Errors)
			}
		})
	}
}

func (c *generationAuditCache) Set(key string, assets []model.UnifiedAsset, ttl time.Duration) {
	c.legacyWrites++
	c.MemoryCache.Set(key, assets, ttl)
}
func (c *generationAuditCache) SetQuerySnapshot(key string, assets []model.UnifiedAsset, metadata utils.QueryCacheMetadata, ttl time.Duration) {
	c.snapshotWrites++
	c.MemoryCache.SetQuerySnapshot(key, assets, metadata, ttl)
}

type legacyQueryCache struct {
	utils.QueryCache
	utils.QueryCacheMetadataCache
}

func TestQueryUsesAtomicSnapshotProtocol(t *testing.T) {
	svc := NewUnifiedServiceWithConfig(nil)
	defer svc.Shutdown()
	svc.cache.Close()
	c := &generationAuditCache{MemoryCache: utils.NewMemoryCache(2, 0)}
	svc.cache = c
	svc.RegisterAdapter(&testMockAdapter{name: "fofa", results: []model.UnifiedAsset{{IP: "192.0.2.1", Source: "fofa"}}})
	req := QueryRequest{Query: `port="443"`, Engines: []string{"fofa"}, PageSize: 10}
	for i := 0; i < 2; i++ {
		response, err := svc.Query(context.Background(), req)
		if err != nil || response == nil || len(response.Assets) != 1 || response.EngineStats["fofa"] != 1 {
			t.Fatalf("query %d: %+v %v", i, response, err)
		}
	}
	if c.snapshotReads != 2 || c.snapshotWrites != 1 || c.legacyReads != 0 || c.legacyWrites != 0 {
		t.Fatalf("snapshot read/write=%d/%d legacy=%d/%d", c.snapshotReads, c.snapshotWrites, c.legacyReads, c.legacyWrites)
	}
	svc.cache = &legacyQueryCache{QueryCache: c, QueryCacheMetadataCache: c}
	if _, hit := svc.handleCachedQueryResult(context.Background(), req, svc.buildQueryCacheKey(req)); hit {
		t.Fatal("legacy backend used")
	}
	if c.legacyReads != 0 {
		t.Fatal("legacy read attempted")
	}
}
