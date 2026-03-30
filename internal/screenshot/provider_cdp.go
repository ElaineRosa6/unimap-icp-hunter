package screenshot

import (
	"context"
	"fmt"
)

// CDPProvider adapts Manager to the Provider interface.
type CDPProvider struct {
	mgr *Manager
}

func NewCDPProvider(mgr *Manager) *CDPProvider {
	return &CDPProvider{mgr: mgr}
}

func (p *CDPProvider) CaptureSearchEngineResult(ctx context.Context, engine, query, queryID string) (string, error) {
	if p == nil || p.mgr == nil {
		return "", fmt.Errorf("screenshot manager not initialized")
	}
	return p.mgr.CaptureSearchEngineResult(ctx, engine, query, queryID)
}

func (p *CDPProvider) CaptureTargetWebsite(ctx context.Context, targetURL, ip, port, protocol, queryID string) (string, error) {
	if p == nil || p.mgr == nil {
		return "", fmt.Errorf("screenshot manager not initialized")
	}
	return p.mgr.CaptureTargetWebsite(ctx, targetURL, ip, port, protocol, queryID)
}

func (p *CDPProvider) CaptureBatchURLs(ctx context.Context, urls []string, batchID string, concurrency int) ([]BatchScreenshotResult, error) {
	if p == nil || p.mgr == nil {
		return nil, fmt.Errorf("screenshot manager not initialized")
	}
	return p.mgr.CaptureBatchURLs(ctx, urls, batchID, concurrency)
}

func (p *CDPProvider) GetScreenshotDirectory() string {
	if p == nil || p.mgr == nil {
		return ""
	}
	return p.mgr.GetScreenshotDirectory()
}
