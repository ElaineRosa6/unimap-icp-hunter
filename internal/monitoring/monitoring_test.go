package monitoring

import (
	"testing"
	"time"
)

// --- LeakDetector Tests ---

func TestNewLeakDetector(t *testing.T) {
	t.Run("uses defaults for zero values", func(t *testing.T) {
		d := NewLeakDetector(0, 0)
		if d.monitorInterval != 30*time.Second {
			t.Errorf("expected default interval 30s, got %v", d.monitorInterval)
		}
		if d.maxLeakDuration != 5*time.Minute {
			t.Errorf("expected default max duration 5m, got %v", d.maxLeakDuration)
		}
	})

	t.Run("uses provided values", func(t *testing.T) {
		d := NewLeakDetector(time.Second, 2*time.Second)
		if d.monitorInterval != time.Second {
			t.Errorf("expected interval 1s, got %v", d.monitorInterval)
		}
		if d.maxLeakDuration != 2*time.Second {
			t.Errorf("expected max duration 2s, got %v", d.maxLeakDuration)
		}
	})

	t.Run("uses defaults for negative values", func(t *testing.T) {
		d := NewLeakDetector(-time.Second, -time.Minute)
		if d.monitorInterval != 30*time.Second {
			t.Errorf("expected default interval 30s, got %v", d.monitorInterval)
		}
		if d.maxLeakDuration != 5*time.Minute {
			t.Errorf("expected default max duration 5m, got %v", d.maxLeakDuration)
		}
	})
}

func TestAcquireAndRelease(t *testing.T) {
	t.Run("acquire and release within threshold", func(t *testing.T) {
		d := NewLeakDetector(time.Second, time.Minute)
		d.Acquire("http_conn", "conn-1")

		resources := d.GetActiveResources()
		if len(resources) != 1 {
			t.Fatalf("expected 1 active resource, got %d", len(resources))
		}
		if resources[0].ResourceID != "conn-1" {
			t.Errorf("expected resource ID 'conn-1', got %q", resources[0].ResourceID)
		}

		d.Release("conn-1")
		leaks := d.GetDetectedLeaks()
		if len(leaks) != 0 {
			t.Errorf("expected 0 leaks for quick release, got %d", len(leaks))
		}
	})

	t.Run("release records leak for long-held resource", func(t *testing.T) {
		d := NewLeakDetector(time.Second, 50*time.Millisecond)
		d.Acquire("db_conn", "db-1")

		// Wait longer than threshold
		time.Sleep(100 * time.Millisecond)

		d.Release("db-1")
		leaks := d.GetDetectedLeaks()
		if len(leaks) != 1 {
			t.Fatalf("expected 1 leak, got %d", len(leaks))
		}
		if leaks[0].ResourceType != "db_conn" {
			t.Errorf("expected type 'db_conn', got %q", leaks[0].ResourceType)
		}
		if leaks[0].ResourceID != "db-1" {
			t.Errorf("expected ID 'db-1', got %q", leaks[0].ResourceID)
		}
		if leaks[0].StackTrace == "" {
			t.Error("expected stack trace to be recorded")
		}
	})

	t.Run("release unknown resource is no-op", func(t *testing.T) {
		d := NewLeakDetector(time.Second, time.Minute)
		// Should not panic
		d.Release("nonexistent")
	})

	t.Run("acquire overwrites existing resource", func(t *testing.T) {
		d := NewLeakDetector(time.Second, time.Minute)
		d.Acquire("http_conn", "conn-1")
		d.Acquire("http_conn", "conn-1") // overwrite

		resources := d.GetActiveResources()
		if len(resources) != 1 {
			t.Errorf("expected 1 active resource after overwrite, got %d", len(resources))
		}
	})
}

func TestDetectLeaks(t *testing.T) {
	t.Run("detects long-held resources", func(t *testing.T) {
		d := NewLeakDetector(time.Second, 50*time.Millisecond)
		d.Acquire("file_handle", "file-1")

		// Wait longer than threshold
		time.Sleep(100 * time.Millisecond)

		d.detectLeaks()

		leaks := d.GetDetectedLeaks()
		if len(leaks) != 1 {
			t.Fatalf("expected 1 leak, got %d", len(leaks))
		}
		// Should be removed from active
		active := d.GetActiveResources()
		if len(active) != 0 {
			t.Errorf("expected 0 active resources, got %d", len(active))
		}
	})

	t.Run("does not flag short-lived resources", func(t *testing.T) {
		d := NewLeakDetector(time.Second, time.Minute)
		d.Acquire("conn", "c-1")

		d.detectLeaks()

		leaks := d.GetDetectedLeaks()
		if len(leaks) != 0 {
			t.Errorf("expected 0 leaks, got %d", len(leaks))
		}
	})
}

func TestClearDetectedLeaks(t *testing.T) {
	d := NewLeakDetector(time.Second, 50*time.Millisecond)
	d.Acquire("conn", "c-1")
	time.Sleep(100 * time.Millisecond)
	d.detectLeaks()

	if len(d.GetDetectedLeaks()) == 0 {
		t.Fatal("expected leaks before clear")
	}

	d.ClearDetectedLeaks()
	if len(d.GetDetectedLeaks()) != 0 {
		t.Error("expected 0 leaks after clear")
	}
}

func TestGetLeakReport(t *testing.T) {
	d := NewLeakDetector(time.Second, time.Minute)
	d.Acquire("conn", "c-1")
	d.Acquire("conn", "c-2")

	report := d.GetLeakReport()
	if report["total_active_resources"] != 2 {
		t.Errorf("expected 2 active resources in report, got %v", report["total_active_resources"])
	}
	if report["total_detected_leaks"] != 0 {
		t.Errorf("expected 0 leaks in report, got %v", report["total_detected_leaks"])
	}
}

func TestResourceTracker(t *testing.T) {
	d := NewLeakDetector(time.Second, time.Minute)
	tracker := NewResourceTracker(d, "http_conn")

	t.Run("track and untrack", func(t *testing.T) {
		tracker.Track("req-1")
		if len(d.GetActiveResources()) != 1 {
			t.Fatalf("expected 1 active resource, got %d", len(d.GetActiveResources()))
		}

		tracker.Untrack("req-1")
		if len(d.GetActiveResources()) != 0 {
			t.Errorf("expected 0 active resources after untrack, got %d", len(d.GetActiveResources()))
		}
	})
}

func TestGetLeakReportConcurrent(t *testing.T) {
	d := NewLeakDetector(time.Second, time.Minute)
	done := make(chan bool)

	for i := 0; i < 20; i++ {
		go func(id int) {
			d.Acquire("conn", resourceID(id))
			d.GetLeakReport()
			d.Release(resourceID(id))
			done <- true
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestLeakDetectorConcurrency(t *testing.T) {
	d := NewLeakDetector(time.Second, time.Minute)
	done := make(chan bool)

	for i := 0; i < 20; i++ {
		go func(id int) {
			d.Acquire("conn", resourceID(id))
			d.Release(resourceID(id))
			d.GetActiveResources()
			d.GetDetectedLeaks()
			d.GetLeakReport()
			done <- true
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

func resourceID(id int) string {
	return "res-" + string(rune('0'+id))
}

// --- SplitLines / JoinLines / Contains Tests ---

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single line", "hello", 1},
		{"two lines", "hello\nworld", 2},
		{"trailing newline", "hello\n", 1}, // splitLines doesn't append trailing empty line
		{"empty string", "", 0},
		{"multiple newlines", "a\nb\nc\nd", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := splitLines(tt.input)
			if len(lines) != tt.want {
				t.Errorf("expected %d lines, got %d", tt.want, len(lines))
			}
		})
	}
}

func TestJoinLines(t *testing.T) {
	t.Run("joins multiple lines", func(t *testing.T) {
		result := joinLines([]string{"a", "b", "c"})
		if result != "a\nb\nc" {
			t.Errorf("expected 'a\\nb\\nc', got %q", result)
		}
	})

	t.Run("single line", func(t *testing.T) {
		result := joinLines([]string{"only"})
		if result != "only" {
			t.Errorf("expected 'only', got %q", result)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		result := joinLines([]string{})
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})
}

func TestContains(t *testing.T) {
	tests := []struct {
		s       string
		substr  string
		want    bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "xyz", false},
		{"", "anything", false},
		{"exact", "exact", true},
		{"path/to/internal/monitoring/leak_detector.go:123", "internal/monitoring/leak_detector.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestGetStackTrace(t *testing.T) {
	trace := getStackTrace()
	if trace == "" {
		t.Error("expected non-empty stack trace")
	}
}

// --- ResourceMonitor Tests ---

func TestNewResourceMonitor(t *testing.T) {
	t.Run("uses default for zero interval", func(t *testing.T) {
		m := NewResourceMonitor(0)
		if m.monitorInterval != 10*time.Second {
			t.Errorf("expected default interval 10s, got %v", m.monitorInterval)
		}
	})

	t.Run("uses provided interval", func(t *testing.T) {
		m := NewResourceMonitor(5 * time.Second)
		if m.monitorInterval != 5*time.Second {
			t.Errorf("expected interval 5s, got %v", m.monitorInterval)
		}
	})
}

func TestGetCurrentStats(t *testing.T) {
	m := NewResourceMonitor(time.Second)
	stats := m.GetCurrentStats()

	if stats.GoroutineCount < 1 {
		t.Errorf("expected at least 1 goroutine, got %d", stats.GoroutineCount)
	}
	if stats.MemoryUsage.Total == 0 {
		t.Error("expected non-zero total memory")
	}
	if stats.MemoryUsage.Used == 0 {
		t.Error("expected non-zero used memory")
	}
	if stats.PoolStats == nil {
		t.Error("expected non-nil pool stats map")
	}
}

func TestGetStatsHistory(t *testing.T) {
	m := NewResourceMonitor(time.Second)

	// Initially empty
	history := m.GetStatsHistory()
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}

	// Manually collect some stats
	m.collectStats()
	m.collectStats()

	history = m.GetStatsHistory()
	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}
}

func TestGetHighWaterMark(t *testing.T) {
	m := NewResourceMonitor(time.Second)

	// Empty history
	watermark := m.GetHighWaterMark()
	if len(watermark) != 0 {
		t.Errorf("expected empty watermark with no history, got %d entries", len(watermark))
	}

	// Collect some stats
	m.collectStats()
	m.collectStats()

	watermark = m.GetHighWaterMark()
	if _, ok := watermark["memory"]; !ok {
		t.Error("expected memory watermark")
	}
	if _, ok := watermark["goroutine"]; !ok {
		t.Error("expected goroutine watermark")
	}
}

func TestCheckResourceUsage(t *testing.T) {
	m := NewResourceMonitor(time.Second)

	t.Run("memory threshold", func(t *testing.T) {
		// Very low threshold should trigger
		alerts := m.CheckResourceUsage(map[string]float64{
			"memory":    0.0001, // 0.0001% - should be exceeded
			"goroutine": 999999,  // very high - should not be exceeded
		})
		if !alerts["memory"] {
			t.Error("expected memory alert for very low threshold")
		}
		if alerts["goroutine"] {
			t.Error("expected no goroutine alert for very high threshold")
		}
	})

	t.Run("pool threshold", func(t *testing.T) {
		m.RegisterPool("test-pool", 100, 10)
		m.UpdatePoolStats("test-pool", PoolStats{
			Name:    "test-pool",
			MaxSize: 100,
			InUse:   80, // 80% usage
		})

		alerts := m.CheckResourceUsage(map[string]float64{
			"pool_test-pool": 50, // 50% threshold
		})
		if !alerts["pool_test-pool"] {
			t.Error("expected pool alert for 80% usage with 50% threshold")
		}
	})
}

func TestGetResourceReport(t *testing.T) {
	m := NewResourceMonitor(time.Second)
	m.collectStats()

	report := m.GetResourceReport()
	if report["current"] == nil {
		t.Error("expected 'current' section in report")
	}
	if report["high_water_mark"] == nil {
		t.Error("expected 'high_water_mark' section in report")
	}
	if report["history_length"] != 1 {
		t.Errorf("expected history_length=1, got %v", report["history_length"])
	}
}

func TestRegisterAndUnregisterPool(t *testing.T) {
	m := NewResourceMonitor(time.Second)

	t.Run("register pool", func(t *testing.T) {
		m.RegisterPool("db-pool", 50, 10)
		stats := m.GetCurrentStats()
		poolStat, ok := stats.PoolStats["db-pool"]
		if !ok {
			t.Fatal("expected db-pool in stats")
		}
		if poolStat.MaxSize != 50 {
			t.Errorf("expected max size 50, got %d", poolStat.MaxSize)
		}
		if poolStat.MinSize != 10 {
			t.Errorf("expected min size 10, got %d", poolStat.MinSize)
		}
		if poolStat.TotalCreated != 10 {
			t.Errorf("expected total created 10, got %d", poolStat.TotalCreated)
		}
	})

	t.Run("unregister pool", func(t *testing.T) {
		m.UnregisterPool("db-pool")
		stats := m.GetCurrentStats()
		if _, ok := stats.PoolStats["db-pool"]; ok {
			t.Error("expected db-pool to be removed")
		}
	})
}

func TestRecordResponseTime(t *testing.T) {
	m := NewResourceMonitor(time.Second)

	t.Run("records single request", func(t *testing.T) {
		m.RecordResponseTime(100.0, "api", true)

		stats := m.GetCurrentStats()
		if stats.ResponseTimeStats.TotalRequests != 1 {
			t.Errorf("expected 1 total request, got %d", stats.ResponseTimeStats.TotalRequests)
		}
		if stats.ResponseTimeStats.SuccessfulRequests != 1 {
			t.Errorf("expected 1 successful request, got %d", stats.ResponseTimeStats.SuccessfulRequests)
		}
		if stats.ResponseTimeStats.MinResponseTime != 100.0 {
			t.Errorf("expected min 100.0, got %v", stats.ResponseTimeStats.MinResponseTime)
		}
		if stats.ResponseTimeStats.MaxResponseTime != 100.0 {
			t.Errorf("expected max 100.0, got %v", stats.ResponseTimeStats.MaxResponseTime)
		}
		if stats.ResponseTimeStats.AvgResponseTime != 100.0 {
			t.Errorf("expected avg 100.0, got %v", stats.ResponseTimeStats.AvgResponseTime)
		}
	})

	t.Run("records failed request", func(t *testing.T) {
		m2 := NewResourceMonitor(time.Second)
		m2.RecordResponseTime(200.0, "api", false)

		stats := m2.GetCurrentStats()
		if stats.ResponseTimeStats.FailedRequests != 1 {
			t.Errorf("expected 1 failed request, got %d", stats.ResponseTimeStats.FailedRequests)
		}
		if stats.ResponseTimeStats.ErrorRate != 1.0 {
			t.Errorf("expected error rate 1.0, got %v", stats.ResponseTimeStats.ErrorRate)
		}
	})

	t.Run("multiple requests update stats", func(t *testing.T) {
		m2 := NewResourceMonitor(time.Second)
		m2.RecordResponseTime(10.0, "", true)
		m2.RecordResponseTime(20.0, "", true)
		m2.RecordResponseTime(30.0, "", true)

		stats := m2.GetCurrentStats()
		if stats.ResponseTimeStats.TotalRequests != 3 {
			t.Errorf("expected 3 total requests, got %d", stats.ResponseTimeStats.TotalRequests)
		}
		if stats.ResponseTimeStats.AvgResponseTime != 20.0 {
			t.Errorf("expected avg 20.0, got %v", stats.ResponseTimeStats.AvgResponseTime)
		}
		if stats.ResponseTimeStats.MinResponseTime != 10.0 {
			t.Errorf("expected min 10.0, got %v", stats.ResponseTimeStats.MinResponseTime)
		}
		if stats.ResponseTimeStats.MaxResponseTime != 30.0 {
			t.Errorf("expected max 30.0, got %v", stats.ResponseTimeStats.MaxResponseTime)
		}
		// Check percentiles are set
		if stats.ResponseTimeStats.P90ResponseTime == 0 {
			t.Error("expected P90 to be set")
		}
	})

	t.Run("request type stats", func(t *testing.T) {
		m2 := NewResourceMonitor(time.Second)
		m2.RecordResponseTime(50.0, "query", true)
		m2.RecordResponseTime(150.0, "query", false)

		stats := m2.GetCurrentStats()
		typeStat, ok := stats.ResponseTimeStats.TypeStats["query"]
		if !ok {
			t.Fatal("expected 'query' type stats")
		}
		if typeStat.TotalRequests != 2 {
			t.Errorf("expected 2 query requests, got %d", typeStat.TotalRequests)
		}
		if typeStat.FailedRequests != 1 {
			t.Errorf("expected 1 failed query, got %d", typeStat.FailedRequests)
		}
	})

	t.Run("empty request type", func(t *testing.T) {
		m2 := NewResourceMonitor(time.Second)
		m2.RecordResponseTime(100.0, "", true)
		stats := m2.GetCurrentStats()
		if len(stats.ResponseTimeStats.TypeStats) != 0 {
			t.Error("expected no type stats for empty request type")
		}
	})
}

func TestCustomMetrics(t *testing.T) {
	m := NewResourceMonitor(time.Second)

	t.Run("record and get", func(t *testing.T) {
		m.RecordCustomMetric("cpu_usage", "gauge", 45.5, map[string]string{"host": "server1"}, "CPU usage percentage")
		metric, ok := m.GetCustomMetric("cpu_usage", map[string]string{"host": "server1"})
		if !ok {
			t.Fatal("expected metric to exist")
		}
		if metric.Name != "cpu_usage" {
			t.Errorf("expected name 'cpu_usage', got %q", metric.Name)
		}
		if metric.Value != 45.5 {
			t.Errorf("expected value 45.5, got %v", metric.Value)
		}
		if metric.Labels["host"] != "server1" {
			t.Errorf("expected label host='server1', got %q", metric.Labels["host"])
		}
	})

	t.Run("list metrics", func(t *testing.T) {
		m.RecordCustomMetric("metric1", "counter", 1, nil, "")
		m.RecordCustomMetric("metric2", "gauge", 2.0, nil, "")
		metrics := m.ListCustomMetrics()
		if len(metrics) < 2 {
			t.Errorf("expected at least 2 metrics, got %d", len(metrics))
		}
	})

	t.Run("update existing metric", func(t *testing.T) {
		m.RecordCustomMetric("counter", "counter", 10, nil, "")
		m.RecordCustomMetric("counter", "counter", 20, nil, "")
		metric, _ := m.GetCustomMetric("counter", nil)
		if metric.Value != 20 {
			t.Errorf("expected value 20 after update, got %v", metric.Value)
		}
	})

	t.Run("delete metric", func(t *testing.T) {
		m.RecordCustomMetric("temp_metric", "gauge", 1.0, nil, "")
		if _, ok := m.GetCustomMetric("temp_metric", nil); !ok {
			t.Fatal("expected metric to exist before delete")
		}
		m.DeleteCustomMetric("temp_metric", nil)
		if _, ok := m.GetCustomMetric("temp_metric", nil); ok {
			t.Error("expected metric to be deleted")
		}
	})

	t.Run("get nonexistent metric", func(t *testing.T) {
		_, ok := m.GetCustomMetric("does_not_exist", nil)
		if ok {
			t.Error("expected false for nonexistent metric")
		}
	})
}

func TestResourceMonitorConcurrency(t *testing.T) {
	m := NewResourceMonitor(time.Second)
	done := make(chan bool)

	for i := 0; i < 20; i++ {
		go func() {
			// After fix: GetCurrentStats no longer has reentrant lock issue
			m.GetCurrentStats()
			m.GetStatsHistory()
			m.RegisterPool("pool", 10, 5)
			m.UpdatePoolStats("pool", PoolStats{Name: "pool", MaxSize: 10})
			m.UnregisterPool("pool")
			m.RecordResponseTime(100.0, "test", true)
			m.RecordCustomMetric("concurrent", "counter", 1, nil, "")
			m.ListCustomMetrics()
			m.GetCustomMetric("concurrent", nil)
			m.DeleteCustomMetric("concurrent", nil)
			done <- true
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
