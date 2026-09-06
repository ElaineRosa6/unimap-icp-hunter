package utils

import (
	"time"

	"github.com/unimap/project/internal/model"
)

// EnginePageMetadata is published atomically with normalized assets. It is not
// inferred from their count: upstream totals and terminal pages are independent.
type EnginePageMetadata struct {
	EngineName string `json:"engine_name"`
	Page       int    `json:"page"`
	Total      int    `json:"total"`
	HasMore    bool   `json:"has_more"`
	Error      string `json:"error,omitempty"`
}

func enginePageCacheKey(key string) string { return "engine-page:v1:" + key }

// GetEnginePageSnapshot never consumes legacy asset-only cache entries.
func GetEnginePageSnapshot(cache QueryCache, key string) (*model.EngineResult, bool) {
	snapshots, ok := cache.(QuerySnapshotCache)
	if !ok {
		return nil, false
	}
	assets, metadata, hit := snapshots.GetQuerySnapshot(enginePageCacheKey(key))
	if !hit || metadata.EnginePage == nil {
		return nil, false
	}
	page := metadata.EnginePage
	return &model.EngineResult{EngineName: page.EngineName, Page: page.Page, Total: page.Total, HasMore: page.HasMore, Error: page.Error, Cached: true, RawData: []interface{}{}, NormalizedData: assets}, true
}

func SetEnginePageSnapshot(cache QueryCache, key string, result *model.EngineResult, assets []model.UnifiedAsset, ttl time.Duration) {
	snapshots, ok := cache.(QuerySnapshotCache)
	if !ok || result == nil {
		return
	}
	metadata := QueryCacheMetadata{EnginePage: &EnginePageMetadata{EngineName: result.EngineName, Page: result.Page, Total: result.Total, HasMore: result.HasMore, Error: result.Error}}
	snapshots.SetQuerySnapshot(enginePageCacheKey(key), assets, metadata, ttl)
}
