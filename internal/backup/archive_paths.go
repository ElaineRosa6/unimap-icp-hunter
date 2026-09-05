package backup

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// validateArchivePaths rejects ambiguous file-only tar layouts before snapshots
// or publication. Keep the established relative names instead of silently
// renaming files and changing existing restore layouts.
func validateArchivePaths(ctx context.Context, files []backupFile) error {
	names := make([]string, 0, len(files))
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := file.archiveName
		if name == "" {
			relative, err := filepath.Rel(file.baseDir, file.path)
			if err != nil {
				return err
			}
			name = relative
		}
		if !filepath.IsLocal(name) || name == "." {
			return fmt.Errorf("invalid backup archive entry")
		}
		names = append(names, filepath.ToSlash(filepath.Clean(name)))
	}
	sort.Strings(names)
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("backup sources map multiple files to archive entry %q; select a common parent source or non-conflicting sources", name)
		}
		parent := name
		for {
			index := strings.LastIndexByte(parent, '/')
			if index < 0 {
				break
			}
			parent = parent[:index]
			if _, exists := seen[parent]; exists {
				return fmt.Errorf("backup archive entry %q requires %q as a directory, but another source maps it to a file", name, parent)
			}
		}
		seen[name] = struct{}{}
	}
	return nil
}
