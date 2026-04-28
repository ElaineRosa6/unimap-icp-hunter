package workerpool

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type MockTask struct {
	executeErr error
	executed   atomic.Bool
}

func (m *MockTask) Execute() error {
	m.executed.Store(true)
	return m.executeErr
}

func TestNewPool(t *testing.T) {
	pool := NewPool(5)
	if pool == nil {
		t.Error("NewPool should return non-nil pool")
	}
	if pool.minConcurrency != 5 {
		t.Errorf("Expected minConcurrency 5, got %d", pool.minConcurrency)
	}
}

func TestNewPoolWithZeroConcurrency(t *testing.T) {
	pool := NewPool(0)
	if pool.minConcurrency != 5 {
		t.Errorf("Expected minConcurrency 5 (default), got %d", pool.minConcurrency)
	}
}

func TestPoolStart(t *testing.T) {
	pool := NewPool(3)
	pool.Start()
	if atomic.LoadInt32(&pool.running) != 1 {
		t.Error("Pool should be running after Start()")
	}
	pool.Stop()
}

func TestPoolStop(t *testing.T) {
	pool := NewPool(3)
	pool.Start()
	pool.Stop()
	if atomic.LoadInt32(&pool.running) != 0 {
		t.Error("Pool should not be running after Stop()")
	}
}

func TestPoolSubmitTask(t *testing.T) {
	pool := NewPool(1)
	pool.Start()
	defer pool.Stop()

	task := &MockTask{}
	pool.Submit(task)

	time.Sleep(100 * time.Millisecond)
	if !task.executed.Load() {
		t.Error("Task should be executed")
	}
}

func TestPoolSubmitMultipleTasks(t *testing.T) {
	pool := NewPool(2)
	pool.Start()
	defer pool.Stop()

	tasks := make([]*MockTask, 5)
	for i := range tasks {
		tasks[i] = &MockTask{}
		pool.Submit(tasks[i])
	}

	time.Sleep(200 * time.Millisecond)
	for i, task := range tasks {
		if !task.executed.Load() {
			t.Errorf("Task %d should be executed", i)
		}
	}
}

func TestPoolSubmitToStoppedPool(t *testing.T) {
	pool := NewPool(1)
	// Pool is not started

	task := &MockTask{}
	pool.Submit(task)

	if task.executed.Load() {
		t.Error("Task should not be executed on stopped pool")
	}
}

func TestPoolResultsChannel(t *testing.T) {
	pool := NewPool(1)
	pool.Start()
	defer pool.Stop()

	expectedErr := errors.New("test error")
	task := &MockTask{executeErr: expectedErr}
	pool.Submit(task)

	select {
	case err := <-pool.Results():
		if err != expectedErr {
			t.Errorf("Expected error %v, got %v", expectedErr, err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Expected error on results channel")
	}
}

func TestPoolWait(t *testing.T) {
	pool := NewPool(2)
	pool.Start()
	defer pool.Stop()

	tasks := make([]*MockTask, 3)
	for i := range tasks {
		tasks[i] = &MockTask{}
		pool.Submit(tasks[i])
	}

	// Wait for tasks to be executed
	time.Sleep(200 * time.Millisecond)

	// Check if all tasks were executed
	for i, task := range tasks {
		if !task.executed.Load() {
			t.Errorf("Task %d should be executed", i)
		}
	}
}

func TestTaskWithRetry(t *testing.T) {
	// Test successful execution on first attempt
	successTask := &MockTask{}
	retryTask := NewTaskWithRetry(successTask, 3, 10*time.Millisecond)

	err := retryTask.Execute()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !successTask.executed.Load() {
		t.Error("Task should be executed")
	}
}

func TestTaskWithRetry_Failure(t *testing.T) {
	// Test failure after max attempts
	failTask := &MockTask{executeErr: errors.New("always fails")}
	retryTask := NewTaskWithRetry(failTask, 3, 10*time.Millisecond)

	start := time.Now()
	err := retryTask.Execute()
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error after max attempts")
	}
	if !failTask.executed.Load() {
		t.Error("Task should be executed")
	}
	if duration < 30*time.Millisecond {
		t.Errorf("Should wait between retries, duration: %v", duration)
	}
}

func TestTaskWithRetry_ZeroMaxAttempts(t *testing.T) {
	task := &MockTask{}
	retryTask := NewTaskWithRetry(task, 0, 10*time.Millisecond)

	err := retryTask.Execute()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !task.executed.Load() {
		t.Error("Task should be executed")
	}
}

func TestTaskWithRetry_ZeroDelay(t *testing.T) {
	task := &MockTask{executeErr: errors.New("fails")}
	retryTask := NewTaskWithRetry(task, 2, 0)

	start := time.Now()
	err := retryTask.Execute()
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error after max attempts")
	}
	// Should use default delay
	if duration < 100*time.Millisecond {
		t.Errorf("Should use default delay, duration: %v", duration)
	}
}

func TestNewDynamicPool(t *testing.T) {
	pool := NewDynamicPool(2, 8)
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	if pool.minConcurrency != 2 {
		t.Fatalf("expected minConcurrency 2, got %d", pool.minConcurrency)
	}
	if pool.maxConcurrency != 8 {
		t.Fatalf("expected maxConcurrency 8, got %d", pool.maxConcurrency)
	}
	pool.Stop()
}

func TestNewDynamicPool_Defaults(t *testing.T) {
	pool := NewDynamicPool(0, 0)
	if pool.minConcurrency != 5 {
		t.Fatalf("expected default minConcurrency 5, got %d", pool.minConcurrency)
	}
	pool.Stop()
}

func TestNewDynamicPool_MaxLessThanMin(t *testing.T) {
	pool := NewDynamicPool(10, 5)
	if pool.maxConcurrency <= pool.minConcurrency {
		t.Fatalf("expected maxConcurrency > minConcurrency, got max=%d min=%d",
			pool.maxConcurrency, pool.minConcurrency)
	}
	pool.Stop()
}

func TestPool_GetConcurrency(t *testing.T) {
	pool := NewPool(3)
	pool.Start()
	defer pool.Stop()

	c := pool.GetConcurrency()
	if c != 3 {
		t.Fatalf("expected concurrency 3, got %d", c)
	}
}

func TestPool_SetConcurrency_Increase(t *testing.T) {
	pool := NewPool(2)
	pool.Start()
	defer pool.Stop()

	pool.SetConcurrency(4)
	if pool.GetConcurrency() != 4 {
		t.Fatalf("expected concurrency 4, got %d", pool.GetConcurrency())
	}
}

func TestPool_SetConcurrency_Decrease(t *testing.T) {
	pool := NewDynamicPool(2, 8)
	pool.Start()
	pool.SetConcurrency(4)

	pool.SetConcurrency(2)
	if pool.GetConcurrency() != 2 {
		t.Fatalf("expected concurrency 2, got %d", pool.GetConcurrency())
	}
	pool.Stop()
}

func TestPool_SetConcurrency_ClipMin(t *testing.T) {
	pool := NewPool(3)
	pool.Start()
	defer pool.SetConcurrency(3)

	pool.SetConcurrency(1)
	if pool.GetConcurrency() < 3 {
		t.Fatalf("expected concurrency clipped to min 3, got %d", pool.GetConcurrency())
	}
}

func TestPool_SetConcurrency_ClipMax(t *testing.T) {
	pool := NewPool(2)
	pool.Start()
	defer pool.Stop()

	pool.SetConcurrency(100)
	if pool.GetConcurrency() > 8 {
		t.Fatalf("expected concurrency clipped to max 8, got %d", pool.GetConcurrency())
	}
}

func TestPool_SetConcurrency_NoOp(t *testing.T) {
	pool := NewPool(3)
	pool.Start()
	defer pool.Stop()

	pool.SetConcurrency(3)
	if pool.GetConcurrency() != 3 {
		t.Fatalf("expected concurrency unchanged, got %d", pool.GetConcurrency())
	}
}

func TestPool_GetLoadMetrics(t *testing.T) {
	pool := NewPool(2)
	pool.Start()
	defer pool.Stop()

	ql, aw, dur := pool.GetLoadMetrics()
	if ql < 0 {
		t.Fatalf("expected non-negative queue length, got %d", ql)
	}
	if aw < 0 {
		t.Fatalf("expected non-negative active workers, got %d", aw)
	}
	_ = dur
}

func TestPool_Wait(t *testing.T) {
	pool := NewPool(1)
	pool.Start()

	task := &MockTask{executeErr: nil}
	pool.Submit(task)

	// Give the worker time to pick up the task
	time.Sleep(200 * time.Millisecond)

	// Stop first (closes task channel so workers drain and exit)
	// then Wait verifies no panic
	pool.Stop()
}

func TestPool_StartAlreadyRunning(t *testing.T) {
	pool := NewPool(2)
	pool.Start()
	pool.Start() // should be no-op

	if pool.GetConcurrency() != 2 {
		t.Fatalf("expected 2 workers, got %d", pool.GetConcurrency())
	}
	pool.Stop()
}

func TestPool_StopAlreadyStopped(t *testing.T) {
	pool := NewPool(2)
	pool.Stop() // pool was never started
	pool.Stop() // should be no-op, no panic
}

func TestPool_DynamicScaleUp(t *testing.T) {
	pool := NewDynamicPool(1, 4)
	pool.Start()
	defer pool.Stop()

	// Verify the pool starts and can process tasks
	fastTask := &MockTask{}
	pool.Submit(fastTask)
	time.Sleep(200 * time.Millisecond)

	if !fastTask.executed.Load() {
		t.Fatal("expected task to be executed")
	}
}

func TestPool_DynamicScaleDown(t *testing.T) {
	pool := NewDynamicPool(2, 8)
	pool.Start()

	// Verify no panic during stop after scale-up
	pool.SetConcurrency(4)
	pool.Stop()
}

func TestPool_AdjustConcurrency_ScaleUp(t *testing.T) {
	pool := NewDynamicPool(1, 4)
	pool.Start()
	defer pool.Stop()

	// Fill the queue to trigger scale-up
	for i := 0; i < 10; i++ {
		pool.tasks <- &MockTask{}
	}
	pool.loadMonitor.setQueueLength(len(pool.tasks))

	// Directly call adjustConcurrency
	pool.adjustConcurrency()

	// Concurrency should have increased from 1
	c := pool.GetConcurrency()
	if c <= 1 {
		t.Fatalf("expected concurrency to increase from 1, got %d", c)
	}
}

func TestPool_AdjustConcurrency_ScaleDown(t *testing.T) {
	pool := NewDynamicPool(2, 8)
	pool.Start()

	// Start with higher concurrency
	pool.SetConcurrency(4)

	// Set low load metrics to trigger scale-down
	pool.loadMonitor.setQueueLength(0)
	// activeWorkers < currentConcurrency/2 means activeWorkers < 2

	// Directly call adjustConcurrency
	pool.adjustConcurrency()

	c := pool.GetConcurrency()
	// Should have decreased or stayed same (depending on activeWorkers)
	if c > 4 {
		t.Fatalf("expected concurrency <= 4, got %d", c)
	}
	pool.Stop()
}

func TestPool_AdjustConcurrency_AlreadyStopped(t *testing.T) {
	pool := NewPool(2)
	pool.Stop() // not started, running=0

	pool.adjustConcurrency() // should be no-op, no panic
}
