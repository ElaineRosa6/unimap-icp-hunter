package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/unimap/project/internal/collection"
	"github.com/unimap/project/internal/config"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/screenshot"
	"github.com/unimap/project/internal/service"
)

// stableEngines is the browser-tested Web UI allowlist.
var stableEngines = map[string]bool{
	"fofa": true, "hunter": true, "zoomeye": true, "quake": true, "shodan": true,
	"censys": true, "daydaymap": true,
}

func filterStableEngines(engines []string) []string {
	out := make([]string, 0, len(engines))
	for _, e := range engines {
		if stableEngines[strings.ToLower(e)] {
			out = append(out, e)
		}
	}
	return out
}

func (s *Server) runBrowserQueryAsync(ctx context.Context, query string, engines []string, enabled bool, action string, queryID string, progress func(done, total int, engine string, err error)) <-chan browserQueryOutcome {
	autoCaptureEnabled := false
	if cfg := s.currentConfig(); cfg != nil {
		autoCaptureEnabled = cfg.Screenshot.AutoCapture.Enabled && cfg.Screenshot.AutoCapture.CaptureSearchResults
	}

	return s.queryApp.RunBrowserQueryAsync(
		ctx,
		query,
		engines,
		enabled,
		action,
		queryID,
		autoCaptureEnabled,
		s.screenshotApp,
		s.screenshotMgr,
		s.screenshotPathToPreviewURL,
		s.browserQueryProvider(),
		progress,
	)
}

func (s *Server) browserQueryProvider() screenshot.Provider {
	if s == nil {
		return nil
	}
	if s.screenshotRouter != nil {
		return s.screenshotRouter
	}
	if s.screenshotMgr != nil {
		return screenshot.NewCDPProvider(s.screenshotMgr)
	}
	return nil
}

// QueryAPIPayload is the typed response for the query API.
type QueryAPIPayload struct {
	Status               string                     `json:"status"`
	Query                string                     `json:"query"`
	Engines              []string                   `json:"engines"`
	Assets               []model.UnifiedAsset       `json:"assets"`
	TotalCount           int                        `json:"totalCount"`
	EngineStats          map[string]int             `json:"engineStats"`
	Errors               []string                   `json:"errors"`
	Persistence          service.PersistenceStatus  `json:"persistence,omitempty"`
	Error                string                     `json:"error,omitempty"`
	BrowserQuery         bool                       `json:"browserQuery"`
	BrowserAction        string                     `json:"browserAction"`
	BrowserOpenedEngines []string                   `json:"browserOpenedEngines"`
	BrowserCollectedData []collection.CollectResult `json:"browserCollectedData"`
	BrowserQueryErrors   []string                   `json:"browserQueryErrors"`
	AutoCapture          bool                       `json:"autoCapture"`
	AutoCaptureQueryID   string                     `json:"autoCaptureQueryID"`
	AutoCapturedPaths    map[string]string          `json:"autoCapturedPaths"`
	AutoCaptureErrors    []string                   `json:"autoCaptureErrors"`
}

// buildQueryAPIPayload 组装查询 API 响应。browserAssetsInResp 为 true 时表示
// resp.Assets 已由 service 层 merge 了浏览器采集资产（HTTP API 走
// ExecuteQueryWithBrowserWorkflow），此处不再追加，避免重复计数；WebSocket
// 路径独立并行，resp 不含浏览器资产，需在本函数内追加。
func buildQueryAPIPayload(query string, engines []string, resp *service.QueryResponse, browserOutcome browserQueryOutcome, browserAction string, browserAssetsInResp bool, explicitErrors ...string) QueryAPIPayload {
	for i := range browserOutcome.CollectedResults {
		collection.NormalizeAssets(browserOutcome.CollectedResults[i].Engine, browserOutcome.CollectedResults[i].Assets)
	}

	// Preserve service diagnostics so HTTP status agrees with persisted history.
	// Opening a tab or receiving an empty collection is not API recovery.
	combinedErrors := []string{}
	if resp != nil {
		combinedErrors = appendUniqueStrings(combinedErrors, resp.Errors)
	}
	combinedErrors = appendUniqueStrings(combinedErrors, browserOutcome.Errors)
	combinedErrors = appendUniqueStrings(combinedErrors, browserOutcome.AutoCaptureErrors)
	combinedErrors = appendUniqueStrings(combinedErrors, explicitErrors)

	assets := []model.UnifiedAsset{}
	totalCount := 0
	engineStats := map[string]int{}
	persistence := service.PersistenceStatus{}
	if resp != nil {
		assets = resp.Assets
		totalCount = resp.TotalCount
		if resp.EngineStats != nil {
			engineStats = resp.EngineStats
		}
		persistence = resp.Persistence
	}
	if !browserAssetsInResp {
		for _, collected := range browserOutcome.CollectedResults {
			assets = append(assets, collected.Assets...)
			if collected.Total > 0 {
				totalCount += collected.Total
			} else {
				totalCount += len(collected.Assets)
			}
			if len(collected.Assets) > 0 {
				engineStats[collected.Engine] += len(collected.Assets)
			}
		}
	}

	status := "success"
	if len(combinedErrors) > 0 {
		if len(assets) > 0 {
			status = "partial"
		} else {
			status = "error"
		}
	}

	return QueryAPIPayload{
		Status:               status,
		Query:                query,
		Engines:              engines,
		Assets:               assets,
		TotalCount:           totalCount,
		EngineStats:          engineStats,
		Errors:               combinedErrors,
		Persistence:          persistence,
		BrowserQuery:         browserOutcome.Enabled,
		BrowserAction:        browserAction,
		BrowserOpenedEngines: browserOutcome.OpenedEngines,
		BrowserCollectedData: browserOutcome.CollectedResults,
		BrowserQueryErrors:   browserOutcome.Errors,
		AutoCapture:          browserOutcome.AutoCaptureEnabled,
		AutoCaptureQueryID:   browserOutcome.AutoCaptureQueryID,
		AutoCapturedPaths:    browserOutcome.AutoCapturedPaths,
		AutoCaptureErrors:    browserOutcome.AutoCaptureErrors,
	}
}

// handleAPIQuery 处理API查询请求（用于异步查询）
func (s *Server) handleAPIQuery(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
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
	// maxPageSize 放开到 3000：DayDayMap API key 单页上限为 2500（更大页返回"积分不足"），
	// 全局上限需覆盖该能力，避免前端/API 无法一次拉取引擎允许的整页。
	const maxPageSize = 3000
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	// 解析引擎列表（支持 engines=a&engines=b 和 engines=a,b 两种形式）
	engines := s.queryApp.ResolveEngines(parseEnginesParam(r))
	if len(engines) == 0 {
		writeAPIError(w, http.StatusServiceUnavailable, "no_engines_available", "no engines configured or registered", nil)
		return
	}

	browserQueryID := fmt.Sprintf("query_%d", time.Now().UnixNano())
	browserAction := strings.TrimSpace(r.FormValue("browser_action"))
	browserEnabled := parseBoolValue(r.FormValue("browser_query"))

	// A query may legitimately outlive the server's general 60s write timeout.
	// Extend only this validated request, keeping an explicit finite deadline.
	queryBudget := service.QueryExecutionTimeout
	if browserEnabled {
		if browserAction == "" {
			browserAction = "collect_and_capture"
		}
		queryBudget = service.BrowserQueryWaitTimeoutForAction(browserAction)
	}
	queryDeadline := time.Now().Add(queryBudget)
	if parentDeadline, ok := r.Context().Deadline(); ok && parentDeadline.Before(queryDeadline) {
		queryDeadline = parentDeadline
	}
	// Allow bounded time to encode and flush a success or timeout response.
	if deadlineErr := http.NewResponseController(w).SetWriteDeadline(queryDeadline.Add(15 * time.Second)); deadlineErr != nil && !errors.Is(deadlineErr, http.ErrNotSupported) {
		writeAPIError(w, http.StatusInternalServerError, "query_deadline_failed", "failed to configure query response deadline", nil)
		return
	}

	var resp *service.QueryResponse
	var browserOutcome browserQueryOutcome
	var err error
	if browserEnabled {
		// Match the synchronous workflow's default before choosing its budget.
		// The legacy WebSocket collect default is intentionally unchanged.
		if browserAction == "" {
			browserAction = "collect_and_capture"
		}
		autoCaptureEnabled := false
		if cfg := s.currentConfig(); cfg != nil {
			autoCaptureEnabled = cfg.Screenshot.AutoCapture.Enabled && cfg.Screenshot.AutoCapture.CaptureSearchResults
		}
		workflowCtx, cancel := context.WithTimeout(r.Context(), service.BrowserQueryWaitTimeoutForAction(browserAction))
		defer cancel()
		resp, browserOutcome, err = s.queryApp.ExecuteQueryWithBrowserWorkflow(workflowCtx, query, engines, pageSize, service.BrowserQueryWorkflowOptions{
			Action:             browserAction,
			QueryID:            browserQueryID,
			AutoCaptureEnabled: autoCaptureEnabled,
			PreviewURLBuilder:  s.screenshotPathToPreviewURL,
			ScreenshotApp:      s.screenshotApp,
			ScreenshotManager:  s.screenshotMgr,
			BrowserRouter:      s.browserQueryProvider(),
		})
	} else {
		resp, err = s.queryApp.ExecuteQuery(r.Context(), query, engines, pageSize)
	}
	if err != nil {
		payload := buildQueryAPIPayload(query, engines, resp, browserOutcome, browserAction, true, fmt.Sprintf("API query failed: %v", err))
		if payload.Status == "partial" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		}
		writeAPIError(
			w,
			http.StatusBadGateway,
			"query_execution_failed",
			fmt.Sprintf("query failed: %v", err),
			payload,
		)
		return
	}

	// 返回JSON结果
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildQueryAPIPayload(query, engines, resp, browserOutcome, browserAction, true))
}

// handleIndex 处理首页请求
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	engines := filterStableEngines(s.orchestrator.ListAdapters())
	var fofaCookies, hunterCookies, quakeCookies, zoomeyeCookies []config.Cookie
	proxyServer := ""
	if cfg := s.currentConfig(); cfg != nil {
		fofaCookies = cfg.Engines.Fofa.Cookies
		hunterCookies = cfg.Engines.Hunter.Cookies
		quakeCookies = cfg.Engines.Quake.Cookies
		zoomeyeCookies = cfg.Engines.Zoomeye.Cookies
		proxyServer = strings.TrimSpace(cfg.Screenshot.ProxyServer)
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
		defaultEngines := filterStableEngines(s.orchestrator.ListAdapters())
		if len(defaultEngines) > 0 {
			engines = []string{defaultEngines[0]}
		}
	}
	if len(engines) == 0 {
		if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "error.html", map[string]interface{}{
			"error": "no engines configured/registered. Please set API keys in configs/config.yaml and enable at least one engine.",
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

	resp, err := s.queryApp.ExecuteQuery(r.Context(), req.Query, req.Engines, req.PageSize)
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
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	engines := []string{}
	if engine := strings.TrimSpace(r.URL.Query().Get("engine")); engine != "" {
		engines = []string{engine}
	}

	// 渲染结果页面
	if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "results.html", map[string]interface{}{
		"query":         query,
		"engines":       engines,
		"assets":        []model.UnifiedAsset{},
		"totalCount":    0,
		"engineStats":   map[string]int{},
		"staticVersion": s.staticVersion,
	}) {
		return
	}
}

// handleQuota 处理配额页面请求
func (s *Server) handleQuota(w http.ResponseWriter, r *http.Request) {
	engines := filterStableEngines(s.orchestrator.ListAdapters())
	quotaInfo, errorInfo := s.fetchEngineQuotas(engines)
	if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "quota.html", map[string]interface{}{
		"engines": engines, "quotaInfo": quotaInfo, "errorInfo": errorInfo, "staticVersion": s.staticVersion,
	}) {
		return
	}
}

// fetchEngineQuotas 并发获取所有引擎配额
func (s *Server) fetchEngineQuotas(engines []string) (map[string]*model.QuotaInfo, map[string]string) {
	type quotaResult struct {
		engine string
		quota  *model.QuotaInfo
		err    error
	}
	quotaInfo := make(map[string]*model.QuotaInfo)
	errorInfo := make(map[string]string)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := make(chan quotaResult, len(engines))
	for _, engine := range engines {
		go func(e string) {
			adapter, exists := s.orchestrator.GetAdapter(e)
			if !exists {
				ch <- quotaResult{engine: e, err: fmt.Errorf("adapter not found")}
				return
			}
			quota, err := adapter.GetQuota()
			select {
			case ch <- quotaResult{engine: e, quota: quota, err: err}:
			case <-ctx.Done():
			}
		}(engine)
	}

	results := make(map[string]quotaResult)
	for i := 0; i < len(engines); i++ {
		select {
		case res := <-ch:
			results[res.engine] = res
		case <-ctx.Done():
			break
		}
	}

	for _, engine := range engines {
		res, ok := results[engine]
		if !ok {
			errorInfo[engine] = "timeout: failed to fetch quota"
		} else if res.err != nil {
			errorInfo[engine] = truncateQuotaError(res.err.Error())
		} else if res.quota == nil {
			errorInfo[engine] = "quota not available"
		} else {
			quotaInfo[engine] = res.quota
		}
	}
	return quotaInfo, errorInfo
}

func truncateQuotaError(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "failed to fetch quota"
	}
	if len(msg) > 120 {
		lines := strings.SplitN(msg, "\n", 2)
		short := strings.TrimSpace(lines[0])
		if len(short) > 120 {
			short = short[:120] + "..."
		}
		return short
	}
	return msg
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
	_ = json.NewEncoder(w).Encode(statusCopy)
}

// handleAccountPage renders the account management page (GET /account).
func (s *Server) handleAccountPage(w http.ResponseWriter, r *http.Request) {
	username := ""
	tokenPrefix := ""
	isMultiUser := s.userRepo != nil
	role := ""
	cfg := s.currentConfig()

	// Try to get current user from session
	currentUser := s.getCurrentUser(r)
	if currentUser != nil && currentUser.ID > 0 {
		// Real user from user DB
		username = currentUser.Username
		role = currentUser.Role
	} else if currentUser != nil {
		// Synthetic admin (token auth, userID=-1)
		role = currentUser.Role
		if cfg != nil {
			token := s.adminToken()
			if len(token) >= 8 {
				tokenPrefix = token[:8]
			}
		}
	} else if cfg != nil {
		// Legacy config-based user
		username = cfg.Web.Auth.Username
		token := s.adminToken()
		if len(token) >= 8 {
			tokenPrefix = token[:8]
		}
	}

	if !s.renderTemplateWithNonce(r, w, http.StatusOK, "account-page", map[string]interface{}{
		"username":      username,
		"tokenPrefix":   tokenPrefix,
		"staticVersion": s.staticVersion,
		"isMultiUser":   isMultiUser,
		"userID":        currentUserID(r),
		"role":          role,
	}) {
		return
	}
}

// handleGetAdminToken returns the admin token for authenticated users (GET /api/account/admin-token).
// Used by the account page to allow copying the token into the browser extension.
func (s *Server) handleGetAdminToken(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	token := s.adminToken()
	// When auth is disabled there is no admin token and no auth model in play;
	// preserve the legacy behavior of returning an empty token (no escalation risk).
	if token == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"token":   "",
		})
		return
	}
	// P0 fix (FINDING-001): a real admin token grants synthetic-admin (userID=-1)
	// that bypasses all role checks. Returning it in plaintext to any logged-in
	// user allowed vertical privilege escalation (normal user → super admin).
	//
	// Authorized identities:
	//   - adminSyntheticUserID (-1): request authenticated via X-Admin-Token
	//   - userID == 0: legacy single-user mode (config admin account, no user DB)
	// Only multi-user DB users must be checked for the admin role.
	uid := currentUserID(r)
	if uid > 0 {
		if ok, reason := s.requireAdmin(r); !ok {
			writeAPIError(w, http.StatusForbidden, "forbidden", reason, nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"token":   token,
	})
}

// currentUserID returns the user ID from context, or 0.
func currentUserID(r *http.Request) int64 {
	if uid, ok := r.Context().Value(contextKeyUserID).(int64); ok {
		return uid
	}
	return 0
}

// handleChangePassword handles POST /api/v1/account/change-password.
// In multi-user mode, redirects user-DB users to /api/v1/users/{id}/password.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Multi-user mode: if the user has a real DB account, redirect to user endpoint
	if s.userRepo != nil {
		uid := currentUserID(r)
		if uid > 0 {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":       "use /api/v1/users/" + fmt.Sprintf("%d", uid) + "/password instead",
				"redirect_to": fmt.Sprintf("/api/v1/users/%d/password", uid),
			})
			return
		}
		// In multi-user mode, legacy endpoint requires admin
		if ok, msg := s.requireAdmin(r); !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
			return
		}
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if s.currentConfig() == nil || s.configManager == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server configuration error"})
		return
	}

	currentHash := s.currentConfig().Web.Auth.PasswordHash
	if currentHash == "" || !config.CheckPassword(req.CurrentPassword, currentHash) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
		return
	}

	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new password must be at least 8 characters"})
		return
	}

	newHash, err := config.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to process password"})
		return
	}

	if _, err := s.updateConfig(func(cfg *config.Config) error {
		cfg.Web.Auth.PasswordHash = newHash
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist config"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"success": "password updated"})
}
