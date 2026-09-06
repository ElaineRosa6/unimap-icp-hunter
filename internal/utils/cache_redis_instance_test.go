package utils

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
)

// Run only against an isolated instance created by the acceptance harness.
func TestRedisInstanceQuerySnapshot(t *testing.T) {
	addr := os.Getenv("UNIMAP_REDIS_FIXTURE_ADDR")
	if addr == "" {
		if os.Getenv("UNIMAP_REDIS_FIXTURE_REQUIRED") == "1" {
			t.Fatal("required Redis fixture address is missing")
		}
		t.Skip("isolated Redis fixture not configured")
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatal("fixture must be loopback")
	}
	prefix := fmt.Sprintf("snapshot-fixture:%d:", time.Now().UnixNano())
	c := NewRedisCache(RedisConfig{Addr: addr, Prefix: prefix})
	if c == nil {
		t.Fatal("fixture unavailable")
	}
	defer c.Close()
	defer c.Clear()
	info, err := c.client.Info(c.ctx, "server").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			t.Log(strings.TrimSpace(line))
		}
	}
	write := func(key, tag string, ttl time.Duration) {
		c.SetQuerySnapshot(key, []model.UnifiedAsset{{Title: tag}}, QueryCacheMetadata{EngineStats: map[string]int{tag: 1}, Errors: []string{tag}}, ttl)
	}
	check := func(key string) bool {
		assets, meta, hit := c.GetQuerySnapshot(key)
		if !hit {
			return false
		}
		if len(assets) != 1 || len(meta.Errors) != 1 || meta.EngineStats[assets[0].Title] != 1 || meta.Errors[0] != assets[0].Title {
			t.Errorf("mixed snapshot: %+v %+v", assets, meta)
		}
		return true
	}
	t.Run("concurrent_generation", func(t *testing.T) {
		var wg sync.WaitGroup
		for worker := 0; worker < 8; worker++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					write("shared", fmt.Sprintf("%d-%d", id, i), time.Minute)
					if !check("shared") {
						t.Error("missing published snapshot")
						return
					}
				}
			}(worker)
		}
		wg.Wait()
	})
	t.Run("ttl", func(t *testing.T) {
		write("ttl", "ttl", 300*time.Millisecond)
		remaining, err := c.client.PTTL(c.ctx, c.querySnapshotKey("ttl")).Result()
		if err != nil || remaining <= 0 || remaining > 300*time.Millisecond {
			t.Fatalf("TTL=%v err=%v", remaining, err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for check("ttl") {
			if time.Now().After(deadline) {
				t.Fatal("snapshot did not expire")
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
	t.Run("legacy_and_malformed", func(t *testing.T) {
		c.Set("legacy", []model.UnifiedAsset{{Title: "old"}}, time.Minute)
		c.SetQueryMetadata("legacy", QueryCacheMetadata{}, time.Minute)
		if check("legacy") {
			t.Error("legacy pair read")
		}
		if err := c.client.Set(c.ctx, c.querySnapshotKey("broken"), `{"version":2,"metadata":{}}`, time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		if check("broken") {
			t.Error("unknown version read")
		}
	})
	t.Run("delete_and_clear_isolation", func(t *testing.T) {
		sentinel := "outside:" + prefix
		if err := c.client.Set(c.ctx, sentinel, "keep", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		defer c.client.Del(c.ctx, sentinel)
		write("delete", "delete", time.Minute)
		c.Delete("delete")
		if check("delete") {
			t.Error("delete left snapshot")
		}
		write("clear", "clear", time.Minute)
		c.Clear()
		if check("clear") {
			t.Error("clear left snapshot")
		}
		value, err := c.client.Get(c.ctx, sentinel).Result()
		if err != nil || value != "keep" {
			t.Fatalf("unrelated key affected: %q %v", value, err)
		}
	})
}
