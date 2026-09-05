package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func readBoundaryArchive(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string]string{}
	for {
		header, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		data, readErr := io.ReadAll(tr)
		if readErr != nil {
			t.Fatal(readErr)
		}
		entries[header.Name] = string(data)
	}
	return entries
}

func TestBackupSourceOutsideLink(t *testing.T) {
	for _, relative := range []bool{false, true} {
		name := "absolute"
		if relative {
			name = "relative"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			src, out, external := filepath.Join(root, "source"), filepath.Join(root, "output"), filepath.Join(root, "external.txt")
			if err := os.Mkdir(src, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(src, "good.txt"), []byte("good"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(external, []byte("outside marker"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := BackupConfig{Sources: []string{src}, OutputDir: out, Prefix: "fixture", MaxBackups: 1}
			previous, err := Backup(cfg)
			if err != nil {
				t.Fatal(err)
			}
			target := external
			if relative {
				target = filepath.Join("..", "external.txt")
			}
			if linkErr := os.Symlink(target, filepath.Join(src, "alias.txt")); linkErr != nil {
				t.Fatal(linkErr)
			}
			result, backupErr := Backup(cfg)
			if result != nil {
				contents := readBoundaryArchive(t, result.Path)
				t.Errorf("published outside-link archive: %v", contents)
			}
			if backupErr == nil {
				t.Error("outside source link accepted")
			}
			if _, statErr := os.Stat(previous.Path); statErr != nil {
				t.Errorf("complete backup was pruned: %v", statErr)
			}
		})
	}
}

func TestBackupSourcePortablePathsAndInternalLink(t *testing.T) {
	src, out := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "data.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("nested", "data.txt"), filepath.Join(src, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	result, err := Backup(BackupConfig{Sources: []string{src}, OutputDir: out})
	if err != nil {
		t.Fatal(err)
	}
	contents := readBoundaryArchive(t, result.Path)
	if contents["nested/data.txt"] != "inside" || contents["alias.txt"] != "inside" || len(contents) != 2 {
		t.Fatalf("nonportable or incomplete archive: %v", contents)
	}
}
