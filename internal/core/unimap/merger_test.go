package unimap

import (
	"testing"

	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestResultMerger_Merge(t *testing.T) {
	merger := NewResultMerger()

	tests := []struct {
		name    string
		results []*model.EngineResult
		want    int // Expected number of unique assets
	}{
		{
			name: "Empty results",
			results: []*model.EngineResult{
				{EngineName: "fofa", Total: 0, RawData: []interface{}{}},
			},
			want: 0,
		},
		{
			name: "Nil result",
			results: []*model.EngineResult{
				nil,
			},
			want: 0,
		},
		{
			name: "Result with error",
			results: []*model.EngineResult{
				{EngineName: "fofa", Error: "test error"},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := merger.Merge(tt.results)
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
