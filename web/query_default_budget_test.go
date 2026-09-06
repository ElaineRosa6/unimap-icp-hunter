package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unimap/project/internal/adapter"
	"github.com/unimap/project/internal/collection"
	"github.com/unimap/project/internal/history"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/screenshot"
	"github.com/unimap/project/internal/service"
)

type queryBudgetProvider struct {
	successfulScreenshotProvider
	budgets chan time.Duration
}

func (p *queryBudgetProvider) CollectSearchEngineResult(ctx context.Context, _, _, _ string) ([]collection.CollectResult, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		p.budgets <- 0
	} else {
		p.budgets <- time.Until(deadline)
	}
	return nil, errors.New("fixture collection completed with error")
}

func TestHTTPQueryDefaultBrowserBudget(t *testing.T) {
	for _, action := range []string{"", "  ", "collect_and_capture", "collect"} {
		t.Run("action="+action, func(t *testing.T) {
			provider := &queryBudgetProvider{budgets: make(chan time.Duration, 1)}
			router := screenshot.NewScreenshotRouter(screenshot.RouterConfig{Priority: screenshot.ModeCDP}, provider, nil, nil)
			s := &Server{queryApp: service.NewQueryAppService(nil, adapter.NewEngineOrchestrator()), screenshotRouter: router}
			form := url.Values{"query": {`port="443"`}, "engines": {"fofa"}, "browser_query": {"true"}, "browser_action": {action}}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/query", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "http://localhost:8448")
			w := httptest.NewRecorder()
			s.handleAPIQuery(w, req)
			select {
			case got := <-provider.budgets:
				want := service.BrowserCollectAndCaptureWaitTimeout
				if action == "collect" {
					want = service.BrowserQueryWaitTimeout
				}
				if got > want || got < want-5*time.Second {
					t.Errorf("browser deadline remaining=%v want approximately %v", got, want)
				}
			default:
				t.Fatalf("browser provider not reached: HTTP %d %s", w.Code, w.Body.String())
			}
		})
	}
}

type delayedQueryProvider struct{ successfulScreenshotProvider }

func (p *delayedQueryProvider) CollectSearchEngineResult(ctx context.Context, _, _, _ string) ([]collection.CollectResult, error) {
	select {
	case <-time.After(200 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nil, errors.New("fixture collection failed after original write deadline")
}
func TestHTTPBrowserQueryOutlivesServerWriteTimeout(t *testing.T) {
	provider := &delayedQueryProvider{}
	router := screenshot.NewScreenshotRouter(screenshot.RouterConfig{Priority: screenshot.ModeCDP}, provider, nil, nil)
	s := &Server{queryApp: service.NewQueryAppService(nil, adapter.NewEngineOrchestrator()), screenshotRouter: router}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exercise the same response-controller unwrapping path as production.
		s.handleAPIQuery(&statusRecorder{ResponseWriter: &auditResponseWriter{ResponseWriter: w, statusCode: 200}, statusCode: 200}, r)
	}))
	ts.Config.WriteTimeout = 50 * time.Millisecond
	ts.Start()
	defer ts.Close()
	form := url.Values{"query": {`port="443"`}, "engines": {"fofa"}, "browser_query": {"true"}, "browser_action": {"collect"}}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/query", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://localhost:8448")
	client := ts.Client()
	client.Timeout = 5 * time.Second
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("query response lost at transport: %v", err)
	}
	defer res.Body.Close()
	var payload map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("query response is not complete JSON: %v", err)
	}
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("HTTP %d, want explicit failed query response", res.StatusCode)
	}
}

func TestHTTPQueryWriteDeadlineIsBounded(t *testing.T) {
	for _, tc := range []struct {
		name, enabled, action string
		parent, want          time.Duration
	}{
		{"api", "false", "", 0, service.QueryExecutionTimeout + 15*time.Second},
		{"collect", "true", "collect", 0, service.BrowserQueryWaitTimeout + 15*time.Second},
		{"default", "true", "", 0, service.BrowserCollectAndCaptureWaitTimeout + 15*time.Second},
		{"parent", "true", "collect_and_capture", time.Second, 16 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{queryApp: service.NewQueryAppService(nil, adapter.NewEngineOrchestrator())}
			form := url.Values{"query": {`port="443"`}, "engines": {"fofa"}, "browser_query": {tc.enabled}, "browser_action": {tc.action}}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/query", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "http://localhost:8448")
			if tc.parent > 0 {
				ctx, cancel := context.WithTimeout(req.Context(), tc.parent)
				defer cancel()
				req = req.WithContext(ctx)
			}
			w := &auditDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
			start := time.Now()
			s.handleAPIQuery(w, req)
			got := w.deadline.Sub(start)
			if got < tc.want-time.Second || got > tc.want+time.Second {
				t.Fatalf("write deadline budget=%s, want %s", got, tc.want)
			}
		})
	}
}

type delayedAssetQueryProvider struct{ successfulScreenshotProvider }

func (p *delayedAssetQueryProvider) CollectSearchEngineResult(ctx context.Context, engine, query, _ string) ([]collection.CollectResult, error) {
	select {
	case <-time.After(200 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return []collection.CollectResult{{Engine: engine, Query: query, Assets: []model.UnifiedAsset{{Host: "asset.example.test", Port: 443, Source: engine}}, RowsFound: 1}}, nil
}

type unsupportedQueryDeadlineWriter struct{ http.ResponseWriter }

func (w *unsupportedQueryDeadlineWriter) SetWriteDeadline(time.Time) error {
	return http.ErrNotSupported
}
func TestHTTPQueryAssetsPersistAndSurviveWriteTimeout(t *testing.T) {
	for _, control := range []bool{true, false} {
		name := "deadline_fixed"
		if control {
			name = "control_without_deadline_support"
		}
		t.Run(name, func(t *testing.T) {
			db, err := history.NewDatabase(filepath.Join(t.TempDir(), "history.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if schemaErr := db.InitSchema(); schemaErr != nil {
				t.Fatal(schemaErr)
			}
			repo := history.NewRepository(db.DB())
			provider := &delayedAssetQueryProvider{}
			router := screenshot.NewScreenshotRouter(screenshot.RouterConfig{Priority: screenshot.ModeCDP}, provider, nil, nil)
			svc := service.NewQueryAppService(nil, adapter.NewEngineOrchestrator())
			svc.SetHistoryRepository(repo)
			s := &Server{queryApp: svc, screenshotRouter: router}
			ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if control {
					w = &unsupportedQueryDeadlineWriter{w}
				}
				s.handleAPIQuery(&statusRecorder{ResponseWriter: &auditResponseWriter{ResponseWriter: w, statusCode: 200}, statusCode: 200}, r)
			}))
			ts.Config.WriteTimeout = 50 * time.Millisecond
			ts.Start()
			defer ts.Close()
			form := url.Values{"query": {`port="443"`}, "engines": {"fofa"}, "browser_query": {"true"}, "browser_action": {"collect"}}
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/query", strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "http://localhost:8448")
			client := ts.Client()
			client.Timeout = 5 * time.Second
			res, requestErr := client.Do(req)
			if res != nil {
				defer res.Body.Close()
			}
			if control {
				if requestErr == nil {
					t.Fatal("control must reproduce transport failure")
				}
				t.Logf("CONTROL_TRANSPORT_FAILURE: %v", requestErr)
			} else {
				if requestErr != nil {
					t.Fatal(requestErr)
				}
				var payload QueryAPIPayload
				if decodeErr := json.NewDecoder(res.Body).Decode(&payload); decodeErr != nil {
					t.Fatal(decodeErr)
				}
				if res.StatusCode != 200 || payload.Status != "partial" || payload.Persistence.Status != "persisted" || len(payload.Assets) != 1 || payload.Assets[0].Host != "asset.example.test" {
					t.Fatalf("unexpected response: HTTP %d %#v", res.StatusCode, payload)
				}
				t.Log("HTTP_200_PARTIAL_ASSET_DELIVERED")
			}
			record, err := repo.GetHistory(1)
			if err != nil {
				t.Fatal(err)
			}
			results, err := repo.GetResults(1)
			if err != nil {
				t.Fatal(err)
			}
			if record.Status != "partial" || record.TotalCount != 1 || len(results) != 1 {
				t.Fatalf("history/result mismatch: %#v %#v", record, results)
			}
			var asset model.UnifiedAsset
			if err := json.Unmarshal([]byte(results[0].Data), &asset); err != nil {
				t.Fatal(err)
			}
			if asset.Host != "asset.example.test" || asset.Extra["collection_method"] != "browser" {
				t.Fatalf("wrong stored asset: %#v", asset)
			}
			t.Log("SQLITE_PARTIAL_ONE_BROWSER_ASSET_CONFIRMED")
		})
	}
}
