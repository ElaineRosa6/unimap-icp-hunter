package screenshot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type bridgeJob struct {
	task   BridgeTask
	respCh chan bridgeJobResult
}

type bridgeJobResult struct {
	result BridgeResult
	err    error
}

// BridgeService provides a task queue + worker execution model for extension bridge calls.
type BridgeService struct {
	client         BridgeClient
	queue          chan bridgeJob
	maxConcurrency int
	retry          int
	taskTimeout    time.Duration
	inFlight       atomic.Int64

	started atomic.Bool
	stopCh  chan struct{}

	wg sync.WaitGroup
}

func NewBridgeService(client BridgeClient, maxConcurrency int, taskTimeout time.Duration) *BridgeService {
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}
	if taskTimeout <= 0 {
		taskTimeout = 30 * time.Second
	}

	return &BridgeService{
		client:         client,
		queue:          make(chan bridgeJob, maxConcurrency*8),
		maxConcurrency: maxConcurrency,
		retry:          1,
		taskTimeout:    taskTimeout,
		stopCh:         make(chan struct{}),
	}
}

// SetRetry updates retry attempts for retryable transport errors.
func (s *BridgeService) SetRetry(retry int) {
	if retry < 0 {
		retry = 0
	}
	s.retry = retry
}

// Start boots fixed workers.
func (s *BridgeService) Start(ctx context.Context) {
	if s == nil || s.client == nil {
		return
	}
	if !s.started.CompareAndSwap(false, true) {
		return
	}

	for i := 0; i < s.maxConcurrency; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}
}

// Stop gracefully stops workers.
func (s *BridgeService) Stop() {
	if s == nil {
		return
	}
	if !s.started.Load() {
		return
	}
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
	s.wg.Wait()
	s.started.Store(false)
}

// Submit enqueues one task and waits for worker result.
func (s *BridgeService) Submit(ctx context.Context, task BridgeTask) (BridgeResult, error) {
	if s == nil || s.client == nil {
		return BridgeResult{}, fmt.Errorf("%w: bridge client not configured", ErrBridgeInternalError)
	}
	if strings.TrimSpace(task.RequestID) == "" {
		return BridgeResult{}, fmt.Errorf("%w: empty request id", ErrBridgeSubmitFailed)
	}
	if strings.TrimSpace(task.URL) == "" {
		return BridgeResult{}, fmt.Errorf("%w: empty url", ErrBridgeSubmitFailed)
	}
	if !s.started.Load() {
		return BridgeResult{}, fmt.Errorf("%w: bridge service not started", ErrBridgeInternalError)
	}

	effectiveTimeout := task.Timeout
	if effectiveTimeout <= 0 {
		effectiveTimeout = s.taskTimeout
	}
	workerCtx, cancel := context.WithTimeout(ctx, effectiveTimeout)
	defer cancel()

	job := bridgeJob{task: task, respCh: make(chan bridgeJobResult, 1)}
	select {
	case s.queue <- job:
	case <-workerCtx.Done():
		if errors.Is(workerCtx.Err(), context.DeadlineExceeded) {
			return BridgeResult{}, fmt.Errorf("%w: enqueue timeout", ErrBridgeTimeout)
		}
		return BridgeResult{}, fmt.Errorf("%w: enqueue canceled", ErrBridgeTaskCanceled)
	}

	select {
	case out := <-job.respCh:
		return out.result, out.err
	case <-workerCtx.Done():
		if errors.Is(workerCtx.Err(), context.DeadlineExceeded) {
			return BridgeResult{}, fmt.Errorf("%w: task timeout", ErrBridgeTimeout)
		}
		return BridgeResult{}, fmt.Errorf("%w: task canceled", ErrBridgeTaskCanceled)
	}
}

func (s *BridgeService) worker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case job := <-s.queue:
			s.inFlight.Add(1)
			result, err := s.executeWithRetry(ctx, job.task)
			s.inFlight.Add(-1)
			job.respCh <- bridgeJobResult{result: result, err: err}
		}
	}
}

func (s *BridgeService) QueueLen() int {
	if s == nil {
		return 0
	}
	return len(s.queue)
}

func (s *BridgeService) WorkerCount() int {
	if s == nil {
		return 0
	}
	return s.maxConcurrency
}

func (s *BridgeService) InFlight() int {
	if s == nil {
		return 0
	}
	return int(s.inFlight.Load())
}

func (s *BridgeService) executeWithRetry(ctx context.Context, task BridgeTask) (BridgeResult, error) {
	attempts := s.retry + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		result, err := s.executeOnce(ctx, task)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isRetryableBridgeError(err) || i == attempts-1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("%w: unknown execution error", ErrBridgeInternalError)
	}
	return BridgeResult{RequestID: task.RequestID, Success: false, Error: lastErr.Error()}, lastErr
}

func (s *BridgeService) executeOnce(ctx context.Context, task BridgeTask) (BridgeResult, error) {
	if err := s.client.SubmitTask(ctx, task); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return BridgeResult{}, fmt.Errorf("%w: submit timeout", ErrBridgeTimeout)
		}
		if errors.Is(err, context.Canceled) {
			return BridgeResult{}, fmt.Errorf("%w: submit canceled", ErrBridgeTaskCanceled)
		}
		return BridgeResult{}, fmt.Errorf("%w: %v", ErrBridgeSubmitFailed, err)
	}

	result, err := s.client.AwaitResult(ctx, task.RequestID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return BridgeResult{}, fmt.Errorf("%w: await timeout", ErrBridgeTimeout)
		}
		if errors.Is(err, context.Canceled) {
			return BridgeResult{}, fmt.Errorf("%w: await canceled", ErrBridgeTaskCanceled)
		}
		return BridgeResult{}, fmt.Errorf("%w: %v", ErrBridgeInternalError, err)
	}

	if strings.TrimSpace(result.RequestID) == "" {
		result.RequestID = task.RequestID
	}
	return result, nil
}

func isRetryableBridgeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrBridgeTimeout) {
		return true
	}
	if errors.Is(err, ErrBridgeSubmitFailed) {
		return true
	}
	errText := strings.ToLower(err.Error())
	if strings.Contains(errText, "timeout") || strings.Contains(errText, "connection") {
		return true
	}
	return false
}
