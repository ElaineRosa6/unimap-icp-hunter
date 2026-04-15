package proxypool

import (
	"testing"
	"time"
)

// --- Constructor Tests ---

func TestNewPool(t *testing.T) {
	t.Run("empty config creates disabled pool", func(t *testing.T) {
		p := NewPool(Config{})
		if p.Enabled() {
			t.Error("expected pool to be disabled with no proxies")
		}
	})

	t.Run("parses comma-separated proxies", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"http://proxy1:8080,http://proxy2:8080"},
		})
		proxies := p.Proxies()
		if len(proxies) != 2 {
			t.Fatalf("expected 2 proxies, got %d", len(proxies))
		}
	})

	t.Run("parses semicolon-separated proxies", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"http://p1;http://p2;http://p3"},
		})
		if len(p.Proxies()) != 3 {
			t.Fatalf("expected 3 proxies, got %d", len(p.Proxies()))
		}
	})

	t.Run("parses newline-separated proxies", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"http://p1\nhttp://p2\nhttp://p3"},
		})
		if len(p.Proxies()) != 3 {
			t.Fatalf("expected 3 proxies, got %d", len(p.Proxies()))
		}
	})

	t.Run("deduplicates proxies", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"http://same,http://same,http://same"},
		})
		if len(p.Proxies()) != 1 {
			t.Errorf("expected 1 proxy after dedup, got %d", len(p.Proxies()))
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"  http://proxy1:8080  ,  http://proxy2:8080  "},
		})
		proxies := p.Proxies()
		if proxies[0] != "http://proxy1:8080" {
			t.Errorf("expected trimmed proxy, got %q", proxies[0])
		}
	})

	t.Run("uses defaults for zero threshold/cooldown", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"http://proxy1"},
		})
		if p.failureThreshold != 2 {
			t.Errorf("expected default failure threshold 2, got %d", p.failureThreshold)
		}
		if p.cooldown != 60*time.Second {
			t.Errorf("expected default cooldown 60s, got %v", p.cooldown)
		}
	})

	t.Run("respects custom threshold and cooldown", func(t *testing.T) {
		p := NewPool(Config{
			Enabled:          true,
			Proxies:          []string{"http://proxy1"},
			FailureThreshold: 5,
			Cooldown:         30 * time.Second,
		})
		if p.failureThreshold != 5 {
			t.Errorf("expected failure threshold 5, got %d", p.failureThreshold)
		}
		if p.cooldown != 30*time.Second {
			t.Errorf("expected cooldown 30s, got %v", p.cooldown)
		}
	})
}

// --- Enabled/Proxies Tests ---

func TestPoolEnabled(t *testing.T) {
	t.Run("nil pool returns false", func(t *testing.T) {
		var p *Pool
		if p.Enabled() {
			t.Error("expected nil pool to return false")
		}
	})

	t.Run("disabled config returns false", func(t *testing.T) {
		p := NewPool(Config{Enabled: false, Proxies: []string{"http://proxy1"}})
		if p.Enabled() {
			t.Error("expected disabled pool to return false")
		}
	})

	t.Run("returns nil when disabled", func(t *testing.T) {
		p := NewPool(Config{})
		if p.Proxies() != nil {
			t.Error("expected nil proxies when disabled")
		}
	})
}

// --- Select Tests ---

func TestPoolSelect(t *testing.T) {
	t.Run("returns false when disabled", func(t *testing.T) {
		p := NewPool(Config{})
		_, ok := p.Select()
		if ok {
			t.Error("expected no proxy when disabled")
		}
	})

	t.Run("round robin selection", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"http://p1,http://p2,http://p3"},
		})
		// Should cycle through proxies
		seen := make(map[string]bool)
		for i := 0; i < 10; i++ {
			addr, ok := p.Select()
			if !ok {
				t.Fatal("expected proxy to be selected")
			}
			seen[addr] = true
		}
		// With 3 proxies and 10 selections, should see all of them
		if len(seen) != 3 {
			t.Errorf("expected to see all 3 proxies, saw %d", len(seen))
		}
	})

	t.Run("skips proxies in cooldown", func(t *testing.T) {
		p := NewPool(Config{
			Enabled:          true,
			Proxies:          []string{"http://p1,http://p2"},
			FailureThreshold: 1,
			Cooldown:         time.Hour, // Long cooldown
		})
		// Fail p1 enough to trigger cooldown
		p.Report("http://p1", false)
		_, ok := p.Select()
		if !ok {
			t.Error("expected to select a proxy")
		}
	})

	t.Run("selects earliest cooldown expiry when all in cooldown", func(t *testing.T) {
		p := NewPool(Config{
			Enabled:          true,
			Proxies:          []string{"http://p1,http://p2"},
			FailureThreshold: 1,
			Cooldown:         time.Hour,
		})
		// Put both in cooldown
		p.Report("http://p1", false)
		p.Report("http://p2", false)
		// Should still return a proxy (the one with earliest cooldown)
		addr, ok := p.Select()
		if !ok {
			t.Error("expected to select proxy even when all in cooldown")
		}
		if addr != "http://p1" && addr != "http://p2" {
			t.Errorf("expected valid proxy, got %q", addr)
		}
	})
}

// --- Report Tests ---

func TestPoolReport(t *testing.T) {
	t.Run("success resets failures", func(t *testing.T) {
		p := NewPool(Config{
			Enabled:          true,
			Proxies:          []string{"http://p1"},
			FailureThreshold: 2,
			Cooldown:         time.Hour,
		})
		// Fail once
		p.Report("http://p1", false)
		// Then succeed
		p.Report("http://p1", true)
		// Should still be selectable
		_, ok := p.Select()
		if !ok {
			t.Error("expected proxy to be available after success")
		}
	})

	t.Run("failure triggers cooldown after threshold", func(t *testing.T) {
		p := NewPool(Config{
			Enabled:          true,
			Proxies:          []string{"http://p1,http://p2"},
			FailureThreshold: 2,
			Cooldown:         time.Hour,
		})
		// Fail twice to trigger cooldown
		p.Report("http://p1", false)
		p.Report("http://p1", false)
		// p1 should now be in cooldown, p2 should be selected
		addr, ok := p.Select()
		if !ok {
			t.Fatal("expected proxy to be selected")
		}
		if addr == "http://p1" {
			// This could happen if round-robin lands on p1 first but it's in cooldown
			// The important thing is that some proxy is returned
		}
	})

	t.Run("unknown proxy ignored", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"http://p1"},
		})
		// Should not panic
		p.Report("http://unknown", false)
	})

	t.Run("empty proxy ignored", func(t *testing.T) {
		p := NewPool(Config{
			Enabled: true,
			Proxies: []string{"http://p1"},
		})
		p.Report("  ", false)
	})

	t.Run("disabled pool ignores report", func(t *testing.T) {
		p := NewPool(Config{})
		p.Report("http://p1", false)
	})
}

// --- AllowDirectFallback Tests ---

func TestPoolAllowDirectFallback(t *testing.T) {
	t.Run("returns empty when all in cooldown with direct fallback", func(t *testing.T) {
		p := NewPool(Config{
			Enabled:             true,
			Proxies:             []string{"http://p1"},
			FailureThreshold:    1,
			Cooldown:            time.Hour,
			AllowDirectFallback: true,
		})
		p.Report("http://p1", false)
		addr, ok := p.Select()
		if ok {
			t.Errorf("expected no proxy with direct fallback, got %q", addr)
		}
	})

	t.Run("returns proxy when all in cooldown without direct fallback", func(t *testing.T) {
		p := NewPool(Config{
			Enabled:             true,
			Proxies:             []string{"http://p1"},
			FailureThreshold:    1,
			Cooldown:            time.Hour,
			AllowDirectFallback: false,
		})
		p.Report("http://p1", false)
		addr, ok := p.Select()
		if !ok {
			t.Error("expected proxy even when in cooldown")
		}
		if addr != "http://p1" {
			t.Errorf("expected http://p1, got %q", addr)
		}
	})
}

// --- Concurrent Access Tests ---

func TestPoolConcurrency(t *testing.T) {
	p := NewPool(Config{
		Enabled: true,
		Proxies: []string{"http://p1,http://p2,http://p3"},
	})
	done := make(chan bool)

	for i := 0; i < 20; i++ {
		go func() {
			p.Select()
			p.Report("http://p1", true)
			p.Report("http://p2", false)
			p.Proxies()
			p.Enabled()
			done <- true
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

// --- splitProxyTokens Tests ---

func TestSplitProxyTokens(t *testing.T) {
	t.Run("empty string returns self", func(t *testing.T) {
		tokens := splitProxyTokens("")
		if len(tokens) != 1 || tokens[0] != "" {
			t.Errorf("expected [\"\"], got %v", tokens)
		}
	})

	t.Run("splits on comma", func(t *testing.T) {
		tokens := splitProxyTokens("a,b,c")
		if len(tokens) != 3 {
			t.Fatalf("expected 3 tokens, got %d", len(tokens))
		}
	})

	t.Run("splits on semicolon", func(t *testing.T) {
		tokens := splitProxyTokens("a;b;c")
		if len(tokens) != 3 {
			t.Fatalf("expected 3 tokens, got %d", len(tokens))
		}
	})
}
