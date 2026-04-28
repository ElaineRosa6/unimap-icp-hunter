package unimap

import (
	"testing"

	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestNewResultMerger(t *testing.T) {
	merger := NewResultMerger()
	if merger == nil {
		t.Fatal("expected non-nil merger")
	}
}

func TestGenerateKey(t *testing.T) {
	merger := NewResultMerger()

	testCases := []struct {
		name  string
		asset model.UnifiedAsset
		want  string
		check func(string) bool
	}{
		{
			name:  "IP and port",
			asset: model.UnifiedAsset{IP: "1.2.3.4", Port: 80},
			check: func(s string) bool { return s == "1.2.3.4:80" },
		},
		{
			name:  "URL only",
			asset: model.UnifiedAsset{URL: "https://example.com"},
			check: func(s string) bool { return s == "url:https://example.com" },
		},
		{
			name:  "Host only",
			asset: model.UnifiedAsset{Host: "example.com"},
			check: func(s string) bool { return s == "host:example.com" },
		},
		{
			name:  "fallback",
			asset: model.UnifiedAsset{},
			check: func(s string) bool { return len(s) > 0 },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := merger.generateKey(tc.asset)
			if !tc.check(got) {
				t.Errorf("generateKey() = %q, check failed", got)
			}
		})
	}
}

func TestMerge(t *testing.T) {
	merger := NewResultMerger()

	assets := []model.UnifiedAsset{
		{IP: "1.2.3.4", Port: 80, Source: "fofa"},
		{IP: "1.2.3.4", Port: 80, Source: "hunter"},
		{IP: "5.6.7.8", Port: 443, Source: "fofa"},
	}

	result := merger.Merge(assets)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Total != 2 {
		t.Errorf("expected Total=2, got %d", result.Total)
	}
	if result.Duplicates != 1 {
		t.Errorf("expected Duplicates=1, got %d", result.Duplicates)
	}
	if len(result.Assets) != 2 {
		t.Errorf("expected 2 unique assets, got %d", len(result.Assets))
	}
}

func TestMergeEmpty(t *testing.T) {
	merger := NewResultMerger()
	result := merger.Merge([]model.UnifiedAsset{})
	if result.Total != 0 {
		t.Errorf("expected Total=0, got %d", result.Total)
	}
}

func TestContains(t *testing.T) {
	testCases := []struct {
		name  string
		list  string
		item  string
		want  bool
	}{
		{"contains single", "fofa", "fofa", true},
		{"contains in list", "fofa,hunter", "hunter", true},
		{"not contains", "fofa", "hunter", false},
		{"with spaces", "fofa, hunter", "hunter", true},
		{"empty list", "", "fofa", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := contains(tc.list, tc.item)
			if got != tc.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tc.list, tc.item, got, tc.want)
			}
		})
	}
}

func TestGetMinPriority(t *testing.T) {
	merger := NewResultMerger()

	testCases := []struct {
		name    string
		sources string
		want    int
	}{
		{"single fofa", "fofa", 1},
		{"single hunter", "hunter", 2},
		{"multiple sources", "fofa,hunter", 1},
		{"unknown source", "unknown", 999},
		{"mixed known unknown", "fofa,unknown", 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := merger.getMinPriority(tc.sources)
			if got != tc.want {
				t.Errorf("getMinPriority(%q) = %d, want %d", tc.sources, got, tc.want)
			}
		})
	}
}

func TestSortAssets(t *testing.T) {
	merger := NewResultMerger()

	assets := []*model.UnifiedAsset{
		{IP: "2.2.2.2", Port: 80},
		{IP: "1.1.1.1", Port: 443},
		{IP: "1.1.1.1", Port: 80},
	}

	merger.SortAssets(assets)

	if assets[0].IP != "1.1.1.1" || assets[0].Port != 80 {
		t.Errorf("expected first asset to be 1.1.1.1:80")
	}
	if assets[1].IP != "1.1.1.1" || assets[1].Port != 443 {
		t.Errorf("expected second asset to be 1.1.1.1:443")
	}
	if assets[2].IP != "2.2.2.2" || assets[2].Port != 80 {
		t.Errorf("expected third asset to be 2.2.2.2:80")
	}
}

func TestGetSortedAssets(t *testing.T) {
	merger := NewResultMerger()

	assets := []model.UnifiedAsset{
		{IP: "2.2.2.2", Port: 80, Source: "fofa"},
		{IP: "1.1.1.1", Port: 80, Source: "fofa"},
	}

	mergeResult := merger.Merge(assets)
	sorted := merger.GetSortedAssets(mergeResult)

	if len(sorted) != 2 {
		t.Errorf("expected 2 sorted assets, got %d", len(sorted))
	}
}

func TestGetSourceStats(t *testing.T) {
	merger := NewResultMerger()

	assets := []model.UnifiedAsset{
		{Source: "fofa"},
		{Source: "fofa"},
		{Source: "hunter"},
	}

	stats := merger.GetSourceStats(assets)
	if stats["fofa"] != 2 {
		t.Errorf("expected fofa count=2, got %d", stats["fofa"])
	}
	if stats["hunter"] != 1 {
		t.Errorf("expected hunter count=1, got %d", stats["hunter"])
	}
}

func TestMergeEngineResults(t *testing.T) {
	merger := NewResultMerger()

	results := []*model.EngineResult{
		{
			EngineName:     "fofa",
			NormalizedData: []model.UnifiedAsset{{IP: "1.2.3.4", Port: 80, Source: "fofa"}},
		},
		nil,
		{
			EngineName: "hunter",
			Error:      "some error",
		},
	}

	result := merger.MergeEngineResults(results, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMergeEngineResultsCached(t *testing.T) {
	merger := NewResultMerger()

	results := []*model.EngineResult{
		{
			EngineName:     "fofa",
			Cached:         true,
			NormalizedData: []model.UnifiedAsset{{IP: "1.2.3.4", Port: 80, Source: "fofa"}},
		},
	}

	result := merger.MergeEngineResults(results, nil)
	if result.Total != 1 {
		t.Errorf("expected Total=1, got %d", result.Total)
	}
}

func TestMergeWithDifferentPriorities(t *testing.T) {
	merger := NewResultMerger()

	assets := []model.UnifiedAsset{
		{IP: "1.2.3.4", Port: 80, Source: "fofa", Title: "fofa title"},
		{IP: "1.2.3.4", Port: 80, Source: "hunter", Title: "hunter title"},
	}

	result := merger.Merge(assets)
	for _, asset := range result.Assets {
		if asset.Title != "fofa title" {
			t.Errorf("expected title from higher priority engine (fofa), got %q", asset.Title)
		}
	}
}

func TestUpdateAssetFields(t *testing.T) {
	merger := NewResultMerger()

	existing := &model.UnifiedAsset{}
	newAsset := model.UnifiedAsset{
		Title:       "test",
		Server:      "nginx",
		BodySnippet: "snippet",
		StatusCode:  200,
		Host:        "example.com",
		URL:         "https://example.com",
		Protocol:    "https",
		CountryCode: "CN",
		Region:      "Beijing",
		City:        "Beijing",
		ASN:         "AS12345",
		Org:         "Test Org",
		ISP:         "Test ISP",
	}

	merger.updateAssetFields(existing, newAsset, true)

	if existing.Title != "test" {
		t.Error("expected title to be updated")
	}
	if existing.Server != "nginx" {
		t.Error("expected server to be updated")
	}
}

func TestUpdateAssetFieldsOnlyMissing(t *testing.T) {
	merger := NewResultMerger()

	existing := &model.UnifiedAsset{Title: "existing"}
	newAsset := model.UnifiedAsset{Title: "new", Server: "nginx"}

	merger.updateAssetFields(existing, newAsset, false)

	if existing.Title != "existing" {
		t.Error("expected title to remain unchanged")
	}
	if existing.Server != "nginx" {
		t.Error("expected server to be filled")
	}
}

func TestCopyAsset(t *testing.T) {
	merger := NewResultMerger()

	dest := &model.UnifiedAsset{
		Headers: make(map[string]string),
		Extra:   make(map[string]interface{}),
	}
	src := model.UnifiedAsset{
		IP:          "1.2.3.4",
		Port:        80,
		Protocol:    "https",
		Host:        "example.com",
		URL:         "https://example.com",
		Title:       "test",
		BodySnippet: "snippet",
		Server:      "nginx",
		StatusCode:  200,
		CountryCode: "CN",
		Region:      "Beijing",
		City:        "Beijing",
		ASN:         "AS12345",
		Org:         "Test Org",
		ISP:         "Test ISP",
		Source:      "fofa",
	}

	merger.copyAsset(dest, src)

	if dest.IP != src.IP {
		t.Error("IP not copied")
	}
}

func TestMergeAssets(t *testing.T) {
	merger := NewResultMerger()

	existing := &model.UnifiedAsset{IP: "1.2.3.4", Port: 80, Source: "fofa"}
	newAsset := model.UnifiedAsset{IP: "1.2.3.4", Port: 80, Source: "hunter"}

	merger.mergeAssets(existing, newAsset)

	if existing.Source != "fofa,hunter" {
		t.Errorf("expected merged sources, got %q", existing.Source)
	}
}
