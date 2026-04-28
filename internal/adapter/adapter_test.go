package adapter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// ===== quakeIsSuccessCode =====

func TestQuakeIsSuccessCode(t *testing.T) {
	tests := []struct {
		name string
		code interface{}
		want bool
	}{
		{"nil", nil, true},
		{"zero int", 0, true},
		{"200 int", 200, true},
		{"400 int", 400, false},
		{"zero int64", int64(0), true},
		{"200 int64", int64(200), true},
		{"400 int64", int64(400), false},
		{"zero float64", float64(0), true},
		{"200 float64", float64(200), true},
		{"400 float64", float64(400), false},
		{"string 0", "0", true},
		{"string 200", "200", true},
		{"string success", "success", true},
		{"string SUCCESS", "SUCCESS", true},
		{"string 400", "400", false},
		{"string whitespace 0", "  0  ", true},
		{"bool type", true, false},
		{"struct type", struct{}{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quakeIsSuccessCode(tt.code)
			if got != tt.want {
				t.Errorf("quakeIsSuccessCode(%v) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

// ===== WebOnlyAdapterBase =====

func TestWebOnlyAdapterBase_Name(t *testing.T) {
	adapter := &mockAdapter{name: "fofa"}
	base := NewWebOnlyAdapterBase(adapter, "fofa-web")
	if got := base.Name(); got != "fofa-web" {
		t.Errorf("Name() = %q, want %q", got, "fofa-web")
	}
}

func TestWebOnlyAdapterBase_Translate(t *testing.T) {
	base := NewWebOnlyAdapterBase(&mockAdapter{}, "test")
	_, err := base.Translate(&model.UQLAST{})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestWebOnlyAdapterBase_Search(t *testing.T) {
	base := NewWebOnlyAdapterBase(&mockAdapter{}, "test")
	_, err := base.Search("test", 1, 10)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestWebOnlyAdapterBase_Normalize(t *testing.T) {
	base := NewWebOnlyAdapterBase(&mockAdapter{}, "test")
	results, err := base.Normalize(&model.EngineResult{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice, got %d items", len(results))
	}
}

func TestWebOnlyAdapterBase_GetQuota(t *testing.T) {
	base := NewWebOnlyAdapterBase(&mockAdapter{}, "test")
	_, err := base.GetQuota()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestWebOnlyAdapterBase_IsWebOnly(t *testing.T) {
	base := NewWebOnlyAdapterBase(&mockAdapter{}, "test")
	if !base.IsWebOnly() {
		t.Error("expected IsWebOnly() = true")
	}
}

// ===== FOFA Adapter: Translate =====

func TestFofaAdapter_Translate(t *testing.T) {
	a := NewFofaAdapter("https://fofa.info", "key", "email@test.com", 3, 30*time.Second)

	tests := []struct {
		name    string
		ast     *model.UQLAST
		want    string
		wantErr bool
	}{
		{
			name:    "nil AST",
			ast:     nil,
			wantErr: true,
		},
		{
			name:    "nil root",
			ast:     &model.UQLAST{Root: nil},
			wantErr: true,
		},
		{
			name: "simple condition",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "80"},
				},
			}},
			want: `port="80"`,
		},
		{
			name: "not equal condition",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "country",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "!="},
					{Type: "value", Value: "CN"},
				},
			}},
			want: `country!="CN"`,
		},
		{
			name: "AND logical",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "logical",
				Value: "AND",
				Children: []*model.UQLNode{
					{Type: "condition", Value: "port", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "80"},
					}},
					{Type: "condition", Value: "ip", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "1.2.3.4"},
					}},
				},
			}},
			want: `(port="80" && ip="1.2.3.4")`,
		},
		{
			name: "OR logical",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "logical",
				Value: "OR",
				Children: []*model.UQLNode{
					{Type: "condition", Value: "protocol", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "http"},
					}},
					{Type: "condition", Value: "protocol", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "https"},
					}},
				},
			}},
			want: `(protocol="http" || protocol="https")`,
		},
		{
			name: "IN operator",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "IN"},
					{Type: "value", Value: "80,443,8080"},
				},
			}},
			want: `(port="80" || port="443" || port="8080")`,
		},
		{
			name: "unknown field passthrough",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "unknown_field",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "test"},
				},
			}},
			want: `unknown_field="test"`,
		},
		{
			name: "nested logical",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "logical",
				Value: "AND",
				Children: []*model.UQLNode{
					{Type: "logical", Value: "OR", Children: []*model.UQLNode{
						{Type: "condition", Value: "port", Children: []*model.UQLNode{
							{Type: "operator", Value: "="},
							{Type: "value", Value: "80"},
						}},
						{Type: "condition", Value: "port", Children: []*model.UQLNode{
							{Type: "operator", Value: "="},
							{Type: "value", Value: "443"},
						}},
					}},
					{Type: "condition", Value: "country", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "CN"},
					}},
				},
			}},
			want: `((port="80" || port="443") && country="CN")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.Translate(tt.ast)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ===== FOFA Adapter: Name, IsWebOnly =====

func TestFofaAdapter_Name(t *testing.T) {
	a := NewFofaAdapter("https://fofa.info", "key", "email", 3, 30*time.Second)
	if got := a.Name(); got != "fofa" {
		t.Errorf("Name() = %q, want %q", got, "fofa")
	}
}

func TestFofaAdapter_IsWebOnly(t *testing.T) {
	a := NewFofaAdapter("https://fofa.info", "key", "email", 3, 30*time.Second)
	if a.IsWebOnly() {
		t.Error("expected IsWebOnly() = false")
	}
}

// ===== FOFA Adapter: Normalize =====

func TestFofaAdapter_Normalize(t *testing.T) {
	a := NewFofaAdapter("https://fofa.info", "key", "email", 3, 30*time.Second)

	t.Run("empty result", func(t *testing.T) {
		results, err := a.Normalize(&model.EngineResult{RawData: []interface{}{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 assets, got %d", len(results))
		}
	})

	t.Run("full fields", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":     "1.2.3.4",
				"port":   float64(80),
				"server": "http",
				"domain": "example.com",
				"title":  "Example",
				"header": "Server: nginx",
				"region": "Beijing",
				"city":   "Beijing",
				"isp":    "China Telecom",
			},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected 1 asset, got %d", len(assets))
		}
		if assets[0].IP != "1.2.3.4" {
			t.Errorf("IP = %q, want %q", assets[0].IP, "1.2.3.4")
		}
		if assets[0].Port != 80 {
			t.Errorf("Port = %d, want 80", assets[0].Port)
		}
		if assets[0].Protocol != "http" {
			t.Errorf("Protocol = %q, want %q", assets[0].Protocol, "http")
		}
		if assets[0].Title != "Example" {
			t.Errorf("Title = %q, want %q", assets[0].Title, "Example")
		}
		if assets[0].Region != "Beijing" {
			t.Errorf("Region = %q, want %q", assets[0].Region, "Beijing")
		}
	})

	t.Run("port as int", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":   "1.2.3.4",
				"port": int(443),
			},
		}}
		results, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 || results[0].Port != 443 {
			t.Errorf("Port = %d, want 443", results[0].Port)
		}
	})

	t.Run("no ip skipped", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"port": float64(80),
			},
		}}
		results, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 assets (no ip), got %d", len(results))
		}
	})

	t.Run("body truncation", func(t *testing.T) {
		body := strings.Repeat("x", 500)
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":   "1.2.3.4",
				"port": float64(80),
				"body": body,
			},
		}}
		results, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results[0].BodySnippet) > 200 {
			t.Errorf("BodySnippet too long: %d chars", len(results[0].BodySnippet))
		}
	})
}

// ===== FOFA Adapter: Search =====

func TestFofaAdapter_Search(t *testing.T) {
	t.Run("empty api key returns error result", func(t *testing.T) {
		a := NewFofaAdapter("https://fofa.info", "", "", 3, 30*time.Second)
		result, err := a.Search("test", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error == "" {
			t.Error("expected error in result, got empty")
		}
	})

	t.Run("successful search", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"results":[["1.2.3.4",80,"http","example.com","Title","nginx","header","CN","Beijing","Beijing","AS123","Org","ISP",200]],"size":1,"full":10}`))
		}))
		defer server.Close()

		a := NewFofaAdapter(server.URL, "key", "email@test.com", 3, 30*time.Second)
		result, err := a.Search("port=80", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error != "" {
			t.Fatalf("expected success, got: %s", result.Error)
		}
		if len(result.RawData) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result.RawData))
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			w.Write([]byte("Internal Server Error"))
		}))
		defer server.Close()

		a := NewFofaAdapter(server.URL, "key", "email@test.com", 3, 30*time.Second)
		result, err := a.Search("test", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error == "" {
			t.Error("expected error result for HTTP 500")
		}
	})
}

// ===== FOFA Adapter: GetQuota =====

func TestFofaAdapter_GetQuota(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewFofaAdapter("https://fofa.info", "", "", 3, 30*time.Second)
		_, err := a.GetQuota()
		if err == nil {
			t.Error("expected error for empty API key")
		}
	})

	t.Run("successful quota fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"fofacli_ver":1,"fofahv":1,"username":"test","isvip":false,"coin":100,"remain_free_point":50,"remain_coin_point":50,"total_coin":200,"expiry_time":"2026-12-31"}`))
		}))
		defer server.Close()

		a := NewFofaAdapter(server.URL, "key", "email@test.com", 3, 30*time.Second)
		quota, err := a.GetQuota()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quota == nil {
			t.Fatal("expected quota info, got nil")
		}
	})
}

// ===== Hunter Adapter: Translate =====

func TestHunterAdapter_Translate(t *testing.T) {
	a := NewHunterAdapter("https://hunter.io", "key", 3, 30*time.Second)

	tests := []struct {
		name    string
		ast     *model.UQLAST
		want    string
		wantErr bool
	}{
		{
			name:    "nil AST",
			ast:     nil,
			wantErr: true,
		},
		{
			name: "simple condition",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "80"},
				},
			}},
			want: `port="80"`,
		},
		{
			name: "not equal",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "country",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "!="},
					{Type: "value", Value: "CN"},
				},
			}},
			want: `ip.country!="CN"`,
		},
		{
			name: "AND",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "logical",
				Value: "AND",
				Children: []*model.UQLNode{
					{Type: "condition", Value: "port", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "80"},
					}},
					{Type: "condition", Value: "ip", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "1.2.3.4"},
					}},
				},
			}},
			want: `(port="80" AND ip="1.2.3.4")`,
		},
		{
			name: "IN operator",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "protocol",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "IN"},
					{Type: "value", Value: "http,https"},
				},
			}},
			want: `(protocol="http" OR protocol="https")`,
		},
		{
			name: "field mapping port",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "443"},
				},
			}},
			want: `port="443"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.Translate(tt.ast)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHunterAdapter_Name(t *testing.T) {
	a := NewHunterAdapter("https://hunter.io", "key", 3, 30*time.Second)
	if got := a.Name(); got != "hunter" {
		t.Errorf("Name() = %q, want %q", got, "hunter")
	}
}

func TestHunterAdapter_IsWebOnly(t *testing.T) {
	a := NewHunterAdapter("https://hunter.io", "key", 3, 30*time.Second)
	if a.IsWebOnly() {
		t.Error("expected IsWebOnly() = false")
	}
}

// ===== Hunter Adapter: Normalize =====

func TestHunterAdapter_Normalize(t *testing.T) {
	a := NewHunterAdapter("https://hunter.io", "key", 3, 30*time.Second)

	t.Run("empty result", func(t *testing.T) {
		results, err := a.Normalize(&model.EngineResult{RawData: []interface{}{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 assets, got %d", len(results))
		}
	})

	t.Run("flat data format", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":       "1.2.3.4",
				"port":     float64(80),
				"protocol": "http",
				"domain":   "example.com",
				"title":    "Example",
				"country":  "CN",
				"province": "Beijing",
				"city":     "Beijing",
				"isp":      "China Telecom",
			},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected 1 asset, got %d", len(assets))
		}
		if assets[0].IP != "1.2.3.4" {
			t.Errorf("IP = %q, want %q", assets[0].IP, "1.2.3.4")
		}
		if assets[0].Port != 80 {
			t.Errorf("Port = %d, want 80", assets[0].Port)
		}
	})

	t.Run("nested data format", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":       "5.6.7.8",
				"port":     float64(443),
				"protocol": "https",
				"web_title": "Secure Site",
				"country":  "China",
				"province": "Shanghai",
				"city":     "Shanghai",
			},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected 1 asset, got %d", len(assets))
		}
		if assets[0].Protocol != "https" {
			t.Errorf("Protocol = %q, want %q", assets[0].Protocol, "https")
		}
		if assets[0].CountryCode != "China" {
			t.Errorf("CountryCode = %q, want %q", assets[0].CountryCode, "China")
		}
	})
}

// ===== Hunter Adapter: Search =====

func TestHunterAdapter_Search(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewHunterAdapter("https://hunter.io", "", 3, 30*time.Second)
		result, err := a.Search("test", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error == "" {
			t.Error("expected error in result for empty API key")
		}
	})

	t.Run("successful search", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"code":200,"data":{"list":[{"ip":"1.2.3.4","port":80,"protocol":"http","domain":"example.com"}],"total":1,"consume_quota":1,"rest_quota":"100/100"}}`))
		}))
		defer server.Close()

		a := NewHunterAdapter(server.URL, "key", 3, 30*time.Second)
		result, err := a.Search("port=80", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error != "" {
			t.Fatalf("expected success, got: %s", result.Error)
		}
		if result.Total != 1 {
			t.Errorf("Total = %d, want 1", result.Total)
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
		}))
		defer server.Close()

		a := NewHunterAdapter(server.URL, "key", 3, 30*time.Second)
		result, err := a.Search("test", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error == "" {
			t.Error("expected error result for HTTP 500")
		}
	})
}

// ===== Hunter Adapter: GetQuota =====

func TestHunterAdapter_GetQuota(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewHunterAdapter("https://hunter.io", "", 3, 30*time.Second)
		_, err := a.GetQuota()
		if err == nil {
			t.Error("expected error for empty API key")
		}
	})

	t.Run("successful quota", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"code":200,"data":{"rest_quota":"50/100","consume_quota":50}}`))
		}))
		defer server.Close()

		a := NewHunterAdapter(server.URL, "key", 3, 30*time.Second)
		quota, err := a.GetQuota()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quota == nil {
			t.Fatal("expected quota info, got nil")
		}
	})
}

// ===== Shodan Adapter: Translate =====

func TestShodanAdapter_Translate(t *testing.T) {
	a := NewShodanAdapter("https://api.shodan.io", "key", 3, 30*time.Second)

	tests := []struct {
		name    string
		ast     *model.UQLAST
		want    string
		wantErr bool
	}{
		{
			name:    "nil AST",
			ast:     nil,
			wantErr: true,
		},
		{
			name: "simple condition",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "80"},
				},
			}},
			want: `port:80`,
		},
		{
			name: "not equal",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "country",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "!="},
					{Type: "value", Value: "CN"},
				},
			}},
			want: `-country:CN`,
		},
		{
			name: "AND logical",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "logical",
				Value: "AND",
				Children: []*model.UQLNode{
					{Type: "condition", Value: "port", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "80"},
					}},
					{Type: "condition", Value: "country", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "US"},
					}},
				},
			}},
			want: `(port:80 AND country:US)`,
		},
		{
			name: "IN operator",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "IN"},
					{Type: "value", Value: "80,443"},
				},
			}},
			want: `(port:80 OR port:443)`,
		},
		{
			name: "field mapping port",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "443"},
				},
			}},
			want: `port:443`,
		},
		{
			name: "field mapping title",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "title",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "nginx"},
				},
			}},
			want: `title:nginx`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.Translate(tt.ast)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShodanAdapter_Name(t *testing.T) {
	a := NewShodanAdapter("https://api.shodan.io", "key", 3, 30*time.Second)
	if got := a.Name(); got != "shodan" {
		t.Errorf("Name() = %q, want %q", got, "shodan")
	}
}

func TestShodanAdapter_IsWebOnly(t *testing.T) {
	a := NewShodanAdapter("https://api.shodan.io", "key", 3, 30*time.Second)
	if a.IsWebOnly() {
		t.Error("expected IsWebOnly() = false")
	}
}

// ===== Shodan Adapter: Normalize =====

func TestShodanAdapter_Normalize(t *testing.T) {
	a := NewShodanAdapter("https://api.shodan.io", "key", 3, 30*time.Second)

	t.Run("full fields", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":          "1.2.3.4",
				"port":        float64(80),
				"transport":   "tcp",
				"product":     "nginx",
				"title":       "Example",
				"country_name": "United States",
				"city":        "San Francisco",
				"org":         "Cloudflare",
				"hostnames":   []interface{}{"example.com"},
			},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected 1 asset, got %d", len(assets))
		}
		if assets[0].IP != "1.2.3.4" {
			t.Errorf("IP = %q, want %q", assets[0].IP, "1.2.3.4")
		}
		if assets[0].Port != 80 {
			t.Errorf("Port = %d, want 80", assets[0].Port)
		}
		if assets[0].Title != "Example" {
			t.Errorf("Title = %q, want %q", assets[0].Title, "Example")
		}
	})
}

// ===== Shodan Adapter: Search =====

func TestShodanAdapter_Search(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewShodanAdapter("https://api.shodan.io", "", 3, 30*time.Second)
		result, err := a.Search("test", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error == "" {
			t.Error("expected error in result for empty API key")
		}
	})

	t.Run("successful search", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"total":1,"matches":[{"ip_str":"1.2.3.4","port":80}]}`))
		}))
		defer server.Close()

		a := NewShodanAdapter(server.URL, "key", 3, 30*time.Second)
		result, err := a.Search("port:80", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error != "" {
			t.Fatalf("expected success, got: %s", result.Error)
		}
		if len(result.RawData) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result.RawData))
		}
	})
}

// ===== Shodan Adapter: GetQuota =====

func TestShodanAdapter_GetQuota(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewShodanAdapter("https://api.shodan.io", "", 3, 30*time.Second)
		_, err := a.GetQuota()
		if err == nil {
			t.Error("expected error for empty API key")
		}
	})

	t.Run("successful quota", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"scan_credits":50}`))
		}))
		defer server.Close()

		a := NewShodanAdapter(server.URL, "key", 3, 30*time.Second)
		quota, err := a.GetQuota()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quota == nil {
			t.Fatal("expected quota info, got nil")
		}
	})
}

// ===== Quake Adapter: Translate =====

func TestQuakeAdapter_Translate(t *testing.T) {
	a := NewQuakeAdapter("https://quake.io", "key", 3, 30*time.Second)

	tests := []struct {
		name    string
		ast     *model.UQLAST
		want    string
		wantErr bool
	}{
		{
			name:    "nil AST",
			ast:     nil,
			wantErr: true,
		},
		{
			name: "simple condition",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "80"},
				},
			}},
			want: `port:"80"`,
		},
		{
			name: "not equal",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "country",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "!="},
					{Type: "value", Value: "CN"},
				},
			}},
			want: `NOT country:"CN"`,
		},
		{
			name: "AND logical",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "logical",
				Value: "AND",
				Children: []*model.UQLNode{
					{Type: "condition", Value: "port", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "80"},
					}},
					{Type: "condition", Value: "ip", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "1.2.3.4"},
					}},
				},
			}},
			want: `(port:"80" AND ip:"1.2.3.4")`,
		},
		{
			name: "IN operator",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "IN"},
					{Type: "value", Value: "80,443"},
				},
			}},
			want: `(port:"80" OR port:"443")`,
		},
		{
			name: "field mapping body->response",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "body",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "nginx"},
				},
			}},
			want: `response:"nginx"`,
		},
		{
			name: "field mapping header->headers",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "header",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "nginx"},
				},
			}},
			want: `headers:"nginx"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.Translate(tt.ast)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuakeAdapter_Name(t *testing.T) {
	a := NewQuakeAdapter("https://quake.io", "key", 3, 30*time.Second)
	if got := a.Name(); got != "quake" {
		t.Errorf("Name() = %q, want %q", got, "quake")
	}
}

func TestQuakeAdapter_IsWebOnly(t *testing.T) {
	a := NewQuakeAdapter("https://quake.io", "key", 3, 30*time.Second)
	if a.IsWebOnly() {
		t.Error("expected IsWebOnly() = false")
	}
}

// ===== Quake Adapter: Normalize =====

func TestQuakeAdapter_Normalize(t *testing.T) {
	a := NewQuakeAdapter("https://quake.io", "key", 3, 30*time.Second)

	t.Run("full fields", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":   "1.2.3.4",
				"port": float64(80),
				"service": map[string]interface{}{
					"name": "http",
					"http": map[string]interface{}{
						"title":       "Example",
						"server":      "nginx",
						"status_code": float64(200),
					},
				},
				"location": map[string]interface{}{
					"country_code": "CN",
					"city_cn":      "Beijing",
					"province_cn":  "Beijing",
				},
			},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected 1 asset, got %d", len(assets))
		}
		if assets[0].IP != "1.2.3.4" {
			t.Errorf("IP = %q, want %q", assets[0].IP, "1.2.3.4")
		}
		if assets[0].Port != 80 {
			t.Errorf("Port = %d, want 80", assets[0].Port)
		}
		if assets[0].Protocol != "http" {
			t.Errorf("Protocol = %q, want %q", assets[0].Protocol, "http")
		}
		if assets[0].Title != "Example" {
			t.Errorf("Title = %q, want %q", assets[0].Title, "Example")
		}
		if assets[0].Server != "nginx" {
			t.Errorf("Server = %q, want %q", assets[0].Server, "nginx")
		}
		if assets[0].StatusCode != 200 {
			t.Errorf("StatusCode = %d, want 200", assets[0].StatusCode)
		}
		if assets[0].CountryCode != "CN" {
			t.Errorf("CountryCode = %q, want %q", assets[0].CountryCode, "CN")
		}
	})

	t.Run("no ip skipped", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{"port": float64(80)},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 0 {
			t.Errorf("expected 0 assets, got %d", len(assets))
		}
	})
}

// ===== Quake Adapter: Search =====

func TestQuakeAdapter_Search(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewQuakeAdapter("https://quake.io", "", 3, 30*time.Second)
		result, err := a.Search("test", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error == "" {
			t.Error("expected error in result for empty API key")
		}
	})

	t.Run("successful search", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"code":0,"message":"success","data":[{"ip":"1.2.3.4","port":80}],"meta":{"pagination":{"total":1,"count":1}}}`))
		}))
		defer server.Close()

		a := NewQuakeAdapter(server.URL, "key", 3, 30*time.Second)
		result, err := a.Search("port:80", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error != "" {
			t.Fatalf("expected success, got: %s", result.Error)
		}
		if len(result.RawData) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result.RawData))
		}
		if result.Total != 1 {
			t.Errorf("Total = %d, want 1", result.Total)
		}
	})
}

// ===== Quake Adapter: GetQuota =====

func TestQuakeAdapter_GetQuota(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewQuakeAdapter("https://quake.io", "", 3, 30*time.Second)
		_, err := a.GetQuota()
		if err == nil {
			t.Error("expected error for empty API key")
		}
	})

	t.Run("successful quota with credit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"code":0,"message":"success","data":{"credit":100,"month_remaining_credit":50}}`))
		}))
		defer server.Close()

		a := NewQuakeAdapter(server.URL, "key", 3, 30*time.Second)
		quota, err := a.GetQuota()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quota == nil {
			t.Fatal("expected quota info, got nil")
		}
		if quota.Total != 100 || quota.Remaining != 50 || quota.Used != 50 {
			t.Errorf("quota: Total=%d, Remaining=%d, Used=%d; want 100, 50, 50", quota.Total, quota.Remaining, quota.Used)
		}
	})
}

// ===== ZoomEye Adapter: Translate =====

func TestZoomEyeAdapter_Translate(t *testing.T) {
	a := NewZoomEyeAdapter("https://api.zoomeye.org", "key", 3, 30*time.Second)

	tests := []struct {
		name    string
		ast     *model.UQLAST
		want    string
		wantErr bool
	}{
		{
			name:    "nil AST",
			ast:     nil,
			wantErr: true,
		},
		{
			name: "simple condition",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "="},
					{Type: "value", Value: "80"},
				},
			}},
			want: `+port:"80"`,
		},
		{
			name: "not equal",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "country",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "!="},
					{Type: "value", Value: "CN"},
				},
			}},
			want: `-country:"CN"`,
		},
		{
			name: "AND logical",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "logical",
				Value: "AND",
				Children: []*model.UQLNode{
					{Type: "condition", Value: "port", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "80"},
					}},
					{Type: "condition", Value: "ip", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "1.2.3.4"},
					}},
				},
			}},
			want: `++port:"80" ++ip:"1.2.3.4"`,
		},
		{
			name: "OR logical",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "logical",
				Value: "OR",
				Children: []*model.UQLNode{
					{Type: "condition", Value: "port", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "80"},
					}},
					{Type: "condition", Value: "port", Children: []*model.UQLNode{
						{Type: "operator", Value: "="},
						{Type: "value", Value: "443"},
					}},
				},
			}},
			want: `+port:"80" +port:"443"`,
		},
		{
			name: "IN operator",
			ast: &model.UQLAST{Root: &model.UQLNode{
				Type: "condition",
				Value: "port",
				Children: []*model.UQLNode{
					{Type: "operator", Value: "IN"},
					{Type: "value", Value: "80,443"},
				},
			}},
			want: `(+port:"80" +port:"443")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.Translate(tt.ast)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Translate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZoomEyeAdapter_Name(t *testing.T) {
	a := NewZoomEyeAdapter("https://api.zoomeye.org", "key", 3, 30*time.Second)
	if got := a.Name(); got != "zoomeye" {
		t.Errorf("Name() = %q, want %q", got, "zoomeye")
	}
}

func TestZoomEyeAdapter_IsWebOnly(t *testing.T) {
	a := NewZoomEyeAdapter("https://api.zoomeye.org", "key", 3, 30*time.Second)
	if a.IsWebOnly() {
		t.Error("expected IsWebOnly() = false")
	}
}

// ===== ZoomEye Adapter: Normalize =====

func TestZoomEyeAdapter_Normalize(t *testing.T) {
	a := NewZoomEyeAdapter("https://api.zoomeye.org", "key", 3, 30*time.Second)

	t.Run("full fields", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":   "1.2.3.4",
				"port": float64(80),
				"service": map[string]interface{}{
					"name": "http",
				},
				"geoinfo": map[string]interface{}{
					"country": map[string]interface{}{"names": map[string]interface{}{"en": "China"}},
					"city":    map[string]interface{}{"names": map[string]interface{}{"en": "Beijing"}},
				},
				"timestamp": "2026-01-01",
			},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected 1 asset, got %d", len(assets))
		}
		if assets[0].IP != "1.2.3.4" {
			t.Errorf("IP = %q, want %q", assets[0].IP, "1.2.3.4")
		}
		if assets[0].Port != 80 {
			t.Errorf("Port = %d, want 80", assets[0].Port)
		}
	})

	t.Run("title as string", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":    "1.2.3.4",
				"port":  float64(80),
				"title": "Example Site",
			},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 1 || assets[0].Title != "Example Site" {
			t.Errorf("Title = %q, want %q", assets[0].Title, "Example Site")
		}
	})

	t.Run("title as array", func(t *testing.T) {
		result := &model.EngineResult{RawData: []interface{}{
			map[string]interface{}{
				"ip":    "1.2.3.4",
				"port":  float64(80),
				"title": []interface{}{"Title1", "Title2"},
			},
		}}
		assets, err := a.Normalize(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(assets) != 1 || assets[0].Title != "Title1" {
			t.Errorf("Title = %q, want %q", assets[0].Title, "Title1")
		}
	})
}

// ===== ZoomEye Adapter: Search =====

func TestZoomEyeAdapter_Search(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewZoomEyeAdapter("https://api.zoomeye.org", "", 3, 30*time.Second)
		result, err := a.Search("test", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error == "" {
			t.Error("expected error in result for empty API key")
		}
	})

	t.Run("successful search", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"code":60000,"error":"","message":"","total":1,"data":[{"ip":"1.2.3.4","port":80}]}`))
		}))
		defer server.Close()

		a := NewZoomEyeAdapter(server.URL, "key", 3, 30*time.Second)
		result, err := a.Search("port:80", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Error != "" {
			t.Fatalf("expected success, got: %s", result.Error)
		}
		if result.Total != 1 {
			t.Errorf("Total = %d, want 1", result.Total)
		}
	})
}

// ===== ZoomEye Adapter: GetQuota =====

func TestZoomEyeAdapter_GetQuota(t *testing.T) {
	t.Run("empty api key", func(t *testing.T) {
		a := NewZoomEyeAdapter("https://api.zoomeye.org", "", 3, 30*time.Second)
		_, err := a.GetQuota()
		if err == nil {
			t.Error("expected error for empty API key")
		}
	})

	t.Run("successful quota", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"code":60000,"plan":"free","resources":{"search":100,"interval":"monthly"},"user_info":{"name":"test","role":"user","expired_at":""},"quota_info":{"remain_free_quota":50,"remain_pay_quota":0,"remain_total_quota":50}}`))
		}))
		defer server.Close()

		a := NewZoomEyeAdapter(server.URL, "key", 3, 30*time.Second)
		quota, err := a.GetQuota()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quota == nil {
			t.Fatal("expected quota info, got nil")
		}
	})
}

// ===== Web-only constructor tests =====

func TestNewFofaAdapterWebOnly(t *testing.T) {
	a := NewFofaAdapterWebOnly()
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if !a.IsWebOnly() {
		t.Error("expected IsWebOnly() = true")
	}
	if a.Name() != "fofa" {
		t.Errorf("Name() = %q, want %q", a.Name(), "fofa")
	}
}

func TestNewHunterAdapterWebOnly(t *testing.T) {
	a := NewHunterAdapterWebOnly()
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if !a.IsWebOnly() {
		t.Error("expected IsWebOnly() = true")
	}
}

func TestNewShodanAdapterWebOnly(t *testing.T) {
	a := NewShodanAdapterWebOnly()
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if !a.IsWebOnly() {
		t.Error("expected IsWebOnly() = true")
	}
}

func TestNewQuakeAdapterWebOnly(t *testing.T) {
	a := NewQuakeAdapterWebOnly()
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if !a.IsWebOnly() {
		t.Error("expected IsWebOnly() = true")
	}
}

func TestNewZoomEyeAdapterWebOnly(t *testing.T) {
	a := NewZoomEyeAdapterWebOnly()
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if !a.IsWebOnly() {
		t.Error("expected IsWebOnly() = true")
	}
}
