package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/service"
)

func (s *Server) runBrowserQueryAsync(ctx context.Context, query string, engines []string, enabled bool, queryID string) <-chan browserQueryOutcome {
	autoCaptureEnabled := false
	if s.config != nil {
		autoCaptureEnabled = s.config.Screenshot.AutoCapture.Enabled && s.config.Screenshot.AutoCapture.CaptureSearchResults
	}

	return s.queryApp.RunBrowserQueryAsync(
		ctx,
		query,
		engines,
		enabled,
		queryID,
		autoCaptureEnabled,
		s.screenshotApp,
		s.screenshotMgr,
		s.screenshotPathToPreviewURL,
	)
}

func buildQueryAPIPayload(query string, engines []string, resp *service.QueryResponse, browserOutcome browserQueryOutcome, explicitErrors ...string) map[string]interface{} {
	combinedErrors := []string{}
	if resp != nil {
		combinedErrors = append(combinedErrors, resp.Errors...)
	}
	combinedErrors = appendUniqueStrings(combinedErrors, browserOutcome.Errors)
	combinedErrors = appendUniqueStrings(combinedErrors, browserOutcome.AutoCaptureErrors)
	combinedErrors = appendUniqueStrings(combinedErrors, explicitErrors)

	assets := []model.UnifiedAsset{}
	totalCount := 0
	engineStats := map[string]int{}
	if resp != nil {
		assets = resp.Assets
		totalCount = resp.TotalCount
		engineStats = resp.EngineStats
	}

	return map[string]interface{}{
		"query":                query,
		"engines":              engines,
		"assets":               assets,
		"totalCount":           totalCount,
		"engineStats":          engineStats,
		"errors":               combinedErrors,
		"browserQuery":         browserOutcome.Enabled,
		"browserOpenedEngines": browserOutcome.OpenedEngines,
		"browserQueryErrors":   browserOutcome.Errors,
		"autoCapture":          browserOutcome.AutoCaptureEnabled,
		"autoCaptureQueryID":   browserOutcome.AutoCaptureQueryID,
		"autoCapturedPaths":    browserOutcome.AutoCapturedPaths,
		"autoCaptureErrors":    browserOutcome.AutoCaptureErrors,
	}
}

// handleAPIQuery 处理API查询请求（用于异步查询）
func (s *Server) handleAPIQuery(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if err := validateQueryInput(query); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
		return
	}

	s.applyCookiesFromRequest(r)

	pageSizeStr := r.FormValue("page_size")

	// 解析页码和页大小
	pageSize := 50
	if pageSizeStr != "" {
		if size, err := strconv.Atoi(pageSizeStr); err == nil && size > 0 {
			pageSize = size
		}
	}

	// 解析引擎列表（支持 engines=a&engines=b 和 engines=a,b 两种形式）
	engines := s.queryApp.ResolveEngines(parseEnginesParam(r))
	if len(engines) == 0 {
		writeAPIError(w, http.StatusServiceUnavailable, "no_engines_available", "no engines configured or registered", nil)
		return
	}

	browserQueryID := fmt.Sprintf("query_%d", time.Now().UnixNano())
	browserQueryCh := s.runBrowserQueryAsync(r.Context(), query, engines, parseBoolValue(r.FormValue("browser_query")), browserQueryID)

	resp, err := s.queryApp.ExecuteQuery(r.Context(), query, engines, pageSize)
	var browserOutcome browserQueryOutcome
	if browserQueryCh != nil {
		browserOutcome = <-browserQueryCh
	}
	if err != nil {
		writeAPIError(
			w,
			http.StatusBadGateway,
			"query_execution_failed",
			fmt.Sprintf("query failed: %v", err),
			buildQueryAPIPayload(query, engines, nil, browserOutcome, fmt.Sprintf("Query failed: %v", err)),
		)
		return
	}

	// 返回JSON结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildQueryAPIPayload(query, engines, resp, browserOutcome))
}

// handleIndex 处理首页请求
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	engines := s.orchestrator.ListAdapters()
	var fofaCookies, hunterCookies, quakeCookies, zoomeyeCookies []config.Cookie
	proxyServer := ""
	if s.config != nil {
		fofaCookies = s.config.Engines.Fofa.Cookies
		hunterCookies = s.config.Engines.Hunter.Cookies
		quakeCookies = s.config.Engines.Quake.Cookies
		zoomeyeCookies = s.config.Engines.Zoomeye.Cookies
		proxyServer = strings.TrimSpace(s.config.Screenshot.ProxyServer)
	}
	if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "index.html", map[string]interface{}{
		"engines":          engines,
		"staticVersion":    s.staticVersion,
		"proxyServer":      proxyServer,
		"cookieFofa":       cookiesToHeader(fofaCookies),
		"cookieHunter":     cookiesToHeader(hunterCookies),
		"cookieQuake":      cookiesToHeader(quakeCookies),
		"cookieZoomeye":    cookiesToHeader(zoomeyeCookies),
		"cookieHasFofa":    hasCookies(fofaCookies),
		"cookieHasHunter":  hasCookies(hunterCookies),
		"cookieHasQuake":   hasCookies(quakeCookies),
		"cookieHasZoomeye": hasCookies(zoomeyeCookies),
	}) {
		return
	}
}

// handleQuery 处理查询请求
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if err := validateQueryInput(query); err != nil {
		if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "error.html", map[string]interface{}{
			"error": err.Error(),
		}) {
			return
		}
		return
	}

	s.applyCookiesFromRequest(r)

	pageSize := 50

	// 解析引擎列表（支持 engines=a&engines=b 和 engines=a,b 两种形式）
	engines := parseEnginesParam(r)
	if len(engines) == 0 {
		// 如果没有选择引擎，使用默认引擎
		defaultEngines := s.orchestrator.ListAdapters()
		if len(defaultEngines) > 0 {
			engines = []string{defaultEngines[0]}
		}
	}
	if len(engines) == 0 {
		if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "error.html", map[string]interface{}{
			"error": "No engines configured/registered. Please set API keys in configs/config.yaml and enable at least one engine.",
		}) {
			return
		}
		return
	}

	// 执行查询
	req := service.QueryRequest{
		Query:       query,
		Engines:     engines,
		PageSize:    pageSize,
		ProcessData: true,
	}

	resp, err := s.service.Query(r.Context(), req)
	if err != nil {
		if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "error.html", map[string]interface{}{
			"error":   fmt.Sprintf("Query failed: %v", err),
			"query":   query,
			"engines": engines,
		}) {
			return
		}
		return
	}

	// 渲染结果页面
	if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "results.html", map[string]interface{}{
		"query":         query,
		"engines":       engines,
		"assets":        resp.Assets,
		"totalCount":    resp.TotalCount,
		"engineStats":   resp.EngineStats,
		"errors":        resp.Errors,
		"staticVersion": s.staticVersion,
	}) {
		return
	}
}

// handleResults 处理结果页面请求
func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	engines := []string{}
	if engine := strings.TrimSpace(r.URL.Query().Get("engine")); engine != "" {
		engines = []string{engine}
	}

	// 渲染结果页面
	if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "results.html", map[string]interface{}{
		"query":         query,
		"engines":       engines,
		"assets":        []model.UnifiedAsset{},
		"staticVersion": s.staticVersion,
	}) {
		return
	}
}

// handleQuota 处理配额页面请求
func (s *Server) handleQuota(w http.ResponseWriter, r *http.Request) {
	engines := s.orchestrator.ListAdapters()
	quotaInfo := make(map[string]*model.QuotaInfo)
	errorInfo := make(map[string]string)

	for _, engine := range engines {
		adapter, exists := s.orchestrator.GetAdapter(engine)
		if exists {
			quota, err := adapter.GetQuota()
			if err != nil {
				msg := strings.TrimSpace(err.Error())
				if msg == "" {
					msg = "failed to fetch quota"
				}
				errorInfo[engine] = msg
				continue
			}
			if quota == nil {
				errorInfo[engine] = "quota not available"
				continue
			}
			quotaInfo[engine] = quota
		}
	}

	if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "quota.html", map[string]interface{}{
		"engines":       engines,
		"quotaInfo":     quotaInfo,
		"errorInfo":     errorInfo,
		"staticVersion": s.staticVersion,
	}) {
		return
	}
}

// handleQueryStatus 处理查询状态请求
func (s *Server) handleQueryStatus(w http.ResponseWriter, r *http.Request) {
	queryID := r.URL.Query().Get("query_id")
	if queryID == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_query_id", "query_id is required", nil)
		return
	}

	// 获取查询状态
	s.queryMutex.RLock()
	status, exists := s.queryStatus[queryID]
	var statusCopy QueryStatus
	if exists && status != nil {
		statusCopy = *status
	}
	s.queryMutex.RUnlock()

	if !exists {
		writeAPIError(w, http.StatusNotFound, "query_not_found", "query not found", map[string]string{"query_id": queryID})
		return
	}

	// 返回JSON结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statusCopy)
}
