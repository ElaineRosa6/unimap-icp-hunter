package web

import (
	"net/http"
	"path/filepath"
)

// Route 定义路由
type Route struct {
	Name        string
	Method      string
	Pattern     string
	Handler     http.HandlerFunc
	RateLimited bool // 是否需要限流
}

// Router 路由管理器
type Router struct {
	routes []Route
	server *Server
}

// NewRouter 创建路由管理器
func NewRouter(s *Server) *Router {
	return &Router{
		routes: make([]Route, 0),
		server: s,
	}
}

// RegisterRoutes 注册所有路由
func (r *Router) RegisterRoutes() http.Handler {
	// 页面路由
	r.addRoute("index", "GET", "/", r.server.handleIndex, false)
	r.addRoute("results", "GET", "/results", r.server.handleResults, false)
	r.addRoute("quota", "GET", "/quota", r.server.handleQuota, false)
	r.addRoute("batch-screenshot", "GET", "/batch-screenshot", r.server.handleBatchScreenshotPage, false)
	r.addRoute("monitor", "GET", "/monitor", r.server.handleMonitorPage, false)

	// API 路由 - 查询相关（限流）
	r.addRoute("health", "GET", "/health", r.server.handleHealth, false)
	r.addRoute("metrics", "GET", "/metrics", r.server.handleMetrics, false)
	r.addRoute("query", "GET", "/query", r.server.handleQuery, true)
	r.addRoute("api-query", "POST", "/api/query", r.server.handleAPIQuery, true)
	r.addRoute("query-status", "GET", "/api/query/status", r.server.handleQueryStatus, true)

	// API 路由 - Cookie 管理
	r.addRoute("cookies-save", "POST", "/api/cookies", r.server.handleSaveCookies, false)
	r.addRoute("cookies-verify", "POST", "/api/cookies/verify", r.server.handleVerifyCookies, false)
	r.addRoute("cookies-import", "POST", "/api/cookies/import", r.server.handleImportCookieJSON, false)

	// API 路由 - CDP
	r.addRoute("cdp-status", "GET", "/api/cdp/status", r.server.handleCDPStatus, false)
	r.addRoute("cdp-connect", "POST", "/api/cdp/connect", r.server.handleCDPConnect, false)

	// API 路由 - WebSocket
	r.addRoute("websocket", "GET", "/api/ws", r.server.handleWebSocket, false)

	// API 路由 - 截图（限流）
	r.addRoute("screenshot", "POST", "/api/screenshot", r.server.handleScreenshot, true)
	r.addRoute("screenshot-engine", "GET", "/api/screenshot/search-engine", r.server.handleSearchEngineScreenshot, true)
	r.addRoute("screenshot-target", "POST", "/api/screenshot/target", r.server.handleTargetScreenshot, true)
	r.addRoute("screenshot-batch", "POST", "/api/screenshot/batch", r.server.handleBatchScreenshot, true)
	r.addRoute("screenshot-batch-urls", "POST", "/api/screenshot/batch-urls", r.server.handleBatchURLsScreenshot, true)
	r.addRoute("screenshot-batches", "GET", "/api/screenshot/batches", r.server.handleScreenshotBatches, false)
	r.addRoute("screenshot-batch-files", "GET", "/api/screenshot/batches/files", r.server.handleScreenshotBatchFiles, false)
	r.addRoute("screenshot-batch-delete", "DELETE", "/api/screenshot/batches/delete", r.server.handleScreenshotBatchDelete, false)
	r.addRoute("screenshot-file-delete", "DELETE", "/api/screenshot/file/delete", r.server.handleScreenshotFileDelete, false)
	r.addRoute("screenshot-file", "GET", "/screenshots/", r.server.handleScreenshotFile, false)

	// API 路由 - 导入（限流）
	r.addRoute("import-urls", "POST", "/api/import/urls", r.server.handleImportURLs, true)
	r.addRoute("url-reachability", "POST", "/api/url/reachability", r.server.handleURLReachability, true)

	// API 路由 - 篡改检测（限流）
	r.addRoute("tamper-check", "POST", "/api/tamper/check", r.server.handleTamperCheck, true)
	r.addRoute("tamper-baseline", "POST", "/api/tamper/baseline", r.server.handleTamperBaseline, true)
	r.addRoute("tamper-baseline-list", "GET", "/api/tamper/baseline/list", r.server.handleTamperBaselineList, false)
	r.addRoute("tamper-baseline-delete", "DELETE", "/api/tamper/baseline/delete", r.server.handleTamperBaselineDelete, false)
	r.addRoute("tamper-history", "GET", "/api/tamper/history", r.server.handleTamperHistory, false)
	r.addRoute("tamper-history-delete", "DELETE", "/api/tamper/history/delete", r.server.handleTamperHistoryDelete, false)

	// 创建 mux
	mux := http.NewServeMux()

	// 注册路由
	for _, route := range r.routes {
		handler := http.Handler(route.Handler)

		// 如果需要限流，包装限流中间件
		if route.RateLimited {
			handler = rateLimitMiddleware(handler)
		}

		mux.Handle(route.Pattern, handler)
	}

	// 静态文件服务
	staticDir := filepath.Join(r.server.webRoot, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	return mux
}

// addRoute 添加路由
func (r *Router) addRoute(name, method, pattern string, handler http.HandlerFunc, rateLimited bool) {
	r.routes = append(r.routes, Route{
		Name:        name,
		Method:      method,
		Pattern:     pattern,
		Handler:     handler,
		RateLimited: rateLimited,
	})
}

// GetRoutes 获取所有路由（用于调试/文档）
func (r *Router) GetRoutes() []Route {
	return r.routes
}
