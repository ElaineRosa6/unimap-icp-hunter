package web

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gorilla/websocket"
	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/screenshot"
	"github.com/unimap-icp-hunter/project/internal/service"
	"github.com/unimap-icp-hunter/project/internal/tamper"
	"github.com/xuri/excelize/v2"
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

// WebSocket连接管理器
type ConnectionManager struct {
	connections map[string]*websocket.Conn
	mutex       sync.RWMutex
}

// Server Web服务器
type Server struct {
	port          int
	httpServer    *http.Server
	templates     *template.Template
	service       *service.UnifiedService
	orchestrator  *adapter.EngineOrchestrator
	upgrader      websocket.Upgrader
	connManager   *ConnectionManager
	queryStatus   map[string]*QueryStatus
	queryMutex    sync.RWMutex
	configMutex   sync.Mutex
	webRoot       string
	staticVersion string
	screenshotMgr *screenshot.Manager
	config        *config.Config
	configManager *config.Manager
	chromeCmd     *os.Process
	chromeCmdMu   sync.Mutex
}

// NewServer 创建Web服务器
func NewServer(port int, service *service.UnifiedService, orchestrator *adapter.EngineOrchestrator, cfg *config.Config, cfgManager *config.Manager) (*Server, error) {
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
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			// 检查是否是本地请求或相同来源
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			// 允许本地开发环境
			if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" {
				return true
			}
			// 生产环境应该配置具体的允许来源
			// 这里暂时返回false，实际使用时应根据需要配置
			return false
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

	return &Server{
		port:          port,
		templates:     templates,
		service:       service,
		orchestrator:  orchestrator,
		upgrader:      upgrader,
		connManager:   &ConnectionManager{connections: make(map[string]*websocket.Conn)},
		queryStatus:   make(map[string]*QueryStatus),
		webRoot:       webRoot,
		staticVersion: strconv.FormatInt(time.Now().Unix(), 10),
		screenshotMgr: screenshotMgr,
		config:        cfg,
		configManager: cfgManager,
	}, nil
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
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/query", s.handleQuery)
	mux.HandleFunc("/api/query", s.handleAPIQuery)
	mux.HandleFunc("/api/cookies", s.handleSaveCookies)
	mux.HandleFunc("/api/cookies/verify", s.handleVerifyCookies)
	mux.HandleFunc("/api/cookies/import", s.handleImportCookieJSON)
	mux.HandleFunc("/api/cdp/status", s.handleCDPStatus)
	mux.HandleFunc("/api/cdp/connect", s.handleCDPConnect)
	mux.HandleFunc("/api/ws", s.handleWebSocket)
	mux.HandleFunc("/api/query/status", s.handleQueryStatus)
	mux.HandleFunc("/api/screenshot", s.handleScreenshot)
	mux.HandleFunc("/api/screenshot/search-engine", s.handleSearchEngineScreenshot)
	mux.HandleFunc("/api/screenshot/target", s.handleTargetScreenshot)
	mux.HandleFunc("/api/screenshot/batch", s.handleBatchScreenshot)
	mux.HandleFunc("/api/screenshot/batch-urls", s.handleBatchURLsScreenshot)
	mux.HandleFunc("/api/import/urls", s.handleImportURLs)
	mux.HandleFunc("/api/tamper/check", s.handleTamperCheck)
	mux.HandleFunc("/api/tamper/baseline", s.handleTamperBaseline)
	mux.HandleFunc("/api/tamper/baseline/list", s.handleTamperBaselineList)
	mux.HandleFunc("/api/tamper/baseline/delete", s.handleTamperBaselineDelete)
	mux.HandleFunc("/results", s.handleResults)
	mux.HandleFunc("/quota", s.handleQuota)
	mux.HandleFunc("/batch-screenshot", s.handleBatchScreenshotPage)
	mux.HandleFunc("/monitor", s.handleMonitorPage)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(s.webRoot, "static")))))

	addr := fmt.Sprintf(":%d", s.port)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: securityMiddleware(mux),
	}

	logger.Infof("Web server started at http://localhost%s", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭Web服务器
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	logger.Info("Shutting down web server...")
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
	for id, conn := range s.connManager.connections {
		if err := conn.Close(); err != nil {
			logger.Warnf("Failed to close WebSocket connection %s: %v", id, err)
		}
	}
	s.connManager.connections = make(map[string]*websocket.Conn)
	s.connManager.mutex.Unlock()

	logger.Info("Web server shutdown completed")
	return nil
}

// handleImportCookieJSON 导入浏览器导出的Cookie JSON
func (s *Server) handleImportCookieJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.config == nil {
		http.Error(w, "Config not loaded", http.StatusServiceUnavailable)
		return
	}

	engine := strings.TrimSpace(r.FormValue("engine"))
	jsonStr := r.FormValue("cookie_json")
	if engine == "" || strings.TrimSpace(jsonStr) == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "engine and cookie_json are required",
		})
		return
	}

	cookies, err := config.ParseCookieJSON(jsonStr, config.DefaultCookieDomain(engine))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "invalid cookie json",
		})
		return
	}
	if len(cookies) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "no cookies parsed",
		})
		return
	}

	s.configMutex.Lock()
	switch strings.ToLower(engine) {
	case "fofa":
		s.config.Engines.Fofa.Cookies = cookies
	case "hunter":
		s.config.Engines.Hunter.Cookies = cookies
	case "quake":
		s.config.Engines.Quake.Cookies = cookies
	case "zoomeye":
		s.config.Engines.Zoomeye.Cookies = cookies
	default:
		s.configMutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "unsupported engine",
		})
		return
	}
	if s.screenshotMgr != nil {
		s.screenshotMgr.SetCookies(engine, convertConfigCookies(cookies))
	}
	if s.configManager != nil {
		if err := s.configManager.Save(); err != nil {
			logger.Warnf("Failed to persist cookies: %v", err)
		}
	}
	s.configMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"cookieHeader": cookiesToHeader(cookies),
	})
}

// handleVerifyCookies 验证Cookie是否可访问搜索结果页
func (s *Server) handleVerifyCookies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.screenshotMgr == nil {
		http.Error(w, "Screenshot manager not initialized", http.StatusServiceUnavailable)
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if err := validateQueryInput(query); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	s.applyCookiesFromRequest(r)

	engines := parseEnginesParam(r)
	if len(engines) == 0 {
		engines = s.orchestrator.ListAdapters()
	}
	if len(engines) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "no engines configured/registered; please set API keys in configs/config.yaml",
		})
		return
	}

	ctx := r.Context()
	results := make(map[string]interface{})
	for _, engine := range engines {
		cookies := s.screenshotMgr.GetCookies(engine)
		ok, title, hint, err := s.screenshotMgr.ValidateSearchEngineResult(ctx, engine, query, cookies)
		payload := map[string]interface{}{
			"ok":    ok,
			"title": title,
			"hint":  hint,
		}
		if err != nil {
			payload["error"] = err.Error()
		}
		results[engine] = payload
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"query":   query,
		"results": results,
	})
}

// handleSaveCookies 处理保存Cookie请求
func (s *Server) handleSaveCookies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.applyCookiesFromRequest(r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// handleCDPStatus 检测CDP端口状态
func (s *Server) handleCDPStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	baseURL := s.resolveCDPURL()
	online, info, err := s.checkCDPStatus(r.Context(), baseURL)

	resp := map[string]interface{}{
		"online": online,
		"url":    baseURL,
	}
	if info != nil {
		resp["version"] = info
	}
	if err != nil && !online {
		resp["error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleCDPConnect 启动Chrome并连接CDP
func (s *Server) handleCDPConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	baseURL := s.resolveCDPURL()
	ctx := r.Context()
	if ok, info, _ := s.checkCDPStatus(ctx, baseURL); ok {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"online":  true,
			"url":     baseURL,
			"version": info,
			"message": "CDP already online",
		})
		return
	}

	if err := s.startCDPChrome(baseURL); err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	online, info, err := s.waitForCDP(ctx, baseURL, 5*time.Second)
	if online {
		s.updateCDPConfig(baseURL)
	}

	w.Header().Set("Content-Type", "application/json")
	if online {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"online":  true,
			"url":     baseURL,
			"version": info,
			"message": "CDP connected",
		})
		return
	}

	msg := "CDP not available"
	if err != nil {
		msg = err.Error()
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"online":  false,
		"url":     baseURL,
		"error":   msg,
	})
}

func (s *Server) resolveCDPURL() string {
	if s.config != nil {
		if raw := strings.TrimSpace(s.config.Screenshot.ChromeRemoteDebugURL); raw != "" {
			if normalized := normalizeCDPBaseURL(raw); normalized != "" {
				return normalized
			}
		}
	}
	if env := strings.TrimSpace(os.Getenv("UNIMAP_CHROME_REMOTE_DEBUG_URL")); env != "" {
		if normalized := normalizeCDPBaseURL(env); normalized != "" {
			return normalized
		}
	}
	return "http://127.0.0.1:9222"
}

// isRemoteDebuggerAvailable 检查远程调试端口是否可用
func isRemoteDebuggerAvailable(remoteURL string) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := client.Get(remoteURL + "/json/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func normalizeCDPBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if isAllDigits(raw) {
		return "http://127.0.0.1:" + raw
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.TrimRight(raw, "/")
	}
	if u.Scheme == "ws" {
		u.Scheme = "http"
	}
	if u.Scheme == "wss" {
		u.Scheme = "https"
	}
	if strings.Contains(u.Path, "/devtools/browser/") {
		u.Path = ""
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}

func isAllDigits(val string) bool {
	for _, r := range val {
		if r < '0' || r > '9' {
			return false
		}
	}
	return val != ""
}

func (s *Server) checkCDPStatus(ctx context.Context, baseURL string) (bool, map[string]interface{}, error) {
	baseURL = normalizeCDPBaseURL(baseURL)
	if baseURL == "" {
		return false, nil, fmt.Errorf("cdp url is empty")
	}

	statusURL := strings.TrimRight(baseURL, "/") + "/json/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return false, nil, err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false, nil, err
	}

	return true, info, nil
}

func (s *Server) waitForCDP(ctx context.Context, baseURL string, timeout time.Duration) (bool, map[string]interface{}, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false, nil, ctx.Err()
		}
		online, info, err := s.checkCDPStatus(ctx, baseURL)
		if online {
			return true, info, nil
		}
		lastErr = err
		time.Sleep(300 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("cdp not available")
	}
	return false, nil, lastErr
}

func (s *Server) startCDPChrome(baseURL string) error {
	s.chromeCmdMu.Lock()
	defer s.chromeCmdMu.Unlock()

	if s.chromeCmd != nil {
		return nil
	}

	chromePath := s.resolveChromePath()
	if chromePath == "" {
		return fmt.Errorf("chrome path not configured; set screenshot.chrome_path or UNIMAP_CHROME_PATH")
	}

	port := resolveCDPPort(baseURL)
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
	}

	if s.config != nil {
		if dir := strings.TrimSpace(s.config.Screenshot.ChromeUserDataDir); dir != "" {
			args = append(args, "--user-data-dir="+dir)
		}
		if profile := strings.TrimSpace(s.config.Screenshot.ChromeProfileDir); profile != "" {
			args = append(args, "--profile-directory="+profile)
		}
		if s.config.Screenshot.Headless != nil && *s.config.Screenshot.Headless {
			args = append(args, "--headless=new")
		}
	}

	cmd := exec.Command(chromePath, args...)
	if err := cmd.Start(); err != nil {
		return err
	}

	s.chromeCmd = cmd.Process
	return nil
}

func (s *Server) resolveChromePath() string {
	if s.config != nil {
		if raw := strings.TrimSpace(s.config.Screenshot.ChromePath); raw != "" {
			return raw
		}
	}
	if env := strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PATH")); env != "" {
		return env
	}
	for _, name := range []string{"chrome", "chrome.exe", "msedge", "msedge.exe", "chromium", "chromium.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func resolveCDPPort(baseURL string) int {
	baseURL = normalizeCDPBaseURL(baseURL)
	if baseURL == "" {
		return 9222
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return 9222
	}
	if strings.Contains(u.Host, ":") {
		_, portStr, err := net.SplitHostPort(u.Host)
		if err == nil {
			if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
				return port
			}
		}
	}
	return 9222
}

func (s *Server) updateCDPConfig(baseURL string) {
	if s.config == nil {
		return
	}

	if s.screenshotMgr != nil {
		s.screenshotMgr.SetRemoteDebugURL(baseURL)
	}

	s.configMutex.Lock()
	s.config.Screenshot.ChromeRemoteDebugURL = baseURL
	s.configMutex.Unlock()

	if s.configManager != nil {
		if err := s.configManager.Save(); err != nil {
			logger.Warnf("Failed to persist CDP URL: %v", err)
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	health := map[string]interface{}{
		"status":  "ok",
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

func (s *Server) applyCookiesFromRequest(r *http.Request) {
	if s.config == nil {
		return
	}

	s.configMutex.Lock()
	defer s.configMutex.Unlock()

	changed := false
	clear := strings.EqualFold(strings.TrimSpace(r.FormValue("clear_cookies")), "true")
	if clear {
		s.config.Engines.Fofa.Cookies = nil
		s.config.Engines.Hunter.Cookies = nil
		s.config.Engines.Quake.Cookies = nil
		s.config.Engines.Zoomeye.Cookies = nil
		changed = true
		if s.screenshotMgr != nil {
			s.screenshotMgr.SetCookies("fofa", nil)
			s.screenshotMgr.SetCookies("hunter", nil)
			s.screenshotMgr.SetCookies("quake", nil)
			s.screenshotMgr.SetCookies("zoomeye", nil)
		}
	}

	apply := func(engine, value string) {
		if clear {
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		cookies := config.ParseCookieHeader(value, config.DefaultCookieDomain(engine))
		if len(cookies) == 0 {
			return
		}

		switch strings.ToLower(engine) {
		case "fofa":
			s.config.Engines.Fofa.Cookies = cookies
		case "hunter":
			s.config.Engines.Hunter.Cookies = cookies
		case "quake":
			s.config.Engines.Quake.Cookies = cookies
		case "zoomeye":
			s.config.Engines.Zoomeye.Cookies = cookies
		default:
			return
		}
		changed = true

		if s.screenshotMgr != nil {
			s.screenshotMgr.SetCookies(engine, convertConfigCookies(cookies))
		}
	}

	apply("fofa", r.FormValue("cookie_fofa"))
	apply("hunter", r.FormValue("cookie_hunter"))
	apply("zoomeye", r.FormValue("cookie_zoomeye"))
	apply("quake", r.FormValue("cookie_quake"))

	if changed && s.configManager != nil {
		if err := s.configManager.Save(); err != nil {
			logger.Warnf("Failed to persist cookies: %v", err)
		}
	}
}

func cookiesToHeader(cookies []config.Cookie) string {
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
	}
	return strings.Join(parts, "; ")
}

func hasCookies(cookies []config.Cookie) bool {
	for _, c := range cookies {
		if strings.TrimSpace(c.Name) != "" {
			return true
		}
	}
	return false
}

// handleAPIQuery 处理API查询请求（用于异步查询）
func (s *Server) handleAPIQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if err := validateQueryInput(query); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
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
	engines := parseEnginesParam(r)
	if len(engines) == 0 {
		// 如果没有选择引擎，使用默认引擎
		defaultEngines := s.orchestrator.ListAdapters()
		if len(defaultEngines) > 0 {
			engines = []string{defaultEngines[0]}
		}
	}
	if len(engines) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "no engines configured/registered; please set API keys in configs/config.yaml",
		})
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   fmt.Sprintf("Query failed: %v", err),
			"query":   query,
			"engines": engines,
		})
		return
	}

	// 返回JSON结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"query":       query,
		"engines":     engines,
		"assets":      resp.Assets,
		"totalCount":  resp.TotalCount,
		"engineStats": resp.EngineStats,
		"errors":      resp.Errors,
	})
}

// handleScreenshot 处理截图请求
func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	// Add scheme if missing. Try https first for security, or http?
	// Usually assets like IPs don't have scheme.
	// But chromdp needs it.
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		http.Error(w, "Invalid url", http.StatusBadRequest)
		return
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(1365, 768),
	)

	// Optional: allow overriding browser binary path (useful on servers / portable installs)
	if chromePath := strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PATH")); chromePath != "" {
		st, statErr := os.Stat(chromePath)
		if statErr != nil || st.IsDir() {
			http.Error(w, "Invalid UNIMAP_CHROME_PATH (file not found or not a file)", http.StatusInternalServerError)
			return
		}
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// 增加超时控制
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var buf []byte
	// 简单截图：导航并截图
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.CaptureScreenshot(&buf),
	); err != nil {
		if strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PATH")) == "" {
			http.Error(w, fmt.Sprintf("Screenshot failed: %v. Hint: set UNIMAP_CHROME_PATH to your Chrome/Chromium executable path.", err), http.StatusInternalServerError)
			return
		}
		http.Error(w, fmt.Sprintf("Screenshot failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(buf)
}

// handleSearchEngineScreenshot 处理搜索引擎结果页面截图请求
func (s *Server) handleSearchEngineScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.screenshotMgr == nil {
		http.Error(w, "Screenshot manager not initialized", http.StatusServiceUnavailable)
		return
	}

	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	queryID := strings.TrimSpace(r.URL.Query().Get("query_id"))

	if engine == "" || query == "" {
		http.Error(w, "Missing engine or query parameter", http.StatusBadRequest)
		return
	}

	// 如果没有提供queryID，生成一个
	if queryID == "" {
		queryID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	ctx := r.Context()
	screenshotPath, err := s.screenshotMgr.CaptureSearchEngineResult(ctx, engine, query, queryID)
	if err != nil {
		logger.Errorf("Failed to capture search engine screenshot: %v", err)
		http.Error(w, fmt.Sprintf("Screenshot failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 返回JSON响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"path":     screenshotPath,
		"engine":   engine,
		"query":    query,
		"query_id": queryID,
	})
}

// handleTargetScreenshot 处理目标网站截图请求（保存到文件）
func (s *Server) handleTargetScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.screenshotMgr == nil {
		http.Error(w, "Screenshot manager not initialized", http.StatusServiceUnavailable)
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	port := strings.TrimSpace(r.URL.Query().Get("port"))
	protocol := strings.TrimSpace(r.URL.Query().Get("protocol"))
	queryID := strings.TrimSpace(r.URL.Query().Get("query_id"))

	if targetURL == "" && ip == "" {
		http.Error(w, "Missing url or ip parameter", http.StatusBadRequest)
		return
	}

	// 如果没有提供queryID，生成一个
	if queryID == "" {
		queryID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	ctx := r.Context()
	screenshotPath, err := s.screenshotMgr.CaptureTargetWebsite(ctx, targetURL, ip, port, protocol, queryID)
	if err != nil {
		logger.Errorf("Failed to capture target screenshot: %v", err)
		http.Error(w, fmt.Sprintf("Screenshot failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 返回JSON响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"path":     screenshotPath,
		"url":      targetURL,
		"ip":       ip,
		"port":     port,
		"protocol": protocol,
		"query_id": queryID,
	})
}

// handleBatchScreenshot 处理批量截图请求
func (s *Server) handleBatchScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.screenshotMgr == nil {
		http.Error(w, "Screenshot manager not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		QueryID string `json:"query_id"`
		Engines []struct {
			Engine string `json:"engine"`
			Query  string `json:"query"`
		} `json:"engines"`
		Targets []struct {
			URL      string `json:"url"`
			IP       string `json:"ip"`
			Port     string `json:"port"`
			Protocol string `json:"protocol"`
		} `json:"targets"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 如果没有提供queryID，生成一个
	if req.QueryID == "" {
		req.QueryID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	ctx := r.Context()
	results := map[string]interface{}{
		"query_id":       req.QueryID,
		"search_engines": []map[string]interface{}{},
		"targets":        []map[string]interface{}{},
		"errors":         []string{},
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// 截图搜索引擎结果页面
	for _, engine := range req.Engines {
		wg.Add(1)
		go func(engine, query string) {
			defer wg.Done()
			path, err := s.screenshotMgr.CaptureSearchEngineResult(ctx, engine, query, req.QueryID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["errors"] = append(results["errors"].([]string), fmt.Sprintf("%s: %v", engine, err))
			} else {
				results["search_engines"] = append(results["search_engines"].([]map[string]interface{}), map[string]interface{}{
					"engine": engine,
					"query":  query,
					"path":   path,
				})
			}
		}(engine.Engine, engine.Query)
	}

	// 截图目标网站
	for _, target := range req.Targets {
		wg.Add(1)
		go func(url, ip, port, protocol string) {
			defer wg.Done()
			path, err := s.screenshotMgr.CaptureTargetWebsite(ctx, url, ip, port, protocol, req.QueryID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["errors"] = append(results["errors"].([]string), fmt.Sprintf("%s:%s: %v", ip, port, err))
			} else {
				results["targets"] = append(results["targets"].([]map[string]interface{}), map[string]interface{}{
					"url":      url,
					"ip":       ip,
					"port":     port,
					"protocol": protocol,
					"path":     path,
				})
			}
		}(target.URL, target.IP, target.Port, target.Protocol)
	}

	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// handleBatchURLsScreenshot 处理批量URL截图请求
func (s *Server) handleBatchURLsScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.screenshotMgr == nil {
		http.Error(w, "Screenshot manager not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		URLs        []string `json:"urls"`
		BatchID     string   `json:"batch_id"`
		Concurrency int      `json:"concurrency"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.URLs) == 0 {
		http.Error(w, "No URLs provided", http.StatusBadRequest)
		return
	}

	// 限制最大URL数量
	if len(req.URLs) > 100 {
		http.Error(w, "Too many URLs (max 100)", http.StatusBadRequest)
		return
	}

	// 如果没有提供batchID，生成一个
	if req.BatchID == "" {
		req.BatchID = fmt.Sprintf("batch_%d", time.Now().UnixNano())
	}

	// 默认并发数
	if req.Concurrency <= 0 || req.Concurrency > 10 {
		req.Concurrency = 5
	}

	ctx := r.Context()
	results, err := s.screenshotMgr.CaptureBatchURLs(ctx, req.URLs, req.BatchID, req.Concurrency)
	if err != nil {
		http.Error(w, fmt.Sprintf("Batch screenshot failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 统计结果
	successCount := 0
	failCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"batch_id":       req.BatchID,
		"total":          len(req.URLs),
		"success":        successCount,
		"failed":         failCount,
		"results":        results,
		"screenshot_dir": s.screenshotMgr.GetScreenshotDirectory(),
	})
}

// handleBatchScreenshotPage 处理批量截图页面
func (s *Server) handleBatchScreenshotPage(w http.ResponseWriter, r *http.Request) {
	s.templates.ExecuteTemplate(w, "batch-screenshot.html", map[string]interface{}{
		"staticVersion": s.staticVersion,
	})
}

// handleMonitorPage 处理网页监控页面
func (s *Server) handleMonitorPage(w http.ResponseWriter, r *http.Request) {
	s.templates.ExecuteTemplate(w, "monitor.html", map[string]interface{}{
		"staticVersion": s.staticVersion,
	})
}

// handleImportURLs 处理URL文件导入
func (s *Server) handleImportURLs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析multipart表单
	err := r.ParseMultipartForm(10 << 20) // 10MB限制
	if err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileName := strings.ToLower(header.Filename)
	var urls []string

	if strings.HasSuffix(fileName, ".xlsx") || strings.HasSuffix(fileName, ".xls") {
		// 解析Excel文件
		urls, err = parseExcelFile(file)
	} else if strings.HasSuffix(fileName, ".csv") {
		// 解析CSV文件
		urls, err = parseCSVFile(file)
	} else if strings.HasSuffix(fileName, ".txt") {
		// 解析TXT文件
		urls, err = parseTXTFile(file)
	} else {
		http.Error(w, "Unsupported file format", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Failed to parse file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 过滤有效URL
	validUrls := filterValidURLs(urls)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":    len(urls),
		"valid":    len(validUrls),
		"urls":     validUrls,
		"filename": header.Filename,
	})
}

// parseExcelFile 解析Excel文件
func parseExcelFile(file io.Reader) ([]string, error) {
	f, err := excelize.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// 获取第一个工作表
	sheetName := f.GetSheetName(0)
	if sheetName == "" {
		return nil, fmt.Errorf("no sheet found")
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}

	var urls []string
	for i, row := range rows {
		if i == 0 {
			// 跳过表头
			continue
		}
		if len(row) > 0 && row[0] != "" {
			urls = append(urls, strings.TrimSpace(row[0]))
		}
	}

	return urls, nil
}

// parseCSVFile 解析CSV文件
func parseCSVFile(file io.Reader) ([]string, error) {
	reader := csv.NewReader(file)
	var urls []string
	isFirstRow := true

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if isFirstRow {
			isFirstRow = false
			// 检查是否是表头
			if len(record) > 0 && (strings.ToLower(record[0]) == "url" ||
				strings.ToLower(record[0]) == "address" ||
				strings.ToLower(record[0]) == "网址") {
				continue
			}
		}

		if len(record) > 0 && record[0] != "" {
			urls = append(urls, strings.TrimSpace(record[0]))
		}
	}

	return urls, nil
}

// parseTXTFile 解析TXT文件
func parseTXTFile(file io.Reader) ([]string, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var urls []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			urls = append(urls, line)
		}
	}

	return urls, nil
}

// filterValidURLs 过滤有效URL
func filterValidURLs(urls []string) []string {
	var valid []string
	seen := make(map[string]bool)

	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}

		// 简单URL验证
		if matched, _ := regexp.MatchString(`^(https?://)?([\w.-]+)(:\d+)?(/.*)?$`, u); matched {
			valid = append(valid, u)
			seen[u] = true
		}
	}

	return valid
}

// handleIndex 处理首页请求
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	engines := s.orchestrator.ListAdapters()
	var fofaCookies, hunterCookies, quakeCookies, zoomeyeCookies []config.Cookie
	if s.config != nil {
		fofaCookies = s.config.Engines.Fofa.Cookies
		hunterCookies = s.config.Engines.Hunter.Cookies
		quakeCookies = s.config.Engines.Quake.Cookies
		zoomeyeCookies = s.config.Engines.Zoomeye.Cookies
	}
	s.templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
		"engines":          engines,
		"staticVersion":    s.staticVersion,
		"cookieFofa":       cookiesToHeader(fofaCookies),
		"cookieHunter":     cookiesToHeader(hunterCookies),
		"cookieQuake":      cookiesToHeader(quakeCookies),
		"cookieZoomeye":    cookiesToHeader(zoomeyeCookies),
		"cookieHasFofa":    hasCookies(fofaCookies),
		"cookieHasHunter":  hasCookies(hunterCookies),
		"cookieHasQuake":   hasCookies(quakeCookies),
		"cookieHasZoomeye": hasCookies(zoomeyeCookies),
	})
}

// handleQuery 处理查询请求
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if err := validateQueryInput(query); err != nil {
		s.templates.ExecuteTemplate(w, "error.html", map[string]interface{}{
			"error": err.Error(),
		})
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
		s.templates.ExecuteTemplate(w, "error.html", map[string]interface{}{
			"error": "No engines configured/registered. Please set API keys in configs/config.yaml and enable at least one engine.",
		})
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
		s.templates.ExecuteTemplate(w, "error.html", map[string]interface{}{
			"error":   fmt.Sprintf("Query failed: %v", err),
			"query":   query,
			"engines": engines,
		})
		return
	}

	// 渲染结果页面
	s.templates.ExecuteTemplate(w, "results.html", map[string]interface{}{
		"query":         query,
		"engines":       engines,
		"assets":        resp.Assets,
		"totalCount":    resp.TotalCount,
		"engineStats":   resp.EngineStats,
		"errors":        resp.Errors,
		"staticVersion": s.staticVersion,
	})
}

// handleResults 处理结果页面请求
func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	engines := []string{}
	if engine := strings.TrimSpace(r.URL.Query().Get("engine")); engine != "" {
		engines = []string{engine}
	}

	// 渲染结果页面
	s.templates.ExecuteTemplate(w, "results.html", map[string]interface{}{
		"query":         query,
		"engines":       engines,
		"assets":        []model.UnifiedAsset{},
		"staticVersion": s.staticVersion,
	})
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

	s.templates.ExecuteTemplate(w, "quota.html", map[string]interface{}{
		"engines":       engines,
		"quotaInfo":     quotaInfo,
		"errorInfo":     errorInfo,
		"staticVersion": s.staticVersion,
	})
}

// handleWebSocket 处理WebSocket连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 验证WebSocket连接请求
	if !s.validateWebSocketRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// 为连接生成唯一ID
	connID := fmt.Sprintf("%d", time.Now().UnixNano())

	// 添加到连接管理器
	s.connManager.mutex.Lock()
	s.connManager.connections[connID] = conn
	s.connManager.mutex.Unlock()

	// 连接关闭时从管理器中移除
	defer func() {
		s.connManager.mutex.Lock()
		delete(s.connManager.connections, connID)
		s.connManager.mutex.Unlock()
		logger.Infof("WebSocket connection closed: %s", connID)
	}()

	// 设置连接读取超时
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// 启动ping协程
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := conn.WriteJSON(map[string]interface{}{"type": "ping"}); err != nil {
					logger.Errorf("WebSocket ping error: %v", err)
					return
				}
			}
		}
	}()

	// 处理WebSocket消息
	for {
		var message map[string]interface{}
		err := conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Errorf("WebSocket read error: %v", err)
			}
			break
		}

		// 处理不同类型的消息
		if messageType, ok := message["type"].(string); ok {
			switch messageType {
			case "ping":
				// 回复ping消息
				if err := conn.WriteJSON(map[string]interface{}{"type": "pong"}); err != nil {
					logger.Errorf("WebSocket write error: %v", err)
					break
				}
			case "pong":
				// 收到pong消息，重置读取超时
				conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			case "query":
				// 处理查询请求
				s.handleWebSocketQuery(conn, message)
			}
		}
	}
}

// validateWebSocketRequest 验证WebSocket连接请求
func (s *Server) validateWebSocketRequest(r *http.Request) bool {
	// 从请求头获取令牌
	token := r.Header.Get("X-WebSocket-Token")

	// 从查询参数获取令牌
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	// 检查是否有配置的令牌
	configToken := os.Getenv("UNIMAP_WS_TOKEN")
	if configToken != "" {
		// 生产环境：强制要求令牌验证
		if token == "" {
			logger.Warn("WebSocket connection rejected: missing token")
			return false
		}
		if token != configToken {
			logger.Warn("WebSocket connection rejected: invalid token")
			return false
		}
		return true
	}

	// 开发环境：允许无令牌连接，但记录警告
	if token == "" {
		logger.Warn("WebSocket connection without token (development mode)")
	}
	return true
}

// maskAPIKey 屏蔽API密钥，用于日志输出
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
}

// handleWebSocketQuery 处理WebSocket查询请求
func (s *Server) handleWebSocketQuery(conn *websocket.Conn, message map[string]interface{}) {
	// 解析查询参数
	query, _ := message["query"].(string)
	query = strings.TrimSpace(query)

	if err := validateQueryInput(query); err != nil {
		if err := conn.WriteJSON(map[string]interface{}{
			"type":  "query_error",
			"error": err.Error(),
		}); err != nil {
			logger.Errorf("WebSocket write error: %v", err)
		}
		return
	}

	pageSize := parseWSInt(message["page_size"], 50)

	engines := parseWSStringList(message["engines"])
	if len(engines) == 0 {
		// 如果没有选择引擎，使用默认引擎
		defaultEngines := s.orchestrator.ListAdapters()
		if len(defaultEngines) > 0 {
			engines = []string{defaultEngines[0]}
		}
	}

	if len(engines) == 0 {
		// 发送查询错误消息
		if err := conn.WriteJSON(map[string]interface{}{
			"type":  "query_error",
			"error": "No engines configured/registered. Please set API keys in configs/config.yaml and enable at least one engine.",
		}); err != nil {
			logger.Errorf("WebSocket write error: %v", err)
		}
		return
	}

	// 生成查询ID
	queryID := fmt.Sprintf("%d", time.Now().UnixNano())

	// 创建查询状态
	status := &QueryStatus{
		ID:         queryID,
		Query:      query,
		Engines:    engines,
		Status:     "running",
		Progress:   0,
		Results:    []model.UnifiedAsset{},
		TotalCount: 0,
		Errors:     []string{},
		StartTime:  time.Now(),
	}

	// 保存查询状态
	s.queryMutex.Lock()
	s.queryStatus[queryID] = status
	s.queryMutex.Unlock()

	// 发送查询开始消息
	if err := conn.WriteJSON(map[string]interface{}{
		"type":     "query_start",
		"query_id": queryID,
		"status":   status,
	}); err != nil {
		logger.Errorf("WebSocket write error: %v", err)
	}

	// 异步执行查询
	go func() {
		// 执行查询
		req := service.QueryRequest{
			Query:       query,
			Engines:     engines,
			PageSize:    pageSize,
			ProcessData: true,
		}

		resp, queryErr := s.service.Query(context.Background(), req)
		endTime := time.Now()

		// 更新查询状态（在锁内修改，避免并发读写竞态）
		s.queryMutex.Lock()
		st := s.queryStatus[queryID]
		if st != nil {
			if queryErr != nil {
				st.Errors = append(st.Errors, fmt.Sprintf("Query failed: %v", queryErr))
				st.Status = "error"
			} else {
				st.Results = resp.Assets
				st.TotalCount = resp.TotalCount
				st.Errors = resp.Errors
				st.Status = "completed"
			}
			st.Progress = 100
			st.EndTime = endTime
		}
		var statusCopy QueryStatus
		if st != nil {
			statusCopy = *st
		}
		s.queryMutex.Unlock()

		// 延迟清理查询状态，允许客户端在一段时间内查询已完成任务的状态
		go func() {
			time.Sleep(5 * time.Minute)
			s.queryMutex.Lock()
			delete(s.queryStatus, queryID)
			s.queryMutex.Unlock()
		}()

		// 发送查询完成消息（发副本，避免边编码边被修改）
		var resultsPayload map[string]interface{}
		if queryErr != nil || resp == nil {
			errMsg := ""
			if queryErr != nil {
				errMsg = fmt.Sprintf("Query failed: %v", queryErr)
			}
			resultsPayload = map[string]interface{}{
				"query":       query,
				"engines":     engines,
				"assets":      []model.UnifiedAsset{},
				"totalCount":  0,
				"engineStats": map[string]int{},
				"errors":      []string{errMsg},
				"error":       errMsg,
			}
		} else {
			resultsPayload = map[string]interface{}{
				"query":       query,
				"engines":     engines,
				"assets":      resp.Assets,
				"totalCount":  resp.TotalCount,
				"engineStats": resp.EngineStats,
				"errors":      resp.Errors,
			}
		}

		if err := conn.WriteJSON(map[string]interface{}{
			"type":     "query_complete",
			"query_id": queryID,
			"status":   statusCopy,
			"results":  resultsPayload,
		}); err != nil {
			logger.Errorf("WebSocket write error: %v", err)
		}
	}()
}

// handleQueryStatus 处理查询状态请求
func (s *Server) handleQueryStatus(w http.ResponseWriter, r *http.Request) {
	queryID := r.URL.Query().Get("query_id")
	if queryID == "" {
		http.Error(w, "Missing query_id", http.StatusBadRequest)
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
		http.Error(w, "Query not found", http.StatusNotFound)
		return
	}

	// 返回JSON结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statusCopy)
}

// 广播消息给所有WebSocket连接
func (s *Server) broadcastMessage(message interface{}) {
	s.connManager.mutex.RLock()
	defer s.connManager.mutex.RUnlock()

	for _, conn := range s.connManager.connections {
		if err := conn.WriteJSON(message); err != nil {
			logger.Errorf("WebSocket broadcast error: %v", err)
		}
	}
}

// 更新查询进度并广播
func (s *Server) updateQueryProgress(queryID string, progress float64) {
	s.queryMutex.Lock()
	defer s.queryMutex.Unlock()

	if status, exists := s.queryStatus[queryID]; exists {
		status.Progress = progress
		s.queryStatus[queryID] = status

		// 广播进度更新
		s.broadcastMessage(map[string]interface{}{
			"type":     "progress_update",
			"query_id": queryID,
			"progress": progress,
		})
	}
}

// handleTamperCheck 处理篡改检测请求
func (s *Server) handleTamperCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URLs        []string `json:"urls"`
		Concurrency int      `json:"concurrency"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.URLs) == 0 {
		http.Error(w, "No URLs provided", http.StatusBadRequest)
		return
	}

	if req.Concurrency <= 0 {
		req.Concurrency = 5
	}

	// 创建篡改检测器
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir: "./hash_store",
	})

	// 执行篡改检测
	results, err := detector.BatchCheckTampering(r.Context(), req.URLs, req.Concurrency)
	if err != nil {
		http.Error(w, fmt.Sprintf("Tamper check failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"results": results,
	})
}

// handleTamperBaseline 处理基线设置请求
func (s *Server) handleTamperBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URLs        []string `json:"urls"`
		Concurrency int      `json:"concurrency"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.URLs) == 0 {
		http.Error(w, "No URLs provided", http.StatusBadRequest)
		return
	}

	if req.Concurrency <= 0 {
		req.Concurrency = 5
	}

	// 创建篡改检测器
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir: "./hash_store",
	})

	// 设置基线
	results, err := detector.BatchSetBaseline(r.Context(), req.URLs, req.Concurrency)
	if err != nil {
		http.Error(w, fmt.Sprintf("Set baseline failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"results": results,
	})
}

// handleTamperBaselineList 处理基线列表请求
func (s *Server) handleTamperBaselineList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 创建篡改检测器
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir: "./hash_store",
	})

	// 获取基线列表
	urls, err := detector.ListBaselines()
	if err != nil {
		http.Error(w, fmt.Sprintf("List baselines failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"urls":    urls,
		"count":   len(urls),
	})
}

// handleTamperBaselineDelete 处理删除基线请求
func (s *Server) handleTamperBaselineDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// 创建篡改检测器
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir: "./hash_store",
	})

	// 删除基线
	if err := detector.DeleteBaseline(req.URL); err != nil {
		http.Error(w, fmt.Sprintf("Delete baseline failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Baseline for %s deleted", req.URL),
	})
}
