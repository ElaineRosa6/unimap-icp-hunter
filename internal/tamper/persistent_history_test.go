//go:build cgo

package tamper

import (
	"fmt"
	"sync"
	"testing"
)

func TestPersistentHistoryLifetime(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPersistentHashStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, info, err := store.HistoryDatabase()
	if err != nil || info == nil {
		t.Fatalf("owner: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err = store.indexCheckRecord(&CheckRecord{ID: fmt.Sprint(i), URL: "https://example.com", Timestamp: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	second, _, err := store.HistoryDatabase()
	if err != nil || first != second {
		t.Fatalf("pool not retained: %v", err)
	}
	records, err := store.listIndexedCheckRecords()
	if err != nil || len(records["https://example.com"]) != 3 {
		t.Fatalf("records=%v err=%v", records, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.HistoryDatabase(); err == nil {
		t.Fatal("closed owner reopened")
	}
	if err = store.indexCheckRecord(&CheckRecord{ID: "closed"}); err == nil {
		t.Fatal("write after close succeeded")
	}
	reopened, err := NewPersistentHashStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	records, err = reopened.listIndexedCheckRecords()
	if err != nil || len(records["https://example.com"]) != 3 {
		t.Fatalf("reopen lost records: %v", err)
	}
}

func TestPersistentHistoryConcurrentClose(t *testing.T) {
	store, err := NewPersistentHashStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			_ = store.indexCheckRecord(&CheckRecord{ID: fmt.Sprint(id), URL: "https://example.com"})
			_, _, _ = store.HistoryDatabase()
		}(i)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if _, _, err = store.HistoryDatabase(); err == nil {
		t.Fatal("closed owner became available")
	}
}
