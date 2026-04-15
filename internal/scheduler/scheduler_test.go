package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewScheduler(t *testing.T) {
	s := NewScheduler("", "", 100)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	defer s.Stop()

	if len(s.tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(s.tasks))
	}
	if s.maxHistory != 100 {
		t.Errorf("expected maxHistory=100, got %d", s.maxHistory)
	}
}

func TestAddTaskInvalidCron(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	// Register a dummy handler so task type validation passes
	s.RegisterHandler(&testHandler{typ: TaskQuery})

	err := s.AddTask(&ScheduledTask{
		Name:     "bad cron",
		Type:     TaskQuery,
		CronExpr: "invalid",
		Enabled:  true,
	})
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestAddTaskUnknownType(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	err := s.AddTask(&ScheduledTask{
		Name:     "bad type",
		Type:     TaskType("unknown_type"),
		CronExpr: "0 0 * * * *",
		Enabled:  true,
	})
	if err == nil {
		t.Fatal("expected error for unknown task type")
	}
}

func TestAddAndGetTask(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	s.RegisterHandler(&testHandler{typ: TaskQuery})

	task := &ScheduledTask{
		Name:       "test query",
		Type:       TaskQuery,
		Enabled:    true,
		CronExpr:   "30 * * * *", // every hour at :30
		Payload:    map[string]interface{}{"query": "test"},
		TimeoutSec: 60,
		MaxRetries: 1,
	}

	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if task.ID == "" {
		t.Fatal("expected task ID to be generated")
	}

	got, err := s.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got.Name != "test query" {
		t.Errorf("expected name=test query, got %s", got.Name)
	}
	if got.Type != TaskQuery {
		t.Errorf("expected type=query, got %s", got.Type)
	}
	if !got.Enabled {
		t.Error("expected task to be enabled")
	}
}

func TestListTasks(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	s.RegisterHandler(&testHandler{typ: TaskQuery})

	s.AddTask(&ScheduledTask{Name: "list-a", Type: TaskQuery, Enabled: true, CronExpr: "*/5 * * * *"})
	time.Sleep(50 * time.Millisecond)
	s.AddTask(&ScheduledTask{Name: "list-b", Type: TaskQuery, Enabled: true, CronExpr: "*/10 * * * *"})
	time.Sleep(50 * time.Millisecond)

	tasks := s.ListTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestDeleteTask(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	s.RegisterHandler(&testHandler{typ: TaskQuery})

	task := &ScheduledTask{Name: "del", Type: TaskQuery, Enabled: true, CronExpr: "*/5 * * * *"}
	s.AddTask(task)

	if err := s.DeleteTask(task.ID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	if _, err := s.GetTask(task.ID); err == nil {
		t.Fatal("expected task to be deleted")
	}
}

func TestEnableDisableTask(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	s.RegisterHandler(&testHandler{typ: TaskQuery})

	task := &ScheduledTask{Name: "toggle", Type: TaskQuery, Enabled: true, CronExpr: "*/5 * * * *"}
	s.AddTask(task)

	if err := s.DisableTask(task.ID); err != nil {
		t.Fatalf("DisableTask failed: %v", err)
	}
	got, _ := s.GetTask(task.ID)
	if got.Enabled {
		t.Error("expected task to be disabled")
	}

	if err := s.EnableTask(task.ID); err != nil {
		t.Fatalf("EnableTask failed: %v", err)
	}
	got, _ = s.GetTask(task.ID)
	if !got.Enabled {
		t.Error("expected task to be enabled")
	}
}

func TestRunTaskNow(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	h := &testHandler{typ: TaskQuery}
	s.RegisterHandler(h)

	task := &ScheduledTask{Name: "run now", Type: TaskQuery, Enabled: false, CronExpr: "*/5 * * * *"}
	s.AddTask(task)

	if err := s.RunTaskNow(task.ID); err != nil {
		t.Fatalf("RunTaskNow failed: %v", err)
	}

	// Give the goroutine time to execute
	time.Sleep(200 * time.Millisecond)

	if h.execCount.Load() < 1 {
		t.Errorf("expected handler to be executed at least once, got %d", h.execCount.Load())
	}
}

func TestGetHistory(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	h := &testHandler{typ: TaskQuery}
	s.RegisterHandler(h)

	task := &ScheduledTask{
		Name:       "hist",
		Type:       TaskQuery,
		Enabled:    false,
		CronExpr:   "*/5 * * * *",
		TimeoutSec: 5,
	}
	s.AddTask(task)

	s.RunTaskNow(task.ID)
	time.Sleep(200 * time.Millisecond)

	history := s.GetHistory(10, "", "")
	if len(history) < 1 {
		t.Fatalf("expected at least 1 history record, got %d", len(history))
	}

	if history[0].Status != "success" {
		t.Errorf("expected status=success, got %s", history[0].Status)
	}
}

func TestGetHistoryFilter(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	s.RegisterHandler(&testHandler{typ: TaskQuery})
	s.RegisterHandler(&testHandler{typ: TaskTamperCheck})

	t1 := &ScheduledTask{Name: "q1", Type: TaskQuery, Enabled: false, CronExpr: "*/5 * * * *", TimeoutSec: 5}
	t2 := &ScheduledTask{Name: "t1", Type: TaskTamperCheck, Enabled: false, CronExpr: "*/5 * * * *", TimeoutSec: 5}
	s.AddTask(t1)
	s.AddTask(t2)

	s.RunTaskNow(t1.ID)
	s.RunTaskNow(t2.ID)
	time.Sleep(800 * time.Millisecond)

	// Debug: check all history
	allHistory := s.GetHistory(100, "", "")
	t.Logf("Total history records: %d", len(allHistory))
	for _, r := range allHistory {
		t.Logf("Record: task_type=%s status=%s", r.TaskType, r.Status)
	}

	// Filter by task type
	qHistory := s.GetHistory(10, string(TaskQuery), "")
	if len(qHistory) < 1 {
		t.Errorf("expected query history records, got %d", len(qHistory))
	}

	tHistory := s.GetHistory(10, string(TaskTamperCheck), "")
	if len(tHistory) < 1 {
		t.Errorf("expected tamper history records, got %d", len(tHistory))
	}
}

func TestStorePersist(t *testing.T) {
	dir := t.TempDir()
	taskPath := filepath.Join(dir, "tasks.json")
	historyPath := filepath.Join(dir, "history.json")

	s1 := NewScheduler(taskPath, historyPath, 100)
	s1.RegisterHandler(&testHandler{typ: TaskQuery})

	task := &ScheduledTask{
		Name:       "persist",
		Type:       TaskQuery,
		Enabled:    true,
		CronExpr:   "0 0 * * *",
		TimeoutSec: 120,
	}
	s1.AddTask(task)
	s1.RunTaskNow(task.ID)
	time.Sleep(500 * time.Millisecond)
	// Force sync save (not async)
	if err := s1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	s1.Stop()

	// Create new scheduler and load
	s2 := NewScheduler(taskPath, historyPath, 100)
	s2.RegisterHandler(&testHandler{typ: TaskQuery})

	if err := s2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	tasks := s2.ListTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task after reload, got %d", len(tasks))
	}
	if tasks[0].Name != "persist" {
		t.Errorf("expected name=persist, got %s", tasks[0].Name)
	}

	history := s2.GetHistory(10, "", "")
	if len(history) < 1 {
		t.Fatalf("expected at least 1 history record after reload, got %d", len(history))
	}

	s2.Stop()
}

func TestStoreNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	s := NewScheduler(filepath.Join(dir, "nonexist_tasks.json"), filepath.Join(dir, "nonexist_history.json"), 100)
	defer s.Stop()

	if err := s.Load(); err != nil {
		t.Fatalf("Load should not fail on non-existent file: %v", err)
	}
}

func TestLoadRebuildsIDCounter(t *testing.T) {
	dir := t.TempDir()
	taskPath := filepath.Join(dir, "tasks.json")
	historyPath := filepath.Join(dir, "history.json")

	s1 := NewScheduler(taskPath, historyPath, 100)
	s1.RegisterHandler(&testHandler{typ: TaskQuery})

	persisted := &ScheduledTask{
		ID:         "task_9",
		Name:       "persisted-9",
		Type:       TaskQuery,
		Enabled:    false,
		CronExpr:   "*/5 * * * *",
		TimeoutSec: 30,
	}
	if err := s1.AddTask(persisted); err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if err := s1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	s1.Stop()

	s2 := NewScheduler(taskPath, historyPath, 100)
	s2.RegisterHandler(&testHandler{typ: TaskQuery})
	if err := s2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	newTask := &ScheduledTask{
		Name:       "new-task",
		Type:       TaskQuery,
		Enabled:    false,
		CronExpr:   "*/10 * * * *",
		TimeoutSec: 30,
	}
	if err := s2.AddTask(newTask); err != nil {
		t.Fatalf("AddTask after load failed: %v", err)
	}

	if newTask.ID != "task_10" {
		t.Fatalf("expected new task id task_10, got %s", newTask.ID)
	}
	s2.Stop()

	// On Windows, give the OS time to release file handles before t.TempDir cleanup
	time.Sleep(50 * time.Millisecond)
}

func TestMaxHistoryTrim(t *testing.T) {
	s := NewScheduler("", "", 3)
	defer s.Stop()

	s.RegisterHandler(&testHandler{typ: TaskQuery})

	for i := 0; i < 5; i++ {
		task := &ScheduledTask{
			Name:       "trim",
			Type:       TaskQuery,
			Enabled:    false,
			CronExpr:   "*/5 * * * *",
			TimeoutSec: 5,
		}
		s.AddTask(task)
		s.RunTaskNow(task.ID)
		time.Sleep(100 * time.Millisecond)
	}

	s.mu.RLock()
	histLen := len(s.history)
	s.mu.RUnlock()

	if histLen > 3 {
		t.Errorf("expected history <= 3, got %d", histLen)
	}
}

func TestTaskTypeLabels(t *testing.T) {
	for _, tt := range AllTaskTypes() {
		label := TaskTypeLabel(tt)
		if label == "" {
			t.Errorf("empty label for task type %s", tt)
		}
	}
	// Unknown type returns the raw string
	if TaskTypeLabel("foobar") != "foobar" {
		t.Error("expected unknown type to return raw string")
	}
}

func TestUpdateTask(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	s.RegisterHandler(&testHandler{typ: TaskQuery})

	task := &ScheduledTask{Name: "update me", Type: TaskQuery, Enabled: true, CronExpr: "*/5 * * * *"}
	s.AddTask(task)

	task.Name = "updated"
	task.CronExpr = "30 * * * *"
	if err := s.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	got, _ := s.GetTask(task.ID)
	if got.Name != "updated" {
		t.Errorf("expected name=updated, got %s", got.Name)
	}
}

func TestUpdateTaskNotFound(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	s.RegisterHandler(&testHandler{typ: TaskQuery})

	err := s.UpdateTask(&ScheduledTask{ID: "nonexistent", Name: "x", Type: TaskQuery, CronExpr: "*/5 * * * *"})
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestDeleteTaskNotFound(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	err := s.DeleteTask("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestRunTaskNowNotFound(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	err := s.RunTaskNow("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestEnableTaskNotFound(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	err := s.EnableTask("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisableTaskNotFound(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	err := s.DisableTask("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTimeoutExecution(t *testing.T) {
	s := NewScheduler("", "", 100)
	defer s.Stop()

	h := &testHandler{typ: TaskQuery, sleepFor: 5 * time.Second}
	s.RegisterHandler(h)

	task := &ScheduledTask{
		Name:       "timeout",
		Type:       TaskQuery,
		Enabled:    false,
		CronExpr:   "*/5 * * * *",
		TimeoutSec: 1, // 1 second timeout
	}
	s.AddTask(task)

	s.RunTaskNow(task.ID)
	time.Sleep(2 * time.Second)

	history := s.GetHistory(10, "", "")
	if len(history) < 1 {
		t.Fatal("expected history record")
	}
	if history[0].Status != "timeout" {
		t.Errorf("expected status=timeout, got %s", history[0].Status)
	}
}

// testHandler is a simple test handler that records execution count.
type testHandler struct {
	typ       TaskType
	execCount atomic.Int64
	sleepFor  time.Duration
}

func (h *testHandler) Type() TaskType { return h.typ }

func (h *testHandler) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	h.execCount.Add(1)
	if h.sleepFor > 0 {
		select {
		case <-time.After(h.sleepFor):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return "test ok", nil
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
