package threshold

import (
	"strings"
	"testing"
	"time"
)

// --- Constructor Tests ---

func TestNewThresholdManager(t *testing.T) {
	tm := NewThresholdManager()
	if tm.thresholds == nil {
		t.Error("expected thresholds map to be initialized")
	}
	if tm.siteSensitivity == nil {
		t.Error("expected siteSensitivity map to be initialized")
	}
	if tm.ruleWeights == nil {
		t.Error("expected ruleWeights map to be initialized")
	}
	if tm.historicalData == nil {
		t.Error("expected historicalData map to be initialized")
	}
}

// --- Threshold Tests ---

func TestGetThreshold(t *testing.T) {
	tm := NewThresholdManager()

	t.Run("default threshold for unknown site", func(t *testing.T) {
		th := tm.GetThreshold("https://unknown.com")
		if th != 0.5 {
			t.Errorf("expected default threshold 0.5, got %.2f", th)
		}
	})
}

func TestGetAdjustedThreshold(t *testing.T) {
	tm := NewThresholdManager()

	t.Run("default adjusted threshold", func(t *testing.T) {
		th := tm.GetAdjustedThreshold("https://test.com")
		// Default: 0.5 / 1.0 = 0.5, clamped to [0.1, 0.9]
		if th != 0.5 {
			t.Errorf("expected 0.5, got %.2f", th)
		}
	})

	t.Run("high sensitivity lowers threshold", func(t *testing.T) {
		_ = tm.SetSiteSensitivity("https://sensitive.com", 5.0)
		th := tm.GetAdjustedThreshold("https://sensitive.com")
		// 0.5 / 5.0 = 0.1, clamped to 0.1
		if th < 0.1 || th > 0.11 {
			t.Errorf("expected ~0.1, got %.4f", th)
		}
	})
}

func TestIsAboveThreshold(t *testing.T) {
	tm := NewThresholdManager()

	if tm.IsAboveThreshold("https://test.com", 0.4) {
		t.Error("expected 0.4 to be below threshold")
	}
	if !tm.IsAboveThreshold("https://test.com", 0.6) {
		t.Error("expected 0.6 to be above threshold")
	}
}

// --- Sensitivity Tests ---

func TestSiteSensitivity(t *testing.T) {
	tm := NewThresholdManager()

	t.Run("set and get", func(t *testing.T) {
		if err := tm.SetSiteSensitivity("https://site.com", 2.0); err != nil {
			t.Fatalf("SetSiteSensitivity failed: %v", err)
		}
		s := tm.GetSiteSensitivity("https://site.com")
		if s != 2.0 {
			t.Errorf("expected sensitivity 2.0, got %.2f", s)
		}
	})

	t.Run("default sensitivity", func(t *testing.T) {
		s := tm.GetSiteSensitivity("https://new.com")
		if s != 1.0 {
			t.Errorf("expected default sensitivity 1.0, got %.2f", s)
		}
	})

	t.Run("rejects value below 0.1", func(t *testing.T) {
		if err := tm.SetSiteSensitivity("https://site.com", 0.05); err == nil {
			t.Error("expected error for sensitivity < 0.1")
		}
	})

	t.Run("rejects value above 10.0", func(t *testing.T) {
		err := tm.SetSiteSensitivity("https://site.com", 15.0)
		if err == nil {
			t.Error("expected error for sensitivity > 10.0")
		}
		if !strings.Contains(err.Error(), "must be between") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

// --- Rule Weight Tests ---

func TestRuleWeights(t *testing.T) {
	tm := NewThresholdManager()

	t.Run("set and get", func(t *testing.T) {
		if err := tm.SetRuleWeight("rule-1", 5.0); err != nil {
			t.Fatalf("SetRuleWeight failed: %v", err)
		}
		w := tm.GetRuleWeight("rule-1")
		if w != 5.0 {
			t.Errorf("expected weight 5.0, got %.2f", w)
		}
	})

	t.Run("default weight", func(t *testing.T) {
		w := tm.GetRuleWeight("unknown-rule")
		if w != 1.0 {
			t.Errorf("expected default weight 1.0, got %.2f", w)
		}
	})

	t.Run("rejects negative weight", func(t *testing.T) {
		if err := tm.SetRuleWeight("rule-2", -1.0); err == nil {
			t.Error("expected error for negative weight")
		}
	})

	t.Run("rejects weight > 10.0", func(t *testing.T) {
		if err := tm.SetRuleWeight("rule-3", 11.0); err == nil {
			t.Error("expected error for weight > 10.0")
		}
	})

	t.Run("update rule weights in bulk", func(t *testing.T) {
		err := tm.UpdateRuleWeights(map[string]float64{
			"r1": 2.0,
			"r2": 3.0,
		})
		if err != nil {
			t.Fatalf("UpdateRuleWeights failed: %v", err)
		}
		if tm.GetRuleWeight("r1") != 2.0 {
			t.Errorf("expected r1 weight 2.0, got %.2f", tm.GetRuleWeight("r1"))
		}
	})

	t.Run("bulk update rejects invalid weight", func(t *testing.T) {
		err := tm.UpdateRuleWeights(map[string]float64{
			"valid": 5.0,
			"invalid": -1.0,
		})
		if err == nil {
			t.Error("expected error for invalid bulk weight")
		}
	})

	t.Run("get all weights", func(t *testing.T) {
		weights := tm.GetAllRuleWeights()
		if len(weights) < 2 {
			t.Errorf("expected at least 2 weights, got %d", len(weights))
		}
	})
}

// --- Historical Data Tests ---

func TestRecordScanResult(t *testing.T) {
	tm := NewThresholdManager()

	t.Run("records true positive", func(t *testing.T) {
		tm.RecordScanResult("https://site.com", true, false)
		data, ok := tm.GetHistoricalData("https://site.com")
		if !ok {
			t.Fatal("expected historical data to exist")
		}
		if data.TotalScans != 1 {
			t.Errorf("expected 1 scan, got %d", data.TotalScans)
		}
		if data.TruePositives != 1 {
			t.Errorf("expected 1 true positive, got %d", data.TruePositives)
		}
	})

	t.Run("records false positive", func(t *testing.T) {
		tm.RecordScanResult("https://site2.com", false, true)
		data, ok := tm.GetHistoricalData("https://site2.com")
		if !ok {
			t.Fatal("expected historical data")
		}
		if data.FalsePositives != 1 {
			t.Errorf("expected 1 false positive, got %d", data.FalsePositives)
		}
	})

	t.Run("threshold adjustment after enough scans", func(t *testing.T) {
		tm2 := NewThresholdManager()
		// Record many scans with high false positive rate
		for i := 0; i < 15; i++ {
			tm2.RecordScanResult("https://fp-site.com", false, true)
		}
		config, exists := tm2.thresholds["https://fp-site.com"]
		if !exists {
			t.Fatal("expected threshold config to be created")
		}
		if config.AdjustmentFactor <= 0 {
			t.Error("expected adjustment factor to increase due to high false positive rate")
		}
	})
}

func TestThresholdAdjustmentLowFalsePositive(t *testing.T) {
	tm := NewThresholdManager()
	// Record many scans with very low false positive rate
	for i := 0; i < 20; i++ {
		tm.RecordScanResult("https://good-site.com", true, false)
	}
	config, exists := tm.thresholds["https://good-site.com"]
	if !exists {
		t.Fatal("expected threshold config to be created")
	}
	// Low false positive rate should decrease adjustment factor
	if config.AdjustmentFactor != 0.0 {
		t.Errorf("expected adjustment factor 0.0, got %.2f", config.AdjustmentFactor)
	}
}

func TestNoAdjustmentWithFewScans(t *testing.T) {
	tm := NewThresholdManager()
	// Record fewer than 10 scans
	for i := 0; i < 5; i++ {
		tm.RecordScanResult("https://few-scans.com", false, true)
	}
	_, exists := tm.thresholds["https://few-scans.com"]
	if exists {
		t.Error("expected no threshold adjustment with fewer than 10 scans")
	}
}

// --- Reset and Cleanup Tests ---

func TestResetSiteData(t *testing.T) {
	tm := NewThresholdManager()
	tm.RecordScanResult("https://reset.com", true, false)
	tm.SetSiteSensitivity("https://reset.com", 2.0)

	tm.ResetSiteData("https://reset.com")

	_, ok := tm.GetHistoricalData("https://reset.com")
	if ok {
		t.Error("expected historical data to be reset")
	}
	// Sensitivity should still exist (ResetSiteData only clears thresholds and historical data)
	s := tm.GetSiteSensitivity("https://reset.com")
	if s != 2.0 {
		t.Errorf("expected sensitivity to remain, got %.2f", s)
	}
}

func TestCleanupOldData(t *testing.T) {
	tm := NewThresholdManager()
	tm.RecordScanResult("https://old.com", true, false)
	// Fake old data
	tm.historicalData["https://old.com"].LastScanTime = time.Now().Add(-2 * time.Hour)

	tm.CleanupOldData(1 * time.Hour)

	_, ok := tm.GetHistoricalData("https://old.com")
	if ok {
		t.Error("expected old data to be cleaned up")
	}
}

// --- Stats Tests ---

func TestGetThresholdStats(t *testing.T) {
	tm := NewThresholdManager()
	tm.RecordScanResult("https://a.com", true, false)
	tm.RecordScanResult("https://b.com", false, true)

	stats := tm.GetThresholdStats()
	if stats["total_sites"] != 2 {
		t.Errorf("expected 2 sites, got %v", stats["total_sites"])
	}
	if stats["total_scans"] != 2 {
		t.Errorf("expected 2 scans, got %v", stats["total_scans"])
	}
	if stats["total_false_positives"] != 1 {
		t.Errorf("expected 1 false positive, got %v", stats["total_false_positives"])
	}
	rate, ok := stats["overall_false_positive_rate"]
	if !ok {
		t.Error("expected false positive rate")
	}
	if rate.(float64) != 0.5 {
		t.Errorf("expected 0.5 false positive rate, got %v", rate)
	}
}

// --- CalculateDynamicScore Tests ---

func TestCalculateDynamicScore(t *testing.T) {
	tm := NewThresholdManager()

	t.Run("uses default sensitivity and base score when no matches", func(t *testing.T) {
		score := tm.CalculateDynamicScore("https://site.com", 0.5, nil)
		if score != 0.5 {
			t.Errorf("expected 0.5, got %.2f", score)
		}
	})

	t.Run("applies rule weights", func(t *testing.T) {
		tm.SetRuleWeight("rule-a", 2.0)
		matches := []RuleMatch{
			{RuleID: "rule-a", Score: 0.5},
		}
		score := tm.CalculateDynamicScore("https://site.com", 0.0, matches)
		if score != 1.0 {
			t.Errorf("expected 1.0 (0.5 * 2.0), got %.2f", score)
		}
	})

	t.Run("applies site sensitivity", func(t *testing.T) {
		tm2 := NewThresholdManager()
		tm2.SetSiteSensitivity("https://sensitive.com", 2.0)
		matches := []RuleMatch{
			{RuleID: "r1", Score: 0.3},
		}
		score := tm2.CalculateDynamicScore("https://sensitive.com", 0.0, matches)
		// 0.3 * 1.0 (default weight) * 2.0 (sensitivity) = 0.6
		if score != 0.6 {
			t.Errorf("expected 0.6, got %.2f", score)
		}
	})
}

// --- Helper Functions Tests ---

func TestMinMaxHelpers(t *testing.T) {
	if min(1.0, 2.0) != 1.0 {
		t.Errorf("expected 1.0, got %.2f", min(1.0, 2.0))
	}
	if min(3.0, 1.0) != 1.0 {
		t.Errorf("expected 1.0, got %.2f", min(3.0, 1.0))
	}
	if max(1.0, 2.0) != 2.0 {
		t.Errorf("expected 2.0, got %.2f", max(1.0, 2.0))
	}
	if max(3.0, 1.0) != 3.0 {
		t.Errorf("expected 3.0, got %.2f", max(3.0, 1.0))
	}
}

// --- Concurrent Access Tests ---

func TestThresholdManagerConcurrency(t *testing.T) {
	tm := NewThresholdManager()
	done := make(chan bool)

	for i := 0; i < 20; i++ {
		go func(id int) {
			site := "https://site-" + string(rune('0'+id)) + ".com"
			rule := "rule-" + string(rune('0'+id))
			_ = tm.SetSiteSensitivity(site, 2.0)
			tm.GetSiteSensitivity(site)
			_ = tm.SetRuleWeight(rule, float64(id))
			tm.GetRuleWeight(rule)
			tm.GetThreshold(site)
			tm.GetAdjustedThreshold(site)
			tm.IsAboveThreshold(site, 0.5)
			tm.RecordScanResult(site, true, false)
			tm.GetHistoricalData(site)
			tm.GetThresholdStats()
			tm.GetAllRuleWeights()
			done <- true
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
