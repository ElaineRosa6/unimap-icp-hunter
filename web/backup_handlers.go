package web

import (
	"fmt"
	"net/http"
	"os"

	"github.com/unimap-icp-hunter/project/internal/backup"
)

// handleCreateBackup POST /api/backup/create
func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST", nil)
		return
	}

	// 从配置读取备份目录
	backupDir := "./backups"
	backupPrefix := "unimap"
	if s.config != nil && s.config.Backup.OutputDir != "" {
		backupDir = s.config.Backup.OutputDir
	}
	if s.config != nil && s.config.Backup.Prefix != "" {
		backupPrefix = s.config.Backup.Prefix
	}

	// 收集要备份的源
	sources := s.buildBackupSources()
	if len(sources) == 0 {
		writeAPIError(w, http.StatusBadRequest, "no_sources", "no backup sources configured", nil)
		return
	}

	cfg := backup.BackupConfig{
		Sources:    sources,
		OutputDir:  backupDir,
		MaxBackups: s.config.Backup.MaxBackups,
		Prefix:     backupPrefix,
	}

	result, err := backup.Backup(cfg)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "backup_failed", err.Error(), nil)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"path":       result.Path,
		"size":       result.Size,
		"created_at": result.CreatedAt,
	})
}

// handleListBackups GET /api/backup/list
func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET", nil)
		return
	}

	backupDir := "./backups"
	backupPrefix := "unimap"
	if s.config != nil && s.config.Backup.OutputDir != "" {
		backupDir = s.config.Backup.OutputDir
	}
	if s.config != nil && s.config.Backup.Prefix != "" {
		backupPrefix = s.config.Backup.Prefix
	}

	backups, err := backup.ListBackups(backupDir, backupPrefix)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_failed", err.Error(), nil)
		return
	}

	// 确保返回空数组而非 null
	if backups == nil {
		backups = []backup.BackupResult{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"backups": backups,
		"count":   len(backups),
	})
}

// buildBackupSources 构建备份源列表
func (s *Server) buildBackupSources() []string {
	// 如果配置了自定义源，使用配置的
	if s.config != nil && len(s.config.Backup.Sources) > 0 {
		var sources []string
		for _, src := range s.config.Backup.Sources {
			if dirExists(src) {
				sources = append(sources, src)
			}
		}
		return sources
	}

	sources := []string{}

	// 始终包含 hash_store（篡改检测基线）
	if dirExists("./hash_store") {
		sources = append(sources, "./hash_store")
	}

	// 包含截图数据
	if dirExists("./screenshots") {
		sources = append(sources, "./screenshots")
	}

	// 包含调度器数据
	if dirExists("./data") {
		sources = append(sources, "./data")
	}

	// 包含配置文件
	if dirExists("./configs") {
		sources = append(sources, "./configs")
	}

	return sources
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func formatBackupSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
