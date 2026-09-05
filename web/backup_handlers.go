package web

import (
	"fmt"
	"net/http"
	"os"

	"github.com/unimap/project/internal/backup"
	"github.com/unimap/project/internal/utils"
)

// isAuthEnabled returns true if the server has auth configured.
func (s *Server) isAuthEnabled() bool {
	cfg := s.currentConfig()
	return cfg != nil && cfg.Web.Auth.Enabled
}

// handleCreateBackup POST /api/v1/backup/create
func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST", nil)
		return
	}
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
		return
	}
	if s.isAuthEnabled() {
		if ok, msg := s.requireAdmin(r); !ok {
			writeAPIError(w, http.StatusForbidden, "forbidden", msg, nil)
			return
		}
	}

	// 从配置读取备份目录
	backupDir := "./backups"
	backupPrefix := "unimap"
	maxBackups := 5
	if current := s.currentConfig(); current != nil {
		if current.Backup.OutputDir != "" {
			backupDir = current.Backup.OutputDir
		}
		if current.Backup.Prefix != "" {
			backupPrefix = current.Backup.Prefix
		}
		if current.Backup.MaxBackups > 0 {
			maxBackups = current.Backup.MaxBackups
		}
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
		MaxBackups: maxBackups,
		Prefix:     backupPrefix,
	}

	result, err := backup.BackupContext(r.Context(), cfg)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "backup_failed", "backup failed", sanitizeError(err.Error()))
		return
	}

	type backupCreateResponse struct {
		Path      string      `json:"path"`
		Size      int64       `json:"size"`
		CreatedAt interface{} `json:"created_at"`
	}
	writeJSON(w, http.StatusCreated, backupCreateResponse{
		Path: result.Path, Size: result.Size, CreatedAt: result.CreatedAt,
	})
}

// handleListBackups GET /api/v1/backup/list
func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET", nil)
		return
	}
	if s.isAuthEnabled() {
		if ok, msg := s.requireAdmin(r); !ok {
			writeAPIError(w, http.StatusForbidden, "forbidden", msg, nil)
			return
		}
	}

	backupDir := "./backups"
	backupPrefix := "unimap"
	if current := s.currentConfig(); current != nil {
		if current.Backup.OutputDir != "" {
			backupDir = current.Backup.OutputDir
		}
		if current.Backup.Prefix != "" {
			backupPrefix = current.Backup.Prefix
		}
	}

	backups, err := backup.ListBackups(backupDir, backupPrefix)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_failed", "list backups failed", sanitizeError(err.Error()))
		return
	}

	// 确保返回空数组而非 null
	if backups == nil {
		backups = []backup.BackupResult{}
	}

	type backupListResponse struct {
		Backups []backup.BackupResult `json:"backups"`
		Count   int                   `json:"count"`
	}
	writeJSON(w, http.StatusOK, backupListResponse{Backups: backups, Count: len(backups)})
}

// buildBackupSources 构建备份源列表
func (s *Server) buildBackupSources() []string {
	// 如果配置了自定义源，使用配置的
	if current := s.currentConfig(); current != nil && len(current.Backup.Sources) > 0 {
		// Keep explicitly configured sources, including files and missing paths.
		// Backup must report a missing source, not silently create a subset.
		return append([]string(nil), current.Backup.Sources...)
	}

	sources := []string{}

	// 始终包含 hash_store（篡改检测基线）
	if dirExists(utils.HashStoreDir()) {
		sources = append(sources, utils.HashStoreDir())
	}

	// 包含截图数据
	if dirExists(utils.ScreenshotsDir()) {
		sources = append(sources, utils.ScreenshotsDir())
	}

	// 包含调度器数据
	if dirExists(utils.AppDataDir()) {
		sources = append(sources, utils.AppDataDir())
	}

	// 注意：不包含 ./configs，避免泄露敏感配置（API keys、tokens）
	// 如需备份配置，请手动添加到自定义源

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
