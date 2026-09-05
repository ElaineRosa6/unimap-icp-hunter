package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBackupNestedOutputExcluded(t *testing.T) {
	for _, order := range []string{"output-first", "payload-first"} {
		t.Run(order, func(t *testing.T) {
			source := t.TempDir()
			payload := "zz_payload"
			outName := "aa_out"
			if order == "payload-first" {
				payload = "aa_payload"
				outName = "zz_out"
			}
			if err := os.WriteFile(filepath.Join(source, payload), []byte("payload"), 0600); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(source, outName)
			seed := t.TempDir()
			if err := os.WriteFile(filepath.Join(seed, "seed"), []byte("old archive"), 0600); err != nil {
				t.Fatal(err)
			}
			old, err := Backup(BackupConfig{Sources: []string{seed}, OutputDir: output})
			if err != nil {
				t.Fatal(err)
			}
			cfg := BackupConfig{Sources: []string{source}, OutputDir: output, MaxBackups: 1}
			for i := 0; i < 2; i++ {
				result, backupErr := Backup(cfg)
				if backupErr != nil {
					t.Fatalf("nested output backup failed: %v", backupErr)
				}
				entries := readBoundaryArchive(t, result.Path)
				var names []string
				for name := range entries {
					names = append(names, name)
				}
				t.Logf("archive_members=%v", names)
				if len(entries) != 1 || entries[payload] != "payload" {
					t.Fatalf("output artifacts entered the backup: members=%v", names)
				}
				files, readErr := os.ReadDir(output)
				if readErr != nil || len(files) != 1 || files[0].Name() != filepath.Base(result.Path) {
					t.Fatalf("retention or temp cleanup failed: %v %v", files, readErr)
				}
			}
			if _, err = os.Stat(old.Path); !os.IsNotExist(err) {
				t.Fatalf("old archive not rotated after success: %v", err)
			}
		})
	}
}

func TestBackupSourceInsideOutputRejected(t *testing.T) {
	for _, kind := range []string{"same-directory", "child-directory", "file"} {
		t.Run(kind, func(t *testing.T) {
			output := t.TempDir()
			seed := t.TempDir()
			if err := os.WriteFile(filepath.Join(seed, "seed"), []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			old, err := Backup(BackupConfig{Sources: []string{seed}, OutputDir: output})
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(old.Path)
			if err != nil {
				t.Fatal(err)
			}
			child := filepath.Join(output, "child")
			if err = os.MkdirAll(child, 0700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(child, "payload")
			if err = os.WriteFile(file, []byte("payload"), 0600); err != nil {
				t.Fatal(err)
			}
			source := output
			if kind == "child-directory" {
				source = child
			}
			if kind == "file" {
				source = file
			}
			result, err := Backup(BackupConfig{Sources: []string{seed, source}, OutputDir: output, MaxBackups: 1})
			if err == nil || result != nil {
				t.Errorf("required source inside output silently accepted: %v %v", result, err)
			}
			after, readErr := os.ReadFile(old.Path)
			if readErr != nil || string(after) != string(before) {
				t.Errorf("old archive changed: %v", readErr)
			}
		})
	}
}

func TestBackupOutputAliasesExcluded(t *testing.T) {
	for _, kind := range []string{"configured-alias", "source-alias"} {
		t.Run(kind, func(t *testing.T) {
			source := t.TempDir()
			for _, name := range []string{"a_payload", "z_payload"} {
				if err := os.WriteFile(filepath.Join(source, name), []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			physical := t.TempDir()
			link := filepath.Join(source, "m_link")
			output := physical
			if kind == "configured-alias" {
				physical = filepath.Join(source, "m_output")
				if err := os.MkdirAll(physical, 0700); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(t.TempDir(), "output")
				output = link
			}
			if err := os.Symlink(physical, link); err != nil {
				t.Skipf("symlink support required: %v", err)
			}
			control := filepath.Join(physical, "keep-output-file")
			if err := os.WriteFile(control, []byte("reserved output"), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := Backup(BackupConfig{Sources: []string{source}, OutputDir: output})
			if err != nil {
				t.Fatal(err)
			}
			entries := readBoundaryArchive(t, result.Path)
			if len(entries) != 2 || entries["a_payload"] != "a_payload" || entries["z_payload"] != "z_payload" {
				t.Fatalf("output alias included or siblings skipped: %v", entries)
			}
			data, err := os.ReadFile(control)
			if err != nil || string(data) != "reserved output" {
				t.Fatalf("output contents changed: %v", err)
			}
		})
	}
}

func TestBackupOutputBoundaryDifferentVolumes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows volume path semantics")
	}
	source := t.TempDir()
	volume := "Z:"
	if strings.EqualFold(filepath.VolumeName(source), volume) {
		volume = "Y:"
	}
	// Exercise the containment calculation without requiring a second mounted drive.
	boundary := &backupOutputBoundary{path: volume + `\output-fixture`}
	if err := boundary.validateSource(source); err != nil {
		t.Fatalf("different-volume source rejected: %v", err)
	}
}
