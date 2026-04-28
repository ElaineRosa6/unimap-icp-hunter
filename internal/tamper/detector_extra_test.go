package tamper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// ===== computeStyleHash =====

func TestDetector_ComputeStyleHash(t *testing.T) {
	d := NewDetector(DetectorConfig{BaseDir: "/tmp", DetectionMode: "normal"})
	html := `<html><head>
<style>body { color: red; }</style>
<style>.foo { margin: 0; }</style>
<link rel="stylesheet" href="/css/main.css">
</head><body></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	result := d.computeStyleHash(doc)
	if result.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if result.Elements != 3 {
		t.Errorf("expected 3 elements, got %d", result.Elements)
	}
	if result.Name != SegmentStyles {
		t.Errorf("expected name %s, got %s", SegmentStyles, result.Name)
	}
}

func TestDetector_ComputeStyleHash_Empty(t *testing.T) {
	d := NewDetector(DetectorConfig{BaseDir: "/tmp", DetectionMode: "normal"})
	html := `<html><body></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	result := d.computeStyleHash(doc)
	if result.Elements != 0 {
		t.Errorf("expected 0 elements, got %d", result.Elements)
	}
}

// ===== computeMetaHash =====

func TestDetector_ComputeMetaHash(t *testing.T) {
	d := NewDetector(DetectorConfig{BaseDir: "/tmp", DetectionMode: "normal"})
	html := `<html><head>
<meta name="viewport" content="width=device-width">
<meta charset="utf-8">
<meta name="description" content="test page">
</head><body></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	result := d.computeMetaHash(doc)
	if result.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if result.Elements != 3 {
		t.Errorf("expected 3 elements, got %d", result.Elements)
	}
}

// ===== computeFaviconHash =====

func TestDetector_ComputeFaviconHash(t *testing.T) {
	d := NewDetector(DetectorConfig{BaseDir: "/tmp", DetectionMode: "normal"})
	html := `<html><head>
<link rel="icon" href="/favicon.ico">
<link rel="apple-touch-icon" href="/icon.png">
</head><body></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	result := d.computeFaviconHash(doc)
	if result.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if result.Elements != 2 {
		t.Errorf("expected 2 elements, got %d", result.Elements)
	}
}

// ===== computeImageHash =====

func TestDetector_ComputeImageHash(t *testing.T) {
	d := NewDetector(DetectorConfig{BaseDir: "/tmp", DetectionMode: "normal"})
	html := `<html><body>
<img src="/img/logo.png" alt="Logo">
<img src="/img/banner.jpg" alt="Banner">
</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	result := d.computeImageHash(doc)
	if result.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if result.Elements != 2 {
		t.Errorf("expected 2 elements, got %d", result.Elements)
	}
}

// ===== computeButtonHash =====

func TestDetector_ComputeButtonHash(t *testing.T) {
	d := NewDetector(DetectorConfig{BaseDir: "/tmp", DetectionMode: "normal"})
	html := `<html><body>
<button id="submit" class="btn primary">Submit</button>
<button id="cancel">Cancel</button>
</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	result := d.computeButtonHash(doc)
	if result.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if result.Elements != 2 {
		t.Errorf("expected 2 elements, got %d", result.Elements)
	}
}

// ===== SetAllocator =====

func TestDetector_SetAllocator(t *testing.T) {
	d := NewDetector(DetectorConfig{BaseDir: "/tmp", DetectionMode: "normal"})

	ctx := context.Background()
	allocCtx, allocCancel := context.WithCancel(ctx)

	d.SetAllocator(ctx, allocCtx, allocCancel)
	// Verify no panic and the method completes
}

// ===== ListBaselines =====

func TestHashStorage_ListBaselines(t *testing.T) {
	dir := t.TempDir()
	storage := NewHashStorage(dir)

	// Empty directory
	urls, err := storage.ListBaselines()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 0 {
		t.Errorf("expected 0 baselines, got %d", len(urls))
	}

	// Create some baseline files with valid URLs
	os.WriteFile(filepath.Join(dir, "http_example_com.json"), []byte(`{"url":"http://example.com"}`), 0644)
	os.WriteFile(filepath.Join(dir, "https_test_org.json"), []byte(`{"url":"https://test.org"}`), 0644)

	urls, err = storage.ListBaselines()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Errorf("expected 2 baselines, got %d: %v", len(urls), urls)
	}
}

func TestHashStorage_ListBaselines_NonExistentDir(t *testing.T) {
	storage := NewHashStorage("/nonexistent/dir/path")
	_, err := storage.ListBaselines()
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}
