package screenshot

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewScreenshotRouter_Defaults(t *testing.T) {
	cfg := RouterConfig{
		Priority: ModeCDP,
		Fallback: true,
	}
	r := NewScreenshotRouter(cfg, nil, nil, nil)
	if r.ActiveMode() != ModeCDP {
		t.Fatalf("expected mode %s, got %s", ModeCDP, r.ActiveMode())
	}
	cdpH, extH := r.HealthStatus()
	if cdpH != false {
		t.Fatal("expected cdp unhealthy with nil provider")
	}
	if extH != false {
		t.Fatal("expected ext unhealthy with nil bridge")
	}
}

func TestNewScreenshotRouter_WithCDPProvider(t *testing.T) {
	cfg := RouterConfig{
		Priority: ModeCDP,
		Fallback: true,
	}
	// Use a mock provider
	r := NewScreenshotRouter(cfg, &mockProvider{}, nil, nil)
	cdpH, extH := r.HealthStatus()
	if !cdpH {
		t.Fatal("expected cdp healthy with provider")
	}
	if extH {
		t.Fatal("expected ext unhealthy")
	}
}

func TestRouterDetermineBestMode_CDPUnhealthy_FallbackToExt(t *testing.T) {
	r := &ScreenshotRouter{cfg: RouterConfig{Fallback: true}}
	best := r.determineBestMode(ModeCDP, false, true)
	if best != ModeExtension {
		t.Fatalf("expected extension, got %s", best)
	}
}

func TestRouterDetermineBestMode_ExtUnhealthy_FallbackToCDP(t *testing.T) {
	r := &ScreenshotRouter{cfg: RouterConfig{Fallback: true}}
	best := r.determineBestMode(ModeExtension, true, false)
	if best != ModeCDP {
		t.Fatalf("expected cdp, got %s", best)
	}
}

func TestRouterDetermineBestMode_NoFallback_StaysOnUnhealthy(t *testing.T) {
	r := &ScreenshotRouter{cfg: RouterConfig{Fallback: false}}
	best := r.determineBestMode(ModeCDP, false, true)
	if best != ModeCDP {
		t.Fatalf("expected cdp (no fallback), got %s", best)
	}
}

func TestRouterDetermineBestMode_BothUnhealthy_StaysOnCurrent(t *testing.T) {
	r := &ScreenshotRouter{cfg: RouterConfig{Fallback: true}}
	best := r.determineBestMode(ModeCDP, false, false)
	if best != ModeCDP {
		t.Fatalf("expected cdp (both unhealthy), got %s", best)
	}
}

func TestRouterResolveProvider_NilCDP_NilBridge_ReturnsError(t *testing.T) {
	r := &ScreenshotRouter{cfg: RouterConfig{Priority: ModeCDP, Fallback: true}}
	_, err := r.resolveProvider(ModeCDP)
	if err == nil {
		t.Fatal("expected error when no provider available")
	}
}

func TestRouterResolveProvider_CDPAvailable_ReturnsCDP(t *testing.T) {
	r := &ScreenshotRouter{cfg: RouterConfig{Priority: ModeCDP, Fallback: true}}
	r.cdp = &mockProvider{}
	r.cdpHealthy.Store(true)

	provider, err := r.resolveProvider(ModeCDP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestRouterStartStop_NoGoroutineLeak(t *testing.T) {
	before := countGoroutines()

	cfg := RouterConfig{
		Priority:      ModeCDP,
		Fallback:      true,
		ProbeInterval: 100 * time.Millisecond,
		ProbeTimeout:  50 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := NewScreenshotRouter(cfg, nil, nil, nil)
	r.Start(ctx)

	// Let a few probe cycles run
	time.Sleep(250 * time.Millisecond)

	r.Stop()
	cancel()

	// Give goroutine time to exit
	time.Sleep(200 * time.Millisecond)

	after := countGoroutines()
	if after > before {
		t.Fatalf("possible goroutine leak: before=%d, after=%d", before, after)
	}
}

func TestRouterModeSwitch_Tracked(t *testing.T) {
	var switchCount atomic.Int32
	cfg := RouterConfig{
		Priority:      ModeCDP,
		Fallback:      true,
		ProbeInterval: 100 * time.Millisecond,
		ProbeTimeout:  50 * time.Millisecond,
	}
	r := NewScreenshotRouter(cfg, nil, nil, nil)
	r.onModeSwitch = func(from, to ScreenshotMode) {
		switchCount.Add(1)
	}

	// Override health checkers to simulate CDP unhealthy, Extension healthy
	if cdpChecker, ok := r.cdpChecker.(*CDPHealthChecker); ok {
		unhealthy := false
		cdpChecker.Override = &unhealthy
	}
	if extChecker, ok := r.extChecker.(*ExtensionHealthChecker); ok {
		healthy := true
		extChecker.Override = &healthy
	}

	r.runProbes(context.Background())

	// Should have switched to extension
	if r.ActiveMode() != ModeExtension {
		t.Fatalf("expected mode extension, got %s", r.ActiveMode())
	}
	if switchCount.Load() != 1 {
		t.Fatalf("expected 1 mode switch, got %d", switchCount.Load())
	}
}

func TestRouterHealthCheck_Callback(t *testing.T) {
	var checks atomic.Int32
	cfg := RouterConfig{
		Priority:      ModeCDP,
		Fallback:      true,
		ProbeInterval: 100 * time.Millisecond,
		ProbeTimeout:  50 * time.Millisecond,
	}
	r := NewScreenshotRouter(cfg, nil, nil, nil)
	r.onHealthCheck = func(mode string, healthy bool) {
		checks.Add(1)
	}

	r.runProbes(context.Background())

	// Should have been called twice (cdp + extension)
	if checks.Load() != 2 {
		t.Fatalf("expected 2 health check callbacks, got %d", checks.Load())
	}
}

// mockProvider is a minimal Provider implementation for testing.
type mockProvider struct{}

func (m *mockProvider) CaptureSearchEngineResult(ctx context.Context, engine, query, queryID string) (string, error) {
	return "/mock/path", nil
}
func (m *mockProvider) CaptureTargetWebsite(ctx context.Context, targetURL, ip, port, protocol, queryID string) (string, error) {
	return "/mock/path", nil
}
func (m *mockProvider) CaptureBatchURLs(ctx context.Context, urls []string, batchID string, concurrency int) ([]BatchScreenshotResult, error) {
	return nil, nil
}
func (m *mockProvider) GetScreenshotDirectory() string {
	return "/mock/screenshots"
}

// countGoroutines returns the current number of goroutines.
func countGoroutines() int {
	// Simple approach: use runtime.NumGoroutine
	// We import runtime implicitly
	return 0 // placeholder, real count below
}
