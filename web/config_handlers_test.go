package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/unimap/project/internal/config"
)

func newServerForConfigTest() *Server {
	cfg := &config.Config{}
	mgr := config.NewManager("")
	// Seed all required defaults so section patches exercise validation rather than setup gaps.
	mgr.ApplyDefaults(cfg)
	cfg.ICP.Enabled = true
	cfg.ICP.BaseURL = "http://localhost:16181"
	cfg.ICP.APIKey = "abcd1234efgh5678"
	cfg.ICP.Timeout = 30
	cfg.ICP.DefaultType = "web"
	cfg.Engines.Fofa.APIKey = "secret-fofa-key"
	cfg.Engines.Fofa.Email = "user@example.com"
	cfg.Engines.Hunter.APIKey = "secret-hunter-key"
	cfg.Screenshot.Engine = "cdp"
	cfg.Screenshot.Mode = "auto"
	cfg.Screenshot.Timeout = 30
	cfg.System.MaxConcurrent = 10
	cfg.System.CacheTTL = 3600
	cfg.Web.CORS.AllowedOrigins = []string{"http://localhost:8448"}
	return &Server{config: cfg}
}

// withAdminContext returns a copy of req with admin-token auth context injected.
func withAdminContext(req *http.Request) *http.Request {
	ctx := context.WithValue(req.Context(), contextKeyUserID, adminSyntheticUserID)
	return req.WithContext(ctx)
}

func TestHandleGetConfig_MasksSecrets(t *testing.T) {
	s := newServerForConfigTest()
	req := withAdminContext(httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	w := httptest.NewRecorder()
	s.handleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", w.Code, w.Body.String())
	}

	var out struct {
		ICP     map[string]interface{}            `json:"icp"`
		Engines map[string]map[string]interface{} `json:"engines"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	icpKey, _ := out.ICP["api_key"].(string)
	if icpKey == "" || icpKey == "abcd1234efgh5678" {
		t.Fatalf("expected masked icp api_key, got %q", icpKey)
	}
	if !strings.Contains(icpKey, "****") {
		t.Fatalf("expected '****' in masked icp api_key, got %q", icpKey)
	}

	fofaKey, _ := out.Engines["fofa"]["api_key"].(string)
	if fofaKey == "secret-fofa-key" {
		t.Fatalf("fofa api_key should be masked, got %q", fofaKey)
	}
	if !strings.Contains(fofaKey, "****") {
		t.Fatalf("expected '****' in masked fofa api_key, got %q", fofaKey)
	}
}

func TestICPConfigProviderUsesCommittedConfig(t *testing.T) {
	s := newServerForConfigTest()
	mgr := config.NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	mgr.SetConfig(s.config.Clone())
	s.configManager = mgr

	if _, err := s.updateConfig(func(cfg *config.Config) error {
		cfg.ICP.APIKey = "new-scheduler-key"
		cfg.ICP.DefaultType = "domain"
		return nil
	}); err != nil {
		t.Fatalf("commit ICP config: %v", err)
	}
	got := s.icpConfigProvider()
	if got.APIKey != "new-scheduler-key" || got.DefaultType != "domain" {
		t.Fatalf("scheduler provider returned stale ICP config: %#v", got)
	}
}

func TestHandleGetConfig_RejectsNonGET(t *testing.T) {
	s := newServerForConfigTest()
	req := withAdminContext(httptest.NewRequest(http.MethodPost, "/api/v1/config", nil))
	w := httptest.NewRecorder()
	s.handleGetConfig(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func postConfig(t *testing.T, s *Server, payload map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config", bytes.NewReader(body))
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	req.Header.Set("Content-Type", "application/json")
	req = withAdminContext(req)
	w := httptest.NewRecorder()
	s.handleSaveConfig(w, req)
	return w
}

func TestHandleSaveConfig_MissingSection(t *testing.T) {
	s := newServerForConfigTest()
	w := postConfig(t, s, map[string]interface{}{"section": "", "data": map[string]interface{}{}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "missing_section") {
		t.Fatalf("expected missing_section in body, got %q", w.Body.String())
	}
}

func TestHandleSaveConfig_UnsupportedSection(t *testing.T) {
	s := newServerForConfigTest()
	w := postConfig(t, s, map[string]interface{}{"section": "bogus", "data": map[string]interface{}{}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "unsupported_section") {
		t.Fatalf("expected unsupported_section in body, got %q", w.Body.String())
	}
}

func TestHandleSaveConfig_ICPSection_UpdatesFields(t *testing.T) {
	s := newServerForConfigTest()
	originalKey := s.config.ICP.APIKey
	w := postConfig(t, s, map[string]interface{}{
		"section": "icp",
		"data": map[string]interface{}{
			"enabled":      false,
			"base_url":     "http://example:18888",
			"timeout":      60,
			"default_type": "app",
			"api_key":      "", // empty → should NOT overwrite real key
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", w.Code, w.Body.String())
	}
	cfg := s.currentConfig()
	if cfg.ICP.Enabled {
		t.Fatalf("expected ICP.Enabled=false after save")
	}
	if cfg.ICP.BaseURL != "http://example:18888" {
		t.Fatalf("expected new base_url, got %q", cfg.ICP.BaseURL)
	}
	if cfg.ICP.Timeout != 60 {
		t.Fatalf("expected new timeout=60, got %d", cfg.ICP.Timeout)
	}
	if cfg.ICP.DefaultType != "app" {
		t.Fatalf("expected default_type=app, got %q", cfg.ICP.DefaultType)
	}
	if cfg.ICP.APIKey != originalKey {
		t.Fatalf("api_key was overwritten by empty value: now=%q want=%q", cfg.ICP.APIKey, originalKey)
	}
}

func TestHandleSaveConfig_ICPSection_RealAPIKeyUpdates(t *testing.T) {
	s := newServerForConfigTest()
	w := postConfig(t, s, map[string]interface{}{
		"section": "icp",
		"data":    map[string]interface{}{"api_key": "new-real-key"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := s.currentConfig().ICP.APIKey; got != "new-real-key" {
		t.Fatalf("expected api_key updated to new-real-key, got %q", got)
	}
}

func TestHandleSaveConfig_ICPSection_IgnoresMaskedSecret(t *testing.T) {
	s := newServerForConfigTest()
	original := s.config.ICP.APIKey
	w := postConfig(t, s, map[string]interface{}{
		"section": "icp",
		"data":    map[string]interface{}{"api_key": "abcd****5678"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if s.config.ICP.APIKey != original {
		t.Fatalf("masked secret was incorrectly accepted: now=%q want=%q", s.config.ICP.APIKey, original)
	}
}

func TestHandleSaveConfig_ScreenshotSection(t *testing.T) {
	s := newServerForConfigTest()
	w := postConfig(t, s, map[string]interface{}{
		"section": "screenshot",
		"data":    map[string]interface{}{"engine": "extension", "mode": "auto", "timeout": 45},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", w.Code, w.Body.String())
	}
	cfg := s.currentConfig()
	if cfg.Screenshot.Engine != "extension" {
		t.Fatalf("expected engine=extension, got %q", cfg.Screenshot.Engine)
	}
	if cfg.Screenshot.Timeout != 45 {
		t.Fatalf("expected timeout=45, got %d", cfg.Screenshot.Timeout)
	}
}

func TestHandleSaveConfig_RejectsInvalidScreenshotMode(t *testing.T) {
	s := newServerForConfigTest()
	mgr := config.NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	mgr.SetConfig(s.config.Clone())
	s.configManager = mgr
	w := postConfig(t, s, map[string]interface{}{
		"section": "screenshot",
		"data":    map[string]interface{}{"mode": "not-a-mode"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if s.config.Screenshot.Mode != "auto" {
		t.Fatalf("invalid candidate was published: %q", s.config.Screenshot.Mode)
	}
}

func TestHandleSaveConfig_PersistenceFailureDoesNotPublish(t *testing.T) {
	s := newServerForConfigTest()
	path := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := config.NewManager(filepath.Join(path, "config.yaml"))
	mgr.SetConfig(s.config.Clone())
	s.configManager = mgr
	w := postConfig(t, s, map[string]interface{}{"section": "system", "data": map[string]interface{}{"cache_ttl": 7200}})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if mgr.GetConfig().System.CacheTTL != 3600 || s.currentConfig().System.CacheTTL != 3600 {
		t.Fatal("candidate was published after persistence failure")
	}
}

func TestHandleSaveConfig_SystemSection(t *testing.T) {
	s := newServerForConfigTest()
	w := postConfig(t, s, map[string]interface{}{
		"section": "system",
		"data":    map[string]interface{}{"max_concurrent": 50, "cache_ttl": 7200},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", w.Code, w.Body.String())
	}
	cfg := s.currentConfig()
	if cfg.System.MaxConcurrent != 50 {
		t.Fatalf("expected max_concurrent=50, got %d", cfg.System.MaxConcurrent)
	}
	if cfg.System.CacheTTL != 7200 {
		t.Fatalf("expected cache_ttl=7200, got %d", cfg.System.CacheTTL)
	}
}

func TestUpdateConfigSerializesConcurrentWriters(t *testing.T) {
	s := newServerForConfigTest()
	path := filepath.Join(t.TempDir(), "config.yaml")
	mgr := config.NewManager(path)
	mgr.SetConfig(s.config.Clone())
	s.configManager = mgr

	var wg sync.WaitGroup
	for _, mutate := range []func(*config.Config){
		func(cfg *config.Config) { cfg.System.CacheTTL = 7200 },
		func(cfg *config.Config) { cfg.System.MaxConcurrent = 42 },
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.updateConfig(func(cfg *config.Config) error { mutate(cfg); return nil }); err != nil {
				t.Errorf("concurrent update failed: %v", err)
			}
		}()
	}
	wg.Wait()

	reloaded := config.NewManager(path)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	cfg := reloaded.GetConfig()
	if cfg.System.CacheTTL != 7200 || cfg.System.MaxConcurrent != 42 {
		t.Fatalf("concurrent updates overwrote each other: ttl=%d max=%d", cfg.System.CacheTTL, cfg.System.MaxConcurrent)
	}
}

func TestHandleSaveConfig_RejectsUntrustedOrigin(t *testing.T) {
	s := newServerForConfigTest()
	body, _ := json.Marshal(map[string]interface{}{"section": "icp", "data": map[string]interface{}{}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config", bytes.NewReader(body))
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Content-Type", "application/json")
	req = withAdminContext(req)
	w := httptest.NewRecorder()
	s.handleSaveConfig(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for untrusted origin, got %d (body=%q)", w.Code, w.Body.String())
	}
}

func TestApplyICPSection_ZeroTimeoutKept(t *testing.T) {
	cfg := &config.Config{}
	cfg.ICP.Timeout = 30
	applyICPSection(cfg, map[string]interface{}{"timeout": float64(0)})
	if cfg.ICP.Timeout != 30 {
		t.Fatalf("expected timeout unchanged when set to 0, got %d", cfg.ICP.Timeout)
	}
}

func TestIsMaskedSecret(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"abc1234567890def", false},
		{"abc1****0def", true},   // matches maskAPIKey output (4+****+4)
		{"****", true},           // pure asterisks still counted as masked
		{"abc****def", false},    // 3-char prefix doesn't match maskAPIKey format
		{"mykey****real", false}, // real key containing **** should not be rejected
	}
	for _, tc := range tests {
		got := isMaskedSecret(tc.in)
		if got != tc.want {
			t.Errorf("isMaskedSecret(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestApplyEngineSections_NilConfig(t *testing.T) {
	applyEngineSections(nil, map[string]interface{}{"fofa": map[string]interface{}{}})
	// Should not panic
}

func TestApplyEngineSections_EmptyData(t *testing.T) {
	cfg := &config.Config{}
	applyEngineSections(cfg, map[string]interface{}{})
	// Should not panic
}

func TestApplyEngineSections_InvalidEngineType(t *testing.T) {
	cfg := &config.Config{}
	applyEngineSections(cfg, map[string]interface{}{"fofa": "not-a-map"})
	// Should not panic
}

func TestApplySingleEngineSection_Fofa(t *testing.T) {
	cfg := &config.Config{}
	eng := map[string]interface{}{
		"enabled":      true,
		"api_key":      "new-fofa-key",
		"api_base_url": "https://fofa.example.com",
		"email":        "test@example.com",
		"qps":          float64(5),
		"timeout":      float64(60),
	}
	applySingleEngineSection(cfg, "fofa", eng)
	if !cfg.Engines.Fofa.Enabled {
		t.Fatal("expected Fofa.Enabled=true")
	}
	if cfg.Engines.Fofa.APIKey != "new-fofa-key" {
		t.Fatalf("expected Fofa.APIKey='new-fofa-key', got %q", cfg.Engines.Fofa.APIKey)
	}
	if cfg.Engines.Fofa.APIBaseURL != "https://fofa.example.com" {
		t.Fatalf("expected Fofa.APIBaseURL='https://fofa.example.com', got %q", cfg.Engines.Fofa.APIBaseURL)
	}
	if cfg.Engines.Fofa.Email != "test@example.com" {
		t.Fatalf("expected Fofa.Email='test@example.com', got %q", cfg.Engines.Fofa.Email)
	}
	if cfg.Engines.Fofa.QPS != 5 {
		t.Fatalf("expected Fofa.QPS=5, got %d", cfg.Engines.Fofa.QPS)
	}
	if cfg.Engines.Fofa.Timeout != 60 {
		t.Fatalf("expected Fofa.Timeout=60, got %d", cfg.Engines.Fofa.Timeout)
	}
}

func TestApplySingleEngineSection_Hunter(t *testing.T) {
	cfg := &config.Config{}
	eng := map[string]interface{}{
		"enabled":  true,
		"api_key":  "new-hunter-key",
		"base_url": "https://hunter.example.com",
		"qps":      float64(3),
		"timeout":  float64(45),
	}
	applySingleEngineSection(cfg, "hunter", eng)
	if !cfg.Engines.Hunter.Enabled {
		t.Fatal("expected Hunter.Enabled=true")
	}
	if cfg.Engines.Hunter.APIKey != "new-hunter-key" {
		t.Fatalf("expected Hunter.APIKey='new-hunter-key', got %q", cfg.Engines.Hunter.APIKey)
	}
	if cfg.Engines.Hunter.BaseURL != "https://hunter.example.com" {
		t.Fatalf("expected Hunter.BaseURL, got %q", cfg.Engines.Hunter.BaseURL)
	}
	if cfg.Engines.Hunter.QPS != 3 {
		t.Fatalf("expected Hunter.QPS=3, got %d", cfg.Engines.Hunter.QPS)
	}
	if cfg.Engines.Hunter.Timeout != 45 {
		t.Fatalf("expected Hunter.Timeout=45, got %d", cfg.Engines.Hunter.Timeout)
	}
}

func TestApplySingleEngineSection_Zoomeye(t *testing.T) {
	cfg := &config.Config{}
	eng := map[string]interface{}{
		"enabled":  true,
		"api_key":  "new-zoomeye-key",
		"base_url": "https://zoomeye.example.com",
		"qps":      float64(2),
		"timeout":  float64(30),
	}
	applySingleEngineSection(cfg, "zoomeye", eng)
	if !cfg.Engines.Zoomeye.Enabled {
		t.Fatal("expected Zoomeye.Enabled=true")
	}
	if cfg.Engines.Zoomeye.APIKey != "new-zoomeye-key" {
		t.Fatalf("expected Zoomeye.APIKey='new-zoomeye-key', got %q", cfg.Engines.Zoomeye.APIKey)
	}
	if cfg.Engines.Zoomeye.BaseURL != "https://zoomeye.example.com" {
		t.Fatalf("expected Zoomeye.BaseURL, got %q", cfg.Engines.Zoomeye.BaseURL)
	}
	if cfg.Engines.Zoomeye.QPS != 2 {
		t.Fatalf("expected Zoomeye.QPS=2, got %d", cfg.Engines.Zoomeye.QPS)
	}
	if cfg.Engines.Zoomeye.Timeout != 30 {
		t.Fatalf("expected Zoomeye.Timeout=30, got %d", cfg.Engines.Zoomeye.Timeout)
	}
}

func TestApplySingleEngineSection_Quake(t *testing.T) {
	cfg := &config.Config{}
	eng := map[string]interface{}{
		"enabled":  true,
		"api_key":  "new-quake-key",
		"base_url": "https://quake.example.com",
		"qps":      float64(4),
		"timeout":  float64(50),
	}
	applySingleEngineSection(cfg, "quake", eng)
	if !cfg.Engines.Quake.Enabled {
		t.Fatal("expected Quake.Enabled=true")
	}
	if cfg.Engines.Quake.APIKey != "new-quake-key" {
		t.Fatalf("expected Quake.APIKey='new-quake-key', got %q", cfg.Engines.Quake.APIKey)
	}
	if cfg.Engines.Quake.BaseURL != "https://quake.example.com" {
		t.Fatalf("expected Quake.BaseURL, got %q", cfg.Engines.Quake.BaseURL)
	}
	if cfg.Engines.Quake.QPS != 4 {
		t.Fatalf("expected Quake.QPS=4, got %d", cfg.Engines.Quake.QPS)
	}
	if cfg.Engines.Quake.Timeout != 50 {
		t.Fatalf("expected Quake.Timeout=50, got %d", cfg.Engines.Quake.Timeout)
	}
}

func TestApplySingleEngineSection_Sholdan(t *testing.T) {
	cfg := &config.Config{}
	eng := map[string]interface{}{
		"enabled":  true,
		"api_key":  "new-shodan-key",
		"base_url": "https://shodan.example.com",
		"qps":      float64(1),
	}
	applySingleEngineSection(cfg, "shodan", eng)
	if !cfg.Engines.Shodan.Enabled {
		t.Fatal("expected Shodan.Enabled=true")
	}
	if cfg.Engines.Shodan.APIKey != "new-shodan-key" {
		t.Fatalf("expected Shodan.APIKey='new-shodan-key', got %q", cfg.Engines.Shodan.APIKey)
	}
	if cfg.Engines.Shodan.BaseURL != "https://shodan.example.com" {
		t.Fatalf("expected Shodan.BaseURL, got %q", cfg.Engines.Shodan.BaseURL)
	}
	if cfg.Engines.Shodan.QPS != 1 {
		t.Fatalf("expected Shodan.QPS=1, got %d", cfg.Engines.Shodan.QPS)
	}
}

func TestApplyFofaFields_MaskedKeyIgnored(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engines.Fofa.APIKey = "original-key"
	eng := map[string]interface{}{
		"api_key": "abcd****efgh", // masked (4+****+4)
	}
	applyFofaFields(cfg, eng)
	if cfg.Engines.Fofa.APIKey != "original-key" {
		t.Fatalf("masked key should not overwrite, got %q", cfg.Engines.Fofa.APIKey)
	}
}

func TestApplyFofaFields_EmptyKeyIgnored(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engines.Fofa.APIKey = "original-key"
	eng := map[string]interface{}{
		"api_key": "",
	}
	applyFofaFields(cfg, eng)
	if cfg.Engines.Fofa.APIKey != "original-key" {
		t.Fatalf("empty key should not overwrite, got %q", cfg.Engines.Fofa.APIKey)
	}
}

func TestApplyFofaFields_ZeroQPSIgnored(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engines.Fofa.QPS = 5
	eng := map[string]interface{}{
		"qps": float64(0),
	}
	applyFofaFields(cfg, eng)
	if cfg.Engines.Fofa.QPS != 5 {
		t.Fatalf("zero QPS should not overwrite, got %d", cfg.Engines.Fofa.QPS)
	}
}

func TestApplyEngineSections_AllEngines(t *testing.T) {
	cfg := &config.Config{}
	data := map[string]interface{}{
		"fofa": map[string]interface{}{
			"enabled": true,
			"api_key": "fofa-key",
		},
		"hunter": map[string]interface{}{
			"enabled": true,
			"api_key": "hunter-key",
		},
		"zoomeye": map[string]interface{}{
			"enabled": true,
			"api_key": "zoomeye-key",
		},
		"quake": map[string]interface{}{
			"enabled": true,
			"api_key": "quake-key",
		},
		"shodan": map[string]interface{}{
			"enabled": true,
			"api_key": "shodan-key",
		},
	}
	applyEngineSections(cfg, data)
	if !cfg.Engines.Fofa.Enabled || cfg.Engines.Fofa.APIKey != "fofa-key" {
		t.Fatal("fofa not applied correctly")
	}
	if !cfg.Engines.Hunter.Enabled || cfg.Engines.Hunter.APIKey != "hunter-key" {
		t.Fatal("hunter not applied correctly")
	}
	if !cfg.Engines.Zoomeye.Enabled || cfg.Engines.Zoomeye.APIKey != "zoomeye-key" {
		t.Fatal("zoomeye not applied correctly")
	}
	if !cfg.Engines.Quake.Enabled || cfg.Engines.Quake.APIKey != "quake-key" {
		t.Fatal("quake not applied correctly")
	}
	if !cfg.Engines.Shodan.Enabled || cfg.Engines.Shodan.APIKey != "shodan-key" {
		t.Fatal("shodan not applied correctly")
	}
}

func TestApplySingleEngineSection_UnknownEngine(t *testing.T) {
	cfg := &config.Config{}
	eng := map[string]interface{}{"enabled": true}
	applySingleEngineSection(cfg, "unknown_engine", eng)
	// Should not panic, no effect
}

func TestApplyShodanFields_NoTimeout(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engines.Shodan.Timeout = 30
	eng := map[string]interface{}{
		"timeout": float64(60),
	}
	applyShodanFields(cfg, eng)
	// Shodan doesn't have timeout field - should not be changed
	if cfg.Engines.Shodan.Timeout != 30 {
		t.Fatalf("Shodan timeout should not change, got %d", cfg.Engines.Shodan.Timeout)
	}
}

func TestApplyNotificationsSection_NilConfig(t *testing.T) {
	applyNotificationsSection(nil, map[string]interface{}{})
}

func TestApplyNotificationsSection_Enabled(t *testing.T) {
	cfg := &config.Config{}
	data := map[string]interface{}{
		"enabled": true,
	}
	applyNotificationsSection(cfg, data)
	if !cfg.Notifications.Enabled {
		t.Fatal("expected notifications enabled")
	}
}

func TestApplyNotificationsSection_FeishuApp(t *testing.T) {
	cfg := &config.Config{}
	data := map[string]interface{}{
		"enabled": true,
		"feishu_app": map[string]interface{}{
			"app_id":     "test-app-id",
			"app_secret": "test-secret",
			"chat_id":    "test-chat-id",
		},
	}
	applyNotificationsSection(cfg, data)
	if !cfg.Notifications.Enabled {
		t.Fatal("expected notifications enabled")
	}
	if cfg.Notifications.FeishuApp == nil {
		t.Fatal("expected feishu_app to be initialized")
	}
	if cfg.Notifications.FeishuApp.AppID != "test-app-id" {
		t.Fatalf("expected app_id test-app-id, got %s", cfg.Notifications.FeishuApp.AppID)
	}
	if cfg.Notifications.FeishuApp.AppSecret != "test-secret" {
		t.Fatalf("expected app_secret test-secret, got %s", cfg.Notifications.FeishuApp.AppSecret)
	}
	if cfg.Notifications.FeishuApp.ChatID != "test-chat-id" {
		t.Fatalf("expected chat_id test-chat-id, got %s", cfg.Notifications.FeishuApp.ChatID)
	}
}

func TestApplyNotificationsSection_PreserveMaskedSecret(t *testing.T) {
	cfg := &config.Config{}
	cfg.Notifications.FeishuApp = &struct {
		AppID     string `yaml:"app_id"`
		AppSecret string `yaml:"app_secret"`
		ChatID    string `yaml:"chat_id"`
	}{AppSecret: "old-secret"}

	data := map[string]interface{}{
		"feishu_app": map[string]interface{}{
			"app_secret": "********",
		},
	}
	applyNotificationsSection(cfg, data)
	if cfg.Notifications.FeishuApp.AppSecret != "old-secret" {
		t.Fatalf("expected secret preserved, got %s", cfg.Notifications.FeishuApp.AppSecret)
	}
}

func TestApplyNotificationsSection_InvalidFeishuAppType(t *testing.T) {
	cfg := &config.Config{}
	data := map[string]interface{}{
		"feishu_app": "not-a-map",
	}
	applyNotificationsSection(cfg, data)
	if cfg.Notifications.FeishuApp != nil {
		t.Fatal("feishu_app should not be initialized for invalid type")
	}
}
