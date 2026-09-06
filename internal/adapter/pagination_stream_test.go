package adapter

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
)

type streamPageAdapter struct{ mockAdapter }

func (a *streamPageAdapter) Search(_ context.Context, query string, page, _ int) (*model.EngineResult, error) {
	return &model.EngineResult{EngineName: a.name, Page: page, Total: 1, NormalizedData: []model.UnifiedAsset{{Title: query}}}, nil
}
func (a *streamPageAdapter) Normalize(r *model.EngineResult) ([]model.UnifiedAsset, error) {
	return r.NormalizedData, nil
}

func TestPaginationBoundedQueueNoLoss(t *testing.T) {
	o := NewEngineOrchestrator()
	defer o.cache.Close()
	o.SetConcurrency(1)
	o.RegisterAdapter(&streamPageAdapter{mockAdapter: mockAdapter{name: "fixture"}})
	queries := make([]model.EngineQuery, 40)
	for i := range queries {
		queries[i] = model.EngineQuery{EngineName: "fixture", Query: string(rune('A' + i))}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results, err := o.SearchEnginesWithPaginationAndContext(ctx, queries, 10, 2)
	if err != nil || len(results) != len(queries) {
		t.Fatalf("results=%d want=%d err=%v", len(results), len(queries), err)
	}
	seen := map[string]bool{}
	for _, r := range results {
		if len(r.NormalizedData) != 1 {
			t.Fatal("missing asset")
		}
		seen[r.NormalizedData[0].Title] = true
	}
	if len(seen) != len(queries) {
		t.Fatal("duplicate or missing query result")
	}
}

func TestPaginationLargeBoundAndInvalidPageSize(t *testing.T) {
	o := NewEngineOrchestrator()
	defer o.cache.Close()
	o.RegisterAdapter(&streamPageAdapter{mockAdapter: mockAdapter{name: "fixture"}})
	q := []model.EngineQuery{{EngineName: "fixture", Query: "fixture"}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	results, err := o.SearchEnginesWithPaginationAndContext(ctx, q, 10, math.MaxInt)
	if err != nil || len(results) != 1 {
		t.Fatalf("terminal query with large limit: %d %v", len(results), err)
	}
	for _, size := range []int{0, -1} {
		if _, err := o.SearchEnginesWithPaginationAndContext(ctx, q, size, 1); err == nil {
			t.Errorf("invalid page size %d accepted", size)
		}
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := o.SearchEnginesWithPaginationAndContext(cancelled, q, 10, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
}

func TestPaginatedSendBackpressureAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan *model.EngineResult)
	task := &PaginatedSearchTask{ctx: ctx, resultChan: ch}
	first := &model.EngineResult{Page: 1}
	done := make(chan struct{})
	go func() { task.sendPaginatedResult(first); close(done) }()
	select {
	case got := <-ch:
		if got != first {
			t.Fatal("wrong result")
		}
	case <-time.After(time.Second):
		t.Fatal("send dropped instead of waiting")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("send did not finish")
	}
	blocked := make(chan struct{})
	go func() { task.sendPaginatedResult(first); close(blocked) }()
	cancel()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("cancel did not release sender")
	}
}
