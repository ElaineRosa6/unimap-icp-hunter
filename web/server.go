package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	port             int
	templates        *template.Template
	service          *service.UnifiedService
	orchestrator     *adapter.EngineOrchestrator
	upgrader         websocket.Upgrader
	connManager      *ConnectionManager
	queryStatus      map[string]*QueryStatus
	queryMutex       sync.RWMutex
	webRoot          string
	staticVersion    string
	screenshotMgr    *screenshot.Manager
	config           *config.Config
}

// NewServer 创建Web服务器
func NewServer(port int, service *service.UnifiedService, orchestrator *adapter.EngineOrchestrator, cfg *config.Config) (*Server, error) {
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
			return true // 允许所有来源的WebSocket连接
		},
	}

	// 初始化截图管理器
	var screenshotMgr *screenshot.Manager
	if cfg != nil && cfg.Screenshot.Enabled {
		screenshotCfg := screenshot.Config{
			BaseDir:      cfg.Screenshot.BaseDir,
			ChromePath:   cfg.Screenshot.ChromePath,
			Timeout:      time.Duration(cfg.Screenshot.Timeout) * time.Second,
			WindowWidth:  cfg.Screenshot.WindowWidth,
			WindowHeight: cfg.Screenshot.WindowHeight,
			WaitTime:     time.Duration(cfg.Screenshot.WaitTime) * time.Millisecond,
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

// Start 启动Web服务器
func (s *Server) Start() error {
	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/health", s.handleHealth)
	http.HandleFunc("/query", s.handleQuery)
	http.HandleFunc("/api/query", s.handleAPIQuery)
	http.HandleFunc("/api/ws", s.handleWebSocket)
	http.HandleFunc("/api/query/status", s.handleQueryStatus)
	http.HandleFunc("/api/screenshot", s.handleScreenshot)
	http.HandleFunc("/api/screenshot/search-engine", s.handleSearchEngineScreenshot)
	http.HandleFunc("/api/screenshot/target", s.handleTargetScreenshot)
	http.HandleFunc("/api/screenshot/batch", s.handleBatchScreenshot)
	http.HandleFunc("/results", s.handleResults)
	http.HandleFunc("/quota", s.handleQuota)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(s.webRoot, "static")))))

	addr := fmt.Sprintf(":%d", s.port)
	logger.Infof("Web server started at http://localhost%s", addr)
	return http.ListenAndServe(addr, nil)
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

// handleAPIQuery 处理API查询请求（用于异步查询）
func (s *Server) handleAPIQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Query cannot be empty",
		})
		return
	}

	// 输入验证：检查查询长度和内容
	if len(query) > 1000 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Query is too long (maximum 1000 characters)",
		})
		return
	}

	// 输入验证：检查是否包含恶意字符
	if strings.Contains(query, "'\"") || strings.Contains(query, "\\\\") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Query contains invalid characters",
		})
		return
	}

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
		"success": true,
		"path":    screenshotPath,
		"engine":  engine,
		"query":   query,
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
		"query_id":          req.QueryID,
		"search_engines":    []map[string]interface{}{},
		"targets":           []map[string]interface{}{},
		"errors":            []string{},
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

// handleIndex 处理首页请求
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	engines := s.orchestrator.ListAdapters()
	s.templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
		"engines":       engines,
		"staticVersion": s.staticVersion,
	})
}

// handleQuery 处理查询请求
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if query == "" {
		s.templates.ExecuteTemplate(w, "error.html", map[string]interface{}{
			"error": "Query cannot be empty",
		})
		return
	}

	// 输入验证：检查查询长度和内容
	if len(query) > 1000 {
		s.templates.ExecuteTemplate(w, "error.html", map[string]interface{}{
			"error": "Query is too long (maximum 1000 characters)",
		})
		return
	}

	// 输入验证：检查是否包含恶意字符
	if strings.Contains(query, "'\"") || strings.Contains(query, "\\\\") {
		s.templates.ExecuteTemplate(w, "error.html", map[string]interface{}{
			"error": "Query contains invalid characters",
		})
		return
	}

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

	if query == "" {
		// 发送查询错误消息
		if err := conn.WriteJSON(map[string]interface{}{
			"type":  "query_error",
			"error": "Query cannot be empty",
		}); err != nil {
			fmt.Printf("WebSocket write error: %v\n", err)
		}
		return
	}

	// 输入验证：检查查询长度和内容
	if len(query) > 1000 {
		// 发送查询错误消息
		if err := conn.WriteJSON(map[string]interface{}{
			"type":  "query_error",
			"error": "Query is too long (maximum 1000 characters)",
		}); err != nil {
			logger.Errorf("WebSocket write error: %v", err)
		}
		return
	}

	// 输入验证：检查是否包含恶意字符
	if strings.Contains(query, "'\"") || strings.Contains(query, "\\\\") {
		// 发送查询错误消息
		if err := conn.WriteJSON(map[string]interface{}{
			"type":  "query_error",
			"error": "Query contains invalid characters",
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
		// 清理查询状态，避免内存泄漏
		delete(s.queryStatus, queryID)
		s.queryMutex.Unlock()

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
