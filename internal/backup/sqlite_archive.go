package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// SQLiteSnapshotter receives metadata from the same constrained source handle
// used to identify SQLite. The owner must match file identity (os.SameFile) to
// an existing connection and create a standalone SQLite database at destination.
// The destination does not exist and is in a private staging directory. The
// callback must respect ctx and return an error for unknown identities. It must
// not open a new source connection from a request-supplied pathname.
type SQLiteSnapshotter func(ctx context.Context, source os.FileInfo, destination string) error

type backupFile struct {
	path        string
	baseDir     string
	archiveName string
}

func prepareSQLiteSnapshots(ctx context.Context, files []backupFile, staging string, snapshot SQLiteSnapshotter) ([]backupFile, error) {
	prepared := make([]backupFile, len(files))
	copy(prepared, files)
	snapshotted := make(map[string]os.FileInfo)
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(file.baseDir, file.path)
		if err != nil || !filepath.IsLocal(rel) || rel == "." {
			return nil, fmt.Errorf("SQLite candidate outside source root")
		}
		destination := filepath.Join(staging, strconv.Itoa(i)+".sqlite")
		info, err := stageSQLiteFile(ctx, file, rel, destination, snapshot)
		if err != nil {
			return nil, err
		}
		if info == nil {
			continue
		}
		key, keyErr := snapshotPathKey(file.path)
		if keyErr != nil {
			return nil, keyErr
		}
		if previous, exists := snapshotted[key]; exists && !os.SameFile(previous, info) {
			return nil, fmt.Errorf("ambiguous SQLite source paths")
		}
		snapshotted[key] = info
		prepared[i] = backupFile{path: destination, baseDir: staging, archiveName: rel}
	}
	result := make([]backupFile, 0, len(prepared))
	for i, file := range files {
		if prepared[i].archiveName != "" {
			// A separately selected SQLite database can itself be named "*-wal".
			// Its detected identity takes precedence over companion filename rules.
			result = append(result, prepared[i])
			continue
		}
		skip := false
		for _, suffix := range []string{"-wal", "-shm"} {
			// Only exact companion paths of a successfully selected main database are
			// omitted, not global suffix matches or sidecars of an unselected database.
			if len(file.path) > len(suffix) && file.path[len(file.path)-len(suffix):] == suffix {
				mainPath := file.path[:len(file.path)-len(suffix)]
				key, keyErr := snapshotPathKey(mainPath)
				if keyErr != nil {
					return nil, keyErr
				}
				if selected, exists := snapshotted[key]; exists {
					mainRel, relErr := filepath.Rel(file.baseDir, mainPath)
					if relErr != nil || !filepath.IsLocal(mainRel) {
						return nil, fmt.Errorf("invalid SQLite companion path")
					}
					root, openErr := os.OpenRoot(file.baseDir)
					if openErr != nil {
						return nil, openErr
					}
					mainInfo, statErr := root.Stat(mainRel)
					_ = root.Close()
					if statErr != nil || !os.SameFile(mainInfo, selected) {
						return nil, fmt.Errorf("SQLite source identity changed before companion filtering")
					}
					skip = true
				}
				break
			}
		}
		if !skip {
			result = append(result, prepared[i])
		}
	}
	return result, nil
}

func stageSQLiteFile(ctx context.Context, file backupFile, rel, destination string, snapshot SQLiteSnapshotter) (os.FileInfo, error) {
	root, err := os.OpenRoot(file.baseDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	source, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("SQLite candidate is not a regular file")
	}
	var header [16]byte
	n, err := source.ReadAt(header[:], 0)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n != len(header) || string(header[:]) != "SQLite format 3\x00" {
		return nil, nil
	}
	if snapshotErr := snapshot(ctx, info, destination); snapshotErr != nil {
		return nil, snapshotErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	created, err := os.Lstat(destination)
	if err != nil {
		return nil, err
	}
	if !created.Mode().IsRegular() || created.Size() < 16 {
		return nil, fmt.Errorf("snapshot callback did not produce a regular database")
	}
	stagedRoot, rootErr := os.OpenRoot(filepath.Dir(destination))
	if rootErr != nil {
		return nil, rootErr
	}
	defer stagedRoot.Close()
	staged, stagedErr := stagedRoot.Open(filepath.Base(destination))
	if stagedErr != nil {
		return nil, stagedErr
	}
	defer staged.Close()
	var stagedHeader [16]byte
	if _, readErr := io.ReadFull(staged, stagedHeader[:]); readErr != nil {
		return nil, readErr
	}
	if string(stagedHeader[:]) != "SQLite format 3\x00" {
		return nil, fmt.Errorf("snapshot callback produced a non-SQLite file")
	}
	return info, nil
}

func snapshotPathKey(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// A candidate match still requires SameFile before dropping a companion.
	// Case-sensitive Windows directories therefore fail on ambiguity, not data loss.
	if runtime.GOOS == "windows" {
		absolute = strings.ToLower(absolute)
	}
	return absolute, nil
}
