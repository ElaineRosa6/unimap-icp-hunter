package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	unierror "github.com/unimap-icp-hunter/project/internal/error"
)

// --- APIKeyManager Tests ---

func TestNewAPIKeyManager(t *testing.T) {
	t.Run("creates manager with empty map", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := NewAPIKeyManager(filepath.Join(tmpDir, "keys.json"))
		if mgr == nil {
			t.Fatal("expected non-nil manager")
		}
		if len(mgr.keys) != 0 {
			t.Errorf("expected empty keys map, got %d", len(mgr.keys))
		}
	})

	t.Run("creates manager with empty storage path", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		if mgr == nil {
			t.Fatal("expected non-nil manager")
		}
	})

	t.Run("loads from existing storage", func(t *testing.T) {
		tmpDir := t.TempDir()
		storagePath := filepath.Join(tmpDir, "keys.json")

		// Pre-populate storage with metadata (no actual key)
		data := `[{"id":"key_test","key":"","description":"pre-existing","created_at":"2025-01-01T00:00:00Z","expires_at":"0001-01-01T00:00:00Z","permissions":["read"],"status":"active"}]`
		if err := os.WriteFile(storagePath, []byte(data), 0600); err != nil {
			t.Fatalf("failed to write test data: %v", err)
		}

		mgr := NewAPIKeyManager(storagePath)
		keys := mgr.ListAPIKeys()
		if len(keys) != 1 {
			t.Errorf("expected 1 key from storage, got %d", len(keys))
		}
	})
}

func TestGenerateAPIKey(t *testing.T) {
	t.Run("generates valid key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, err := mgr.GenerateAPIKey("test key", []string{"read", "write"}, 24*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if apiKey == nil {
			t.Fatal("expected non-nil API key")
		}
		if !strings.HasPrefix(apiKey.ID, "key_") {
			t.Errorf("expected ID prefix 'key_', got %q", apiKey.ID)
		}
		if apiKey.Key == "" {
			t.Error("expected non-empty key")
		}
		if apiKey.Description != "test key" {
			t.Errorf("expected description 'test key', got %q", apiKey.Description)
		}
		if apiKey.Status != "active" {
			t.Errorf("expected status 'active', got %q", apiKey.Status)
		}
		if len(apiKey.Permissions) != 2 {
			t.Errorf("expected 2 permissions, got %d", len(apiKey.Permissions))
		}
		if apiKey.ExpiresAt.Before(time.Now()) {
			t.Error("expected future expiration")
		}
	})

	t.Run("generates unique keys", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		key1, _ := mgr.GenerateAPIKey("key1", nil, time.Hour)
		key2, _ := mgr.GenerateAPIKey("key2", nil, time.Hour)
		if key1.Key == key2.Key {
			t.Error("expected unique keys")
		}
		if key1.ID == key2.ID {
			t.Error("expected unique IDs")
		}
	})

	t.Run("saves to storage", func(t *testing.T) {
		tmpDir := t.TempDir()
		storagePath := filepath.Join(tmpDir, "keys.json")
		mgr := NewAPIKeyManager(storagePath)

		_, err := mgr.GenerateAPIKey("persisted", []string{"read"}, time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify file was created
		if _, err := os.Stat(storagePath); os.IsNotExist(err) {
			t.Error("expected storage file to exist")
		}
	})

	t.Run("empty permissions", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, err := mgr.GenerateAPIKey("no perms", nil, time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(apiKey.Permissions) != 0 {
			t.Errorf("expected 0 permissions, got %d", len(apiKey.Permissions))
		}
	})

	t.Run("zero expiration", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, err := mgr.GenerateAPIKey("no expiry", []string{"read"}, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if apiKey.ExpiresAt.Before(time.Now()) {
			t.Error("expected future or zero expiration")
		}
	})
}

func TestValidateAPIKey(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read"}, time.Hour)
		validated, err := mgr.ValidateAPIKey(apiKey.Key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if validated.ID != apiKey.ID {
			t.Errorf("expected ID %q, got %q", apiKey.ID, validated.ID)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		_, err := mgr.ValidateAPIKey("nonexistent-key")
		if err == nil {
			t.Fatal("expected error for invalid key")
		}
		uniErr, ok := err.(*unierror.UnimapError)
		if !ok {
			t.Fatalf("expected *UnimapError, got %T", err)
		}
		if uniErr.Code != unierror.ErrAPIUnauthorized {
			t.Errorf("expected code %d, got %d", unierror.ErrAPIUnauthorized, uniErr.Code)
		}
	})

	t.Run("revoked key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read"}, time.Hour)
		_ = mgr.RevokeAPIKey(apiKey.ID)
		_, err := mgr.ValidateAPIKey(apiKey.Key)
		if err == nil {
			t.Fatal("expected error for revoked key")
		}
	})

	t.Run("expired key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		// Generate with negative duration to simulate already expired
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read"}, -time.Hour)
		_, err := mgr.ValidateAPIKey(apiKey.Key)
		if err == nil {
			t.Fatal("expected error for expired key")
		}
	})
}

func TestCheckPermission(t *testing.T) {
	t.Run("has permission", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read", "write"}, time.Hour)
		if !mgr.CheckPermission(apiKey.Key, "read") {
			t.Error("expected key to have 'read' permission")
		}
	})

	t.Run("admin has all permissions", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("admin", []string{"admin"}, time.Hour)
		if !mgr.CheckPermission(apiKey.Key, "anything") {
			t.Error("expected admin key to have all permissions")
		}
	})

	t.Run("missing permission", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read"}, time.Hour)
		if mgr.CheckPermission(apiKey.Key, "delete") {
			t.Error("expected key to NOT have 'delete' permission")
		}
	})

	t.Run("invalid key has no permissions", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		if mgr.CheckPermission("invalid-key", "read") {
			t.Error("expected invalid key to have no permissions")
		}
	})
}

func TestGetAPIKey(t *testing.T) {
	t.Run("finds by ID", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("test", nil, time.Hour)
		found := mgr.GetAPIKey(apiKey.ID)
		if found == nil {
			t.Fatal("expected to find key by ID")
		}
		if found.ID != apiKey.ID {
			t.Errorf("expected ID %q, got %q", apiKey.ID, found.ID)
		}
	})

	t.Run("returns nil for unknown ID", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		if mgr.GetAPIKey("unknown-id") != nil {
			t.Error("expected nil for unknown ID")
		}
	})
}

func TestListAPIKeys(t *testing.T) {
	t.Run("lists all keys without actual key values", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		_, _ = mgr.GenerateAPIKey("key1", nil, time.Hour)
		_, _ = mgr.GenerateAPIKey("key2", nil, time.Hour)

		keys := mgr.ListAPIKeys()
		if len(keys) != 2 {
			t.Fatalf("expected 2 keys, got %d", len(keys))
		}
		for _, k := range keys {
			if k.Key != "" {
				t.Error("expected listed keys to not contain actual key value")
			}
		}
	})

	t.Run("empty list", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		keys := mgr.ListAPIKeys()
		if len(keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(keys))
		}
	})
}

func TestRevokeAPIKey(t *testing.T) {
	t.Run("revokes existing key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("test", nil, time.Hour)
		err := mgr.RevokeAPIKey(apiKey.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Verify status changed
		found := mgr.GetAPIKey(apiKey.ID)
		if found.Status != "revoked" {
			t.Errorf("expected status 'revoked', got %q", found.Status)
		}
	})

	t.Run("fails for unknown ID", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		err := mgr.RevokeAPIKey("unknown-id")
		if err == nil {
			t.Fatal("expected error for unknown ID")
		}
	})

	t.Run("revoked key cannot be validated", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("test", nil, time.Hour)
		_ = mgr.RevokeAPIKey(apiKey.ID)
		_, err := mgr.ValidateAPIKey(apiKey.Key)
		if err == nil {
			t.Fatal("expected validation to fail for revoked key")
		}
	})
}

func TestUpdateLastUsed(t *testing.T) {
	t.Run("updates timestamp for existing key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("test", nil, time.Hour)
		before := apiKey.LastUsed

		time.Sleep(10 * time.Millisecond)
		mgr.UpdateLastUsed(apiKey.Key)

		// Verify by looking up - LastUsed should have been set
		found := mgr.keys[apiKey.Key]
		if !found.LastUsed.After(before) {
			t.Error("expected LastUsed to be updated")
		}
	})

	t.Run("no-op for unknown key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		// Should not panic
		mgr.UpdateLastUsed("nonexistent")
	})
}

func TestCleanupExpiredKeys(t *testing.T) {
	t.Run("marks expired keys", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		// Generate a key that is already expired
		apiKey, _ := mgr.GenerateAPIKey("test", nil, -time.Hour)
		if apiKey.Status != "active" {
			t.Fatalf("expected initial status 'active', got %q", apiKey.Status)
		}

		mgr.CleanupExpiredKeys()

		cleaned := mgr.GetAPIKey(apiKey.ID)
		if cleaned.Status != "expired" {
			t.Errorf("expected status 'expired', got %q", cleaned.Status)
		}
	})

	t.Run("does not affect valid keys", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		apiKey, _ := mgr.GenerateAPIKey("valid", nil, time.Hour)

		mgr.CleanupExpiredKeys()

		found := mgr.GetAPIKey(apiKey.ID)
		if found.Status != "active" {
			t.Errorf("expected status 'active', got %q", found.Status)
		}
	})
}

func TestGetKeyStats(t *testing.T) {
	t.Run("correct stats", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		_, _ = mgr.GenerateAPIKey("active1", nil, time.Hour)
		_, _ = mgr.GenerateAPIKey("active2", nil, time.Hour)
		// Generate an already-expired key and force cleanup to mark it
		_, _ = mgr.GenerateAPIKey("expired", nil, -time.Hour)
		mgr.CleanupExpiredKeys()

		// Revoke one of the active keys
		statsBefore := mgr.GetKeyStats()
		if statsBefore["total"] != 3 {
			t.Fatalf("expected 3 total, got %d", statsBefore["total"])
		}
		if statsBefore["expired"] != 1 {
			t.Errorf("expected 1 expired, got %d", statsBefore["expired"])
		}
		if statsBefore["active"] != 2 {
			t.Errorf("expected 2 active, got %d", statsBefore["active"])
		}
	})

	t.Run("empty stats", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		stats := mgr.GetKeyStats()
		if stats["total"] != 0 {
			t.Errorf("expected 0 total, got %d", stats["total"])
		}
	})
}

func TestSaveToStorageErrors(t *testing.T) {
	t.Run("no-op with empty storage path", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		_, _ = mgr.GenerateAPIKey("test", nil, time.Hour)
		// Should not panic
		mgr.saveToStorage()
	})

	t.Run("handles invalid path gracefully", func(t *testing.T) {
		mgr := NewAPIKeyManager(string([]byte{0})) // null byte - invalid path
		_, _ = mgr.GenerateAPIKey("test", nil, time.Hour)
		// Should not panic
		mgr.saveToStorage()
	})
}

func TestLoadFromStorageErrors(t *testing.T) {
	t.Run("handles missing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := NewAPIKeyManager(filepath.Join(tmpDir, "nonexistent.json"))
		// Should not panic, should have no keys
		if len(mgr.keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(mgr.keys))
		}
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		storagePath := filepath.Join(tmpDir, "keys.json")
		if err := os.WriteFile(storagePath, []byte("not json"), 0600); err != nil {
			t.Fatalf("failed to write test data: %v", err)
		}

		mgr := NewAPIKeyManager(storagePath)
		if len(mgr.keys) != 0 {
			t.Errorf("expected 0 keys from invalid JSON, got %d", len(mgr.keys))
		}
	})
}

// --- Middleware Tests ---

func TestRequireAPIKey(t *testing.T) {
	t.Run("rejects missing key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler := mw.RequireAPIKey("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not reach next handler")
		}))
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "API key is required") {
			t.Errorf("expected 'API key is required' in body, got %q", rec.Body.String())
		}
	})

	t.Run("rejects invalid key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Apikey invalid-key-123")
		rec := httptest.NewRecorder()

		handler := mw.RequireAPIKey("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not reach next handler")
		}))
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("accepts valid key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read"}, time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Apikey "+apiKey.Key)
		rec := httptest.NewRecorder()

		called := false
		handler := mw.RequireAPIKey("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			if r.Header.Get("X-API-Key-ID") != apiKey.ID {
				t.Errorf("expected X-API-Key-ID header %q, got %q", apiKey.ID, r.Header.Get("X-API-Key-ID"))
			}
		}))
		handler.ServeHTTP(rec, req)

		if !called {
			t.Error("expected next handler to be called")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("checks permission", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read"}, time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Apikey "+apiKey.Key)
		rec := httptest.NewRecorder()

		handler := mw.RequireAPIKey("write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not reach next handler")
		}))
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Insufficient permissions") {
			t.Errorf("expected 'Insufficient permissions' in body, got %q", rec.Body.String())
		}
	})

	t.Run("empty permission string skips check", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)
		apiKey, _ := mgr.GenerateAPIKey("test", nil, time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Apikey "+apiKey.Key)
		rec := httptest.NewRecorder()

		called := false
		handler := mw.RequireAPIKey("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		handler.ServeHTTP(rec, req)

		if !called {
			t.Error("expected next handler to be called")
		}
	})

	t.Run("sets permission header", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read", "write"}, time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key", apiKey.Key)
		rec := httptest.NewRecorder()

		handler := mw.RequireAPIKey("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			perms := r.Header.Get("X-API-Key-Permissions")
			if perms != "read,write" {
				t.Errorf("expected permissions 'read,write', got %q", perms)
			}
		}))
		handler.ServeHTTP(rec, req)
	})
}

func TestOptionalAPIKey(t *testing.T) {
	t.Run("proceeds without key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		called := false
		handler := mw.OptionalAPIKey()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		handler.ServeHTTP(rec, req)

		if !called {
			t.Error("expected next handler to be called even without API key")
		}
	})

	t.Run("accepts valid key when present", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)
		apiKey, _ := mgr.GenerateAPIKey("test", []string{"read"}, time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey.Key)
		rec := httptest.NewRecorder()

		handler := mw.OptionalAPIKey()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-API-Key-ID") != apiKey.ID {
				t.Errorf("expected X-API-Key-ID header, got %q", r.Header.Get("X-API-Key-ID"))
			}
		}))
		handler.ServeHTTP(rec, req)
	})

	t.Run("ignores invalid key", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Apikey invalid-key")
		rec := httptest.NewRecorder()

		called := false
		handler := mw.OptionalAPIKey()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		handler.ServeHTTP(rec, req)

		if !called {
			t.Error("expected next handler to be called even with invalid key")
		}
	})
}

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		query   string
		want    string
	}{
		{
			name:    "no key anywhere",
			headers: map[string]string{},
			query:   "",
			want:    "",
		},
		{
			name:    "Apikey prefix",
			headers: map[string]string{"Authorization": "Apikey test-key-123"},
			query:   "",
			want:    "test-key-123",
		},
		{
			name:    "Bearer prefix",
			headers: map[string]string{"Authorization": "Bearer bearer-key-456"},
			query:   "",
			want:    "bearer-key-456",
		},
		{
			name:    "X-API-Key header",
			headers: map[string]string{"X-API-Key": "header-key-789"},
			query:   "",
			want:    "header-key-789",
		},
		{
			name:    "query parameter",
			headers: map[string]string{},
			query:   "api_key=query-key-000&other=value",
			want:    "query-key-000",
		},
		{
			name:    "Authorization takes precedence over X-API-Key",
			headers: map[string]string{"Authorization": "Apikey auth-key", "X-API-Key": "header-key"},
			query:   "",
			want:    "auth-key",
		},
		{
			name:    "X-API-Key takes precedence over query",
			headers: map[string]string{"X-API-Key": "header-key"},
			query:   "api_key=query-key",
			want:    "header-key",
		},
		{
			name:    "trims whitespace",
			headers: map[string]string{"Authorization": "Apikey  spaced-key  "},
			query:   "",
			want:    "spaced-key",
		},
		{
			name:    "case insensitive prefix",
			headers: map[string]string{"Authorization": "APIKEY case-key"},
			query:   "",
			want:    "case-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewAPIKeyManager("")
			mw := NewAuthMiddleware(mgr)

			req := httptest.NewRequest(http.MethodGet, "/test?"+tt.query, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := mw.extractAPIKey(req)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestWriteAuthError(t *testing.T) {
	t.Run("JSON format", func(t *testing.T) {
		mgr := NewAPIKeyManager("")
		mw := NewAuthMiddleware(mgr)

		rec := httptest.NewRecorder()
		mw.writeAuthError(rec, "test error message")

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", rec.Header().Get("Content-Type"))
		}
		body := rec.Body.String()
		if !strings.Contains(body, "test error message") {
			t.Errorf("expected body to contain error message, got %q", body)
		}
		if !strings.Contains(body, "unauthorized") {
			t.Errorf("expected body to contain 'unauthorized', got %q", body)
		}
	})
}

// --- Helper Function Tests ---

func TestGetAPIKeyID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key-ID", "key_123")
	if id := GetAPIKeyID(req); id != "key_123" {
		t.Errorf("expected 'key_123', got %q", id)
	}
}

func TestGetAPIKeyPermissions(t *testing.T) {
	t.Run("parses permissions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key-Permissions", "read,write,admin")
		perms := GetAPIKeyPermissions(req)
		if len(perms) != 3 {
			t.Fatalf("expected 3 permissions, got %d", len(perms))
		}
	})

	t.Run("empty header returns empty slice", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		perms := GetAPIKeyPermissions(req)
		if len(perms) != 0 {
			t.Errorf("expected 0 permissions, got %d", len(perms))
		}
	})
}

func TestHasPermission(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key-Permissions", "read,write")
		if !HasPermission(req, "read") {
			t.Error("expected hasPermission to return true for 'read'")
		}
	})

	t.Run("admin wildcard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key-Permissions", "admin")
		if !HasPermission(req, "anything") {
			t.Error("expected admin to have all permissions")
		}
	})

	t.Run("no match", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key-Permissions", "read")
		if HasPermission(req, "delete") {
			t.Error("expected hasPermission to return false for 'delete'")
		}
	})

	t.Run("empty permissions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		if HasPermission(req, "read") {
			t.Error("expected hasPermission to return false with no permissions")
		}
	})
}
