package utils

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/unimap/project/internal/model"
)

func TestMemoryQuerySnapshotGeneration(t *testing.T) {
	c := NewMemoryCache(2, 0)
	defer c.Close()
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				tag := fmt.Sprintf("%d-%d", id, i)
				c.SetQuerySnapshot("k", []model.UnifiedAsset{{Title: tag}}, QueryCacheMetadata{EngineStats: map[string]int{tag: 1}, Errors: []string{tag}}, time.Hour)
				assets, meta, hit := c.GetQuerySnapshot("k")
				if !hit || len(assets) != 1 || len(meta.Errors) != 1 {
					t.Error("incomplete snapshot")
					return
				}
				if meta.EngineStats[assets[0].Title] != 1 || meta.Errors[0] != assets[0].Title {
					t.Error("mixed generation")
				}
				assets[0].Title = "changed"
				meta.Errors[0] = "changed"
			}
		}(worker)
	}
	wg.Wait()
}

func TestMemoryQuerySnapshotLifecycle(t *testing.T) {
	c := NewMemoryCache(1, 0)
	defer c.Close()
	assets := []model.UnifiedAsset{{Title: "original"}}
	meta := QueryCacheMetadata{Errors: []string{"original"}}
	c.SetQuerySnapshot("k", assets, meta, time.Hour)
	assets[0].Title = "changed"
	meta.Errors[0] = "changed"
	// Ordinary asset reads must preserve the attached snapshot metadata.
	c.Get("k")
	out, m, hit := c.GetQuerySnapshot("k")
	if !hit || out[0].Title != "original" || m.Errors[0] != "original" {
		t.Fatal("snapshot lost or aliased")
	}
	c.SetQueryMetadata("k", QueryCacheMetadata{Errors: []string{"legacy"}}, time.Hour)
	_, m, _ = c.GetQuerySnapshot("k")
	if m.Errors[0] != "original" {
		t.Fatal("legacy metadata changed snapshot")
	}
	c.Set("k", assets, time.Hour)
	if _, _, hit := c.GetQuerySnapshot("k"); hit {
		t.Fatal("legacy assets accepted as snapshot")
	}
	c.SetQuerySnapshot("k", assets, meta, time.Hour)
	c.SetQuerySnapshot("other", assets, meta, time.Hour)
	if _, _, hit := c.GetQuerySnapshot("k"); hit {
		t.Fatal("eviction failed")
	}
	item := c.cache["other"]
	item.expiryTime = time.Now().Add(-time.Hour)
	c.cache["other"] = item
	if _, _, hit := c.GetQuerySnapshot("other"); hit {
		t.Fatal("expired snapshot hit")
	}
	c.SetQuerySnapshot("k", assets, meta, time.Hour)
	c.Delete("k")
	if _, _, hit := c.GetQuerySnapshot("k"); hit {
		t.Fatal("deleted snapshot hit")
	}
}

type snapshotCommandHook struct {
	values   map[string]string
	commands [][]interface{}
}

func (h *snapshotCommandHook) DialHook(_ redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, fmt.Errorf("network forbidden in command fixture")
	}
}
func (h *snapshotCommandHook) ProcessPipelineHook(_ redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error { return fmt.Errorf("unexpected pipeline") }
}
func (h *snapshotCommandHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		h.commands = append(h.commands, append([]interface{}(nil), args...))
		key, ok := args[1].(string)
		if !ok {
			return fmt.Errorf("non-string key")
		}
		switch c := cmd.(type) {
		case *redis.StatusCmd:
			if cmd.Name() != "set" {
				return fmt.Errorf("unexpected status command")
			}
			data, ok := args[2].([]byte)
			if !ok {
				return fmt.Errorf("non-byte envelope")
			}
			h.values[key] = string(data)
			c.SetVal("OK")
		case *redis.StringCmd:
			value, ok := h.values[key]
			if !ok {
				c.SetErr(redis.Nil)
				return redis.Nil
			}
			c.SetVal(value)
		case *redis.IntCmd:
			for _, arg := range args[1:] {
				k, ok := arg.(string)
				if !ok {
					return fmt.Errorf("invalid delete key")
				}
				delete(h.values, k)
			}
			c.SetVal(1)
		default:
			return fmt.Errorf("unexpected command %T", cmd)
		}
		return nil
	}
}

func TestRedisQuerySnapshotSingleEnvelope(t *testing.T) {
	hook := &snapshotCommandHook{values: map[string]string{}}
	client := redis.NewClient(&redis.Options{Addr: "unused.invalid:6379"})
	defer client.Close()
	client.AddHook(hook)
	c := &RedisCache{client: client, ctx: context.Background(), prefix: "fixture:"}
	assets := []model.UnifiedAsset{{Title: "generation-A"}}
	meta := QueryCacheMetadata{EngineStats: map[string]int{"A": 1}, Errors: []string{"A"}}
	c.SetQuerySnapshot("k", assets, meta, time.Minute)
	if len(hook.commands) != 1 || len(hook.commands[0]) != 5 {
		t.Fatalf("not single TTL SET: %v", hook.commands)
	}
	if hook.commands[0][3] != "ex" || hook.commands[0][4] != int64(60) {
		t.Fatalf("wrong TTL: %v", hook.commands[0])
	}
	got, m, hit := c.GetQuerySnapshot("k")
	if !hit || !reflect.DeepEqual(got, assets) || !reflect.DeepEqual(m, meta) {
		t.Fatalf("roundtrip changed: %+v %+v %v", got, m, hit)
	}
	if len(hook.commands) != 2 || hook.commands[1][0] != "get" || hook.commands[1][1] != hook.commands[0][1] {
		t.Fatal("not one matching GET")
	}
	for _, bad := range []string{`{}`, `{"version":2,"metadata":{}}`, `{"version":1}`, `invalid`} {
		hook.values[c.querySnapshotKey("k")] = bad
		if _, _, hit := c.GetQuerySnapshot("k"); hit {
			t.Fatalf("accepted %s", bad)
		}
	}
	c.SetQuerySnapshot("k", assets, meta, time.Minute)
	c.Delete("k")
	if _, _, hit := c.GetQuerySnapshot("k"); hit {
		t.Fatal("delete left snapshot")
	}
	hook.values[c.prefix+"k"] = `[]`
	hook.values[c.prefix+"k:query-metadata"] = `{}`
	if _, _, hit := c.GetQuerySnapshot("k"); hit {
		t.Fatal("legacy keys used")
	}
}
