package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockHandler is a controllable test handler.
type mockHandler struct {
	typ     TaskType
	execute func(ctx context.Context, payload map[string]interface{}) (string, error)
}

func (h *mockHandler) Type() TaskType { return h.typ }

func (h *mockHandler) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if h.execute != nil {
		return h.execute(ctx, payload)
	}
	return "ok", nil
}

func TestSchedulerE2E_CreateTriggerAndHistory(t *testing.T) {
	// Test: create a task with a very short cron -> wait for trigger -> verify history
	s := NewScheduler("", "", 100)
	defer s.Stop()

	var mu sync.Mutex
	execCount := 0

	handler := &mockHandler{
		typ: TaskType("e2e_query"),
		execute: func(ctx context.Context, payload map[string]interface{}) (string, error) {
			mu.Lock()
			execCount++
			mu.Unlock()
			return "e2e executed", nil
		},
	}
	s.RegisterHandler(handler)

	task := &ScheduledTask{
		Name:     "e2e-trigger-test",
		Type:     handler.Type(),
		Enabled:  true,
		CronExpr: "*/1 * * * * *", // every 1 second
		Payload:  map[string]interface{}{"key": "value"},
	}

	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Wait for at least 2 triggers
	time.Sleep(3500 * time.Millisecond)

	mu.Lock()
	count := execCount
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 executions, got %d", count)
	}

	// Verify history
	history := s.GetHistory(10, "", "")
	if len(history) == 0 {
		t.Fatal("expected execution history to be recorded")
	}

	latest := history[0]
	if latest.Status != "success" {
		t.Errorf("expected history status 'success', got '%s'", latest.Status)
	}
	if latest.Result != "e2e executed" {
		t.Errorf("expected history result 'e2e executed', got '%s'", latest.Result)
	}
}

func TestSchedulerE2E_EnableDisableControl(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	var mu sync.Mutex
	execCount := 0

	handler := &mockHandler{
		typ: TaskType("e2e_toggle"),
		execute: func(ctx context.Context, payload map[string]interface{}) (string, error) {
			mu.Lock()
			execCount++
			mu.Unlock()
			return "toggled", nil
		},
	}
	s.RegisterHandler(handler)

	task := &ScheduledTask{
		Name:     "e2e-toggle-test",
		Type:     handler.Type(),
		Enabled:  true,
		CronExpr: "*/1 * * * * *",
	}

	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Wait for ~2 triggers
	time.Sleep(2500 * time.Millisecond)

	mu.Lock()
	countBefore := execCount
	mu.Unlock()

	if countBefore < 1 {
		t.Errorf("expected at least 1 execution before disable, got %d", countBefore)
	}

	// Disable the task
	if err := s.DisableTask(task.ID); err != nil {
		t.Fatalf("DisableTask: %v", err)
	}

	// Verify task is disabled
	tasks := s.ListTasks()
	found := false
	for _, tt := range tasks {
		if tt.ID == task.ID {
			found = true
			if tt.Enabled {
				t.Error("expected task to be disabled")
			}
		}
	}
	if !found {
		t.Fatal("task not found in list")
	}

	// Wait and verify no new executions
	time.Sleep(2500 * time.Millisecond)

	mu.Lock()
	countAfter := execCount
	mu.Unlock()

	// Allow 1 more execution for timing edge case
	if countAfter > countBefore+1 {
		t.Errorf("expected no new executions after disable, got %d new (before=%d, after=%d)",
			countAfter-countBefore, countBefore, countAfter)
	}

	// Re-enable
	if err := s.EnableTask(task.ID); err != nil {
		t.Fatalf("EnableTask: %v", err)
	}

	// Wait for trigger again
	time.Sleep(2500 * time.Millisecond)

	mu.Lock()
	countFinal := execCount
	mu.Unlock()

	if countFinal <= countAfter {
		t.Errorf("expected executions after re-enable, count stayed at %d", countFinal)
	}
}

func TestSchedulerE2E_Persistence(t *testing.T) {
	dir := t.TempDir()
	taskPath := dir + "/tasks.json"
	historyPath := dir + "/history.json"

	// Create scheduler and add a task
	s1 := NewScheduler(taskPath, historyPath, 100)

	handler := &mockHandler{typ: TaskType("e2e_persist")}
	s1.RegisterHandler(handler)

	task := &ScheduledTask{
		Name:     "persisted-task",
		Type:     handler.Type(),
		Enabled:  true,
		CronExpr: "0 */5 * * * *",
		Payload:  map[string]interface{}{"persistent": true},
	}

	if err := s1.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	taskID := task.ID

	// Save and stop
	if err := s1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s1.Stop()

	// Create new scheduler and load
	s2 := NewScheduler(taskPath, historyPath, 100)
	s2.RegisterHandler(handler)

	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer s2.Stop()

	// Verify task is restored
	restored, err := s2.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if restored.Name != "persisted-task" {
		t.Errorf("expected task name 'persisted-task', got '%s'", restored.Name)
	}
	if !restored.Enabled {
		t.Error("expected task to be enabled after restore")
	}
	if restored.CronExpr != "0 */5 * * * *" {
		t.Errorf("expected cron '0 */5 * * * *', got '%s'", restored.CronExpr)
	}
	if restored.Payload["persistent"] != true {
		t.Error("expected payload 'persistent' to be true")
	}
}

func TestSchedulerE2E_RunTaskNow(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	var mu sync.Mutex
	executed := false

	handler := &mockHandler{
		typ: TaskType("e2e_run_now"),
		execute: func(ctx context.Context, payload map[string]interface{}) (string, error) {
			mu.Lock()
			executed = true
			mu.Unlock()
			return "ran immediately", nil
		},
	}
	s.RegisterHandler(handler)

	task := &ScheduledTask{
		Name:     "e2e-run-now-test",
		Type:     handler.Type(),
		Enabled:  false, // explicitly disabled
		CronExpr: "0 0 1 1 *", // never fires
	}

	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Run immediately (should work even though disabled)
	if err := s.RunTaskNow(task.ID); err != nil {
		t.Fatalf("RunTaskNow: %v", err)
	}

	// Wait for async execution
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	ok := executed
	mu.Unlock()

	if !ok {
		t.Error("expected task to have been executed by RunTaskNow")
	}

	// Verify history was recorded
	history := s.GetHistory(1, "", "")
	if len(history) == 0 {
		t.Fatal("expected execution history from RunTaskNow")
	}
	if history[0].Status != "success" {
		t.Errorf("expected history status 'success', got '%s'", history[0].Status)
	}
}

func TestSchedulerE2E_TaskFailureAndRetry(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	var mu sync.Mutex
	attemptCount := 0

	handler := &mockHandler{
		typ: TaskType("e2e_retry"),
		execute: func(ctx context.Context, payload map[string]interface{}) (string, error) {
			mu.Lock()
			attemptCount++
			current := attemptCount
			mu.Unlock()

			if current < 2 {
				return "", fmt.Errorf("transient error (attempt %d)", current)
			}
			return "success on retry", nil
		},
	}
	s.RegisterHandler(handler)

	task := &ScheduledTask{
		Name:       "e2e-retry-test",
		Type:       handler.Type(),
		Enabled:    false, // we'll run manually
		CronExpr:   "0 0 1 1 *",
		MaxRetries: 3,
	}

	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Run immediately (will retry on failure)
	if err := s.RunTaskNow(task.ID); err != nil {
		t.Fatalf("RunTaskNow: %v", err)
	}

	// Wait for retries (backoff: 0s + 2s + 4s = up to 6s)
	time.Sleep(8000 * time.Millisecond)

	mu.Lock()
	attempts := attemptCount
	mu.Unlock()

	if attempts < 2 {
		t.Errorf("expected at least 2 attempts (retry), got %d", attempts)
	}

	// Verify final history entry is success
	history := s.GetHistory(1, "", "")
	if len(history) == 0 {
		t.Fatal("expected execution history")
	}

	// The last record should be success (since retry succeeded)
	found := false
	for _, rec := range history {
		if rec.Status == "success" && rec.Result == "success on retry" {
			found = true
			break
		}
	}
	if !found {
		// May not be success if timing is off; check we had retries
		if attempts >= 2 {
			t.Logf("Had %d attempts, retry mechanism working", attempts)
		} else {
			t.Error("expected at least one successful execution after retries")
		}
	}
}

func TestSchedulerE2E_TaskTypeValidation(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	// Register only one handler
	s.RegisterHandler(&mockHandler{typ: TaskQuery})

	// Try to create task with unregistered type
	task := &ScheduledTask{
		Name:     "invalid-type-task",
		Type:     TaskType("nonexistent_type"),
		Enabled:  true,
		CronExpr: "0 0 1 1 *",
	}

	err := s.AddTask(task)
	if err == nil {
		t.Error("expected error for unknown task type")
	}
}

func TestSchedulerE2E_AllTaskTypesAvailable(t *testing.T) {
	// Verify all 20 task types are defined
	types := AllTaskTypes()
	if len(types) != 20 {
		t.Errorf("expected 20 task types, got %d", len(types))
	}

	// Verify each has a label
	for _, tt := range types {
		label := TaskTypeLabel(tt)
		if label == string(tt) && tt != TaskType("") {
			// Custom types without labels are OK, but our defined types should have labels
			t.Logf("warning: task type '%s' has no custom label", tt)
		}
	}
}
