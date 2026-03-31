package distributed

import (
	"testing"
)

func TestTaskQueueEnqueueClaimResult(t *testing.T) {
	q := NewTaskQueue()

	_, err := q.Enqueue(TaskEnvelope{
		TaskID:       "task-1",
		TaskType:     "port_scan",
		Priority:     10,
		RequiredCaps: []string{"port_scan"},
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	rec, err := q.Claim("node-a", []string{"port_scan"})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec == nil || rec.TaskID != "task-1" {
		t.Fatalf("expected task-1 to be claimed, got %+v", rec)
	}
	if rec.Status != TaskStatusClaimed {
		t.Fatalf("expected status claimed, got %s", rec.Status)
	}

	updated, err := q.SubmitResult(TaskResult{TaskID: "task-1", NodeID: "node-a", Status: "completed"})
	if err != nil {
		t.Fatalf("submit result failed: %v", err)
	}
	if updated.Status != TaskStatusCompleted {
		t.Fatalf("expected completed, got %s", updated.Status)
	}
}

func TestTaskQueueCapabilityFilter(t *testing.T) {
	q := NewTaskQueue()
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-a", TaskType: "screenshot", RequiredCaps: []string{"screenshot"}})
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-b", TaskType: "port_scan", RequiredCaps: []string{"port_scan"}})

	rec, err := q.Claim("node-x", []string{"port_scan"})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec == nil || rec.TaskID != "task-b" {
		t.Fatalf("expected task-b, got %+v", rec)
	}
}
