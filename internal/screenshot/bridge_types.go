package screenshot

import (
	"context"
	"errors"
	"time"
)

var (
	ErrBridgeSubmitFailed  = errors.New("bridge_submit_failed")
	ErrBridgeTimeout       = errors.New("bridge_timeout")
	ErrBridgeTaskCanceled  = errors.New("bridge_task_canceled")
	ErrBridgeInternalError = errors.New("bridge_internal_error")
)

// BridgeTask describes a screenshot job sent to the extension bridge.
type BridgeTask struct {
	RequestID      string        `json:"request_id"`
	URL            string        `json:"url"`
	BatchID        string        `json:"batch_id,omitempty"`
	ViewportWidth  int           `json:"viewport_width,omitempty"`
	ViewportHeight int           `json:"viewport_height,omitempty"`
	WaitStrategy   string        `json:"wait_strategy,omitempty"`
	Timeout        time.Duration `json:"timeout"`
}

// BridgeResult is the normalized extension execution result.
type BridgeResult struct {
	RequestID  string `json:"request_id"`
	Success    bool   `json:"success"`
	ImagePath  string `json:"image_path,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// BridgeClient abstracts transport between backend and browser extension.
type BridgeClient interface {
	SubmitTask(ctx context.Context, task BridgeTask) error
	AwaitResult(ctx context.Context, requestID string) (BridgeResult, error)
}
