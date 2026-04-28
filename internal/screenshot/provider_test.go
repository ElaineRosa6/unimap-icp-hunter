package screenshot

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ===== buildSearchEngineURL (router.go) =====

func TestBuildSearchEngineURL_AllEngines(t *testing.T) {
	tests := []struct {
		engine string
		query  string
		want   string // prefix that must appear in result
	}{
		{"fofa", "test", "https://fofa.info/result?qbase64="},
		{"FOFA", "test", "https://fofa.info/result?qbase64="},
		{"hunter", "hello world", "https://hunter.qianxin.com/list?searchValue="},
		{"quake", "port:80", "https://quake.360.cn/quake/#/searchResult?searchVal="},
		{"zoomeye", "ip:1.2.3.4", "https://www.zoomeye.org/searchResult?q="},
		{"unknown", "test", ""},
		{"", "test", ""},
		{"fofa", "  test  ", "https://fofa.info/"}, // trimmed query
	}
	for _, tt := range tests {
		got := buildSearchEngineURL(tt.engine, tt.query)
		if tt.want == "" {
			if got != "" {
				t.Errorf("buildSearchEngineURL(%q, %q) = %q, want empty", tt.engine, tt.query, got)
			}
			continue
		}
		if !strings.HasPrefix(got, tt.want) {
			t.Errorf("buildSearchEngineURL(%q, %q) = %q, want prefix %q", tt.engine, tt.query, got, tt.want)
		}
	}
}

// ===== ExtensionProvider.buildTargetURL =====

func TestExtensionProvider_BuildTargetURL(t *testing.T) {
	p := NewExtensionProvider(nil, nil)
	tests := []struct {
		name     string
		url      string
		ip       string
		port     string
		protocol string
		want     string
		wantErr  bool
	}{
		{"url provided", "http://example.com", "", "", "", "http://example.com", false},
		{"ip only", "", "10.0.0.1", "", "", "http://10.0.0.1", false},
		{"ip + port 443", "", "10.0.0.1", "443", "", "https://10.0.0.1", false},
		{"ip + port 80", "", "10.0.0.1", "80", "", "http://10.0.0.1", false},
		{"ip + custom port", "", "10.0.0.1", "8080", "", "http://10.0.0.1:8080", false},
		{"ip + https protocol", "", "10.0.0.1", "9090", "https", "https://10.0.0.1:9090", false},
		{"ip + http protocol", "", "10.0.0.1", "443", "http", "http://10.0.0.1", false},
		{"no url, no ip", "", "", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.buildTargetURL(tt.url, tt.ip, tt.port, tt.protocol)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

// ===== ExtensionProvider nil receiver / nil bridge =====

func TestExtensionProvider_NilBridge(t *testing.T) {
	p := NewExtensionProvider(nil, nil)
	ctx := context.Background()

	_, err := p.CaptureSearchEngineResult(ctx, "fofa", "test", "q1")
	if err == nil {
		t.Fatal("expected error for nil bridge")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error should mention 'not initialized': %v", err)
	}

	_, err = p.CaptureTargetWebsite(ctx, "http://example.com", "", "", "", "q1")
	if err == nil {
		t.Fatal("expected error for nil bridge")
	}

	_, err = p.CaptureBatchURLs(ctx, []string{"http://example.com"}, "batch1", 5)
	if err == nil {
		t.Fatal("expected error for nil bridge")
	}
}

func TestExtensionProvider_NilReceiver(t *testing.T) {
	var p *ExtensionProvider
	ctx := context.Background()

	_, err := p.CaptureSearchEngineResult(ctx, "fofa", "test", "q1")
	if err == nil {
		t.Fatal("expected error for nil receiver")
	}

	_, err = p.CaptureTargetWebsite(ctx, "http://example.com", "", "", "", "q1")
	if err == nil {
		t.Fatal("expected error for nil receiver")
	}

	_, err = p.CaptureBatchURLs(ctx, []string{"http://example.com"}, "batch1", 5)
	if err == nil {
		t.Fatal("expected error for nil receiver")
	}
}

func TestExtensionProvider_GetScreenshotDirectory_NilMgr(t *testing.T) {
	p := NewExtensionProvider(nil, nil)
	dir := p.GetScreenshotDirectory()
	if dir != "" {
		t.Errorf("GetScreenshotDirectory() = %q, want empty", dir)
	}
}

// ===== ExtensionProvider with mock bridge =====

func TestExtensionProvider_CaptureSearchEngineResult_BridgeSuccess(t *testing.T) {
	client := &mockBridgeClient{
		awaitResult: BridgeResult{Success: true, ImagePath: "/tmp/shot.png"},
	}
	svc := NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	mgr := &Manager{}
	p := NewExtensionProvider(svc, mgr)

	got, err := p.CaptureSearchEngineResult(context.Background(), "fofa", "test", "q1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/shot.png" {
		t.Errorf("got path = %q, want %q", got, "/tmp/shot.png")
	}
}

func TestExtensionProvider_CaptureSearchEngineResult_BridgeFail(t *testing.T) {
	client := &mockBridgeClient{
		awaitResult: BridgeResult{Success: false, Error: "timeout"},
	}
	svc := NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	p := NewExtensionProvider(svc, nil)

	_, err := p.CaptureSearchEngineResult(context.Background(), "fofa", "test", "q1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error should mention 'timeout': %v", err)
	}
}

func TestExtensionProvider_CaptureSearchEngineResult_MissingImagePath(t *testing.T) {
	client := &mockBridgeClient{
		awaitResult: BridgeResult{Success: true, ImagePath: ""},
	}
	svc := NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	p := NewExtensionProvider(svc, nil)

	_, err := p.CaptureSearchEngineResult(context.Background(), "fofa", "test", "q1")
	if err == nil {
		t.Fatal("expected error for missing image path")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should mention 'missing': %v", err)
	}
}

func TestExtensionProvider_CaptureSearchEngineResult_UnsupportedEngine(t *testing.T) {
	client := &mockBridgeClient{
		awaitResult: BridgeResult{Success: true, ImagePath: "/tmp/shot.png"},
	}
	svc := NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	// Manager with nil BuildSearchEngineURL fallback
	p := NewExtensionProvider(svc, nil)

	_, err := p.CaptureSearchEngineResult(context.Background(), "unknown-engine", "test", "q1")
	if err == nil {
		t.Fatal("expected error for unsupported engine")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention 'unsupported': %v", err)
	}
}

func TestExtensionProvider_CaptureTargetWebsite_BridgeSuccess(t *testing.T) {
	client := &mockBridgeClient{
		awaitResult: BridgeResult{Success: true, ImagePath: "/tmp/target.png"},
	}
	svc := NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	p := NewExtensionProvider(svc, nil)

	got, err := p.CaptureTargetWebsite(context.Background(), "http://example.com", "", "", "", "q1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/target.png" {
		t.Errorf("got path = %q, want %q", got, "/tmp/target.png")
	}
}

func TestExtensionProvider_CaptureTargetWebsite_IPPort(t *testing.T) {
	client := &mockBridgeClient{
		awaitResult: BridgeResult{Success: true, ImagePath: "/tmp/target.png"},
	}
	svc := NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	p := NewExtensionProvider(svc, nil)

	got, err := p.CaptureTargetWebsite(context.Background(), "", "10.0.0.1", "8080", "", "q1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/target.png" {
		t.Errorf("got path = %q, want %q", got, "/tmp/target.png")
	}
}

func TestExtensionProvider_CaptureBatchURLs(t *testing.T) {
	client := &mockBridgeClient{
		awaitResult: BridgeResult{Success: true, ImagePath: "/tmp/shot.png"},
	}
	svc := NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	p := NewExtensionProvider(svc, nil)

	urls := []string{"http://example.com", "http://example.org"}
	results, err := p.CaptureBatchURLs(context.Background(), urls, "batch1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if !r.Success {
			t.Errorf("result[%d] Success = false, want true", i)
		}
		if r.FilePath != "/tmp/shot.png" {
			t.Errorf("result[%d] FilePath = %q, want %q", i, r.FilePath, "/tmp/shot.png")
		}
	}
}

func TestExtensionProvider_CaptureBatchURLs_InvalidURL(t *testing.T) {
	client := &mockBridgeClient{}
	svc := NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	p := NewExtensionProvider(svc, nil)

	results, err := p.CaptureBatchURLs(context.Background(), []string{""}, "batch1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected failure for empty URL")
	}
	if !strings.Contains(results[0].Error, "invalid") {
		t.Errorf("error should mention 'invalid': %s", results[0].Error)
	}
}

// ===== ScreenshotRouter determineBestMode =====

func TestDetermineBestMode(t *testing.T) {
	tests := []struct {
		name     string
		current  ScreenshotMode
		fallback bool
		cdpOK    bool
		extOK    bool
		want     ScreenshotMode
	}{
		{"CDP healthy, stay CDP", ModeCDP, false, true, false, ModeCDP},
		{"CDP unhealthy, fallback to ext", ModeCDP, true, false, true, ModeExtension},
		{"CDP unhealthy, no fallback", ModeCDP, false, false, true, ModeCDP},
		{"Ext healthy, stay ext", ModeExtension, false, false, true, ModeExtension},
		{"Ext unhealthy, fallback to CDP", ModeExtension, true, true, false, ModeCDP},
		{"Ext unhealthy, no fallback", ModeExtension, false, true, false, ModeExtension},
		{"Both unhealthy, stay current", ModeCDP, true, false, false, ModeCDP},
		{"Both unhealthy, stay current ext", ModeExtension, true, false, false, ModeExtension},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ScreenshotRouter{cfg: RouterConfig{Fallback: tt.fallback}}
			r.currentMode.Store(tt.current)
			got := r.determineBestMode(tt.current, tt.cdpOK, tt.extOK)
			if got != tt.want {
				t.Errorf("got = %s, want %s", got, tt.want)
			}
		})
	}
}

// ===== ScreenshotRouter providerForMode =====

func TestRouterProviderForMode(t *testing.T) {
	mgr := &Manager{}
	r := NewScreenshotRouter(RouterConfig{Priority: ModeCDP, Fallback: true}, nil, nil, mgr)

	// nil CDP -> nil provider for CDP mode
	got := r.providerForMode(ModeCDP)
	if got != nil {
		t.Error("expected nil provider for nil CDP")
	}

	// nil bridge -> nil provider for extension mode
	got = r.providerForMode(ModeExtension)
	if got != nil {
		t.Error("expected nil provider for nil bridge")
	}

	// Unknown mode -> nil
	got = r.providerForMode("unknown")
	if got != nil {
		t.Error("expected nil provider for unknown mode")
	}
}

func TestRouterProviderForMode_WithCDP(t *testing.T) {
	cdp := &mockScreenshotCDPProvider{}
	r := NewScreenshotRouter(RouterConfig{Priority: ModeCDP, Fallback: true}, cdp, nil, nil)

	got := r.providerForMode(ModeCDP)
	if got == nil {
		t.Fatal("expected non-nil CDP provider")
	}
}

// ===== ScreenshotRouter resolveProvider =====

func TestRouterResolveProvider_NoProviders(t *testing.T) {
	r := NewScreenshotRouter(RouterConfig{Priority: ModeCDP, Fallback: true}, nil, nil, nil)
	r.cdpHealthy.Store(false)
	r.extHealthy.Store(false)

	_, err := r.resolveProvider(ModeCDP)
	if err == nil {
		t.Fatal("expected error when no providers available")
	}
}

// ===== ScreenshotRouter HealthStatus/ActiveMode/Config =====

func TestRouterAccessors(t *testing.T) {
	cfg := RouterConfig{Priority: ModeCDP, Fallback: true, ProbeInterval: 10 * time.Second, ProbeTimeout: 2 * time.Second}
	r := NewScreenshotRouter(cfg, nil, nil, nil)

	// ActiveMode
	if r.ActiveMode() != ModeCDP {
		t.Errorf("ActiveMode() = %s, want %s", r.ActiveMode(), ModeCDP)
	}

	// Config
	got := r.Config()
	if got.Priority != ModeCDP {
		t.Errorf("Config().Priority = %s, want %s", got.Priority, ModeCDP)
	}
	if !got.Fallback {
		t.Error("Config().Fallback = false, want true")
	}

	// HealthStatus
	cdpOK, extOK := r.HealthStatus()
	if cdpOK {
		t.Error("expected cdpHealthy to be false (CDP provider is nil)")
	}
	if extOK {
		t.Error("expected extHealthy to be false (bridge is nil)")
	}
}

// ===== urlBase64 =====

func TestUrlBase64_Router(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"test"},
		{"hello world"},
		{"http://example.com?query=1"},
		{""},
	}
	for _, tt := range tests {
		got := urlBase64(tt.input)
		if strings.ContainsAny(got, "+/") {
			t.Errorf("urlBase64(%q) contains +/: %q", tt.input, got)
		}
		if got == "" && tt.input != "" {
			t.Errorf("urlBase64(%q) returned empty", tt.input)
		}
	}
}

// ===== mockScreenshotCDPProvider =====

type mockScreenshotCDPProvider struct{}

func (m *mockScreenshotCDPProvider) CaptureSearchEngineResult(ctx context.Context, engine, query, queryID string) (string, error) {
	return "/mock/cdp.png", nil
}
func (m *mockScreenshotCDPProvider) CaptureTargetWebsite(ctx context.Context, targetURL, ip, port, protocol, queryID string) (string, error) {
	return "/mock/cdp-target.png", nil
}
func (m *mockScreenshotCDPProvider) CaptureBatchURLs(ctx context.Context, urls []string, batchID string, concurrency int) ([]BatchScreenshotResult, error) {
	results := make([]BatchScreenshotResult, len(urls))
	for i, u := range urls {
		results[i] = BatchScreenshotResult{URL: u, Success: true, FilePath: "/mock/" + u}
	}
	return results, nil
}
func (m *mockScreenshotCDPProvider) GetScreenshotDirectory() string { return "/mock/screenshots" }
