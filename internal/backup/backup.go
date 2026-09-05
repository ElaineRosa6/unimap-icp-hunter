package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/unimap/project/internal/logger"
)

// BackupConfig 备份配置
type BackupConfig struct {
	// Sources 要备份的目录/文件列表（绝对路径或相对路径）
	Sources []string
	// OutputDir 备份输出目录
	OutputDir string
	// MaxBackups 最大保留备份数量，0 表示不限制
	MaxBackups int
	// Prefix 备份文件名前缀
	Prefix string
	// SQLiteSnapshotter optionally binds SQLite files to existing owned connections.
	// When configured, every detected SQLite file must be snapshotted successfully;
	// an unknown source must return an error, never fall back to copying raw bytes.
	SQLiteSnapshotter SQLiteSnapshotter
}

// BackupResult 备份结果
type BackupResult struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// Backup 执行备份
func Backup(cfg BackupConfig) (*BackupResult, error) {
	return BackupContext(context.Background(), cfg)
}

// BackupContext creates an archive while observing cancellation during source
// collection and copying. Cancellation observed before publication discards the
// temporary archive and preserves existing recovery points. Cancellation racing
// with the final rename may arrive after publication; a published backup succeeds.
func BackupContext(ctx context.Context, cfg BackupConfig) (*BackupResult, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = "./backups"
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "unimap"
	}

	// 确保输出目录存在
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup dir: %w", err)
	}

	outputBoundary, boundaryErr := newBackupOutputBoundary(cfg.OutputDir)
	if boundaryErr != nil {
		return nil, fmt.Errorf("resolve backup output: %w", boundaryErr)
	}

	// Write to an exclusive sibling and publish only after the archive is complete.
	timestamp := time.Now().Format("20060102_150405.000000000")
	tmpFile, err := os.CreateTemp(cfg.OutputDir, fmt.Sprintf(".%s_backup_%s_*.tmp", cfg.Prefix, timestamp))
	if err != nil {
		return nil, fmt.Errorf("failed to create backup file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}
	defer cleanup()
	gw := gzip.NewWriter(tmpFile)
	tw := tar.NewWriter(gw)

	// 收集所有要备份的文件（带基础目录信息）
	var files []backupFile
	var sourceErrors []error
	for _, src := range cfg.Sources {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		srcFiles, baseDir, collectErr := collectFilesContextExcluding(ctx, src, outputBoundary)
		if collectErr != nil {
			sourceErrors = append(sourceErrors, fmt.Errorf("source %s: %w", src, collectErr))
			continue
		}
		for _, f := range srcFiles {
			files = append(files, backupFile{path: f, baseDir: baseDir})
		}
	}

	// A configured source is required: never publish a partial archive or let
	// its retention cleanup remove an older complete recovery point.
	if len(sourceErrors) > 0 {
		return nil, fmt.Errorf("collect backup sources: %w", errors.Join(sourceErrors...))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found to backup")
	}

	if pathErr := validateArchivePaths(ctx, files); pathErr != nil {
		return nil, pathErr
	}

	if cfg.SQLiteSnapshotter != nil {
		staging, stageErr := os.MkdirTemp(cfg.OutputDir, ".sqlite-snapshots-*")
		if stageErr != nil {
			return nil, fmt.Errorf("create SQLite staging: %w", stageErr)
		}
		defer os.RemoveAll(staging)
		files, stageErr = prepareSQLiteSnapshots(ctx, files, staging, cfg.SQLiteSnapshotter)
		if stageErr != nil {
			return nil, fmt.Errorf("prepare SQLite snapshots: %w", stageErr)
		}
	}
	for _, f := range files {
		if tarErr := addNamedFileToTarContext(ctx, tw, f.path, f.baseDir, f.archiveName); tarErr != nil {
			return nil, fmt.Errorf("add backup file %s: %w", f.path, tarErr)
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if closeErr := tw.Close(); closeErr != nil {
		return nil, fmt.Errorf("finalize tar archive: %w", closeErr)
	}
	if closeErr := gw.Close(); closeErr != nil {
		return nil, fmt.Errorf("finalize gzip archive: %w", closeErr)
	}
	if syncErr := tmpFile.Sync(); syncErr != nil {
		return nil, fmt.Errorf("sync backup archive: %w", syncErr)
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		return nil, fmt.Errorf("close backup archive: %w", closeErr)
	}
	uniqueSuffix := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(tmpPath), fmt.Sprintf(".%s_backup_%s_", cfg.Prefix, timestamp)), ".tmp")
	filename := fmt.Sprintf("%s_backup_%s_%s.tar.gz", cfg.Prefix, timestamp, uniqueSuffix)
	outputPath := filepath.Join(cfg.OutputDir, filename)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if publishErr := os.Rename(tmpPath, outputPath); publishErr != nil {
		return nil, fmt.Errorf("publish backup archive: %w", publishErr)
	}
	tmpPath = ""

	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("stat completed backup file: %w", err)
	}

	result := &BackupResult{
		Path:      outputPath,
		Size:      info.Size(),
		CreatedAt: time.Now(),
	}

	logger.Infof("Backup created: %s (%d bytes, %d files)", outputPath, info.Size(), len(files))

	// 清理旧备份
	if cfg.MaxBackups > 0 {
		cleanupOldBackups(cfg.OutputDir, cfg.Prefix, cfg.MaxBackups)
	}

	return result, nil
}

// ListBackups 列出备份文件
func ListBackups(outputDir, prefix string) ([]BackupResult, error) {
	if outputDir == "" {
		outputDir = "./backups"
	}
	if prefix == "" {
		prefix = "unimap"
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, err
	}

	var results []BackupResult
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".tar.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		results = append(results, BackupResult{
			Path:      filepath.Join(outputDir, name),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}

	// 按时间倒序
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return results, nil
}

// collectFiles 递归收集目录下的所有文件，返回文件列表和基础目录
func collectFiles(path string) ([]string, string, error) {
	return collectFilesContext(context.Background(), path)
}

func collectFilesContext(ctx context.Context, path string) ([]string, string, error) {
	return collectFilesContextExcluding(ctx, path, nil)
}

func collectFilesContextExcluding(ctx context.Context, path string, output *backupOutputBoundary) ([]string, string, error) {
	if output != nil {
		if err := output.validateSource(path); err != nil {
			return nil, "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}

	if !info.IsDir() {
		return []string{path}, filepath.Dir(path), nil
	}

	var files []string
	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if output != nil {
			excluded, directory := output.excluded(path, info)
			if excluded {
				if directory {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, path, err
}

// addFileToTar 将文件添加到 tar
func addFileToTarContext(ctx context.Context, tw *tar.Writer, path string, baseDir string) error {
	return addNamedFileToTarContext(ctx, tw, path, baseDir, "")
}

func addNamedFileToTarContext(ctx context.Context, tw *tar.Writer, path string, baseDir string, archiveName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	relPath, err := filepath.Rel(baseDir, path)
	if err != nil || !filepath.IsLocal(relPath) || relPath == "." {
		return fmt.Errorf("backup file is outside its source directory")
	}
	// Follow only links constrained by the source root. Keep the same handle
	// for metadata and contents instead of reopening a checked pathname.
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return err
	}
	defer root.Close()
	f, err := root.Open(relPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup source entry is not a regular file")
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	// Tar member names always use '/', including archives made on Windows.
	header.Name = filepath.ToSlash(relPath)
	if archiveName != "" {
		name := archiveName
		if !filepath.IsLocal(name) || name == "." {
			return fmt.Errorf("invalid snapshot archive name")
		}
		header.Name = filepath.ToSlash(name)
	}
	if writeErr := tw.WriteHeader(header); writeErr != nil {
		return writeErr
	}
	_, err = io.Copy(tw, contextReader{ctx: ctx, reader: f})
	return err
}

// cleanupOldBackups 清理超过 maxBackups 的旧备份
func cleanupOldBackups(dir, prefix string, maxBackups int) {
	backups, err := ListBackups(dir, prefix)
	if err != nil {
		logger.Warnf("Failed to list backups for cleanup: %v", err)
		return
	}

	if len(backups) <= maxBackups {
		return
	}

	// 删除最旧的
	toDelete := backups[maxBackups:]
	for _, b := range toDelete {
		if err := os.Remove(b.Path); err != nil {
			logger.Warnf("Failed to remove old backup %s: %v", b.Path, err)
		} else {
			logger.Infof("Removed old backup: %s", b.Path)
		}
	}
}

// contextReader intentionally exposes only Read, so io.Copy cannot select a
// file WriteTo fast path that bypasses cancellation checks. In-flight filesystem
// syscalls themselves are not interrupted; cancellation is checked when they return.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return 0, ctxErr
	}
	return n, err
}
