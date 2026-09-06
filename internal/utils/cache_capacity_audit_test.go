package utils

import (
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
)

func TestAuditCacheOverwriteCapacity(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "single-control"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			cache := NewMemoryCache(2, 0)
			defer cache.Close()
			cache.Set("cold", []model.UnifiedAsset{{IP: "192.0.2.1"}}, time.Hour)
			cache.Set("hot", []model.UnifiedAsset{{IP: "192.0.2.2"}}, time.Hour)
			cache.Get("hot")
			cache.Get("hot")
			updated := []model.UnifiedAsset{{IP: "192.0.2.3"}}
			if batch {
				cache.SetMulti(map[string][]model.UnifiedAsset{"hot": updated}, time.Hour)
			} else {
				cache.Set("hot", updated, time.Hour)
			}
			if got, ok := cache.Get("hot"); !ok || len(got) != 1 || got[0].IP != "192.0.2.3" {
				t.Fatal("updated value missing")
			}
			if _, ok := cache.Get("cold"); !ok || cache.Size() != 2 {
				t.Fatalf("overwrite evicted unrelated entry: size=%d cold=%v", cache.Size(), ok)
			}
		})
	}
}

func TestAuditCacheEmptyKeyEviction(t *testing.T) {
	cache := NewMemoryCache(1, 0)
	defer cache.Close()
	cache.Set("", []model.UnifiedAsset{{IP: "192.0.2.1"}}, time.Hour)
	cache.Set("next", []model.UnifiedAsset{{IP: "192.0.2.2"}}, time.Hour)
	if cache.Size() != 1 {
		t.Fatalf("capacity=1 but size=%d after evicting empty key", cache.Size())
	}
	if _, ok := cache.Get("next"); !ok {
		t.Fatal("new value missing")
	}
}

func TestCacheCapacityEvictionKeepsLFUAndRemovesMetadata(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "single"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			cache := NewMemoryCache(2, 0)
			defer cache.Close()
			cache.Set("", []model.UnifiedAsset{{IP: "192.0.2.1"}}, time.Hour)
			cache.SetQueryMetadata("", QueryCacheMetadata{EngineStats: map[string]int{"fixture": 1}}, time.Hour)
			cache.Set("hot", []model.UnifiedAsset{{IP: "192.0.2.2"}}, time.Hour)
			cache.Get("hot")
			cache.Get("hot")
			value := []model.UnifiedAsset{{IP: "192.0.2.3"}}
			if batch {
				cache.SetMulti(map[string][]model.UnifiedAsset{"next": value}, time.Hour)
			} else {
				cache.Set("next", value, time.Hour)
			}
			if cache.Size() != 2 {
				t.Fatalf("size=%d", cache.Size())
			}
			if _, ok := cache.Get(""); ok {
				t.Fatal("least-frequent empty key retained")
			}
			if _, ok := cache.GetQueryMetadata(""); ok {
				t.Fatal("evicted key metadata retained")
			}
			for _, key := range []string{"hot", "next"} {
				if _, ok := cache.Get(key); !ok {
					t.Fatalf("required key missing: %s", key)
				}
			}
		})
	}
}
