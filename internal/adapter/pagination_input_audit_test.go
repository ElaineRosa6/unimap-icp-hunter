package adapter

import (
	"context"
	"math"
	"testing"

	"github.com/unimap/project/internal/model"
)

func TestAuditPaginationInputBoundary(t *testing.T) {
	for _, tc := range []struct {
		name         string
		pages, count int
		wantError    bool
	}{
		{"valid_control", 1, 1, false}, {"zero", 0, 1, true}, {"negative", -1, 1, true}, {"multiply_overflow", math.MaxInt, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("pagination input panicked instead of returning error: %v", p)
				}
			}()
			o := NewEngineOrchestrator()
			defer o.cache.Close()
			a := &pageAuditAdapter{mockAdapter: mockAdapter{name: "fixture"}}
			o.RegisterAdapter(a)
			queries := make([]model.EngineQuery, tc.count)
			for i := range queries {
				queries[i] = model.EngineQuery{EngineName: "fixture", Query: "fixture", Page: 1}
			}
			results, err := o.SearchEnginesWithPaginationAndContext(context.Background(), queries, 10, tc.pages)
			if tc.wantError && err == nil {
				t.Errorf("invalid pages=%d queries=%d returned success with %d results", tc.pages, tc.count, len(results))
			}
			if !tc.wantError && (err != nil || len(results) != 1) {
				t.Fatalf("valid input failed: results=%d err=%v", len(results), err)
			}
		})
	}
}
