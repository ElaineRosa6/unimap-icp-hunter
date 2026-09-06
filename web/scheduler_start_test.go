package web

import (
	"testing"

	"github.com/unimap/project/internal/config"
)

type schedulerStartProbe struct{ calls int }

func (p *schedulerStartProbe) Start() { p.calls++ }
func TestSchedulerStartupHonorsExplicitDisable(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Scheduler.Enabled = enabled
		p := &schedulerStartProbe{}
		startSchedulerIfEnabled(p, cfg)
		want := 0
		if enabled {
			want = 1
		}
		if p.calls != want {
			t.Fatalf("enabled=%v: start calls=%d want=%d", enabled, p.calls, want)
		}
	}
	p := &schedulerStartProbe{}
	startSchedulerIfEnabled(p, nil)
	if p.calls != 1 {
		t.Fatal("nil config compatibility changed")
	}
}
