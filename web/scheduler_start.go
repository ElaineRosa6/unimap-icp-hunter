package web

import "github.com/unimap/project/internal/config"

// The flag controls automatic cron at startup. Keep task management and manual
// execution available; changing this startup setting requires a restart.
func startSchedulerIfEnabled(sched interface{ Start() }, cfg *config.Config) {
	if cfg == nil || cfg.Scheduler.Enabled {
		sched.Start()
	}
}
