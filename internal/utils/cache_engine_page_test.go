package utils

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/unimap/project/internal/model"
)

func TestEnginePageSnapshotProtocol(t *testing.T) {
	for _, backend := range []string{"memory", "redis-command"} {
		t.Run(backend, func(t *testing.T) {
			var cache QueryCache = NewMemoryCache(4, 0)
			if backend == "redis-command" {
				cache.Close()
				h := &snapshotCommandHook{values: map[string]string{}}
				client := redis.NewClient(&redis.Options{Addr: "unused.invalid:6379"})
				client.AddHook(h)
				cache = &RedisCache{client: client, ctx: context.Background(), prefix: "fixture:"}
			}
			defer cache.Close()
			assets := []model.UnifiedAsset{{Title: "original"}}
			cache.Set("k", assets, time.Hour)
			if _, hit := GetEnginePageSnapshot(cache, "k"); hit {
				t.Fatal("legacy assets accepted")
			}
			result := &model.EngineResult{EngineName: "fixture", Page: 3, Total: 37, HasMore: true, Error: "partial fixture"}
			SetEnginePageSnapshot(cache, "k", result, assets, time.Hour)
			result.Page = 9
			assets[0].Title = "changed"
			for i := 0; i < 2; i++ {
				got, hit := GetEnginePageSnapshot(cache, "k")
				if !hit || got.Page != 3 || got.Total != 37 || !got.HasMore || got.EngineName != "fixture" || got.Error != "partial fixture" || !got.Cached || got.NormalizedData[0].Title != "original" {
					t.Fatalf("changed snapshot: %+v hit=%v", got, hit)
				}
				got.Page = 12
				got.NormalizedData[0].Title = "changed"
			}
			cache.Delete("k")
			if _, hit := GetEnginePageSnapshot(cache, "k"); hit {
				t.Fatal("Delete left page snapshot")
			}
		})
	}
}
