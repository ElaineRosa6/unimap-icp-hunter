package utils

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
)

func TestCacheAssetSnapshotTypes(t *testing.T) {
	type record struct{ Values []int }
	type namedSlice []int
	n := 7
	input := []model.UnifiedAsset{{Extra: map[string]interface{}{
		"integer": int64(9007199254740993), "slice": namedSlice{1, 2}, "array": [1][]byte{{3}},
		"pointer": &n, "struct": record{Values: []int{4}}, "time": time.Now(), "nil": (*int)(nil),
	}}}
	c := NewMemoryCache(2, 0)
	defer c.Close()
	c.Set("k", input, time.Hour)
	got, ok := c.Get("k")
	if !ok || !reflect.DeepEqual(got, input) {
		t.Fatalf("type/value loss: %+v", got)
	}
	*got[0].Extra["pointer"].(*int) = 8
	got[0].Extra["struct"].(record).Values[0] = 8
	got[0].Extra["array"].([1][]byte)[0][0] = 8
	again, _ := c.Get("k")
	if !reflect.DeepEqual(again, input) {
		t.Fatal("nested snapshot mutation leaked")
	}
	for _, assets := range [][]model.UnifiedAsset{nil, {}, {{}}} {
		c.Set("shape", assets, time.Hour)
		out, hit := c.Get("shape")
		if !hit || !reflect.DeepEqual(out, assets) {
			t.Fatalf("nil/empty shape changed: %#v -> %#v", assets, out)
		}
	}
}

func TestCacheAssetUnsupportedBypasses(t *testing.T) {
	cycle := map[string]interface{}{}
	cycle["self"] = cycle
	type opaque struct{ data []int }
	for _, value := range []interface{}{cycle, make(chan int), func() {}, opaque{data: []int{1}}, map[*int]string{}, make([]int, 100001)} {
		for _, batch := range []bool{false, true} {
			c := NewMemoryCache(2, 0)
			c.Set("old", nil, time.Hour)
			c.SetQueryMetadata("old", QueryCacheMetadata{}, time.Hour)
			c.Set("keep", nil, time.Hour)
			invalid := []model.UnifiedAsset{{Extra: map[string]interface{}{"value": value}}}
			if batch {
				c.SetMulti(map[string][]model.UnifiedAsset{"old": invalid}, time.Hour)
			} else {
				c.Set("old", invalid, time.Hour)
			}
			if _, hit := c.Get("old"); hit {
				t.Errorf("cached unsupported type %T", value)
			}
			if _, hit := c.GetQueryMetadata("old"); hit {
				t.Error("stale metadata retained")
			}
			if _, hit := c.Get("keep"); !hit {
				t.Error("unrelated entry evicted")
			}
			c.Close()
		}
	}
}

func TestCacheAssetConcurrentReaders(t *testing.T) {
	c := NewMemoryCache(2, 0)
	defer c.Close()
	c.Set("k", []model.UnifiedAsset{{Headers: map[string]string{"k": "original"}, Extra: map[string]interface{}{"list": []int{1}}}}, time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got, ok := c.Get("k")
				if !ok {
					t.Error("cache miss")
					return
				}
				got[0].Headers["k"] = "changed"
				got[0].Extra["list"].([]int)[0] = 2
				batch := c.GetMulti([]string{"k"})["k"]
				if batch[0].Headers["k"] != "original" || batch[0].Extra["list"].([]int)[0] != 1 {
					t.Error("cross-reader mutation")
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkCacheAssetSnapshot(b *testing.B) {
	c := NewMemoryCache(2, 0)
	defer c.Close()
	assets := make([]model.UnifiedAsset, 100)
	for i := range assets {
		assets[i] = model.UnifiedAsset{Title: "fixture", Headers: map[string]string{"k": "value"}, Extra: map[string]interface{}{"list": []string{"a", "b"}}}
	}
	c.Set("k", assets, time.Hour)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := c.Get("k"); !ok {
			b.Fatal("miss")
		}
	}
}
