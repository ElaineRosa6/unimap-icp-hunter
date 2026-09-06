package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
)

type disabledPolicyHandler struct{ ran chan struct{} }

func (disabledPolicyHandler) Type() TaskType { return TaskQuery }
func (h disabledPolicyHandler) Execute(context.Context, *model.TaskPayload) (string, error) {
	if h.ran != nil {
		h.ran <- struct{}{}
	}
	return "fixture", nil
}
func TestAutomaticDisabledDoesNotArmAnySchedule(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, kind := range []string{"cron", "once", "delay"} {
			s := NewSchedulerWithAutomaticEnabled("", "", 10, enabled)
			s.RegisterHandler(disabledPolicyHandler{})
			future := time.Now().Add(time.Hour)
			task := &ScheduledTask{ID: "fixture", Name: "fixture", Type: TaskQuery, Enabled: true, ScheduleType: kind, CronExpr: "0 0 * * * *", RunAt: &future, DelaySeconds: 3600}
			s.tasks[task.ID] = task
			if err := s.scheduleTask(task); err != nil {
				t.Fatal(err)
			}
			armed := task.timer != nil || len(s.cronIDs) > 0
			if armed != enabled {
				t.Fatalf("%s enabled=%v armed=%v", kind, enabled, armed)
			}
			if !enabled && task.RuntimeStatus != "scheduler_disabled" {
				t.Fatal(task.RuntimeStatus)
			}
			s.Stop()
		}
	}
}

func TestAutomaticDisabledStillAllowsManualRun(t *testing.T) {
	s := NewSchedulerWithAutomaticEnabled("", "", 10, false)
	defer s.Stop()
	ran := make(chan struct{}, 1)
	s.RegisterHandler(disabledPolicyHandler{ran: ran})
	s.tasks["manual"] = &ScheduledTask{ID: "manual", Type: TaskQuery, Enabled: true, TimeoutSec: 5}
	if err := s.RunTaskNow("manual"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("manual execution did not run")
	}
}
