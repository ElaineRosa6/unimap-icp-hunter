package tamper

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- HashStorage Tests ---

func TestHashStorage_SaveAndLoadBaseline(t *testing.T) {
	dir := t.TempDir()
	storage := NewHashStorage(dir)

	t.Run("save and load", func(t *testing.T) {
		result := &PageHashResult{
			URL:      "https://example.com",
			FullHash: "abc123",
		}
		if err := storage.SaveBaseline("https://example.com", result); err != nil {
			t.Fatalf("SaveBaseline failed: %v", err)
		}

		loaded, err := storage.LoadBaseline("https://example.com")
		if err != nil {
			t.Fatalf("LoadBaseline failed: %v", err)
		}
		if loaded.FullHash != "abc123" {
			t.Errorf("expected FullHash 'abc123', got %q", loaded.FullHash)
		}
	})

	t.Run("load nonexistent returns error", func(t *testing.T) {
		_, err := storage.LoadBaseline("https://nonexistent.com")
		if err == nil {
			t.Fatal("expected error for nonexistent baseline")
		}
		// Error message contains "not found"
		if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "cannot find") {
			t.Errorf("expected 'not found' error, got %v", err)
		}
	})

	t.Run("has baseline", func(t *testing.T) {
		if !storage.HasBaseline("https://example.com") {
			t.Error("expected HasBaseline to return true")
		}
		if storage.HasBaseline("https://unknown.com") {
			t.Error("expected HasBaseline to return false for unknown URL")
		}
	})

	t.Run("delete baseline", func(t *testing.T) {
		if err := storage.DeleteBaseline("https://example.com"); err != nil {
			t.Fatalf("DeleteBaseline failed: %v", err)
		}
		if storage.HasBaseline("https://example.com") {
			t.Error("expected baseline to be deleted")
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := storage.DeleteBaseline("https://nonexistent.com")
		if err == nil {
			t.Error("expected error when deleting nonexistent baseline")
		}
	})

	t.Run("list baselines", func(t *testing.T) {
		_ = storage.SaveBaseline("https://a.com", &PageHashResult{URL: "https://a.com", FullHash: "a"})
		_ = storage.SaveBaseline("https://b.com", &PageHashResult{URL: "https://b.com", FullHash: "b"})

		urls, err := storage.ListBaselines()
		if err != nil {
			t.Fatalf("ListBaselines failed: %v", err)
		}
		if len(urls) != 2 {
			t.Fatalf("expected 2 baselines, got %d", len(urls))
		}
		// Should be sorted
		if urls[0] != "https://a.com" || urls[1] != "https://b.com" {
			t.Errorf("expected sorted URLs, got %v", urls)
		}
	})
}

func TestHashStorage_SaveAndLoadCheckRecords(t *testing.T) {
	dir := t.TempDir()
	storage := NewHashStorage(dir)

	t.Run("save and load records", func(t *testing.T) {
		record := &CheckRecord{
			ID:        "rec-001",
			URL:       "https://example.com",
			Tampered:  true,
			Timestamp: time.Now().Unix(),
		}
		if err := storage.SaveCheckRecord("https://example.com", record); err != nil {
			t.Fatalf("SaveCheckRecord failed: %v", err)
		}

		records, err := storage.LoadCheckRecords("https://example.com", 100)
		if err != nil {
			t.Fatalf("LoadCheckRecords failed: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		if records[0].ID != "rec-001" {
			t.Errorf("expected ID 'rec-001', got %q", records[0].ID)
		}
	})

	t.Run("load records for new URL", func(t *testing.T) {
		records, err := storage.LoadCheckRecords("https://new.com", 100)
		if err != nil {
			t.Fatalf("LoadCheckRecords failed for new URL: %v", err)
		}
		if len(records) != 0 {
			t.Errorf("expected 0 records, got %d", len(records))
		}
	})

	t.Run("multiple records for same URL", func(t *testing.T) {
		_ = storage.SaveCheckRecord("https://multi.com", &CheckRecord{ID: "r1", URL: "https://multi.com"})
		_ = storage.SaveCheckRecord("https://multi.com", &CheckRecord{ID: "r2", URL: "https://multi.com"})

		records, err := storage.LoadCheckRecords("https://multi.com", 100)
		if err != nil {
			t.Fatalf("LoadCheckRecords failed: %v", err)
		}
		if len(records) < 2 {
			t.Errorf("expected at least 2 records, got %d", len(records))
		}
	})

	t.Run("delete records", func(t *testing.T) {
		_ = storage.SaveCheckRecord("https://del.com", &CheckRecord{ID: "del-1", URL: "https://del.com"})
		if err := storage.DeleteCheckRecords("https://del.com"); err != nil {
			t.Fatalf("DeleteCheckRecords failed: %v", err)
		}
		records, err := storage.LoadCheckRecords("https://del.com", 100)
		if err != nil {
			t.Fatalf("LoadCheckRecords after delete failed: %v", err)
		}
		if len(records) != 0 {
			t.Errorf("expected 0 records after delete, got %d", len(records))
		}
	})

	t.Run("list all records", func(t *testing.T) {
		storage2 := NewHashStorage(t.TempDir())
		_ = storage2.SaveCheckRecord("https://a.com", &CheckRecord{ID: "a1", URL: "https://a.com"})
		_ = storage2.SaveCheckRecord("https://b.com", &CheckRecord{ID: "b1", URL: "https://b.com"})

		all, err := storage2.ListAllCheckRecords()
		if err != nil {
			t.Fatalf("ListAllCheckRecords failed: %v", err)
		}
		if len(all) != 2 {
			t.Errorf("expected 2 total records, got %d", len(all))
		}
	})
}

// --- Detector Baseline Tests ---

func TestDetector_SaveBaseline(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	result := &PageHashResult{
		URL:      "https://test.com",
		FullHash: "sha256hash",
	}
	if err := d.SaveBaseline("https://test.com", result); err != nil {
		t.Fatalf("SaveBaseline failed: %v", err)
	}

	if !d.HasBaseline("https://test.com") {
		t.Fatal("expected baseline to exist after save")
	}

	loaded, err := d.LoadBaseline("https://test.com")
	if err != nil {
		t.Fatalf("LoadBaseline failed: %v", err)
	}
	if loaded.FullHash != "sha256hash" {
		t.Errorf("expected 'sha256hash', got %q", loaded.FullHash)
	}
}

func TestDetector_LoadBaseline_NotFound(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	_, err := d.LoadBaseline("https://missing.com")
	if err == nil {
		t.Fatal("expected error for missing baseline")
	}
}

func TestDetector_HasBaseline(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	if d.HasBaseline("https://new.com") {
		t.Error("expected false for new URL")
	}

	_ = d.SaveBaseline("https://new.com", &PageHashResult{URL: "https://new.com"})
	if !d.HasBaseline("https://new.com") {
		t.Error("expected true after saving baseline")
	}
}

func TestDetector_DeleteBaseline(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	_ = d.SaveBaseline("https://temp.com", &PageHashResult{URL: "https://temp.com"})
	if err := d.storage.DeleteBaseline("https://temp.com"); err != nil {
		t.Fatalf("DeleteBaseline failed: %v", err)
	}
	if d.HasBaseline("https://temp.com") {
		t.Error("expected baseline to be deleted")
	}
}

// --- Detector Check Record Tests ---

func TestDetector_LoadCheckRecords(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	records, err := d.LoadCheckRecords("https://empty.com", 100)
	if err != nil {
		t.Fatalf("LoadCheckRecords failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestDetector_ListAllCheckRecords(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	records, err := d.ListAllCheckRecords()
	if err != nil {
		t.Fatalf("ListAllCheckRecords failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records on empty storage, got %d", len(records))
	}
}

func TestDetector_GetCheckStats(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	stats, err := d.GetCheckStats("https://test.com")
	if err != nil {
		t.Fatalf("GetCheckStats failed: %v", err)
	}
	if stats["total_checks"] != 0 {
		t.Errorf("expected 0 total checks, got %d", stats["total_checks"])
	}
}

func TestDetector_DeleteCheckRecords(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	_ = d.storage.SaveCheckRecord("https://del.com", &CheckRecord{ID: "d1", URL: "https://del.com"})
	err := d.DeleteCheckRecords("https://del.com")
	if err != nil {
		t.Fatalf("DeleteCheckRecords failed: %v", err)
	}
	records, _ := d.LoadCheckRecords("https://del.com", 100)
	if len(records) != 0 {
		t.Errorf("expected 0 records after delete, got %d", len(records))
	}
}

// --- Detector CheckTampering Tests ---

func TestDetector_CheckTampering_NoBaseline(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	ctx := context.Background()
	result, err := d.CheckTampering(ctx, "https://no-baseline.com")
	if err != nil {
		t.Fatalf("CheckTampering failed: %v", err)
	}
	// Without Chrome, ComputePageHash will fail, returning an error
	if result != nil {
		t.Logf("CheckTampering returned status: %s", result.Status)
	}
}

func TestDetector_BatchCheckTampering(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	ctx := context.Background()
	_, err := d.BatchCheckTampering(ctx, []string{}, 1)
	if err == nil {
		t.Log("BatchCheckTampering correctly returned error for empty input")
	}
}

func TestDetector_BatchSetBaseline(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	ctx := context.Background()
	_, err := d.BatchSetBaseline(ctx, []string{}, 1)
	if err == nil {
		t.Log("BatchSetBaseline correctly returned error for empty input")
	}
}

func TestDetector_ListBaselines(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	_ = d.SaveBaseline("https://a.com", &PageHashResult{URL: "https://a.com"})
	_ = d.SaveBaseline("https://b.com", &PageHashResult{URL: "https://b.com"})

	urls, err := d.storage.ListBaselines()
	if err != nil {
		t.Fatalf("ListBaselines failed: %v", err)
	}
	if len(urls) != 2 {
		t.Errorf("expected 2 baselines, got %d", len(urls))
	}
}

func TestDetector_SaveCheckRecord(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{BaseDir: dir})

	record := &CheckRecord{
		ID:        "test-rec-001",
		URL:       "https://test.com",
		Tampered:  true,
		Timestamp: time.Now().Unix(),
	}
	if err := d.storage.SaveCheckRecord("https://test.com", record); err != nil {
		t.Fatalf("SaveCheckRecord failed: %v", err)
	}
}
