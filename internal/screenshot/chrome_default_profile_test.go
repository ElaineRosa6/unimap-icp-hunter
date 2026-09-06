package screenshot

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAllocatorRejectsDefaultChromeDataDirBeforeLaunch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows Chrome directory convention")
	}
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("UNIMAP_CHROME_REMOTE_DEBUG_URL", "")
	for _, envOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "config", true: "environment"}[envOnly], func(t *testing.T) {
			dir := filepath.Join(local, "Google", "Chrome", "User Data")
			m := NewManager(Config{ChromePath: filepath.Join(local, "missing-chrome.exe"), UserDataDir: dir, ProfileDir: "Profile 1"})
			if envOnly {
				m.userDataDir = ""
				t.Setenv("UNIMAP_CHROME_USER_DATA_DIR", dir)
			}
			_, cancel, err := m.newAllocator(context.Background())
			if cancel != nil {
				cancel()
			}
			if err == nil || !strings.Contains(err.Error(), "non-default user data directory") {
				t.Fatalf("want actionable profile error before launch, got %v", err)
			}
		})
	}
}

func TestDefaultChromeDataDirCheckAllowsIsolatedOrTemporaryDirectory(t *testing.T) {
	t.Setenv("UNIMAP_CHROME_USER_DATA_DIR", "")
	for _, dir := range []string{"", t.TempDir()} {
		m := NewManager(Config{UserDataDir: dir})
		if err := m.validateLocalChromeDataDir(); err != nil {
			t.Fatal(err)
		}
	}
}
