package screenshot

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ScreenshotMode represents the active screenshot capture mode.
type ScreenshotMode string

const (
	ModeCDP       ScreenshotMode = "cdp"
	ModeExtension ScreenshotMode = "extension"
)

// RouterConfig holds the routing configuration.
type RouterConfig struct {
	Priority      ScreenshotMode // Primary mode to prefer
	Fallback      bool           // Whether to fall back to the other mode on failure
	ProbeInterval time.Duration  // How often to run health checks
	ProbeTimeout  time.Duration  // Timeout per probe
}

// ScreenshotRouter routes screenshot requests between CDP and Extension modes
// with automatic health-based failover.
type ScreenshotRouter struct {
	cfg       RouterConfig
	cdp       Provider
	extBridge *BridgeService
	mgr       *Manager

	cdpChecker HealthChecker
	extChecker HealthChecker

	// Current active mode (atomic for lock-free reads)
	currentMode atomic.Value // ScreenshotMode

	// Per-mode health status
	cdpHealthy atomic.Bool
	extHealthy atomic.Bool

	// Probe goroutine lifecycle
	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool

	// Metrics hooks
	onModeSwitch  func(from, to ScreenshotMode)
	onHealthCheck func(mode string, healthy bool)
}

// SetMetricsHooks registers callback functions for Prometheus metrics.
func (r *ScreenshotRouter) SetMetricsHooks(onModeSwitch func(from, to ScreenshotMode), onHealthCheck func(mode string, healthy bool)) {
	r.onModeSwitch = onModeSwitch
	r.onHealthCheck = onHealthCheck
}

// NewScreenshotRouter creates a new ScreenshotRouter.
func NewScreenshotRouter(cfg RouterConfig, cdp Provider, extBridge *BridgeService, mgr *Manager) *ScreenshotRouter {
	if cfg.ProbeInterval <= 0 {
		cfg.ProbeInterval = 30 * time.Second
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = 5 * time.Second
	}

	r := &ScreenshotRouter{
		cfg:       cfg,
		cdp:       cdp,
		extBridge: extBridge,
		mgr:       mgr,
		stopCh:    make(chan struct{}),
	}

	// Initialize health checkers
	if cdp != nil {
		remoteURL := ""
		if mgr != nil {
			remoteURL = mgr.RemoteDebugURL()
		}
		r.cdpChecker = &CDPHealthChecker{RemoteDebugURL: remoteURL}
		r.cdpHealthy.Store(true) // CDP available
	} else {
		r.cdpChecker = &CDPHealthChecker{RemoteDebugURL: ""}
		r.cdpHealthy.Store(false)
	}

	isMock := extBridge != nil && isMockBridgeClient(extBridge)
	r.extChecker = &ExtensionHealthChecker{BridgeService: extBridge, IsMock: isMock}
	r.extHealthy.Store(extBridge != nil)

	// Set initial mode
	r.currentMode.Store(cfg.Priority)

	return r
}

// Start launches the health probe goroutine.
func (r *ScreenshotRouter) Start(ctx context.Context) {
	// Run initial probes synchronously
	r.runProbes(ctx)

	go r.probeLoop(ctx)
}

// Stop terminates the health probe goroutine.
func (r *ScreenshotRouter) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.stopped {
		r.stopped = true
		close(r.stopCh)
	}
}

func (r *ScreenshotRouter) probeLoop(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.runProbes(ctx)
		}
	}
}

func (r *ScreenshotRouter) runProbes(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, r.cfg.ProbeTimeout)
	defer cancel()

	cdpOK, _ := r.cdpChecker.Check(probeCtx)
	r.cdpHealthy.Store(cdpOK)

	extOK, _ := r.extChecker.Check(probeCtx)
	r.extHealthy.Store(extOK)

	// Metrics
	if r.onHealthCheck != nil {
		if cdpOK {
			r.onHealthCheck("cdp", true)
		} else {
			r.onHealthCheck("cdp", false)
		}
		if extOK {
			r.onHealthCheck("extension", true)
		} else {
			r.onHealthCheck("extension", false)
		}
	}

	// Determine best mode
	current := r.currentMode.Load().(ScreenshotMode)
	best := r.determineBestMode(current, cdpOK, extOK)
	if best != current {
		r.currentMode.Store(best)
		if r.onModeSwitch != nil {
			r.onModeSwitch(current, best)
		}
	}
}

func (r *ScreenshotRouter) determineBestMode(current ScreenshotMode, cdpOK, extOK bool) ScreenshotMode {
	switch current {
	case ModeCDP:
		if cdpOK {
			return ModeCDP
		}
		if r.cfg.Fallback && extOK {
			return ModeExtension
		}
		return ModeCDP
	case ModeExtension:
		if extOK {
			return ModeExtension
		}
		if r.cfg.Fallback && cdpOK {
			return ModeCDP
		}
		return ModeExtension
	}
	return current
}

// ActiveMode returns the current active screenshot mode.
func (r *ScreenshotRouter) ActiveMode() ScreenshotMode {
	return r.currentMode.Load().(ScreenshotMode)
}

// HealthStatus returns the health status of both modes.
func (r *ScreenshotRouter) HealthStatus() (cdpHealthy, extHealthy bool) {
	return r.cdpHealthy.Load(), r.extHealthy.Load()
}

// Config returns the router configuration.
func (r *ScreenshotRouter) Config() RouterConfig {
	return r.cfg
}

// CaptureSearchEngineResult captures a search engine result using the active mode.
func (r *ScreenshotRouter) CaptureSearchEngineResult(ctx context.Context, engine, query, queryID string) (string, error) {
	provider, err := r.resolveProvider(ModeCDP)
	if err != nil {
		return "", err
	}
	return provider.CaptureSearchEngineResult(ctx, engine, query, queryID)
}

// CaptureTargetWebsite captures a target website using the active mode.
func (r *ScreenshotRouter) CaptureTargetWebsite(ctx context.Context, targetURL, ip, port, protocol, queryID string) (string, error) {
	provider, err := r.resolveProvider(ModeCDP)
	if err != nil {
		return "", err
	}
	return provider.CaptureTargetWebsite(ctx, targetURL, ip, port, protocol, queryID)
}

// CaptureBatchURLs captures a batch of URLs using the active mode.
func (r *ScreenshotRouter) CaptureBatchURLs(ctx context.Context, urls []string, batchID string, concurrency int) ([]BatchScreenshotResult, error) {
	provider, err := r.resolveProvider(ModeCDP)
	if err != nil {
		return nil, err
	}
	return provider.CaptureBatchURLs(ctx, urls, batchID, concurrency)
}

// GetScreenshotDirectory returns the screenshot base directory.
func (r *ScreenshotRouter) GetScreenshotDirectory() string {
	if r.mgr != nil {
		return r.mgr.GetScreenshotDirectory()
	}
	return ""
}

// resolveProvider returns the best available Provider based on current health and fallback config.
func (r *ScreenshotRouter) resolveProvider(primaryMode ScreenshotMode) (Provider, error) {
	mode := r.determineBestMode(primaryMode, r.cdpHealthy.Load(), r.extHealthy.Load())

	// Try the determined mode first
	if provider := r.providerForMode(mode); provider != nil {
		return provider, nil
	}

	// Fallback to the other mode
	other := ModeCDP
	if mode == ModeCDP {
		other = ModeExtension
	}
	if r.cfg.Fallback {
		if provider := r.providerForMode(other); provider != nil {
			return provider, nil
		}
	}

	return nil, fmt.Errorf("no screenshot provider available (cdp=%v, extension=%v, mode=%s, fallback=%v)",
		r.cdp != nil, r.extBridge != nil, mode, r.cfg.Fallback)
}

// providerForMode returns the Provider for the given mode, or nil if unavailable.
func (r *ScreenshotRouter) providerForMode(mode ScreenshotMode) Provider {
	switch mode {
	case ModeCDP:
		return r.cdp
	case ModeExtension:
		if r.extBridge == nil {
			return nil
		}
		return NewExtensionProvider(r.extBridge, r.mgr)
	default:
		return nil
	}
}

// ExtensionProvider implements Provider using the Extension Bridge.
type ExtensionProvider struct {
	bridge *BridgeService
	mgr    *Manager
}

// NewExtensionProvider creates a Provider that routes through the Extension Bridge.
func NewExtensionProvider(bridge *BridgeService, mgr *Manager) *ExtensionProvider {
	return &ExtensionProvider{bridge: bridge, mgr: mgr}
}

func (p *ExtensionProvider) CaptureSearchEngineResult(ctx context.Context, engine, query, queryID string) (string, error) {
	if p == nil || p.bridge == nil {
		return "", fmt.Errorf("extension provider not initialized")
	}
	// Use the app's bridge capture method
	searchURL := ""
	if p.mgr != nil {
		searchURL = strings.TrimSpace(p.mgr.BuildSearchEngineURL(engine, query))
	}
	if searchURL == "" {
		searchURL = buildSearchEngineURL(engine, query)
	}
	if searchURL == "" {
		return "", fmt.Errorf("unsupported engine: %s", engine)
	}

	task := BridgeTask{
		RequestID:    fmt.Sprintf("router_search_%d", time.Now().UnixNano()),
		URL:          searchURL,
		BatchID:      queryID,
		WaitStrategy: "load",
	}
	result, err := p.bridge.Submit(ctx, task)
	if err != nil {
		return "", fmt.Errorf("extension bridge capture failed: %w", err)
	}
	if !result.Success {
		errMsg := strings.TrimSpace(result.Error)
		if errMsg == "" {
			errMsg = strings.TrimSpace(result.ErrorCode)
		}
		if errMsg == "" {
			errMsg = "unknown bridge error"
		}
		return "", fmt.Errorf("extension bridge capture failed: %s", errMsg)
	}
	if strings.TrimSpace(result.ImagePath) == "" {
		return "", fmt.Errorf("extension bridge capture missing image path")
	}
	return strings.TrimSpace(result.ImagePath), nil
}

func (p *ExtensionProvider) CaptureTargetWebsite(ctx context.Context, targetURL, ip, port, protocol, queryID string) (string, error) {
	if p == nil || p.bridge == nil {
		return "", fmt.Errorf("extension provider not initialized")
	}
	resolvedURL, err := p.buildTargetURL(targetURL, ip, port, protocol)
	if err != nil {
		return "", err
	}

	task := BridgeTask{
		RequestID:    fmt.Sprintf("router_target_%d", time.Now().UnixNano()),
		URL:          resolvedURL,
		BatchID:      queryID,
		WaitStrategy: "load",
	}
	result, err := p.bridge.Submit(ctx, task)
	if err != nil {
		return "", fmt.Errorf("extension bridge capture failed: %w", err)
	}
	if !result.Success {
		errMsg := strings.TrimSpace(result.Error)
		if errMsg == "" {
			errMsg = strings.TrimSpace(result.ErrorCode)
		}
		if errMsg == "" {
			errMsg = "unknown bridge error"
		}
		return "", fmt.Errorf("extension bridge capture failed: %s", errMsg)
	}
	if strings.TrimSpace(result.ImagePath) == "" {
		return "", fmt.Errorf("extension bridge capture missing image path")
	}
	return strings.TrimSpace(result.ImagePath), nil
}

func (p *ExtensionProvider) CaptureBatchURLs(ctx context.Context, urls []string, batchID string, concurrency int) ([]BatchScreenshotResult, error) {
	if p == nil || p.bridge == nil {
		return nil, fmt.Errorf("extension provider not initialized")
	}

	results := make([]BatchScreenshotResult, len(urls))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, rawURL := range urls {
		wg.Add(1)
		go func(idx int, inputURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			normalizedURL := normalizeURL(inputURL)
			result := BatchScreenshotResult{URL: inputURL, Timestamp: time.Now().Unix()}
			if normalizedURL == "" {
				result.Success = false
				result.Error = "invalid URL"
				results[idx] = result
				return
			}

			task := BridgeTask{
				RequestID:    fmt.Sprintf("router_batch_%d_%d", time.Now().UnixNano(), idx),
				URL:          normalizedURL,
				BatchID:      batchID,
				WaitStrategy: "load",
			}
			bridgeResult, err := p.bridge.Submit(ctx, task)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				results[idx] = result
				return
			}

			result.Success = bridgeResult.Success
			result.FilePath = bridgeResult.ImagePath
			if !bridgeResult.Success {
				if strings.TrimSpace(bridgeResult.Error) != "" {
					result.Error = bridgeResult.Error
				} else {
					result.Error = bridgeResult.ErrorCode
				}
			}
			results[idx] = result
		}(i, rawURL)
	}

	wg.Wait()
	return results, nil
}

func (p *ExtensionProvider) GetScreenshotDirectory() string {
	if p.mgr != nil {
		return p.mgr.GetScreenshotDirectory()
	}
	return ""
}

// buildSearchEngineURL builds a search engine result URL for bridge capture.
func buildSearchEngineURL(engine, query string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "fofa":
		return fmt.Sprintf("https://fofa.info/result?qbase64=%s", urlBase64(query))
	case "hunter":
		return fmt.Sprintf("https://hunter.qianxin.com/list?searchValue=%s", urlBase64(query))
	case "quake":
		return fmt.Sprintf("https://quake.360.cn/quake/#/searchResult?searchVal=%s", url.QueryEscape(query))
	case "zoomeye":
		return fmt.Sprintf("https://www.zoomeye.org/searchResult?q=%s", url.QueryEscape(query))
	default:
		return ""
	}
}

// buildTargetURL builds a target website URL for bridge capture.
func (p *ExtensionProvider) buildTargetURL(targetURL, ip, port, protocol string) (string, error) {
	resolvedURL := strings.TrimSpace(targetURL)
	if resolvedURL == "" {
		resolvedIP := strings.TrimSpace(ip)
		if resolvedIP == "" {
			return "", fmt.Errorf("target URL or IP is required")
		}
		proto := "http"
		if p := strings.TrimSpace(protocol); p != "" {
			proto = strings.ToLower(p)
		} else if port == "443" {
			proto = "https"
		}
		resolvedPort := strings.TrimSpace(port)
		if resolvedPort != "" && resolvedPort != "80" && resolvedPort != "443" {
			resolvedURL = fmt.Sprintf("%s://%s:%s", proto, resolvedIP, resolvedPort)
		} else {
			resolvedURL = fmt.Sprintf("%s://%s", proto, resolvedIP)
		}
	}
	if !strings.HasPrefix(resolvedURL, "http://") && !strings.HasPrefix(resolvedURL, "https://") {
		resolvedURL = "http://" + resolvedURL
	}
	return resolvedURL, nil
}

// normalizeURL ensures a URL has a scheme prefix.
func normalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		trimmed = "http://" + trimmed
	}
	return trimmed
}

// isMockBridgeClient detects if the BridgeService wraps a mock client.
// Since we cannot inspect the wrapped client type from the BridgeService,
// this is handled at creation time via the ExtensionHealthChecker.
func isMockBridgeClient(svc *BridgeService) bool {
	return false
}

// urlBase64 encodes a string as URL-safe base64.
func urlBase64(s string) string {
	return url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(s)))
}
