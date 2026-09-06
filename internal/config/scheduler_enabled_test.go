package config

import "testing"

func TestSchedulerEnabledConfiguration(t *testing.T) {
	m := NewManager("")
	for _, tc := range []struct {
		name, yaml string
		want       bool
	}{
		{"omitted", "{}", true},
		{"empty_section", "scheduler: {}", true},
		{"disabled", "scheduler:\n  enabled: false", false},
		{"enabled", "scheduler:\n  enabled: true", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := m.parseConfig([]byte(tc.yaml))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Scheduler.Enabled != tc.want {
				t.Fatalf("enabled=%v, want %v", cfg.Scheduler.Enabled, tc.want)
			}
			m.ApplyDefaults(cfg)
			if cfg.Scheduler.Enabled != tc.want {
				t.Fatal("defaults changed explicit switch")
			}
		})
	}
	cfg := &Config{}
	m.ApplyDefaults(cfg)
	if cfg.Scheduler.Enabled {
		t.Fatal("programmatic false was overwritten")
	}
}
