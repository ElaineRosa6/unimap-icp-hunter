package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unimap/project/internal/adapter"
	"github.com/unimap/project/internal/collection"
	"github.com/unimap/project/internal/model"
)

type stubBrowserRouter struct {
	mu              sync.Mutex
	openErrByEngine map[string]error
	collectResults  map[string][]collection.CollectResult
	openCalls       int
}

func (r *stubBrowserRouter) OpenSearchEngineResult(_ context.Context, engine, _ string) (string, error) {
	r.mu.Lock()
	r.openCalls++
	r.mu.Unlock()
	if err := r.openErrByEngine[engine]; err != nil {
		return "", err
	}
	return "https://example.test/search", nil
}

func (r *stubBrowserRouter) CollectSearchEngineResult(_ context.Context, engine, query, _ string) ([]collection.CollectResult, error) {
	if results, ok := r.collectResults[engine]; ok {
		return results, nil
	}
	return []collection.CollectResult{{Engine: engine, Query: query}}, nil
}

type stubCombinedBrowserRouter struct {
	stubBrowserRouter
	combinedCalls int
}

func (r *stubCombinedBrowserRouter) CollectAndCaptureSearchEngineResult(_ context.Context, engine, query, queryID string) ([]collection.CollectResult, string, error) {
	r.mu.Lock()
	r.combinedCalls++
	r.mu.Unlock()
	return []collection.CollectResult{{Engine: engine, Query: queryID, Assets: []model.UnifiedAsset{{URL: query}}}}, "/tmp/capture.png", nil
}

func TestRunBrowserQueryAsync_ReportsProgressForEachEngine(t *testing.T) {
	svc := NewQueryAppService(nil, nil)
	router := &stubBrowserRouter{
		openErrByEngine: map[string]error{"hunter": errors.New("login required")},
	}

	var mu sync.Mutex
	var calls []struct {
		done   int
		total  int
		engine string
		err    error
	}
	ch := svc.RunBrowserQueryAsync(
		context.Background(),
		"test",
		[]string{"fofa", "hunter"},
		true,
		"open",
		"q1",
		false,
		nil,
		nil,
		nil,
		router,
		func(done, total int, engine string, err error) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, struct {
				done   int
				total  int
				engine string
				err    error
			}{done: done, total: total, engine: engine, err: err})
		},
	)

	outcome := <-ch
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("expected 2 progress calls, got %d", len(calls))
	}
	// Parallel execution: order is non-deterministic, so verify by engine.
	byEngine := map[string]bool{}
	doneValues := map[int]bool{}
	for _, c := range calls {
		if c.total != 2 {
			t.Fatalf("expected total=2 for %s, got %d", c.engine, c.total)
		}
		byEngine[c.engine] = true
		doneValues[c.done] = true
	}
	if !byEngine["fofa"] || !byEngine["hunter"] {
		t.Fatalf("expected both fofa and hunter in progress calls, got %v", byEngine)
	}
	if !doneValues[1] || !doneValues[2] {
		t.Fatalf("expected done values 1 and 2, got %v", doneValues)
	}
	if len(outcome.Errors) != 1 {
		t.Fatalf("expected one browser error, got %#v", outcome.Errors)
	}
}

func TestRunBrowserQueryAsync_CollectsStructuredAssets(t *testing.T) {
	svc := NewQueryAppService(nil, nil)
	router := &stubBrowserRouter{
		collectResults: map[string][]collection.CollectResult{
			"fofa": {{
				Engine: "fofa",
				Query:  "test",
				Assets: []model.UnifiedAsset{{URL: "https://example.test", Title: "Example"}},
				Total:  1,
			}},
		},
	}

	ch := svc.RunBrowserQueryAsync(context.Background(), "test", []string{"fofa"}, true, "open", "q1", false, nil, nil, nil, router, nil)
	outcome := <-ch

	if len(outcome.OpenedEngines) != 1 || outcome.OpenedEngines[0] != "fofa" {
		t.Fatalf("expected fofa to be opened, got %#v", outcome.OpenedEngines)
	}
	if len(outcome.Errors) != 0 {
		t.Fatalf("expected no errors, got %#v", outcome.Errors)
	}
}

func TestRunBrowserQueryAsync_CollectAndCaptureSkipsPreOpenForCombinedRouter(t *testing.T) {
	svc := NewQueryAppService(nil, nil)
	router := &stubCombinedBrowserRouter{}
	screenshotApp := NewScreenshotAppServiceWithProvider(t.TempDir(), &mockScreenshotProvider{})

	ch := svc.RunBrowserQueryAsync(
		context.Background(), "test", []string{"fofa"}, true, "collect_and_capture", "q1", true,
		screenshotApp, nil, func(path string) string { return "preview:" + path }, router, nil,
	)
	outcome := <-ch

	if router.openCalls != 0 {
		t.Fatalf("expected combined collect+capture to skip pre-open, got %d opens", router.openCalls)
	}
	if router.combinedCalls != 1 {
		t.Fatalf("expected one combined call, got %d", router.combinedCalls)
	}
	if len(outcome.OpenedEngines) != 1 || outcome.OpenedEngines[0] != "fofa" {
		t.Fatalf("expected combined flow to mark fofa opened, got %#v", outcome.OpenedEngines)
	}
	if got := outcome.AutoCapturedPaths["fofa"]; got != "preview:/tmp/capture.png" {
		t.Fatalf("unexpected preview path: %q", got)
	}
}

func TestBrowserQueryWaitTimeoutForAction(t *testing.T) {
	if got := BrowserQueryWaitTimeoutForAction("collect_and_capture"); got != BrowserCollectAndCaptureWaitTimeout {
		t.Fatalf("collect_and_capture wait timeout = %s, want %s", got, BrowserCollectAndCaptureWaitTimeout)
	}
	if got := BrowserQueryWaitTimeoutForAction(" collect_and_capture "); got != BrowserCollectAndCaptureWaitTimeout {
		t.Fatalf("trimmed collect_and_capture wait timeout = %s", got)
	}
	if got := BrowserQueryWaitTimeoutForAction("collect"); got != BrowserQueryWaitTimeout {
		t.Fatalf("collect wait timeout = %s, want %s", got, BrowserQueryWaitTimeout)
	}
	if BrowserCollectAndCaptureWaitTimeout < 2*time.Minute {
		t.Fatalf("collect_and_capture wait timeout should allow serial extension work, got %s", BrowserCollectAndCaptureWaitTimeout)
	}
}

func TestWithoutDeadlineKeepsCancellationButNotDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelParent()
	child := withoutDeadline(parent)
	if _, ok := child.Deadline(); ok {
		t.Fatal("withoutDeadline must not inherit the parent deadline")
	}
	select {
	case <-child.Done():
		t.Fatal("child context must not be canceled before the parent")
	default:
	}
	cancelParent()
	select {
	case <-child.Done():
	case <-time.After(time.Second):
		t.Fatal("withoutDeadline must propagate parent cancellation")
	}
	if child.Err() != context.Canceled {
		t.Fatalf("child error = %v, want context.Canceled", child.Err())
	}
}

func TestWithoutDeadlineReusesDeadlineFreeContext(t *testing.T) {
	plain := context.Background()
	if got := withoutDeadline(plain); got != plain {
		t.Fatal("withoutDeadline must reuse a context that already has no deadline")
	}
	var nilCtx context.Context
	//nolint:staticcheck // withoutDeadline documents nil handling; verify it returns Background for nil.
	if got := withoutDeadline(nilCtx); got == nil {
		t.Fatal("withoutDeadline(nil) must return a non-nil context")
	}
}

func TestTranslateBrowserQueryWithoutRegisteredAPIAdapter(t *testing.T) {
	svc := NewQueryAppService(nil, adapter.NewEngineOrchestrator())
	translated, err := svc.translateBrowserQuery(`port="443"`, "fofa")
	if err != nil {
		t.Fatalf("translate browser-only query: %v", err)
	}
	if translated == "" {
		t.Fatal("browser-only translation returned empty query")
	}
}

func TestTranslateBrowserQueryWithoutRegisteredAPIAdapter_NewBrowserEngines(t *testing.T) {
	svc := NewQueryAppService(nil, adapter.NewEngineOrchestrator())
	tests := []struct {
		engine string
		want   string
	}{
		{engine: "censys", want: `host.services.port=443`},
		{engine: "daydaymap", want: `ip.port="443"`},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			translated, err := svc.translateBrowserQuery(`port="443"`, tt.engine)
			if err != nil {
				t.Fatalf("translate browser query: %v", err)
			}
			if translated != tt.want {
				t.Fatalf("translated query = %q, want %q", translated, tt.want)
			}
		})
	}
}

func TestExecuteQueryWithBrowserWorkflow_CanceledContextCannotSucceed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := NewQueryAppService(nil, nil)
	_, _, err := svc.ExecuteQueryWithBrowserWorkflow(ctx, `port="443"`, []string{"fofa"}, 10, BrowserQueryWorkflowOptions{})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("canceled workflow error = %v", err)
	}
}

func TestExecuteQueryWithBrowserWorkflow_APIUnavailable_BrowserAssetsSucceed(t *testing.T) {
	router := &stubCombinedBrowserRouter{}
	svc := NewQueryAppService(nil, nil)
	screenshotApp := NewScreenshotAppServiceWithProvider(t.TempDir(), &mockScreenshotProvider{})

	resp, outcome, err := svc.ExecuteQueryWithBrowserWorkflow(
		context.Background(), `port="443"`, []string{"fofa"}, 10,
		BrowserQueryWorkflowOptions{
			Action:        "collect_and_capture",
			QueryID:       "browser-success",
			BrowserRouter: router,
			ScreenshotApp: screenshotApp,
		},
	)
	if err != nil {
		t.Fatalf("browser assets should keep the workflow successful when API is unavailable: %v", err)
	}
	if len(outcome.CollectedResults) != 1 || len(outcome.CollectedResults[0].Assets) != 1 {
		t.Fatalf("unexpected browser outcome: %#v", outcome)
	}
	if len(resp.Assets) != 1 || resp.Assets[0].Extra["collection_method"] != "browser" {
		t.Fatalf("browser asset was not merged/tagged: %#v", resp.Assets)
	}
	if len(resp.Errors) == 0 || !strings.Contains(resp.Errors[0], "API query failed; Bridge results used") {
		t.Fatalf("expected preserved API failure diagnostic, got %#v", resp.Errors)
	}
}

func TestExecuteQueryWithBrowserWorkflow_APIUnavailable_EmptyBrowserResultFails(t *testing.T) {
	router := &stubBrowserRouter{
		collectResults: map[string][]collection.CollectResult{
			"fofa": {{Engine: "fofa", Query: `port="443"`}},
		},
	}
	svc := NewQueryAppService(nil, nil)

	_, outcome, err := svc.ExecuteQueryWithBrowserWorkflow(
		context.Background(), `port="443"`, []string{"fofa"}, 10,
		BrowserQueryWorkflowOptions{
			Action:        "collect",
			QueryID:       "browser-empty",
			BrowserRouter: router,
		},
	)
	if err == nil {
		t.Fatalf("empty browser collection must not mask API failure: %#v", outcome)
	}
}

func TestValidateBrowserQueryWorkflow_EmptyEnvelopeIsIncomplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.png")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := validateBrowserQueryWorkflow([]string{"fofa"}, "collect_and_capture", BrowserQueryOutcome{
		CollectedResults:  []collection.CollectResult{{Engine: "fofa"}},
		AutoCapturedPaths: map[string]string{"fofa": path},
	}, true)
	if err == nil || !strings.Contains(err.Error(), "no structured assets") {
		t.Fatalf("empty collection envelope should be incomplete, got %v", err)
	}
}

type emptyRowsCombinedRouter struct{ stubBrowserRouter }

func (r *emptyRowsCombinedRouter) CollectAndCaptureSearchEngineResult(ctx context.Context, engine, query, id string) ([]collection.CollectResult, string, error) {
	results, err := r.CollectSearchEngineResult(ctx, engine, query, id)
	return results, "/tmp/evidence.png", err
}
func TestRunBrowserQueryAsyncReportsUnextractedRows(t *testing.T) {
	for _, action := range []string{"collect", "collect_and_capture"} {
		for _, rows := range []int{0, 10} {
			t.Run(fmt.Sprintf("%s/rows=%d", action, rows), func(t *testing.T) {
				router := &emptyRowsCombinedRouter{stubBrowserRouter{collectResults: map[string][]collection.CollectResult{"quake": {{Engine: "quake", RowsFound: rows, ExtractionMethod: "selector"}}}}}
				svc := NewQueryAppService(nil, nil)
				out := <-svc.RunBrowserQueryAsync(context.Background(), `port="80"`, []string{"quake"}, true, action, "rows-check", false, nil, nil, func(p string) string { return p }, router, nil)
				if len(out.CollectedResults) != 1 {
					t.Fatalf("lost collection evidence: %#v", out)
				}
				if rows == 0 && len(out.Errors) != 0 {
					t.Fatalf("genuine zero results must remain valid: %v", out.Errors)
				}
				if rows > 0 && (len(out.Errors) != 1 || !strings.Contains(out.Errors[0], "10 DOM rows")) {
					t.Fatalf("missing extraction diagnostic: %v", out.Errors)
				}
				merged := mergeBrowserQueryResponse(&QueryResponse{Assets: []model.UnifiedAsset{{IP: "192.0.2.1"}}}, out)
				if rows > 0 && len(merged.Errors) == 0 {
					t.Fatal("API assets masked browser extraction failure")
				}
			})
		}
	}
}
