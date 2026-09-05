package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// backupOutputBoundary identifies the reserved output tree through its resolved
// path and directory identity, including when OutputDir is a link/junction.
type backupOutputBoundary struct {
	path string
	info os.FileInfo
}

func resolveBackupPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}
func newBackupOutputBoundary(path string) (*backupOutputBoundary, error) {
	resolved, err := resolveBackupPath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("backup output is not a directory")
	}
	return &backupOutputBoundary{path: resolved, info: info}, nil
}
func (out *backupOutputBoundary) validateSource(path string) error {
	resolved, err := resolveBackupPath(path)
	if err != nil {
		return err
	}
	// Different Windows volumes cannot have an ancestor/descendant relation.
	if !strings.EqualFold(filepath.VolumeName(out.path), filepath.VolumeName(resolved)) {
		return nil
	}
	relative, err := filepath.Rel(out.path, resolved)
	if err != nil {
		return err
	}
	if relative == "." || filepath.IsLocal(relative) {
		return fmt.Errorf("backup source is inside the reserved output directory")
	}
	return nil
}

// excluded tests only directory identity; it does not read file contents through
// a link. Walk does not descend into directory links, so a matching link is
// simply omitted without skipping its siblings.
func (out *backupOutputBoundary) excluded(path string, info os.FileInfo) (bool, bool) {
	if info == nil {
		return false, false
	}
	if info.IsDir() {
		return os.SameFile(out.info, info), true
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Stat(path)
		if err == nil && target.IsDir() && os.SameFile(out.info, target) {
			return true, false
		}
	}
	return false, false
}
