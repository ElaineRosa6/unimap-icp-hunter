package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/appversion"
	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/distributed"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/proxypool"
	"github.com/unimap-icp-hunter/project/internal/requestid"
	"github.com/unimap-icp-hunter/project/internal/screenshot"
	"github.com/unimap-icp-hunter/project/internal/service"
)

// 查询状态
type QueryStatus struct {
	ID         string
	Query      string
	Engines    []string
	Status     string
	Progress   float64
	Results    []model.UnifiedAsset
	TotalCount int
	Errors     []string
	StartTime  time.Time
	EndTime    time.Time
}

type browserQueryOutcome = service.BrowserQueryOutcome

type managedConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// WebSocket连接管理器
type ConnectionManager struct {
	connections map[string]*managedConn
	mutex       sync.RWMutex
}

// Server Web服务器
type Server struct {
	port                 int
	httpServer           *http.Server
	templates            *template.Template
	service              *service.UnifiedService
	queryApp             *service.QueryAppService
	monitorApp           *service.MonitorAppService
	tamperApp            *service.TamperAppService
	screenshotApp        *service.ScreenshotAppService
	orchestrator         *adapter.EngineOrchestrator
	upgrader             websocket.Upgrader
	connManager          *ConnectionManager
	queryStatus          map[string]*QueryStatus
	queryMutex           sync.RWMutex
	configMutex          sync.Mutex
	webRoot              string
	staticVersion        string
	screenshotMgr        *screenshot.Manager
	config               *config.Config
	configManager        *config.Manager
	chromeCmd            *os.Process
	chromeCmdMu          sync.Mutex
	bridgeService        *screenshot.BridgeService
	bridgeMock           *bridgeMockClient
	bridgeTokens         map[string]int64
	bridgeCallbackNonces map[string]int64
	bridgeLastErr        string
	bridgeLastAt         int64
	bridgeTokenLastSeen  map[string]int64
	proxyPool            *proxypool.Pool
	nodeRegistry         *distributed.Registry
	nodeTaskQueue        *distributed.TaskQueue
	distributedEnabled   bool
	shutdownCtx          context.Context
	shutdownCancel       context.CancelFunc
}

// NewServer 创建Web服务器
func NewServer(port int, unifiedSvc *service.UnifiedService, orchestrator *adapter.EngineOrchestrator, cfg *config.Config, cfgManager *config.Manager) (*Server, error) {
	// 创建模板函数映射
	funcMap := template.FuncMap{
		"mul": func(a, b float64) float64 {

			return a * b
		},
		"div": func(a, b float64) float64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"float": func(a int) float64 {
			return float64(a)
		},
		"join": func(elems []string, sep string) string {
			return strings.Join(elems, sep)
		},
	}

	webRoot, err := resolveWebRoot()
	if err != nil {
		return nil, err
	}

	// 创建模板并添加自定义函数
	tmpl := template.New("").Funcs(funcMap)
	templates, err := tmpl.ParseGlob(filepath.Join(webRoot, "templates", "*.html"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates from %s: %w", webRoot, err)
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return isOriginAllowed(r.Header.Get("Origin"), r.Host, allowedOriginsFromConfig(cfg))
		},
	}

	// 初始化截图管理器
	var screenshotMgr *screenshot.Manager
	if cfg != nil && cfg.Screenshot.Enabled {
		headless := true
		if cfg.Screenshot.Headless != nil {
			headless = *cfg.Screenshot.Headless
		}

		// 检查配置的远程调试URL是否可用，不可用则清空
		remoteDebugURL := cfg.Screenshot.ChromeRemoteDebugURL
		if remoteDebugURL != "" && !isRemoteDebuggerAvailable(remoteDebugURL) {
			logger.Warnf("Configured remote debugger not available at %s, will use local Chrome", remoteDebugURL)
			remoteDebugURL = ""
		}

		screenshotCfg := screenshot.Config{
			BaseDir:        cfg.Screenshot.BaseDir,
			ChromePath:     cfg.Screenshot.ChromePath,
			ProxyServer:    cfg.Screenshot.ProxyServer,
			UserDataDir:    cfg.Screenshot.ChromeUserDataDir,
			ProfileDir:     cfg.Screenshot.ChromeProfileDir,
			RemoteDebugURL: remoteDebugURL,
			Headless:       headless,
			Timeout:        time.Duration(cfg.Screenshot.Timeout) * time.Second,
			WindowWidth:    cfg.Screenshot.WindowWidth,
			WindowHeight:   cfg.Screenshot.WindowHeight,
			WaitTime:       time.Duration(cfg.Screenshot.WaitTime) * time.Millisecond,
		}
		screenshotMgr = screenshot.NewManager(screenshotCfg)

		// 加载各引擎的Cookie
		if cfg.Engines.Fofa.Enabled && len(cfg.Engines.Fofa.Cookies) > 0 {
			fofaCookies := convertConfigCookies(cfg.Engines.Fofa.Cookies)
			screenshotMgr.SetCookies("fofa", fofaCookies)
		}
		if cfg.Engines.Hunter.Enabled && len(cfg.Engines.Hunter.Cookies) > 0 {
			hunterCookies := convertConfigCookies(cfg.Engines.Hunter.Cookies)
			screenshotMgr.SetCookies("hunter", hunterCookies)
		}
		if cfg.Engines.Quake.Enabled && len(cfg.Engines.Quake.Cookies) > 0 {
			quakeCookies := convertConfigCookies(cfg.Engines.Quake.Cookies)
			screenshotMgr.SetCookies("quake", quakeCookies)
		}
		if cfg.Engines.Zoomeye.Enabled && len(cfg.Engines.Zoomeye.Cookies) > 0 {
			zoomeyeCookies := convertConfigCookies(cfg.Engines.Zoomeye.Cookies)
			screenshotMgr.SetCookies("zoomeye", zoomeyeCookies)
		}

		logger.Infof("Screenshot manager initialized with base dir: %s", cfg.Screenshot.BaseDir)
	}

	// 解析截图基础目录
	screenshotBaseDir := "./screenshots"
	if cfg != nil && strings.TrimSpace(cfg.Screenshot.BaseDir) != "" {
		screenshotBaseDir = strings.TrimSpace(cfg.Screenshot.BaseDir)
	}

	var screenshotProvider screenshot.Provider
	if screenshotMgr != nil {
		screenshotProvider = screenshot.NewCDPProvider(screenshotMgr)
	}

	screenshotApp := service.NewScreenshotAppServiceWithProvider(screenshotBaseDir, screenshotProvider)
	if cfg != nil {
		screenshotApp.SetEngine(cfg.Screenshot.Engine)
		screenshotApp.SetFallbackToCDP(cfg.Screenshot.Extension.FallbackToCDP)
	}

	var proxyPool *proxypool.Pool
	if cfg != nil {
		proxyPool = proxypool.NewPool(proxypool.Config{
			Enabled:             cfg.Network.ProxyPool.Enabled,
			Proxies:             cfg.Network.ProxyPool.Proxies,
			FailureThreshold:    cfg.Network.ProxyPool.FailureThreshold,
			Cooldown:            time.Duration(cfg.Network.ProxyPool.CooldownSeconds) * time.Second,
			AllowDirectFallback: cfg.Network.ProxyPool.AllowDirectFallback,
		})
		if proxyPool.Enabled() {
			logger.Infof("Proxy pool enabled: %d proxies, strategy=%s", len(proxyPool.Proxies()), cfg.Network.ProxyPool.Strategy)
		}
	}

	heartbeatTimeout := 30 * time.Second
	maxReassign := 1
	distributedEnabled := false
	if cfg != nil {
		heartbeatTimeout = time.Duration(cfg.Distributed.HeartbeatTimeoutSeconds) * time.Second
		if heartbeatTimeout <= 0 {
			heartbeatTimeout = 30 * time.Second
		}
		maxReassign = cfg.Distributed.MaxReassignAttempts
		distributedEnabled = cfg.Distributed.Enabled
	}

	nodeRegistry := distributed.NewRegistry(heartbeatTimeout)
	nodeTaskQueue := distributed.NewTaskQueue()
	nodeTaskQueue.SetDefaultMaxReassign(maxReassign)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	srv := &Server{
		port:                 port,
		templates:            templates,
		service:              unifiedSvc,
		queryApp:             service.NewQueryAppService(unifiedSvc, orchestrator),
		monitorApp:           service.NewMonitorAppService(proxyPool),
		tamperApp:            service.NewTamperAppService("./hash_store"),
		screenshotApp:        screenshotApp,
		orchestrator:         orchestrator,
		upgrader:             upgrader,
		connManager:          &ConnectionManager{connections: make(map[string]*managedConn)},
		queryStatus:          make(map[string]*QueryStatus),
		webRoot:              webRoot,
		staticVersion:        strconv.FormatInt(time.Now().Unix(), 10),
		screenshotMgr:        screenshotMgr,
		config:               cfg,
		configManager:        cfgManager,
		bridgeTokens:         make(map[string]int64),
		bridgeTokenLastSeen:  make(map[string]int64),
		bridgeCallbackNonces: make(map[string]int64),
		proxyPool:            proxyPool,
		nodeRegistry:         nodeRegistry,
		nodeTaskQueue:        nodeTaskQueue,
		distributedEnabled:   distributedEnabled,
		shutdownCtx:          shutdownCtx,
		shutdownCancel:       shutdownCancel,
	}

	if cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Screenshot.Engine), "extension") {
		mockClient := newBridgeMockClient()
		bridgeSvc := screenshot.NewBridgeService(mockClient, cfg.Screenshot.Extension.MaxConcurrency, time.Duration(cfg.Screenshot.Extension.TaskTimeoutSeconds)*time.Second)
		bridgeSvc.Start(context.Background())
		srv.bridgeMock = mockClient
		srv.bridgeService = bridgeSvc
		screenshotApp.SetBridgeService(bridgeSvc)
	}

	return srv, nil
}

// convertConfigCookies 转换配置Cookie到截图管理器Cookie
func convertConfigCookies(cfgCookies []config.Cookie) []screenshot.Cookie {
	cookies := make([]screenshot.Cookie, len(cfgCookies))
	for i, c := range cfgCookies {
		cookies[i] = screenshot.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
		}
	}
	return cookies
}

func resolveWebRoot() (string, error) {
	if env := strings.TrimSpace(os.Getenv("UNIMAP_WEB_ROOT")); env != "" {
		if ok := isWebRoot(env); ok {
			return env, nil
		}
		return "", fmt.Errorf("UNIMAP_WEB_ROOT=%s is not a valid web root", env)
	}

	exePath, _ := os.Executable()
	exeDir := ""
	if exePath != "" {
		exeDir = filepath.Dir(exePath)
	}

	candidates := []string{
		filepath.Join(".", "web"),
	}
	if exeDir != "" {
		candidates = append(candidates,
			filepath.Join(exeDir, "web"),
			filepath.Join(exeDir, "..", "web"),
		)
	}

	for _, c := range candidates {
		if ok := isWebRoot(c); ok {
			return c, nil
		}
	}

	return "", fmt.Errorf("unable to locate web root; set UNIMAP_WEB_ROOT or run from project root")
}

func isWebRoot(dir string) bool {
	tmplDir := filepath.Join(dir, "templates")
	staticDir := filepath.Join(dir, "static")
	if st, err := os.Stat(tmplDir); err != nil || !st.IsDir() {
		return false
	}
	if st, err := os.Stat(staticDir); err != nil || !st.IsDir() {
		return false
	}
	return true
}

// securityMiddleware 添加安全响应头的中间件
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 防止点击劫持
		w.Header().Set("X-Frame-Options", "DENY")
		// 防止MIME类型嗅探
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// XSS保护
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		// 内容安全策略
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:;")
		// 引用策略
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// 权限策略
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next.ServeHTTP(w, r)
	})
}

// Start 启动Web服务器
func (s *Server) Start() error {
	// 使用统一路由注册
	router := NewRouter(s)
	mux := router.RegisterRoutes()

	rateLimitEnabled := true
	if s.config != nil {
		rateLimitEnabled = s.config.Web.RateLimit.Enabled
	}
	SetRateLimitEnabled(rateLimitEnabled)
	if rateLimitEnabled && s.config != nil {
		SetRateLimitConfig(s.config.Web.RateLimit.RequestsPerWindow, time.Duration(s.config.Web.RateLimit.WindowSeconds)*time.Second)
	}

	maxBodyBytes := int64(10 * 1024 * 1024)
	if s.config != nil && s.config.Web.RequestLimits.MaxBodyBytes > 0 {
		maxBodyBytes = s.config.Web.RequestLimits.MaxBodyBytes
	}

	allowedOrigins := allowedOriginsFromConfig(s.config)
	allowedMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	allowedHeaders := []string{"Content-Type", "Authorization", "X-Requested-With", "X-WebSocket-Token", requestid.HeaderName}
	exposedHeaders := []string{requestid.HeaderName}
	allowCredentials := true
	maxAge := 600
	if s.config != nil {
		if len(s.config.Web.CORS.AllowedMethods) > 0 {
			allowedMethods = s.config.Web.CORS.AllowedMethods
		}
		if len(s.config.Web.CORS.AllowedHeaders) > 0 {
			allowedHeaders = s.config.Web.CORS.AllowedHeaders
		}
		if len(s.config.Web.CORS.ExposedHeaders) > 0 {
			exposedHeaders = s.config.Web.CORS.ExposedHeaders
		}
		allowCredentials = s.config.Web.CORS.AllowCredentials
		if s.config.Web.CORS.MaxAge > 0 {
			maxAge = s.config.Web.CORS.MaxAge
		}
	}

	handler := securityMiddleware(mux)
	handler = requestIDMiddleware(handler)
	handler = requestSizeLimitMiddleware(maxBodyBytes)(handler)
	handler = corsMiddleware(allowedOrigins, allowedMethods, allowedHeaders, exposedHeaders, allowCredentials, maxAge)(handler)
	handler = metricsMiddleware(handler)

	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isWebSocket := strings.Contains(r.Header.Get("Connection"), "Upgrade") &&
			strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		
		if isWebSocket && r.URL.Path == "/api/ws" {
			s.handleWebSocket(w, r)
			return
		}
		
		isBridgeAPI := strings.HasPrefix(r.URL.Path, "/api/screenshot/bridge/")
		if isBridgeAPI {
			mux.ServeHTTP(w, r)
			return
		}
		
		handler.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf(":%d", s.port)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: rootHandler,
	}

	logger.Infof("Web server started at http://localhost%s", addr)
	logger.Infof("Registered %d routes", len(router.GetRoutes()))
	logger.Infof("Web security config loaded: cors_origins=%d rate_limit_enabled=%t max_body_bytes=%d",
		len(allowedOrigins), rateLimitEnabled, maxBodyBytes)
	return s.httpServer.ListenAndServe()
}

func allowedOriginsFromConfig(cfg *config.Config) []string {
	if cfg == nil || len(cfg.Web.CORS.AllowedOrigins) == 0 {
		return []string{"http://localhost:8448", "http://127.0.0.1:8448"}
	}
	origins := make([]string, 0, len(cfg.Web.CORS.AllowedOrigins))
	for _, origin := range cfg.Web.CORS.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		origins = append(origins, origin)
	}
	if len(origins) == 0 {
		return []string{"http://localhost:8448", "http://127.0.0.1:8448"}
	}
	return origins
}

// Shutdown 优雅关闭Web服务器
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	logger.Info("Shutting down web server...")

	// 取消所有后台goroutine
	if s.shutdownCancel != nil {
		s.shutdownCancel()
	}

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("web server shutdown error: %w", err)
	}

	// 关闭Chrome进程
	s.chromeCmdMu.Lock()
	if s.chromeCmd != nil {
		logger.Info("Shutting down Chrome process...")
		if err := s.chromeCmd.Kill(); err != nil {
			logger.Warnf("Failed to kill Chrome process: %v", err)
		} else {
			_, err := s.chromeCmd.Wait()
			if err != nil {
				logger.Warnf("Failed to wait for Chrome process: %v", err)
			}
		}
		s.chromeCmd = nil
	}
	s.chromeCmdMu.Unlock()

	// 关闭所有WebSocket连接
	s.connManager.mutex.Lock()
	for id, managed := range s.connManager.connections {
		if err := managed.conn.Close(); err != nil {
			logger.Warnf("Failed to close WebSocket connection %s: %v", id, err)
		}
	}
	s.connManager.connections = make(map[string]*managedConn)
	s.connManager.mutex.Unlock()

	logger.Info("Web server shutdown completed")
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	health := map[string]interface{}{
		"status":  "ok",
		"version": appversion.Full(),
		"time":    time.Now().UTC().Format(time.RFC3339),
		"engines": s.orchestrator.ListAdapters(),
	}

	if s.service != nil {
		health["plugins"] = s.service.HealthCheck()
	}

	_ = json.NewEncoder(w).Encode(health)
}

func parseEnginesParam(r *http.Request) []string {
	_ = r.ParseForm()

	seen := make(map[string]struct{})
	engines := make([]string, 0)
	for _, raw := range r.Form["engines"] {
		for _, part := range strings.Split(raw, ",") {
			engine := strings.TrimSpace(part)
			if engine == "" {
				continue
			}
			if _, ok := seen[engine]; ok {
				continue
			}
			seen[engine] = struct{}{}
			engines = append(engines, engine)
		}
	}

	return engines
}

func parseWSStringList(val interface{}) []string {
	if val == nil {
		return nil
	}

	sanitizeAndAppend := func(out []string, raw string) []string {
		for _, part := range strings.Split(raw, ",") {
			item := strings.TrimSpace(part)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	}

	switch v := val.(type) {
	case string:
		return sanitizeAndAppend(nil, v)
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = sanitizeAndAppend(out, item)
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = sanitizeAndAppend(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func parseWSInt(val interface{}, defaultValue int) int {
	if val == nil {
		return defaultValue
	}

	switch v := val.(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
		return defaultValue
	case int:
		if v > 0 {
			return v
		}
		return defaultValue
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return defaultValue
		}
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
		return defaultValue
	default:
		return defaultValue
	}
}

func validateQueryInput(query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("Query cannot be empty")
	}
	if len(query) > 1000 {
		return fmt.Errorf("Query is too long (maximum 1000 characters)")
	}
	for _, r := range query {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return fmt.Errorf("Query contains invalid characters")
		}
	}
	return nil
}

func parseBoolValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseWSBool(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return parseBoolValue(value)
	case float64:
		return value != 0
	case int:
		return value != 0
	default:
		return false
	}
}

func appendUniqueStrings(base []string, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, item := range base {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	for _, item := range extra {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	return merged
}

// maskAPIKey 屏蔽API密钥，用于日志输出
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
}
