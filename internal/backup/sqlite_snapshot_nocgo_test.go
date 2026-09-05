//go:build !cgo

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteSnapshotWithoutCGO(t *testing.T) {
	target := filepath.Join(t.TempDir(), "snapshot.sqlite")
	err := SnapshotSQLite(context.Background(), nil, target)
	if err == nil || !strings.Contains(err.Error(), "CGO") {
		t.Fatalf("expected unsupported snapshot: %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("unsupported snapshot created a file: %v", statErr)
	}
}
