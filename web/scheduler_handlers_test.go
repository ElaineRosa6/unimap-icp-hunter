package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/scheduler"
)

// setupScheduler creates a scheduler with the "query" handler registered for tests
func setupScheduler(t *testing.T) *scheduler.Scheduler {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "scheduler-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})
	sched := scheduler.NewScheduler(tmpDir+"/tasks.json", tmpDir+"/history.json", 500)
	sched.RegisterHandler(&mockQueryHandler{})
	return sched
}

// mockQueryHandler is a minimal handler for testing
type mockQueryHandler struct{}

func (h *mockQueryHandler) Type() scheduler.TaskType            { return scheduler.TaskQuery }
func (h *mockQueryHandler) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	return "mock query result", nil
}

func TestHandleCreateTask_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"name":      "test",
		"type":      "query",
		"cron_expr": "0 * * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleCreateTask(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleCreateTask_GetMethod_Returns405(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/tasks", nil)
	s.handleCreateTask(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleCreateTask_EmptyName_Returns400(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"name":      "",
		"type":      "query",
		"cron_expr": "0 * * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleCreateTask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCreateTask_EmptyCron_Returns400(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"name":      "test",
		"type":      "query",
		"cron_expr": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleCreateTask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCreateTask_Success(t *testing.T) {
	sched := setupScheduler(t)

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"name":      "test task",
		"type":      "query",
		"enabled":   true,
		"cron_expr": "0 * * * *",
		"payload":   map[string]interface{}{"query": "country=\"CN\""},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleCreateTask(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] != "task created" {
		t.Fatalf("expected 'task created', got %v", resp["message"])
	}
	if resp["id"] == "" {
		t.Fatal("expected non-empty task ID")
	}
}

func TestHandleListTasks_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/tasks", nil)
	s.handleListTasks(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleListTasks_Empty(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/tasks", nil)
	s.handleListTasks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleListTasks_AfterCreate(t *testing.T) {
	sched := setupScheduler(t)
	defer sched.Stop()

	// 先创建一个任务
	task := &scheduler.ScheduledTask{
		Name:     "list-test",
		Type:     "query",
		Enabled:  true,
		CronExpr: "0 * * * *",
	}
	sched.AddTask(task)

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/tasks", nil)
	s.handleListTasks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var tasks []map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&tasks)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0]["name"] != "list-test" {
		t.Fatalf("expected name 'list-test', got %v", tasks[0]["name"])
	}
}

func TestHandleUpdateTask_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"id":        "some-id",
		"name":      "updated",
		"cron_expr": "0 * * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleUpdateTask(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleUpdateTask_MissingID_Returns400(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"name":      "updated",
		"cron_expr": "0 * * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleUpdateTask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUpdateTask_NotFound_Returns404(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"id":        "nonexistent",
		"name":      "updated",
		"cron_expr": "0 * * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleUpdateTask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 4xx, got %d", rec.Code)
	}
}

func TestHandleRunTaskNow_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"id": "some-id"})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleRunTaskNow(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleRunTaskNow_MissingID_Returns400(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleRunTaskNow(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleRunTaskNow_NotFound_Returns404(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"id": "nonexistent"})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleRunTaskNow(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleRunTaskNow_Success(t *testing.T) {
	sched := setupScheduler(t)
	defer sched.Stop()

	// 先创建任务
	task := &scheduler.ScheduledTask{
		Name:     "run-now-test",
		Type:     "query",
		Enabled:  true,
		CronExpr: "0 * * * *",
	}
	sched.AddTask(task)

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"id": task.ID})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleRunTaskNow(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] != "task scheduled for immediate execution" {
		t.Fatalf("unexpected message: %v", resp["message"])
	}
}

func TestHandleDeleteTask_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"id": "some-id"})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleDeleteTask(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleTaskHistory_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/history", nil)
	s.handleTaskHistory(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleTaskHistory_Empty(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/history", nil)
	s.handleTaskHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleEnableTask_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"id": "some-id"})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/enable", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleEnableTask(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleDisableTask_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"id": "some-id"})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks/disable", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleDisableTask(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleSchedulerPage(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scheduler", nil)
	s.handleSchedulerPage(rec, req)

	// 模板可能不存在，但不应 panic
}

func TestHandleGetTask_NoScheduler_Returns503(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/tasks?id=1", nil)
	s.handleGetTask(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleGetTask_MissingID_Returns400(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/tasks", nil)
	s.handleGetTask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCreateTask_BadJSON_Returns400(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	s.handleCreateTask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWriteSchedulerJSONError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSchedulerJSONError(rec, http.StatusBadRequest, "test error")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "test error" {
		t.Fatalf("expected 'test error', got %v", resp["error"])
	}
}

func TestWriteJSON_SchedulerTest(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"key": "value"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleTaskHistory_WithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := tmpDir + "/tasks.json"
	historyPath := tmpDir + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/history?limit=10", nil)
	s.handleTaskHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// 验证 scheduler 初始化不泄漏文件描述符
func TestScheduler_CleanupOnClose(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := tmpDir + "/tasks.json"
	historyPath := tmpDir + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	sched.Stop()

	// 再次调用 Stop 不应 panic
	sched.Stop()
}

func TestCreateTask_InvalidCronExpression(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{
		"name":      "bad-cron",
		"type":      "query",
		"cron_expr": "not a valid cron",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleCreateTask(rec, req)

	// 无效 cron 应被拒绝
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cron, got %d", rec.Code)
	}
}

func TestHandleRunTaskNow_GetMethod_Returns405(t *testing.T) {
	storePath := t.TempDir() + "/tasks.json"
	historyPath := t.TempDir() + "/history.json"
	sched := scheduler.NewScheduler(storePath, historyPath, 500)
	defer sched.Stop()

	s := &Server{scheduler: sched}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/tasks/run", nil)
	s.handleRunTaskNow(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
