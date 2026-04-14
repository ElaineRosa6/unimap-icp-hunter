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
