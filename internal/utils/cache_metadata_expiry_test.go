package utils

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
)

func TestCacheMetadataExpiryOwnership(t *testing.T) {
	for _, scenario := range []string{"expired_get", "late_write_after_eviction", "metadata_expires_first", "overwrite_single", "overwrite_batch"} {
		t.Run(scenario, func(t *testing.T) {
			c := NewMemoryCache(1, 0)
			defer c.Close()
			assets := []model.UnifiedAsset{{Title: "fixture"}}
			c.Set("old", assets, time.Hour)
			c.SetQueryMetadata("old", QueryCacheMetadata{EngineStats: map[string]int{"fixture": 1}}, time.Hour)
			switch scenario {
			case "expired_get":
				item := c.cache["old"]
				item.expiryTime = time.Now().Add(-time.Hour)
				c.cache["old"] = item
				if _, ok := c.Get("old"); ok {
					t.Fatal("expired assets hit")
				}
				c.cleanupExpired()
			case "late_write_after_eviction":
				c.Set("new", assets, time.Hour)
				c.SetQueryMetadata("old", QueryCacheMetadata{}, time.Hour)
			case "metadata_expires_first":
				item := c.metadata["old"]
				item.expiryTime = time.Now().Add(-time.Hour)
				c.metadata["old"] = item
				c.cleanupExpired()
				if _, ok := c.Get("old"); !ok {
					t.Fatal("live assets lost")
				}
			case "overwrite_single":
				c.Set("old", assets, time.Hour)
			case "overwrite_batch":
				c.SetMulti(map[string][]model.UnifiedAsset{"old": assets}, time.Hour)
			}
			if _, ok := c.metadata["old"]; ok {
				t.Fatalf("stale metadata retained: assets=%d metadata=%d", len(c.cache), len(c.metadata))
			}
		})
	}
}

func TestCacheMetadataLiveBoundsAndCopies(t *testing.T) {
	c := NewMemoryCache(2, 0)
	defer c.Close()
	c.Set("live", []model.UnifiedAsset{{Title: "fixture"}}, time.Hour)
	source := QueryCacheMetadata{EngineStats: map[string]int{"fixture": 1}, Errors: []string{"fixture error"}}
	c.SetQueryMetadata("live", source, 2*time.Hour)
	source.EngineStats["fixture"] = 9
	source.Errors[0] = "changed"
	got, ok := c.GetQueryMetadata("live")
	if !ok || got.EngineStats["fixture"] != 1 || got.Errors[0] != "fixture error" {
		t.Fatalf("copy on set: %+v hit=%v", got, ok)
	}
	got.EngineStats["fixture"] = 8
	got, _ = c.GetQueryMetadata("live")
	if got.EngineStats["fixture"] != 1 {
		t.Fatal("copy on get lost")
	}
	if c.metadata["live"].expiryTime != c.cache["live"].expiryTime {
		t.Fatal("metadata outlives assets")
	}
	c.SetQueryMetadata("missing", source, time.Hour)
	if _, ok := c.metadata["missing"]; ok {
		t.Fatal("orphan inserted")
	}
	item := c.cache["live"]
	item.expiryTime = time.Now().Add(-time.Hour)
	c.cache["live"] = item
	if _, ok := c.GetQueryMetadata("live"); ok {
		t.Fatal("metadata hit for expired assets")
	}
	c.SetQueryMetadata("live", source, time.Hour)
	if len(c.metadata) != 0 {
		t.Fatal("metadata written for expired assets")
	}
	c.Set("live", nil, time.Hour)
	c.SetQueryMetadata("live", source, -time.Second)
	if len(c.metadata) != 0 {
		t.Fatal("already expired metadata retained")
	}
}

func TestCacheMetadataConcurrentCapacity(t *testing.T) {
	c := NewMemoryCache(4, 0)
	defer c.Close()
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("%d-%d", id, i)
				c.Set(key, nil, time.Hour)
				c.SetQueryMetadata(key, QueryCacheMetadata{}, time.Hour)
				c.GetQueryMetadata(key)
				if i%2 == 0 {
					c.Delete(key)
				}
			}
		}(worker)
	}
	wg.Wait()
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if len(c.metadata) > len(c.cache) || len(c.cache) > c.maxSize {
		t.Fatalf("assets=%d metadata=%d", len(c.cache), len(c.metadata))
	}
	for key := range c.metadata {
		if _, ok := c.cache[key]; !ok {
			t.Fatalf("orphan %s", key)
		}
	}
}
