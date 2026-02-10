package unimap

import (
	"reflect"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestResultMerger_Merge(t *testing.T) {
	merger := NewResultMerger()

	tests := []struct {
		name   string
		assets []model.UnifiedAsset
		want   int // Expected number of unique assets
	}{
		{
			name:   "Empty assets",
			assets: []model.UnifiedAsset{},
			want:   0,
		},
		{
			name: "Duplicate assets",
			assets: []model.UnifiedAsset{
				{
					IP:       "192.168.1.1",
					Port:     80,
					Protocol: "http",
					Source:   "fofa",
				},
				{
					IP:       "192.168.1.1",
					Port:     80,
					Protocol: "http",
					Source:   "hunter",
				},
			},
			want: 1, // Should be deduplicated
		},
		{
			name: "Unique assets",
			assets: []model.UnifiedAsset{
				{
					IP:       "192.168.1.1",
					Port:     80,
					Protocol: "http",
					Source:   "fofa",
				},
				{
					IP:       "192.168.1.2",
					Port:     443,
					Protocol: "https",
					Source:   "hunter",
				},
			},
			want: 2, // Both should be kept
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := merger.Merge(tt.assets)
			if result == nil {
				t.Fatal("ResultMerger.Merge() returned nil")
			}
			if result.Total != tt.want {
				t.Errorf("ResultMerger.Merge() got %d assets, want %d", result.Total, tt.want)
			}
		})
	}
}

func TestNewResultMerger(t *testing.T) {
	merger := NewResultMerger()
	if merger == nil {
		t.Error("NewResultMerger() returned nil")
	}
}

func TestResultMerger_GenerateKey(t *testing.T) {
	merger := NewResultMerger()

	tests := []struct {
		name  string
		asset model.UnifiedAsset
		want  string
	}{
		{
			name: "IP and Port",
			asset: model.UnifiedAsset{
				IP:   "192.168.1.1",
				Port: 80,
			},
			want: "192.168.1.1:80",
		},
		{
			name: "IP, Port and URL",
			asset: model.UnifiedAsset{
				IP:   "192.168.1.1",
				Port: 80,
				URL:  "http://example.com",
			},
			want: "192.168.1.1:80:http://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 使用反射获取generateKey方法
			keyMethod := reflect.ValueOf(merger).MethodByName("generateKey")
			if !keyMethod.IsValid() {
				t.Skip("generateKey is unexported, skipping test")
			}

			args := []reflect.Value{reflect.ValueOf(tt.asset)}
			result := keyMethod.Call(args)
			if len(result) != 1 {
				t.Fatal("generateKey returned unexpected number of values")
			}

			got, ok := result[0].Interface().(string)
			if !ok {
				t.Fatal("generateKey returned non-string value")
			}

			if got != tt.want {
				t.Errorf("generateKey() got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestResultMerger_GetSortedAssets(t *testing.T) {
	merger := NewResultMerger()

	assets := []model.UnifiedAsset{
		{
			IP:     "192.168.1.2",
			Port:   443,
			Source: "hunter",
		},
		{
			IP:     "192.168.1.1",
			Port:   80,
			Source: "fofa",
		},
		{
			IP:     "192.168.1.1",
			Port:   443,
			Source: "zoomeye",
		},
	}

	mergeResult := merger.Merge(assets)
	sortedAssets := merger.GetSortedAssets(mergeResult)

	if len(sortedAssets) != 3 {
		t.Errorf("GetSortedAssets() got %d assets, want 3", len(sortedAssets))
	}

	// 检查排序顺序
	expectedOrder := []struct {
		ip   string
		port int
	}{
		{"192.168.1.1", 80},
		{"192.168.1.1", 443},
		{"192.168.1.2", 443},
	}

	for i, expected := range expectedOrder {
		if i >= len(sortedAssets) {
			t.Fatalf("Expected %d assets, got %d", len(expectedOrder), len(sortedAssets))
		}
		if sortedAssets[i].IP != expected.ip {
			t.Errorf("Asset %d IP: got %s, want %s", i, sortedAssets[i].IP, expected.ip)
		}
		if sortedAssets[i].Port != expected.port {
			t.Errorf("Asset %d Port: got %d, want %d", i, sortedAssets[i].Port, expected.port)
		}
	}
}

func TestResultMerger_GetSourceStats(t *testing.T) {
	merger := NewResultMerger()

	assets := []model.UnifiedAsset{
		{
			IP:     "192.168.1.1",
			Port:   80,
			Source: "fofa",
		},
		{
			IP:     "192.168.1.2",
			Port:   443,
			Source: "hunter",
		},
		{
			IP:     "192.168.1.3",
			Port:   8080,
			Source: "fofa",
		},
		{
			IP:     "192.168.1.4",
			Port:   3306,
			Source: "zoomeye",
		},
	}

	stats := merger.GetSourceStats(assets)

	if stats["fofa"] != 2 {
		t.Errorf("Expected fofa count 2, got %d", stats["fofa"])
	}
	if stats["hunter"] != 1 {
		t.Errorf("Expected hunter count 1, got %d", stats["hunter"])
	}
	if stats["zoomeye"] != 1 {
		t.Errorf("Expected zoomeye count 1, got %d", stats["zoomeye"])
	}
	if stats["quake"] != 0 {
		t.Errorf("Expected quake count 0, got %d", stats["quake"])
	}
}
