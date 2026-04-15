package objectpool

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNewSimpleObjectPool(t *testing.T) {
	t.Run("uses defaults for zero values", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{})
		stats := pool.GetStats()
		if stats.TotalObjects != 0 {
			t.Errorf("expected 0 total objects, got %d", stats.TotalObjects)
		}
		// Verify pool is not closed
		p := pool.(*SimpleObjectPool)
		if p.closed {
			t.Error("expected pool to not be closed")
		}
	})

	t.Run("respects provided values", func(t *testing.T) {
		factoryCalled := false
		pool := NewSimpleObjectPool(Config{
			MaxSize:       5,
			InitialSize:   3,
			MaxWaitTime:   time.Second,
			ObjectFactory: func() (interface{}, error) {
				factoryCalled = true
				return "obj", nil
			},
		})
		if !factoryCalled {
			t.Error("expected factory to be called for initial objects")
		}
		stats := pool.GetStats()
		if stats.TotalObjects != 3 {
			t.Errorf("expected 3 initial objects, got %d", stats.TotalObjects)
		}
		if stats.IdleObjects != 3 {
			t.Errorf("expected 3 idle objects, got %d", stats.IdleObjects)
		}
	})

	t.Run("handles factory errors during initialization", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{
			MaxSize:     5,
			InitialSize: 3,
			ObjectFactory: func() (interface{}, error) {
				return nil, errors.New("factory error")
			},
		})
		stats := pool.GetStats()
		if stats.TotalObjects != 0 {
			t.Errorf("expected 0 objects after factory errors, got %d", stats.TotalObjects)
		}
	})
}

func TestAcquireAndRelease(t *testing.T) {
	t.Run("acquire creates new objects when pool is empty", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{
			MaxSize: 10,
			ObjectFactory: func() (interface{}, error) {
				return "new-obj", nil
			},
		})

		obj, err := pool.Acquire()
		if err != nil {
			t.Fatalf("Acquire failed: %v", err)
		}
		if obj != "new-obj" {
			t.Errorf("expected 'new-obj', got %v", obj)
		}

		stats := pool.GetStats()
		if stats.ActiveObjects != 1 {
			t.Errorf("expected 1 active object, got %d", stats.ActiveObjects)
		}
		if stats.AcquireCount != 1 {
			t.Errorf("expected acquire count 1, got %d", stats.AcquireCount)
		}
	})

	t.Run("release returns object to pool", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{
			MaxSize: 10,
			ObjectFactory: func() (interface{}, error) {
				return "obj", nil
			},
		})

		obj, _ := pool.Acquire()
		pool.Release(obj)

		stats := pool.GetStats()
		if stats.ActiveObjects != 0 {
			t.Errorf("expected 0 active objects after release, got %d", stats.ActiveObjects)
		}
		if stats.ReleaseCount != 1 {
			t.Errorf("expected release count 1, got %d", stats.ReleaseCount)
		}
		if stats.IdleObjects != 1 {
			t.Errorf("expected 1 idle object, got %d", stats.IdleObjects)
		}
	})

	t.Run("acquire reuses pooled objects", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{
			MaxSize:       10,
			InitialSize:   1,
			ObjectFactory: func() (interface{}, error) { return "factory-obj", nil },
		})

		obj1, _ := pool.Acquire()
		pool.Release(obj1)

		obj2, _ := pool.Acquire()
		// Should get the same object back from pool
		if obj2 != obj1 {
			t.Errorf("expected to get pooled object back, got different object")
		}
	})
}

func TestPoolClose(t *testing.T) {
	t.Run("close destroys all objects", func(t *testing.T) {
		destroyCount := 0
		pool := NewSimpleObjectPool(Config{
			MaxSize:       10,
			InitialSize:   3,
			ObjectFactory: func() (interface{}, error) { return "obj", nil },
			ObjectDestroyer: func(obj interface{}) {
				destroyCount++
			},
		})

		pool.Close()
		if destroyCount != 3 {
			t.Errorf("expected 3 objects destroyed, got %d", destroyCount)
		}
	})

	t.Run("acquire fails on closed pool", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{
			MaxSize:       10,
			ObjectFactory: func() (interface{}, error) { return "obj", nil },
		})
		pool.Close()

		_, err := pool.Acquire()
		if err != ErrPoolClosed {
			t.Errorf("expected ErrPoolClosed, got %v", err)
		}
	})

	t.Run("release destroys objects on closed pool", func(t *testing.T) {
		destroyCount := 0
		pool := NewSimpleObjectPool(Config{
			MaxSize: 10,
			ObjectFactory: func() (interface{}, error) { return "obj", nil },
			ObjectDestroyer: func(obj interface{}) {
				destroyCount++
			},
		})

		obj, _ := pool.Acquire()
		pool.Close()

		// Release after close should destroy
		pool.Release(obj)
		if destroyCount < 1 {
			t.Error("expected released object to be destroyed after pool closed")
		}
	})

	t.Run("close is idempotent", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{MaxSize: 10})
		pool.Close()
		pool.Close() // should not panic
	})
}

func TestInvalidObjectHandling(t *testing.T) {
	t.Run("invalid objects are destroyed and new one created", func(t *testing.T) {
		destroyCount := 0
		callCount := 0
		pool := NewSimpleObjectPool(Config{
			MaxSize:     10,
			InitialSize: 1,
			ObjectFactory: func() (interface{}, error) {
				callCount++
				return fmt.Sprintf("obj-%d", callCount), nil
			},
			ObjectValidator: func(obj interface{}) bool {
				return obj == "valid-obj"
			},
			ObjectDestroyer: func(obj interface{}) {
				destroyCount++
			},
		})

		// First acquire gets the initial object, but it's invalid
		obj, err := pool.Acquire()
		if err != nil {
			t.Fatalf("Acquire failed: %v", err)
		}
		// Since initial object is invalid, a new one should be created
		if obj != "obj-2" {
			t.Errorf("expected new object 'obj-2', got %v", obj)
		}
		if destroyCount != 1 {
			t.Errorf("expected 1 object destroyed, got %d", destroyCount)
		}
	})
}

func TestPoolMaxSize(t *testing.T) {
	t.Run("does not exceed max size", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{
			MaxSize:       3,
			ObjectFactory: func() (interface{}, error) { return "obj", nil },
		})

		// Acquire all 3 objects
		objs := make([]interface{}, 3)
		for i := 0; i < 3; i++ {
			obj, err := pool.Acquire()
			if err != nil {
				t.Fatalf("Acquire %d failed: %v", i, err)
			}
			objs[i] = obj
		}

		// Release them back
		for _, obj := range objs {
			pool.Release(obj)
		}

		stats := pool.GetStats()
		if stats.TotalObjects != 3 {
			t.Errorf("expected 3 total objects, got %d", stats.TotalObjects)
		}
	})
}

func TestWaitTimeout(t *testing.T) {
	t.Run("returns timeout when pool is full and all objects acquired", func(t *testing.T) {
		pool := NewSimpleObjectPool(Config{
			MaxSize:     1,
			MaxWaitTime: 50 * time.Millisecond,
			ObjectFactory: func() (interface{}, error) {
				return "obj", nil
			},
		})

		// Acquire the only object
		obj, err := pool.Acquire()
		if err != nil {
			t.Fatalf("first Acquire failed: %v", err)
		}
		defer pool.Release(obj)

		// Now try to acquire again - this should timeout
		_, err = pool.Acquire()
		if err != ErrWaitTimeout {
			t.Errorf("expected ErrWaitTimeout, got %v", err)
		}
	})
}

func TestGetStats(t *testing.T) {
	pool := NewSimpleObjectPool(Config{
		MaxSize:       10,
		InitialSize:   2,
		ObjectFactory: func() (interface{}, error) { return "obj", nil },
	})

	stats := pool.GetStats()
	if stats.TotalObjects != 2 {
		t.Errorf("expected 2 total objects, got %d", stats.TotalObjects)
	}
	if stats.IdleObjects != 2 {
		t.Errorf("expected 2 idle objects, got %d", stats.IdleObjects)
	}
	if stats.ActiveObjects != 0 {
		t.Errorf("expected 0 active objects, got %d", stats.ActiveObjects)
	}
}

func TestErrors(t *testing.T) {
	if ErrPoolClosed.Error() != "object pool is closed" {
		t.Errorf("unexpected ErrPoolClosed message: %v", ErrPoolClosed)
	}
	if ErrWaitTimeout.Error() != "wait timeout when acquiring object" {
		t.Errorf("unexpected ErrWaitTimeout message: %v", ErrWaitTimeout)
	}
	if ErrInvalidObject.Error() != "invalid object" {
		t.Errorf("unexpected ErrInvalidObject message: %v", ErrInvalidObject)
	}
}
