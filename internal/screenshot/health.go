package screenshot

import (
	"context"
	"net/http"
	"time"
)

// HealthChecker probes a screenshot mode for liveness.
type HealthChecker interface {
	Check(ctx context.Context) (bool, error)
	Mode() string // "cdp" or "extension"
}

// CDPHealthChecker checks CDP mode health via remote debugger endpoint.
type CDPHealthChecker struct {
	RemoteDebugURL string
	// Override, if non-nil, overrides the actual health check result (for testing).
	Override *bool
}

func (c *CDPHealthChecker) Mode() string { return "cdp" }

func (c *CDPHealthChecker) Check(ctx context.Context) (bool, error) {
	if c.Override != nil {
		return *c.Override, nil
	}
	// No remote URL configured — assume local Chrome path will be spawned on demand
	if c.RemoteDebugURL == "" {
		return true, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.RemoteDebugURL+"/json/version", nil)
	if err != nil {
		return false, err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// ExtensionHealthChecker checks Extension Bridge mode health.
type ExtensionHealthChecker struct {
	BridgeService *BridgeService
	IsMock        bool
	// Override, if non-nil, overrides the actual health check result (for testing).
	Override *bool
}

func (e *ExtensionHealthChecker) Mode() string { return "extension" }

func (e *ExtensionHealthChecker) Check(ctx context.Context) (bool, error) {
	if e.Override != nil {
		return *e.Override, nil
	}
	if e.BridgeService == nil {
		return false, nil
	}
	if !e.BridgeService.IsStarted() {
		return false, nil
	}
	// Mock client runs in-process — cannot fail network checks
	if e.IsMock {
		return true, nil
	}
	// Overload check: if queue is excessively backed up, consider unhealthy
	queueLen := e.BridgeService.QueueLen()
	const maxQueueThreshold = 50
	if queueLen > maxQueueThreshold {
		return false, nil
	}
	return true, nil
}
