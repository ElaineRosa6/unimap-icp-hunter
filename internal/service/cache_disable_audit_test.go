package service

import (
	"context"
	"testing"
	"time"

	"github.com/unimap/project/internal/config"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/utils"
)

func TestAuditDisabledCacheConfiguration(t *testing.T) {
	for _, ttl := range []int{0, 60} {
		name := "ttl_zero"
		if ttl > 0 {
			name = "ttl_positive"
		}
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Cache.Engines = map[string]config.EngineCacheConfig{"fofa": {Enabled: false, TTL: ttl}}
			svc := NewUnifiedServiceWithConfig(cfg)
			defer svc.Shutdown()
			if svc.orchestrator.IsCacheEnabledForEngine("fofa") {
				t.Error("disabled flag lost during configuration")
			}
			req := QueryRequest{Query: `port="443"`, Engines: []string{"fofa"}, PageSize: 10}
			key := svc.buildQueryCacheKey(req)
			snapshots, ok := svc.cache.(utils.QuerySnapshotCache)
			if !ok {
				t.Fatal("missing snapshots")
			}
			snapshots.SetQuerySnapshot(key, []model.UnifiedAsset{{Title: "stale"}}, utils.QueryCacheMetadata{}, time.Hour)
			if _, hit := svc.handleCachedQueryResult(context.Background(), req, key); hit {
				t.Error("service returned snapshot containing disabled engine")
			}
		})
	}
}

func TestQueryCachePolicyCombinedAndToggle(t *testing.T) {
	svc := NewUnifiedServiceWithConfig(nil)
	defer svc.Shutdown()
	svc.cache.Close()
	c := &generationAuditCache{MemoryCache: utils.NewMemoryCache(4, 0)}
	svc.cache = c
	for _, name := range []string{"fofa", "hunter"} {
		svc.RegisterAdapter(&testMockAdapter{name: name, results: []model.UnifiedAsset{{IP: "192.0.2.1", Source: name}}})
	}
	req := QueryRequest{Query: `port="443"`, Engines: []string{"fofa", "hunter"}, PageSize: 10}
	query := func() {
		response, err := svc.Query(context.Background(), req)
		if err != nil || response == nil || len(response.Assets) == 0 {
			t.Fatalf("query: %+v %v", response, err)
		}
	}
	query()
	if c.snapshotWrites != 1 {
		t.Fatal("enabled snapshot not published")
	}
	reads, writes := c.snapshotReads, c.snapshotWrites
	svc.orchestrator.SetEngineCacheTTL("hunter", time.Hour, false)
	query()
	query()
	if c.snapshotReads != reads || c.snapshotWrites != writes {
		t.Fatal("disabled combined query touched snapshot")
	}
	svc.orchestrator.SetEngineCacheTTL("hunter", time.Hour, true)
	query()
	if c.snapshotReads != reads+1 || c.snapshotWrites != writes {
		t.Fatal("reenabled query failed to reuse unexpired snapshot")
	}
	// A disabled first request must not publish any snapshot either.
	c.Clear()
	svc.orchestrator.SetEngineCacheTTL("fofa", time.Hour, false)
	query()
	if c.Size() != 0 || c.snapshotWrites != writes {
		t.Fatal("disabled cold query populated snapshot")
	}
}
