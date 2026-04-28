package utils

import (
	"sync"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestNewAssetPool(t *testing.T) {
	pool := NewAssetPool()
	if pool == nil {
		t.Fatal("NewAssetPool() returned nil")
	}
}

func TestAssetPool_Get(t *testing.T) {
	pool := NewAssetPool()

	asset := pool.Get()
	if asset == nil {
		t.Fatal("AssetPool.Get() returned nil")
	}

	// Check that asset is properly initialized
	if asset.Headers == nil {
		t.Error("AssetPool.Get() Headers should be initialized")
	}
	if asset.Extra == nil {
		t.Error("AssetPool.Get() Extra should be initialized")
	}

	// Check that fields are reset
	if asset.IP != "" {
		t.Errorf("AssetPool.Get() IP = %v, want empty", asset.IP)
	}
	if asset.Port != 0 {
		t.Errorf("AssetPool.Get() Port = %d, want 0", asset.Port)
	}
}

func TestAssetPool_Get_ReturnsResetObject(t *testing.T) {
	pool := NewAssetPool()

	// Get, modify, put back
	asset1 := pool.Get()
	asset1.IP = "192.168.1.1"
	asset1.Port = 80
	asset1.Headers["X-Test"] = "value"
	asset1.Extra["custom"] = 123

	pool.Put(asset1)

	// Get again - should be reset
	asset2 := pool.Get()
	if asset2.IP != "" {
		t.Errorf("AssetPool.Get() after Put IP = %v, want empty", asset2.IP)
	}
	if asset2.Port != 0 {
		t.Errorf("AssetPool.Get() after Put Port = %d, want 0", asset2.Port)
	}
	if len(asset2.Headers) != 0 {
		t.Errorf("AssetPool.Get() after Put Headers = %d items, want 0", len(asset2.Headers))
	}
	if len(asset2.Extra) != 0 {
		t.Errorf("AssetPool.Get() after Put Extra = %d items, want 0", len(asset2.Extra))
	}
}

func TestAssetPool_Put_Nil(t *testing.T) {
	pool := NewAssetPool()

	// Put nil should be safe
	pool.Put(nil)

	// Should still be able to get valid objects
	asset := pool.Get()
	if asset == nil {
		t.Error("AssetPool.Get() should still work after Put(nil)")
	}
}

func TestAssetPool_Concurrent(t *testing.T) {
	pool := NewAssetPool()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			asset := pool.Get()
			asset.IP = "test"
			pool.Put(asset)
		}()
	}

	wg.Wait()

	// Pool should still be functional
	asset := pool.Get()
	if asset == nil {
		t.Error("AssetPool.Concurrent() pool broken after concurrent use")
	}
	pool.Put(asset)
}

func TestNewSlicePool(t *testing.T) {
	pool := NewSlicePool()
	if pool == nil {
		t.Fatal("NewSlicePool() returned nil")
	}
}

func TestSlicePool_Get(t *testing.T) {
	pool := NewSlicePool()

	slice := pool.Get()
	if slice == nil {
		t.Fatal("SlicePool.Get() returned nil")
	}

	// Check that slice is empty (reset)
	if len(*slice) != 0 {
		t.Errorf("SlicePool.Get() len = %d, want 0", len(*slice))
	}
}

func TestSlicePool_Get_ReturnsResetSlice(t *testing.T) {
	pool := NewSlicePool()

	// Get, modify, put back
	slice1 := pool.Get()
	*slice1 = append(*slice1, model.UnifiedAsset{IP: "1.1.1.1"})
	*slice1 = append(*slice1, model.UnifiedAsset{IP: "2.2.2.2"})

	pool.Put(slice1)

	// Get again - should be empty
	slice2 := pool.Get()
	if len(*slice2) != 0 {
		t.Errorf("SlicePool.Get() after Put len = %d, want 0", len(*slice2))
	}
}

func TestSlicePool_Put_Nil(t *testing.T) {
	pool := NewSlicePool()

	pool.Put(nil)

	slice := pool.Get()
	if slice == nil {
		t.Error("SlicePool.Get() should still work after Put(nil)")
	}
}

func TestSlicePool_Concurrent(t *testing.T) {
	pool := NewSlicePool()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slice := pool.Get()
			*slice = append(*slice, model.UnifiedAsset{IP: "test"})
			pool.Put(slice)
		}()
	}

	wg.Wait()
}

func TestNewMapPool(t *testing.T) {
	pool := NewMapPool()
	if pool == nil {
		t.Fatal("NewMapPool() returned nil")
	}
}

func TestMapPool_Get(t *testing.T) {
	pool := NewMapPool()

	m := pool.Get()
	if m == nil {
		t.Fatal("MapPool.Get() returned nil")
	}

	// Check that map is empty
	if len(m) != 0 {
		t.Errorf("MapPool.Get() len = %d, want 0", len(m))
	}
}

func TestMapPool_Get_ReturnsResetMap(t *testing.T) {
	pool := NewMapPool()

	// Get, modify, put back
	m1 := pool.Get()
	m1["key1"] = "value1"
	m1["key2"] = "value2"

	pool.Put(m1)

	// Get again - should be empty
	m2 := pool.Get()
	if len(m2) != 0 {
		t.Errorf("MapPool.Get() after Put len = %d, want 0", len(m2))
	}
}

func TestMapPool_Put_Nil(t *testing.T) {
	pool := NewMapPool()

	pool.Put(nil)

	m := pool.Get()
	if m == nil {
		t.Error("MapPool.Get() should still work after Put(nil)")
	}
}

func TestNewInterfaceMapPool(t *testing.T) {
	pool := NewInterfaceMapPool()
	if pool == nil {
		t.Fatal("NewInterfaceMapPool() returned nil")
	}
}

func TestInterfaceMapPool_Get(t *testing.T) {
	pool := NewInterfaceMapPool()

	m := pool.Get()
	if m == nil {
		t.Fatal("InterfaceMapPool.Get() returned nil")
	}

	if len(m) != 0 {
		t.Errorf("InterfaceMapPool.Get() len = %d, want 0", len(m))
	}
}

func TestInterfaceMapPool_Get_ReturnsResetMap(t *testing.T) {
	pool := NewInterfaceMapPool()

	m1 := pool.Get()
	m1["key1"] = "value1"
	m1["key2"] = 123
	m1["key3"] = true

	pool.Put(m1)

	m2 := pool.Get()
	if len(m2) != 0 {
		t.Errorf("InterfaceMapPool.Get() after Put len = %d, want 0", len(m2))
	}
}

func TestInterfaceMapPool_Put_Nil(t *testing.T) {
	pool := NewInterfaceMapPool()

	pool.Put(nil)

	m := pool.Get()
	if m == nil {
		t.Error("InterfaceMapPool.Get() should still work after Put(nil)")
	}
}