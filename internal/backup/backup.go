package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// BackupConfig 备份配置
type BackupConfig struct {
	// Sources 要备份的目录/文件列表（相对于 baseDir 或绝对路径）
	Sources []string
	// OutputDir 备份输出目录
	OutputDir string
	// MaxBackups 最大保留备份数量，0 表示不限制
	MaxBackups int
	// Prefix 备份文件名前缀
	Prefix string
}

// BackupResult 备份结果
type BackupResult struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// Backup 执行备份
func Backup(cfg BackupConfig) (*BackupResult, error) {
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

	// 生成备份文件名
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_backup_%s.tar.gz", cfg.Prefix, timestamp)
	outputPath := filepath.Join(cfg.OutputDir, filename)

	// 创建 gzip 文件
	outFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup file: %w", err)
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// 收集所有要备份的文件
	var files []string
	for _, src := range cfg.Sources {
		srcFiles, err := collectFiles(src)
		if err != nil {
			logger.Warnf("Backup source %s: %v", src, err)
			continue
		}
		files = append(files, srcFiles...)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found to backup")
	}

	// 写入 tar
	for _, file := range files {
		if err := addFileToTar(tw, file); err != nil {
			logger.Warnf("Failed to add %s to backup: %v", file, err)
		}
	}

	// 获取文件大小
	info, err := outFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat backup file: %w", err)
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

// collectFiles 递归收集目录下的所有文件
func collectFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	var files []string
	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// addFileToTar 将文件添加到 tar
func addFileToTar(tw *tar.Writer, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	// 使用相对路径
	header.Name = path
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(tw, f)
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
