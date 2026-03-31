package proxypool

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config defines runtime behavior for the proxy pool.
type Config struct {
	Enabled             bool
	Proxies             []string
	FailureThreshold    int
	Cooldown            time.Duration
	AllowDirectFallback bool
}

type proxyState struct {
	addr          string
	failures      int
	cooldownUntil time.Time
}

// Pool provides round-robin proxy selection with failure cooldown.
type Pool struct {
	enabled             bool
	allowDirectFallback bool
	failureThreshold    int
	cooldown            time.Duration
	states              []proxyState
	rr                  uint64
	mu                  sync.Mutex
}

func NewPool(cfg Config) *Pool {
	states := make([]proxyState, 0, len(cfg.Proxies))
	seen := make(map[string]struct{}, len(cfg.Proxies))
	for _, raw := range cfg.Proxies {
		for _, token := range splitProxyTokens(raw) {
			proxy := strings.TrimSpace(token)
			if proxy == "" {
				continue
			}
			if _, ok := seen[proxy]; ok {
				continue
			}
			seen[proxy] = struct{}{}
			states = append(states, proxyState{addr: proxy})
		}
	}

	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 2
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 60 * time.Second
	}

	return &Pool{
		enabled:             cfg.Enabled && len(states) > 0,
		allowDirectFallback: cfg.AllowDirectFallback,
		failureThreshold:    cfg.FailureThreshold,
		cooldown:            cfg.Cooldown,
		states:              states,
	}
}

func splitProxyTokens(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t'
	})
	if len(parts) == 0 {
		return []string{raw}
	}
	return parts
}

func (p *Pool) Enabled() bool {
	return p != nil && p.enabled && len(p.states) > 0
}

func (p *Pool) Proxies() []string {
	if !p.Enabled() {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.states))
	for _, state := range p.states {
		out = append(out, state.addr)
	}
	return out
}

// Select returns a proxy address and whether a proxy should be used.
func (p *Pool) Select() (string, bool) {
	if !p.Enabled() {
		return "", false
	}

	n := len(p.states)
	start := int(atomic.AddUint64(&p.rr, 1) % uint64(n))
	now := time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()

	for i := 0; i < n; i++ {
		idx := (start + i) % n
		state := &p.states[idx]
		if now.Before(state.cooldownUntil) {
			continue
		}
		return state.addr, true
	}

	if p.allowDirectFallback {
		return "", false
	}

	selected := 0
	for i := 1; i < n; i++ {
		if p.states[i].cooldownUntil.Before(p.states[selected].cooldownUntil) {
			selected = i
		}
	}
	return p.states[selected].addr, true
}

// Report records proxy execution outcome for cooldown management.
func (p *Pool) Report(proxy string, success bool) {
	if !p.Enabled() || strings.TrimSpace(proxy) == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.states {
		state := &p.states[i]
		if state.addr != proxy {
			continue
		}

		if success {
			state.failures = 0
			state.cooldownUntil = time.Time{}
			return
		}

		state.failures++
		if state.failures >= p.failureThreshold {
			state.failures = 0
			state.cooldownUntil = time.Now().Add(p.cooldown)
		}
		return
	}
}
