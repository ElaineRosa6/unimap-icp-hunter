package tamper

import (
	"strings"
	"testing"
)

// 表驱动测试：篡改检测模式
func TestIsMeaningfulTamper(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		changes    []SegmentChange
		wantTamper bool
	}{
		// 宽松模式测试
		{"relaxed: empty changes", DetectionModeRelaxed, nil, false},
		{"relaxed: single non-critical modified", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentImages, ChangeType: "modified"}}, false},
		{"relaxed: critical segment modified", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentMain, ChangeType: "modified"}}, true},
		{"relaxed: article modified", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentArticle, ChangeType: "modified"}}, true},
		{"relaxed: forms modified", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentForms, ChangeType: "modified"}}, true},
		{"relaxed: two stable modified", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentImages, ChangeType: "modified"}, {Segment: SegmentAside, ChangeType: "modified"}}, true},
		{"relaxed: segment added", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentMain, ChangeType: "added"}}, true},
		{"relaxed: segment removed", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentMain, ChangeType: "removed"}}, true},
		{"relaxed: volatile segment modified", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentScripts, ChangeType: "modified"}}, false},
		{"relaxed: links modified", DetectionModeRelaxed, []SegmentChange{{Segment: SegmentLinks, ChangeType: "modified"}}, false},

		// 严格模式测试
		{"strict: empty changes", DetectionModeStrict, nil, false},
		{"strict: any change", DetectionModeStrict, []SegmentChange{{Segment: SegmentLinks, ChangeType: "modified"}}, true},
		{"strict: scripts changed", DetectionModeStrict, []SegmentChange{{Segment: SegmentScripts, ChangeType: "modified"}}, true},
		{"strict: multiple changes", DetectionModeStrict, []SegmentChange{{Segment: SegmentLinks, ChangeType: "modified"}, {Segment: SegmentImages, ChangeType: "modified"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDetector(DetectorConfig{DetectionMode: tt.mode})
			got := d.isMeaningfulTamper(tt.changes)
			if got != tt.wantTamper {
				t.Errorf("isMeaningfulTamper() = %v, want %v", got, tt.wantTamper)
			}
		})
	}
}

func TestIsMeaningfulTamperModes(t *testing.T) {
	relaxed := NewDetector(DetectorConfig{DetectionMode: DetectionModeRelaxed})
	strict := NewDetector(DetectorConfig{DetectionMode: DetectionModeStrict})

	if relaxed.isMeaningfulTamper([]SegmentChange{{Segment: SegmentImages, ChangeType: "modified"}}) {
		t.Fatalf("single non-critical stable modified segment should not be tampered in relaxed mode")
	}

	if !relaxed.isMeaningfulTamper([]SegmentChange{{Segment: SegmentMain, ChangeType: "modified"}}) {
		t.Fatalf("critical stable segment should be tampered in relaxed mode")
	}

	if !relaxed.isMeaningfulTamper([]SegmentChange{
		{Segment: SegmentImages, ChangeType: "modified"},
		{Segment: SegmentAside, ChangeType: "modified"},
	}) {
		t.Fatalf("two stable modified segments should be tampered in relaxed mode")
	}

	if !strict.isMeaningfulTamper([]SegmentChange{{Segment: SegmentLinks, ChangeType: "modified"}}) {
		t.Fatalf("any change should be tampered in strict mode")
	}
}

// 测试 findChangedSegments
func TestFindChangedSegments(t *testing.T) {
	tests := []struct {
		name           string
		current        *PageHashResult
		baseline       *PageHashResult
		wantSegments   int
		wantChanges    int
		wantChangeType string
	}{
		{
			name: "no changes",
			current: &PageHashResult{SegmentHashes: []SegmentHash{
				{Name: SegmentMain, Hash: "same"},
			}},
			baseline: &PageHashResult{SegmentHashes: []SegmentHash{
				{Name: SegmentMain, Hash: "same"},
			}},
			wantSegments: 0,
			wantChanges:  0,
		},
		{
			name: "segment modified",
			current: &PageHashResult{SegmentHashes: []SegmentHash{
				{Name: SegmentMain, Hash: "new-hash"},
			}},
			baseline: &PageHashResult{SegmentHashes: []SegmentHash{
				{Name: SegmentMain, Hash: "old-hash"},
			}},
			wantSegments:   1,
			wantChanges:    1,
			wantChangeType: "modified",
		},
		{
			name: "segment added",
			current: &PageHashResult{SegmentHashes: []SegmentHash{
				{Name: SegmentMain, Hash: "hash"},
			}},
			baseline:       &PageHashResult{SegmentHashes: []SegmentHash{}},
			wantSegments:   1,
			wantChanges:    1,
			wantChangeType: "added",
		},
		{
			name:    "segment removed",
			current: &PageHashResult{SegmentHashes: []SegmentHash{}},
			baseline: &PageHashResult{SegmentHashes: []SegmentHash{
				{Name: SegmentMain, Hash: "hash"},
			}},
			wantSegments:   1,
			wantChanges:    1,
			wantChangeType: "removed",
		},
		{
			name: "structure changed",
			current: &PageHashResult{SegmentHashes: []SegmentHash{
				{Name: SegmentMain, Hash: "new", Length: 100, Elements: 5},
			}},
			baseline: &PageHashResult{SegmentHashes: []SegmentHash{
				{Name: SegmentMain, Hash: "old", Length: 50, Elements: 3},
			}},
			wantSegments:   1,
			wantChanges:    1,
			wantChangeType: "structure_changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDetector(DetectorConfig{DetectionMode: DetectionModeStrict})
			segments, changes := d.findChangedSegments(tt.current, tt.baseline)

			if len(segments) != tt.wantSegments {
				t.Errorf("expected %d segments, got %d", tt.wantSegments, len(segments))
			}
			if len(changes) != tt.wantChanges {
				t.Errorf("expected %d changes, got %d", tt.wantChanges, len(changes))
			}
			if tt.wantChangeType != "" && len(changes) > 0 {
				if changes[0].ChangeType != tt.wantChangeType {
					t.Errorf("expected change type %s, got %s", tt.wantChangeType, changes[0].ChangeType)
				}
			}
		})
	}
}

func TestFindChangedSegmentsIgnoresCompatibilityOptional(t *testing.T) {
	d := NewDetector(DetectorConfig{DetectionMode: DetectionModeRelaxed})

	current := &PageHashResult{SegmentHashes: []SegmentHash{
		{Name: SegmentMain, Hash: "main-new"},
	}}
	baseline := &PageHashResult{SegmentHashes: []SegmentHash{
		{Name: SegmentMain, Hash: "main-old"},
		{Name: SegmentJSFiles, Hash: "js-old"},
	}}

	segments, changes := d.findChangedSegments(current, baseline)
	if len(changes) != 1 {
		t.Fatalf("expected only 1 change (main modified), got %d", len(changes))
	}
	if changes[0].Segment != SegmentMain || changes[0].ChangeType != "modified" {
		t.Fatalf("unexpected change: %+v", changes[0])
	}
	if len(segments) != 1 || segments[0] != SegmentMain {
		t.Fatalf("unexpected tampered segments: %+v", segments)
	}
}

// 测试 cleanHTML
func TestCleanHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
		excludes string
	}{
		{"empty", "", "", ""},
		{"normalize whitespace", "  multiple   spaces  ", "multiple spaces", "  "},
		{"remove comments", "<!-- comment -->content", "content", "comment"},
		{"remove data images", `<img src="data:image/png;base64,abc">`, "DATA_IMAGE_REMOVED", "data:image"},
		{"remove nonce", `<script nonce="abc123">`, "", "nonce="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDetector(DetectorConfig{})
			result := d.cleanHTML(tt.input)

			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected result to contain %q, got %q", tt.contains, result)
			}
			if tt.excludes != "" && strings.Contains(result, tt.excludes) {
				t.Errorf("expected result to not contain %q, got %q", tt.excludes, result)
			}
		})
	}
}

// 测试 computeSHA256
func TestComputeSHA256(t *testing.T) {
	tests := []struct {
		input    string
		expected string // SHA256 of empty string
	}{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"test", "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := computeSHA256(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// 测试 normalizeDetectionMode
func TestNormalizeDetectionMode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", DetectionModeRelaxed},
		{"relaxed", DetectionModeRelaxed},
		{"RELAXED", DetectionModeRelaxed},
		{"Relaxed", DetectionModeRelaxed},
		{"strict", DetectionModeStrict},
		{"STRICT", DetectionModeStrict},
		{"Strict", DetectionModeStrict},
		{"unknown", DetectionModeRelaxed},
		{"invalid", DetectionModeRelaxed},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeDetectionMode(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// 测试 sanitizeFilenameForStorage
func TestSanitizeFilenameForStorage(t *testing.T) {
	tests := []struct {
		input    string
		contains string
		excludes string
	}{
		{"https://example.com/path", "example", "://"},
		{"http://test.com", "test", "http://"},
		{"https://example.com?q=1&a=2", "example", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeFilenameForStorage(tt.input)
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected result to contain %q, got %q", tt.contains, result)
			}
			if tt.excludes != "" && strings.Contains(result, tt.excludes) {
				t.Errorf("expected result to not contain %q, got %q", tt.excludes, result)
			}
		})
	}
}

// 测试 classifyTamperError
func TestClassifyTamperError(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "unknown"},
		{"baseline not found", "baseline"},
		{"dns name_not_resolved", "dns"},
		{"connection timed out", "timeout"},
		{"ssl certificate error", "tls"},
		{"connection refused", "connection_refused"},
		{"connection reset by peer", "connection_reset"},
		{"unknown network error", "network"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := classifyTamperError(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// 测试 isStableSegment
func TestIsStableSegment(t *testing.T) {
	relaxed := NewDetector(DetectorConfig{DetectionMode: DetectionModeRelaxed})
	strict := NewDetector(DetectorConfig{DetectionMode: DetectionModeStrict})

	// 宽松模式下的易变分段
	volatileSegments := []string{SegmentHead, SegmentBody, SegmentHeader, SegmentNav, SegmentFooter, SegmentLinks, SegmentScripts, SegmentStyles, SegmentMeta, SegmentFullContent}
	for _, seg := range volatileSegments {
		if relaxed.isStableSegment(seg) {
			t.Errorf("expected %s to be volatile in relaxed mode", seg)
		}
	}

	// 严格模式下所有分段都是稳定的
	if !strict.isStableSegment(SegmentLinks) {
		t.Error("expected all segments to be stable in strict mode")
	}
}

// 测试 isCriticalStableSegment
func TestIsCriticalStableSegment(t *testing.T) {
	critical := []string{SegmentMain, SegmentArticle, SegmentForms}
	for _, seg := range critical {
		if !isCriticalStableSegment(seg) {
			t.Errorf("expected %s to be critical", seg)
		}
	}

	nonCritical := []string{SegmentImages, SegmentLinks, SegmentAside}
	for _, seg := range nonCritical {
		if isCriticalStableSegment(seg) {
			t.Errorf("expected %s to be non-critical", seg)
		}
	}
}

// 测试 isCompatibilityOptionalSegment
func TestIsCompatibilityOptionalSegment(t *testing.T) {
	optional := []string{SegmentJSFiles, SegmentFavicon, SegmentButtons}
	for _, seg := range optional {
		if !isCompatibilityOptionalSegment(seg) {
			t.Errorf("expected %s to be optional", seg)
		}
	}

	nonOptional := []string{SegmentMain, SegmentBody, SegmentHead}
	for _, seg := range nonOptional {
		if isCompatibilityOptionalSegment(seg) {
			t.Errorf("expected %s to be non-optional", seg)
		}
	}
}

func TestCollectOrderedTamperCheckResults(t *testing.T) {
	ch := make(chan tamperBatchCheckResult, 5)
	ch <- tamperBatchCheckResult{index: 2, result: TamperCheckResult{URL: "u2", Status: "normal"}}
	ch <- tamperBatchCheckResult{index: 0, result: TamperCheckResult{URL: "u0", Status: "tampered"}}
	ch <- tamperBatchCheckResult{index: 1, result: TamperCheckResult{URL: "u1", Status: "unreachable"}}
	ch <- tamperBatchCheckResult{index: -1, result: TamperCheckResult{URL: "bad-low"}}
	ch <- tamperBatchCheckResult{index: 99, result: TamperCheckResult{URL: "bad-high"}}
	close(ch)

	results := collectOrderedTamperCheckResults(ch, 3)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].URL != "u0" || results[1].URL != "u1" || results[2].URL != "u2" {
		t.Fatalf("unexpected order: %+v", results)
	}
}

func TestCollectOrderedTamperBaselineResults(t *testing.T) {
	ch := make(chan tamperBatchBaselineResult, 5)
	ch <- tamperBatchBaselineResult{index: 1, result: PageHashResult{URL: "u1", Status: "ok"}}
	ch <- tamperBatchBaselineResult{index: 0, result: PageHashResult{URL: "u0", Status: "ok"}}
	ch <- tamperBatchBaselineResult{index: 2, result: PageHashResult{URL: "u2", Status: "error"}}
	ch <- tamperBatchBaselineResult{index: -3, result: PageHashResult{URL: "bad-low"}}
	ch <- tamperBatchBaselineResult{index: 12, result: PageHashResult{URL: "bad-high"}}
	close(ch)

	results := collectOrderedTamperBaselineResults(ch, 3)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].URL != "u0" || results[1].URL != "u1" || results[2].URL != "u2" {
		t.Fatalf("unexpected order: %+v", results)
	}
}

// 基准测试
func BenchmarkCleanHTML(b *testing.B) {
	d := NewDetector(DetectorConfig{})
	html := `<html><head><!-- comment --></head><body class="test"><img src="data:image/png;base64,abc"><script nonce="xyz">code</script></body></html>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.cleanHTML(html)
	}
}

func BenchmarkComputeSHA256(b *testing.B) {
	data := strings.Repeat("test data ", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeSHA256(data)
	}
}

// --- detectMaliciousContent Tests ---

func TestDetectMaliciousContent(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantLen  int
		wantFlag string
	}{
		{"clean html", "<html><body><h1>Hello</h1></body></html>", 0, ""},
		{"eval script", "<script>eval('bad')</script>", 1, "suspicious_script: eval("},
		{"document.write", "<script>document.write('x')</script>", 1, "suspicious_script: document.write("},
		{"multiple script keywords", "<script>eval( atob( crypto miner</script>", 4, "suspicious_script:"},
		{"single domain keyword", "<p>casino</p>", 0, ""},
		{"multiple domain keywords", "<p>casino bitcoin</p>", 1, "suspicious_domain_keywords"},
		{"hidden iframe", `<iframe style="display:none" src="evil"></iframe>`, 1, "hidden_iframe"},
		{"dangerous event handler", `<body onload="eval(document.cookie)">`, 2, "dangerous_event_handler"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := detectMaliciousContent(tt.html)
			if len(flags) < tt.wantLen {
				t.Errorf("expected at least %d flags, got %d: %v", tt.wantLen, len(flags), flags)
			}
			if tt.wantFlag != "" && len(flags) > 0 {
				found := false
				for _, f := range flags {
					if strings.Contains(f, tt.wantFlag) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected flag containing %q, got %v", tt.wantFlag, flags)
				}
			}
		})
	}
}

func TestNormalizePerformanceMode(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"", PerformanceModeBalanced},
		{"balanced", PerformanceModeBalanced},
		{"BALANCED", PerformanceModeBalanced},
		{"fast", PerformanceModeFast},
		{"FAST", PerformanceModeFast},
		{"comprehensive", PerformanceModeComprehensive},
		{"COMPREHENSIVE", PerformanceModeComprehensive},
		{"unknown", PerformanceModeBalanced},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			if got := normalizePerformanceMode(tt.mode); got != tt.want {
				t.Errorf("normalizePerformanceMode(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}
