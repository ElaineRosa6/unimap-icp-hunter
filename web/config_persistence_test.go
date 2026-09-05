package web

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/unimap/project/internal/config"
)

func TestHandleSaveConfig_PersistenceBoundaries(t *testing.T) {
	for _, stage := range []string{"create", "replace", "success"} {
		t.Run(stage, func(t *testing.T) {
			s := newServerForConfigTest()
			root := t.TempDir()
			path := filepath.Join(root, "config.yaml")
			var sentinel string
			switch stage {
			case "create":
				sentinel = filepath.Join(root, "blocked")
				path = filepath.Join(sentinel, "config.yaml")
			case "replace":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				sentinel = filepath.Join(path, "sentinel")
			}
			if sentinel != "" {
				if err := os.WriteFile(sentinel, []byte("preserved"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			manager := config.NewManager(path)
			manager.SetConfig(s.config)
			s.configManager = manager
			before := manager.GetConfig().System.CacheTTL
			response := postConfig(t, s, map[string]interface{}{"section": "system", "data": map[string]interface{}{"cache_ttl": 7200}})
			if stage == "success" {
				if response.Code != http.StatusOK {
					t.Fatalf("status=%d", response.Code)
				}
				reloaded := config.NewManager(path)
				if err := reloaded.Load(); err != nil {
					t.Fatal(err)
				}
				if reloaded.GetConfig().System.CacheTTL != 7200 || manager.GetConfig().System.CacheTTL != 7200 || s.currentConfig().System.CacheTTL != 7200 {
					t.Fatal("successful commit differs across disk, manager and server")
				}
			} else {
				if response.Code != http.StatusInternalServerError {
					t.Fatalf("status=%d", response.Code)
				}
				if manager.GetConfig().System.CacheTTL != before || s.currentConfig().System.CacheTTL != before {
					t.Fatal("failed persistence published candidate")
				}
				data, err := os.ReadFile(sentinel)
				if err != nil || string(data) != "preserved" {
					t.Fatalf("original content changed: %v", err)
				}
			}
			leftovers, err := filepath.Glob(filepath.Join(root, "config.yaml.tmp-*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary config leftovers: %v %v", leftovers, err)
			}
		})
	}
}
