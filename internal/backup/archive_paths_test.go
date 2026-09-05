package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupArchivePathConflictsPreserveRecoveryPoint(t *testing.T) {
	for _, kind := range []string{"same-name", "file-parent"} {
		t.Run(kind, func(t *testing.T) {
			first, second, output := t.TempDir(), t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(first, "payload"), []byte("first"), 0600); err != nil {
				t.Fatal(err)
			}
			name := "payload"
			if kind == "file-parent" {
				name = filepath.Join("payload", "nested")
				if err := os.MkdirAll(filepath.Join(second, "payload"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(second, name), []byte("second"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := BackupConfig{Sources: []string{first}, OutputDir: output, MaxBackups: 1}
			old, err := Backup(cfg)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(old.Path)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Sources = []string{first, second}
			result, err := Backup(cfg)
			if err == nil || result != nil {
				t.Errorf("ambiguous archive published: result=%v error=%v", result, err)
			}
			after, readErr := os.ReadFile(old.Path)
			if readErr != nil || string(after) != string(before) {
				t.Errorf("old recovery point changed: %v", readErr)
			}
			entries, readErr := os.ReadDir(output)
			if readErr != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(old.Path) {
				t.Errorf("unexpected archive or staging: %v %v", entries, readErr)
			}
		})
	}
}

func TestBackupCommonParentPreservesSameBasenames(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"first", "second"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "payload"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Backup(BackupConfig{Sources: []string{root}, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	entries := readBoundaryArchive(t, result.Path)
	if len(entries) != 2 || entries["first/payload"] != "first" || entries["second/payload"] != "second" {
		t.Fatalf("distinct paths not preserved: %v", entries)
	}
}

func TestBackupMultipleDistinctSourcesPreserved(t *testing.T) {
	var sources []string
	for _, name := range []string{"first", "second"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, dir)
	}
	result, err := Backup(BackupConfig{Sources: sources, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	entries := readBoundaryArchive(t, result.Path)
	if len(entries) != 2 || entries["first"] != "first" || entries["second"] != "second" {
		t.Fatalf("distinct source paths changed: %v", entries)
	}
}
