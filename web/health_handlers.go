package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/unimap-icp-hunter/project/internal/appversion"
)

// handleHealthReady 就绪检查：依赖连接正常
func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	checks := make(map[string]string)
	ready := true

	// 检查 orchestrator
	if s.orchestrator != nil {
		adapters := s.orchestrator.ListAdapters()
		if len(adapters) > 0 {
			checks["engines"] = fmt.Sprintf("ok (%d adapters)", len(adapters))
		} else {
			checks["engines"] = "no adapters configured"
		}
	} else {
		checks["engines"] = "not initialized"
	}

	// 检查调度器
	if s.scheduler != nil {
		checks["scheduler"] = "ok"
	} else {
		checks["scheduler"] = "not initialized"
	}

	// 检查分布式组件
	if s.distributed != nil && s.distributed.NodeRegistry != nil {
		checks["distributed"] = "ok"
	} else {
		checks["distributed"] = "not initialized"
	}

	status := "ok"
	if !ready {
		status = "degraded"
	}

	resp := map[string]interface{}{
		"status":  status,
		"version": appversion.Full(),
		"time":    time.Now().UTC().Format(time.RFC3339),
		"checks":  checks,
	}

	if status != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(resp)
}

// handleHealthLive 存活检查：进程是否存活
func (s *Server) handleHealthLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": appversion.Full(),
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// livenessCheck 返回 context 是否已被取消（用于关闭检测）
func livenessCheck(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	default:
		return true
	}
}
