package scheduler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unimap/project/internal/backup"
	"github.com/unimap/project/internal/model"
)

func TestBackupRunnerCancellationPreservesRecoveryPoint(t *testing.T) {
	for _, kind := range []string{"cancelled", "expired"} {
		t.Run(kind, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "payload.txt")
			if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
				t.Fatal(err)
			}
			output := t.TempDir()
			old, err := backup.Backup(backup.BackupConfig{Sources: []string{source}, OutputDir: output, Prefix: "cancel"})
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(old.Path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if kind == "expired" {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				want = context.DeadlineExceeded
			}
			cancel()
			payload := &model.TaskPayload{Extra: map[string]any{"sources": []string{source}, "output_dir": output, "prefix": "cancel", "max_backups": 1}}
			message, err := NewBackupRunner().Execute(ctx, payload)
			if !errors.Is(err, want) || message != "" {
				t.Errorf("cancelled runner message=%q error=%v, want %v", message, err, want)
			}
			entries, err := os.ReadDir(output)
			if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(old.Path) {
				t.Errorf("cancelled runner changed recovery points: %v %v", entries, err)
			}
			current, err := os.ReadFile(old.Path)
			if err != nil || string(current) != string(original) {
				t.Errorf("old recovery point changed: %v", err)
			}
		})
	}
}
