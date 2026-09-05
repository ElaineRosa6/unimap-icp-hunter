package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/unimap/project/internal/adapter"
	"github.com/unimap/project/internal/alerting"
	"github.com/unimap/project/internal/auth"
	"github.com/unimap/project/internal/backup"
	"github.com/unimap/project/internal/collection"
	"github.com/unimap/project/internal/config"
	"github.com/unimap/project/internal/distributed"
	historydb "github.com/unimap/project/internal/history"
	icpdb "github.com/unimap/project/internal/icp/database"
	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/metrics"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/notify"
	"github.com/unimap/project/internal/proxypool"
	"github.com/unimap/project/internal/requestid"
	"github.com/unimap/project/internal/scheduler"
	"github.com/unimap/project/internal/screenshot"
	"github.com/unimap/project/internal/screenshot/batchdb"
	"github.com/unimap/project/internal/service"
	"github.com/unimap/project/internal/utils"
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

// browserBackendAdapter adapts screenshot.Provider to adapter.BrowserQueryBackend.
type browserBackendAdapter struct {
	provider screenshot.Provider
}

func (a *browserBackendAdapter) CollectSearchEngineResult(ctx context.Context, engine, query, queryID string) ([]collection.CollectResult, error) {
	if a.provider == nil {
		return nil, fmt.Errorf("browser backend not initialized")
	}
	return a.provider.CollectSearchEngineResult(ctx, engine, query, queryID)
}

// WebSocket连接管理器
type ConnectionManager struct {
	connections map[string]*managedConn
	mutex       sync.RWMutex
}

// Server Web服务器
type Server struct {
	backupSnapshotter   backup.SQLiteSnapshotter
	port                int
	httpServer          *http.Server
	templates           *template.Template
	service             *service.UnifiedService
	queryApp            *service.QueryAppService
	monitorApp          *service.MonitorAppService
	tamperApp           *service.TamperAppService
	screenshotApp       *service.ScreenshotAppService
	orchestrator        *adapter.EngineOrchestrator
	upgrader            websocket.Upgrader
	connManager         *ConnectionManager
	queryStatus         map[string]*QueryStatus
	queryMutex          sync.RWMutex
	configMutex         sync.Mutex
	webRoot             string
	staticVersion       string
	screenshotMgr       *screenshot.Manager
	screenshotRouter    *screenshot.ScreenshotRouter
	batchJobs           *batchJobStore
	batchDB             *batchdb.Database
	config              *config.Config
	configManager       *config.Manager
	ephemeralAdminToken string
	chromeCmd           *os.Process
	chromeCmdMu         sync.Mutex
	bridge              *BridgeState
	proxyPool           *proxypool.Pool
	distributed         *DistributedState
	scheduler           *scheduler.Scheduler
	icpDB               *icpdb.Database
	icpRepo             icpdb.ICPResultRepository
	notifyRegistry      *notify.Registry
	apiAuth             *auth.AuthMiddleware
	permissionManager   *auth.PermissionManager
	userDB              *auth.UserDB
	userRepo            auth.UserRepository
	registrationMutex   sync.Mutex
	historyDB           *historydb.Database
	historyRepo         *historydb.Repository
	shutdownCtx         context.Context
	shutdownCancel      context.CancelFunc
	revocationStore     *sessionRevocationStore
}

// NewServer 创建Web服务器
func NewServer(port int, unifiedSvc *service.UnifiedService, orchestrator *adapter.EngineOrchestrator, cfg *config.Config, cfgManager *config.Manager) (*Server, error) {
	webRoot, err := resolveWebRoot()
	if err != nil {
		return nil, err
	}

	templates, err := loadTemplates(webRoot)
	if err != nil {
		return nil, err
	}

	upgrader := newWebSocketUpgrader(cfg)
	screenshotMgr := initScreenshotManager(cfg)
	screenshotApp := initScreenshotAppService(cfg, screenshotMgr)
	proxyPool := initProxyPool(cfg)
	nodeRegistry, nodeTaskQueue := initDistributedNodes(cfg)
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	alertManager := initAlertManager(cfg)

	srv := newServerStruct(port, webRoot, templates, upgrader, unifiedSvc, orchestrator,
		cfg, cfgManager, screenshotMgr, screenshotApp, proxyPool,
		nodeRegistry, nodeTaskQueue, alertManager, shutdownCtx, shutdownCancel)

	initICPDatabase(srv, cfg)
	initHistoryDatabase(srv, cfg)
	initUserDatabase(srv)
	initScreenshotBatchDB(srv)
	initBackupSnapshots(srv)

	// Initialize the final screenshot/Bridge router before constructing task
	// runners. QueryRunner must receive the combined collect+capture provider,
	// not the temporary CDP-only fallback returned during early startup.
	initScreenshotMode(srv, cfg, screenshotCDPProvider(screenshotMgr), screenshotMgr, screenshotApp, shutdownCtx)
	wireBrowserBackend(srv, cfg, unifiedSvc)

	sched := initScheduler(srv, cfg, screenshotApp, screenshotMgr, alertManager, orchestrator, unifiedSvc, nodeTaskQueue)
	srv.notifyRegistry = initNotifySystem(cfg, cfgManager, sched)

	// 所有 handler 注册完毕且数据加载完成后再启动 cron
	sched.Start()
	srv.scheduler = sched

	return srv, nil
}

// loadTemplates parses all HTML templates with custom template functions.
func loadTemplates(webRoot string) (*template.Template, error) {
	tmpl := template.New("").Funcs(newTemplateFuncMap())
	templates, err := tmpl.ParseGlob(filepath.Join(webRoot, "templates", "*.html"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates from %s: %w", webRoot, err)
	}
	return templates, nil
}

// newTemplateFuncMap returns the custom template function map for HTML rendering.
func newTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
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
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call: odd number of arguments")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings, got %T", values[i])
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
	}
}

// screenshotCDPProvider returns a CDP provider if the manager is non-nil, nil otherwise.
func screenshotCDPProvider(mgr *screenshot.Manager) screenshot.Provider {
	if mgr != nil {
		return screenshot.NewCDPProvider(mgr)
	}
	return nil
}

// newServerStruct constructs the Server struct with all initialized components.
func newServerStruct(port int, webRoot string, templates *template.Template,
	upgrader websocket.Upgrader, unifiedSvc *service.UnifiedService,
	orchestrator *adapter.EngineOrchestrator, cfg *config.Config, cfgManager *config.Manager,
	screenshotMgr *screenshot.Manager, screenshotApp *service.ScreenshotAppService,
	proxyPool *proxypool.Pool, nodeRegistry *distributed.Registry,
	nodeTaskQueue *distributed.TaskQueue, alertManager *alerting.Manager,
	shutdownCtx context.Context, shutdownCancel context.CancelFunc) *Server {

	srv := &Server{
		port:          port,
		templates:     templates,
		service:       unifiedSvc,
		queryApp:      service.NewQueryAppService(unifiedSvc, orchestrator),
		monitorApp:    service.NewMonitorAppService(proxyPool),
		screenshotApp: screenshotApp,
		orchestrator:  orchestrator,
		upgrader:      upgrader,
		connManager:   &ConnectionManager{connections: make(map[string]*managedConn)},
		queryStatus:   make(map[string]*QueryStatus),
		webRoot:       webRoot,
		staticVersion: strconv.FormatInt(time.Now().Unix(), 10),
		screenshotMgr: screenshotMgr,
		batchJobs:     newBatchJobStore(),
		config:        cfg,
		configManager: cfgManager,
		bridge: &BridgeState{
			Tokens:         make(map[string]int64),
			LastSeen:       make(map[string]int64),
			CallbackNonces: make(map[string]int64),
		},
		proxyPool: proxyPool,
		distributed: &DistributedState{
			NodeRegistry:  nodeRegistry,
			NodeTaskQueue: nodeTaskQueue,
		},
		apiAuth:           auth.NewAuthMiddleware(auth.NewAPIKeyManager(filepath.Join(utils.AppDataDir(), "api_keys.json"))),
		permissionManager: auth.NewPermissionManager(),
		shutdownCtx:       shutdownCtx,
		shutdownCancel:    shutdownCancel,
		revocationStore:   newSessionRevocationStore(),
	}
	persistentTamper, tamperErr := service.NewPersistentTamperAppService(utils.HashStoreDir(), alertManager, srv.tamperRuntimeConfig)
	if tamperErr != nil {
		logger.Warnf("persistent tamper history unavailable: %v", tamperErr)
		srv.tamperApp = service.NewTamperAppService(utils.HashStoreDir(), alertManager, srv.tamperRuntimeConfig)
	} else {
		srv.tamperApp = persistentTamper
	}
	return srv
}

// newWebSocketUpgrader creates a WebSocket upgrader with origin checking from config.
func newWebSocketUpgrader(cfg *config.Config) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return isOriginAllowed(r.Header.Get("Origin"), r.Host, allowedOriginsFromConfig(cfg))
		},
	}
}

// initScreenshotManager creates and configures the screenshot manager from config.
func initScreenshotManager(cfg *config.Config) *screenshot.Manager {
	if cfg == nil || !cfg.Screenshot.Enabled {
		return nil
	}

	headless := true
	if cfg.Screenshot.Headless != nil {
		headless = *cfg.Screenshot.Headless
	}

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
		NoSandbox:      cfg.Screenshot.NoSandbox,
		Timeout:        time.Duration(cfg.Screenshot.Timeout) * time.Second,
		WindowWidth:    cfg.Screenshot.WindowWidth,
		WindowHeight:   cfg.Screenshot.WindowHeight,
		WaitTime:       time.Duration(cfg.Screenshot.WaitTime) * time.Millisecond,
		MaxSessions:    cfg.Screenshot.MaxSessions,
	}
	mgr := screenshot.NewManager(screenshotCfg)

	loadEngineCookies(mgr, cfg)

	logger.Infof("Screenshot manager initialized with base dir: %s", cfg.Screenshot.BaseDir)
	return mgr
}

// loadEngineCookies loads per-engine cookies from config into the screenshot manager.
func loadEngineCookies(mgr *screenshot.Manager, cfg *config.Config) {
	for _, engine := range browserEngines() {
		cookies := engineCookies(cfg, engine)
		if len(cookies) > 0 {
			mgr.SetCookies(engine, convertConfigCookies(cookies))
		}
	}
}

// initScreenshotAppService creates the screenshot app service from config and manager.
func initScreenshotAppService(cfg *config.Config, screenshotMgr *screenshot.Manager) *service.ScreenshotAppService {
	screenshotBaseDir := utils.ScreenshotsDir()
	if cfg != nil && strings.TrimSpace(cfg.Screenshot.BaseDir) != "" {
		screenshotBaseDir = strings.TrimSpace(cfg.Screenshot.BaseDir)
	}

	var provider screenshot.Provider
	if screenshotMgr != nil {
		provider = screenshot.NewCDPProvider(screenshotMgr)
	}

	app := service.NewScreenshotAppServiceWithProvider(screenshotBaseDir, provider)
	if cfg != nil {
		app.SetEngine(cfg.Screenshot.Engine)
		app.SetFallbackToCDP(cfg.Screenshot.Extension.FallbackToCDP)
	}
	return app
}

// initProxyPool creates the proxy pool from config.
func initProxyPool(cfg *config.Config) *proxypool.Pool {
	if cfg == nil {
		return nil
	}
	pool := proxypool.NewPool(proxypool.Config{
		Enabled:             cfg.Network.ProxyPool.Enabled,
		Proxies:             cfg.Network.ProxyPool.Proxies,
		FailureThreshold:    cfg.Network.ProxyPool.FailureThreshold,
		Cooldown:            time.Duration(cfg.Network.ProxyPool.CooldownSeconds) * time.Second,
		AllowDirectFallback: cfg.Network.ProxyPool.AllowDirectFallback,
	})
	if pool.Enabled() {
		logger.Infof("Proxy pool enabled: %d proxies, strategy=%s", len(pool.Proxies()), cfg.Network.ProxyPool.Strategy)
	}
	return pool
}

// initDistributedNodes creates the distributed node registry and task queue.
func initDistributedNodes(cfg *config.Config) (*distributed.Registry, *distributed.TaskQueue) {
	heartbeatTimeout := 30 * time.Second
	maxReassign := 1
	if cfg != nil {
		heartbeatTimeout = time.Duration(cfg.Distributed.HeartbeatTimeoutSeconds) * time.Second
		if heartbeatTimeout <= 0 {
			heartbeatTimeout = 30 * time.Second
		}
		maxReassign = cfg.Distributed.MaxReassignAttempts
	}

	registry := distributed.NewRegistry(heartbeatTimeout)
	taskQueue := distributed.NewTaskQueueWithPath(filepath.Join(utils.AppDataDir(), "distributed_tasks.json"))
	registry.SetTaskQueue(taskQueue)
	taskQueue.SetDefaultMaxReassign(maxReassign)

	if cfg != nil && cfg.Distributed.Scheduler.Strategy != "" {
		strategy := distributed.SchedulerStrategy(cfg.Distributed.Scheduler.Strategy)
		sched := distributed.NewSchedulerFromStrategy(strategy)
		taskQueue.SetScheduler(sched)
	}

	return registry, taskQueue
}

// initAlertManager creates the alert manager and registers configured channels.
func initAlertManager(cfg *config.Config) *alerting.Manager {
	mgr := alerting.NewManager()
	if err := mgr.SetPersistencePath(filepath.Join(utils.AppDataDir(), "alerts.json")); err != nil {
		logger.Warnf("alert persistence unavailable: %v", err)
	}

	logChannel := alerting.NewLogChannel(true)
	mgr.RegisterChannel(logChannel)

	if cfg != nil && cfg.Alerting.Webhook.Enabled && cfg.Alerting.Webhook.URL != "" {
		headers := make(map[string]string)
		if cfg.Alerting.Webhook.AuthToken != "" {
			headers["Authorization"] = "Bearer " + cfg.Alerting.Webhook.AuthToken
		}
		webhookChannel := alerting.NewWebhookChannel(cfg.Alerting.Webhook.URL, headers, true)
		mgr.RegisterChannel(webhookChannel)
	}

	return mgr
}

// initICPDatabase initializes the ICP result database on the server.
func initICPDatabase(srv *Server, cfg *config.Config) {
	if cfg == nil || strings.TrimSpace(cfg.ICP.DatabasePath) == "" {
		return
	}
	dbPath := cfg.ICP.DatabasePath
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		logger.Warnf("ICP result DB dir create failed (%s): %v", dbPath, err)
		return
	}
	db, err := icpdb.NewDatabase(dbPath)
	if err != nil {
		logger.Warnf("ICP result DB unavailable at %s: %v", dbPath, err)
		return
	}
	if err := db.InitSchema(); err != nil {
		logger.Warnf("ICP result DB schema init failed: %v", err)
		_ = db.Close()
		return
	}
	srv.icpDB = db
	srv.icpRepo = icpdb.NewICPResultRepository(db.DB())
}

// initHistoryDatabase initializes the operation history database on the server.
func initHistoryDatabase(srv *Server, cfg *config.Config) {
	if cfg == nil || !cfg.History.Enabled {
		return
	}
	dbPath := cfg.History.DatabasePath
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		logger.Warnf("history DB dir create failed (%s): %v", dbPath, err)
		return
	}
	db, err := historydb.NewDatabase(dbPath)
	if err != nil {
		logger.Warnf("history DB unavailable at %s: %v", dbPath, err)
		return
	}
	if err := db.InitSchema(); err != nil {
		logger.Warnf("history DB schema init failed: %v", err)
		_ = db.Close()
		return
	}
	srv.historyDB = db
	srv.historyRepo = historydb.NewRepository(db.DB())
	srv.queryApp.SetHistoryRepository(srv.historyRepo)
}

// initScreenshotBatchDB initializes the screenshot batch job metadata database.
// On failure it logs a warning and leaves batchJobs as memory-only (graceful degradation).
func initScreenshotBatchDB(srv *Server) {
	dbPath := filepath.Join(utils.AppDataDir(), "screenshot_batches.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		logger.Warnf("screenshot batch DB dir create failed (%s): %v", dbPath, err)
		return
	}
	db, err := batchdb.NewDatabase(dbPath)
	if err != nil {
		logger.Warnf("screenshot batch DB unavailable at %s: %v", dbPath, err)
		return
	}
	if err := db.InitSchema(); err != nil {
		logger.Warnf("screenshot batch DB schema init failed: %v", err)
		_ = db.Close()
		return
	}
	srv.batchDB = db
	srv.batchJobs.setRepo(batchdb.NewRepository(db.DB()))
	logger.Infof("screenshot batch DB initialized at %s", dbPath)
}

// initUserDatabase initializes the user database on the server.
func initUserDatabase(srv *Server) {
	userDBPath := filepath.Join(utils.AppDataDir(), "users.db")
	if err := os.MkdirAll(filepath.Dir(userDBPath), 0o755); err != nil {
		logger.Warnf("user DB dir create failed (%s): %v", userDBPath, err)
		return
	}
	udb, err := auth.NewUserDB(userDBPath)
	if err != nil {
		logger.Warnf("user DB unavailable at %s: %v", userDBPath, err)
		return
	}
	if err := udb.InitSchema(); err != nil {
		logger.Warnf("user DB schema init failed: %v", err)
		_ = udb.Close()
		return
	}
	srv.userDB = udb
	srv.userRepo = auth.NewUserRepository(udb.DB())
	logger.Infof("user database initialized at %s", userDBPath)
}

// initScheduler creates the task scheduler and registers all runner handlers.
func initScheduler(srv *Server, cfg *config.Config, screenshotApp *service.ScreenshotAppService,
	screenshotMgr *screenshot.Manager, alertManager *alerting.Manager,
	orchestrator *adapter.EngineOrchestrator, unifiedSvc *service.UnifiedService,
	nodeTaskQueue *distributed.TaskQueue) *scheduler.Scheduler {

	maxHistory := 500
	if cfg != nil && cfg.Scheduler.MaxHistory > 0 {
		maxHistory = cfg.Scheduler.MaxHistory
	}

	sched := scheduler.NewScheduler(
		filepath.Join(utils.AppDataDir(), "scheduler_tasks.json"),
		filepath.Join(utils.AppDataDir(), "scheduler_history.json"),
		maxHistory)

	// Shared session health tracker: enables per-engine circuit breaking across
	// LoginStatusCheckRunner (records success/failure) and QueryRunner (skips
	// browser tasks for circuit-open engines).
	sessionHealth := scheduler.NewSessionHealthTracker()

	// 高优先级 Runner (ST-01 ~ ST-08)
	sched.RegisterHandler(scheduler.NewQueryRunnerWithBrowser(srv.queryApp, screenshotApp, screenshotMgr, srv.browserQueryProvider(), sessionHealth).WithExportDir(utils.AppDataDir("exports")))
	sched.RegisterHandler(scheduler.NewSearchScreenshotRunner(screenshotApp, screenshotMgr))
	var batchRepo *batchdb.Repository
	if srv.batchDB != nil {
		batchRepo = batchdb.NewRepository(srv.batchDB.DB())
	}
	sched.RegisterHandler(scheduler.NewBatchScreenshotRunner(screenshotApp, screenshotMgr, batchRepo))

	// TamperCheckRunner: inject a browser allocator from the screenshot manager
	// so scheduled tamper patrols can render JS-heavy/SPA pages. Without an
	// allocator the detector falls back to HTTP/Fast mode and may produce empty
	// hashes for SPA targets, causing false "tampered" or "unreachable" results.
	// When screenshotMgr is unavailable we keep the historical nil behavior.
	var tamperPageLoader service.TamperPageLoader
	if screenshotMgr != nil {
		tamperPageLoader = screenshotMgr
	}
	sched.RegisterHandler(scheduler.NewTamperCheckRunnerWithEvidence(
		srv.tamperApp,
		tamperPageLoader,
		screenshotApp,
		screenshotMgr,
		func() bool {
			current := srv.currentConfig()
			return current != nil && current.Tamper.EvidenceScreenshotEnabled
		},
	))
	sched.RegisterHandler(scheduler.NewURLReachabilityRunner(srv.monitorApp, alertManager))
	sched.RegisterHandler(scheduler.NewCookieVerifyRunner(screenshotApp, screenshotMgr))
	sched.RegisterHandler(scheduler.NewLoginStatusCheckRunner(screenshotMgr, sessionHealth))
	sched.RegisterHandler(scheduler.NewDistributedSubmitRunner(nodeTaskQueue))

	// 中优先级 Runner (ST-09 ~ ST-16)
	sched.RegisterHandler(scheduler.NewExportRunner(srv.queryApp, orchestrator, utils.AppDataDir("exports")))
	sched.RegisterHandler(scheduler.NewPortScanRunner(srv.monitorApp, alertManager))
	sched.RegisterHandler(scheduler.NewScreenshotCleanupRunner(screenshotApp, 30))
	sched.RegisterHandler(scheduler.NewTamperCleanupRunner(srv.tamperApp, 90))
	sched.RegisterHandler(scheduler.NewQuotaMonitorRunner(orchestrator, 10))
	sched.RegisterHandler(scheduler.NewAlertSummaryRunner(alertManager))
	sched.RegisterHandler(scheduler.NewBaselineRefreshRunner(srv.tamperApp, tamperPageLoader))
	sched.RegisterHandler(scheduler.NewURLImportRunner(utils.AppDataDir("imports")))
	sched.RegisterHandler(scheduler.NewBackupRunner(srv.snapshotSQLite))

	// 低优先级 Runner (ST-17 ~ ST-20)
	sched.RegisterHandler(scheduler.NewPluginHealthRunner(unifiedSvc))
	sched.RegisterHandler(scheduler.NewBridgeHealthCheckRunnerWithProvider(func() *screenshot.BridgeService {
		if srv == nil || srv.bridge == nil {
			return nil
		}
		return srv.bridge.service()
	}))
	sched.RegisterHandler(scheduler.NewAlertSilenceRunner(alertManager))
	sched.RegisterHandler(scheduler.NewCacheWarmupRunner())

	// ICP Runner (ST-21 ~ ST-22)
	sched.RegisterHandler(scheduler.NewICPQueryRunner(srv.icpConfigProvider, srv.icpRepo, alertManager))
	sched.RegisterHandler(scheduler.NewICPImportRunner(utils.AppDataDir("icp_imports"), sched))

	// 持久化推送审计：每次通知推送追加到 history 库的 notification_push_log，
	// 与仅作 only_new 去重状态的 notify_push_state 区分（这是 append-only 推送事件日志）。
	if srv != nil && srv.historyRepo != nil {
		repo := srv.historyRepo
		sched.SetNotificationLogRecorder(func(rec scheduler.PushLogRecord) error {
			return repo.RecordNotificationLog(historydb.NotificationPushLog{
				TaskID:        rec.TaskID,
				TaskName:      rec.TaskName,
				ChannelIDs:    rec.ChannelIDs,
				Status:        rec.Status,
				ResultCount:   rec.ResultCount,
				ResultSummary: rec.ResultSummary,
				Error:         rec.Error,
			})
		})
	}

	if err := sched.Load(); err != nil {
		logger.Warnf("Failed to load scheduled tasks: %v", err)
	}

	return sched
}

// initNotifySystem initializes the notification registry and configures channels.
func initNotifySystem(cfg *config.Config, cfgManager *config.Manager,
	sched *scheduler.Scheduler) *notify.Registry {

	reg := notify.NewRegistry()
	reg.Register(notify.NewLogChannel("builtin-log", true)) //nolint:errcheck

	sched.SetNotifyRegistry(reg)

	if cfg != nil {
		sched.SetNotifyCfgProvider(func() *notify.NotifyGlobalCfg {
			c := cfgManager.GetConfig()
			return &notify.NotifyGlobalCfg{
				Enabled:        c.Notifications.Enabled,
				SendTimeoutSec: c.Notifications.SendTimeoutSec,
				MaxRetries:     c.Notifications.MaxRetries,
			}
		})
		reloadNotifyChannelConfigs(reg, cfg)
		registerFeishuAppChannel(reg, cfg)
	}

	return reg
}

// registerFeishuAppChannel registers the Feishu app notification channel if configured.
func registerFeishuAppChannel(reg *notify.Registry, cfg *config.Config) {
	if cfg == nil || cfg.Notifications.FeishuApp == nil {
		return
	}
	feishuApp := cfg.Notifications.FeishuApp
	if feishuApp.AppID == "" || feishuApp.AppSecret == "" || feishuApp.ChatID == "" {
		return
	}
	ch := notify.NewFeishuAppChannel(
		feishuApp.AppID,
		feishuApp.AppSecret,
		feishuApp.ChatID,
		cfg.Notifications.Enabled,
	)
	reg.Remove("feishu_app")
	if err := reg.Register(ch); err != nil {
		logger.Warnf("Failed to register feishu app channel: %v", err)
		return
	}
	reg.Pin("feishu_app") // 不受 Reload 影响
	logger.Infof("Feishu app channel registered (chat_id=%s)", feishuApp.ChatID)
}

// reloadNotifyChannelConfigs reloads channel configurations from config into the registry.
func reloadNotifyChannelConfigs(reg *notify.Registry, cfg *config.Config) {
	var chanCfgs []notify.ChannelConfig
	for _, cc := range cfg.Notifications.Channels {
		chanCfgs = append(chanCfgs, notify.ChannelConfig{
			ID:                       cc.ID,
			Type:                     cc.Type,
			Enabled:                  cc.Enabled,
			WebhookURL:               cc.WebhookURL,
			Secret:                   cc.Secret,
			AppID:                    cc.AppID,
			AppSecret:                cc.AppSecret,
			ChatID:                   cc.ChatID,
			Headers:                  cc.Headers,
			AllowPrivateIP:           cc.AllowPrivateIP,
			WeComMsgType:             cc.WeComMsgType,
			WeComMentionedList:       cc.WeComMentionedList,
			WeComMentionedMobileList: cc.WeComMentionedMobileList,
		})
	}
	reg.Reload(chanCfgs)
}

// initScreenshotMode initializes one unified router for every screenshot mode.
func initScreenshotMode(srv *Server, cfg *config.Config, screenshotProvider screenshot.Provider,
	screenshotMgr *screenshot.Manager, screenshotApp *service.ScreenshotAppService,
	shutdownCtx context.Context) {

	screenshotMode := "cdp"
	if cfg != nil {
		screenshotMode = strings.ToLower(strings.TrimSpace(cfg.Screenshot.Mode))
		if screenshotMode == "" {
			screenshotMode = "cdp"
		}
	}

	initScreenshotRouter(srv, cfg, screenshotProvider, screenshotMgr, screenshotApp, shutdownCtx)
	if srv.screenshotRouter != nil {
		srv.screenshotRouter.SetMode(screenshot.ScreenshotMode(screenshotMode))
		screenshotApp.SetProvider(srv.screenshotRouter)
		screenshotApp.SetEngine("auto")
	}
}

// bridgeServiceResult holds the mock client and bridge service pair.
type bridgeServiceResult struct {
	mock    *bridgeMockClient
	service *screenshot.BridgeService
}

// createBridgeService creates a bridge service with mock client from config.
func createBridgeService(cfg *config.Config, shutdownCtx context.Context) bridgeServiceResult {
	extMaxConcurrency := 5
	extTaskTimeout := 30
	if cfg != nil {
		extMaxConcurrency = cfg.Screenshot.Extension.MaxConcurrency
		extTaskTimeout = cfg.Screenshot.Extension.TaskTimeoutSeconds
	}
	mockClient := newBridgeMockClient()
	bridgeSvc := screenshot.NewBridgeService(mockClient, extMaxConcurrency, time.Duration(extTaskTimeout)*time.Second)
	bridgeSvc.Start(shutdownCtx)
	return bridgeServiceResult{mock: mockClient, service: bridgeSvc}
}

// initScreenshotRouter sets up the dual-mode (auto) screenshot router with CDP and extension.
func initScreenshotRouter(srv *Server, cfg *config.Config, screenshotProvider screenshot.Provider,
	screenshotMgr *screenshot.Manager, screenshotApp *service.ScreenshotAppService,
	shutdownCtx context.Context) {

	cdpProvider := screenshotProvider
	if cdpProvider == nil && screenshotMgr != nil {
		cdpProvider = screenshot.NewCDPProvider(screenshotMgr)
	}

	bsr := createBridgeService(cfg, shutdownCtx)
	srv.bridge.setService(bsr.mock, bsr.service)
	screenshotApp.SetBridgeService(bsr.service)

	fallback := true
	priority := screenshot.ScreenshotMode("cdp")
	mode := screenshot.ModeCDP
	if cfg != nil {
		if cfg.Screenshot.Fallback != nil {
			fallback = *cfg.Screenshot.Fallback
		}
		priority = screenshot.ScreenshotMode(strings.ToLower(strings.TrimSpace(cfg.Screenshot.Priority)))
		if priority == "" {
			priority = screenshot.ModeCDP
		}
		mode = screenshot.ScreenshotMode(strings.ToLower(strings.TrimSpace(cfg.Screenshot.Mode)))
		if mode == "" {
			mode = screenshot.ModeCDP
		}
	}

	routerCfg := screenshot.RouterConfig{
		Mode:          mode,
		Priority:      priority,
		Fallback:      fallback,
		ProbeInterval: 30 * time.Second,
		ProbeTimeout:  5 * time.Second,
	}

	router := screenshot.NewScreenshotRouter(routerCfg, cdpProvider, bsr.service, screenshotMgr)
	router.SetMetricsHooks(func(from, to screenshot.ScreenshotMode) {
		metrics.IncScreenshotModeSwitch(string(from), string(to))
	}, func(mode string, healthy bool) {
		metrics.IncScreenshotHealthCheck(mode, healthy)
	})
	setExtensionHealthSignals(router, srv.bridge)

	router.Start(shutdownCtx)
	srv.screenshotRouter = router

	logger.Infof("Screenshot router initialized: mode=%s, priority=%s, fallback=%v", mode, priority, fallback)
}

// setExtensionHealthSignals injects extension health signal callbacks into the router.
func setExtensionHealthSignals(router *screenshot.ScreenshotRouter, bridge *BridgeState) {
	const recentActivityCutoff = 5 * time.Minute
	router.SetExtensionHealthSignals(
		func() bool {
			return activeBridgeLiveTokenCount(bridge) > 0
		},
		func() int64 {
			bridge.mu.Lock()
			defer bridge.mu.Unlock()
			pull := bridge.LastTaskPullAt
			cb := bridge.LastCallbackAt
			if pull > cb {
				return pull
			}
			return cb
		},
		recentActivityCutoff,
	)
}

// wireBrowserBackend connects the browser query backend and fallback config to the unified service.
func wireBrowserBackend(srv *Server, cfg *config.Config, unifiedSvc *service.UnifiedService) {
	if unifiedSvc == nil {
		return
	}
	if provider := srv.browserQueryProvider(); provider != nil {
		unifiedSvc.SetWebOnlyBrowserBackend(&browserBackendAdapter{provider: provider})
	}
	if cfg != nil && cfg.Query.BrowserFallback.Enabled {
		bfEngines := make(map[string]bool)
		for _, e := range cfg.Query.BrowserFallback.Engines {
			bfEngines[strings.ToLower(e)] = true
		}
		unifiedSvc.SetBrowserFallbackConfig(service.BrowserFallbackConfig{
			Enabled:       true,
			OnAPIError:    cfg.Query.BrowserFallback.OnAPIError,
			OnEmptyResult: cfg.Query.BrowserFallback.OnEmptyResult,
			Engines:       bfEngines,
		})
	}
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

	exePath, exeErr := os.Executable()
	if exeErr != nil {
		logger.Warnf("os.Executable failed, falling back to current directory: %v", exeErr)
	}
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

// icpConfigProvider returns a snapshot of the current ICP config for the scheduler runner.
func (s *Server) icpConfigProvider() adapter.ICPConfig {
	cfg := s.currentConfig()
	if cfg == nil {
		return adapter.ICPConfig{}
	}
	return adapter.ICPConfig{
		Enabled:     cfg.ICP.Enabled,
		BaseURL:     cfg.ICP.BaseURL,
		APIKey:      cfg.ICP.APIKey,
		Timeout:     cfg.ICP.Timeout,
		DefaultType: cfg.ICP.DefaultType,
	}
}

// Start 启动Web服务器
func (s *Server) Start() error {
	router := NewRouter(s)
	mux := router.RegisterRoutes()
	rateLimitEnabled, maxBodyBytes := s.configureServerLimits()
	allowedOrigins := allowedOriginsFromConfig(s.config)
	s.initExtensionIDs()
	handler := s.buildServerMiddlewareChain(mux, rateLimitEnabled, maxBodyBytes, allowedOrigins)
	rootHandler := s.buildServerRootHandler(handler)

	addr := fmt.Sprintf("%s:%d", s.bindAddr(), s.port)
	s.httpServer = &http.Server{Addr: addr, Handler: rootHandler, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second}
	go s.cleanupStaleQueries()
	go s.cleanupStaleBridgeTokens()
	go s.cleanupStaleBatchJobs()
	logger.Infof("Web server started at http://%s:%d", s.bindAddr(), s.port)
	logger.Infof("Registered %d routes", len(router.GetRoutes()))
	logger.Infof("Web security config loaded: cors_origins=%d rate_limit_enabled=%t max_body_bytes=%d", len(allowedOrigins), rateLimitEnabled, maxBodyBytes)
	if bindAddr := s.bindAddr(); bindAddr != "127.0.0.1" && bindAddr != "localhost" {
		logger.Warnf("⚠️  Server bound to %s (non-loopback). Ensure a reverse proxy with TLS terminates HTTPS in front of this server.", bindAddr)
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) configureServerLimits() (rateLimitEnabled bool, maxBodyBytes int64) {
	rateLimitEnabled = true
	if s.config != nil {
		rateLimitEnabled = s.config.Web.RateLimit.Enabled
	}
	SetRateLimitEnabled(rateLimitEnabled)
	if rateLimitEnabled && s.config != nil {
		SetRateLimitConfig(s.config.Web.RateLimit.RequestsPerWindow, time.Duration(s.config.Web.RateLimit.WindowSeconds)*time.Second)
		if err := SetTrustedProxyCIDRs(s.config.Web.RateLimit.TrustedProxyCIDRs); err != nil {
			logger.Warnf("invalid trusted proxy configuration: %v", err)
		}
	} else {
		_ = SetTrustedProxyCIDRs(nil)
	}
	maxBodyBytes = int64(10 * 1024 * 1024)
	if s.config != nil && s.config.Web.RequestLimits.MaxBodyBytes > 0 {
		maxBodyBytes = s.config.Web.RequestLimits.MaxBodyBytes
	}
	return
}

func (s *Server) initExtensionIDs() {
	extIDs := allowedExtensionIDsFromConfig(s.config)
	if len(extIDs) == 0 {
		if envVal := os.Getenv("UNIMAP_ALLOWED_EXTENSION_IDS"); envVal != "" {
			for _, part := range strings.Split(envVal, ",") {
				if part = strings.TrimSpace(part); part != "" {
					extIDs = append(extIDs, part)
				}
			}
		}
	}
	SetAllowedExtensionIDs(extIDs)
}

func (s *Server) buildServerMiddlewareChain(mux http.Handler, rateLimitEnabled bool, maxBodyBytes int64, allowedOrigins []string) http.Handler {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	headers := []string{"Content-Type", "Authorization", "X-Admin-Token", "X-Requested-With", "X-WebSocket-Token", "X-Bridge-Timestamp", "X-Bridge-Nonce", "X-Bridge-Signature", requestid.HeaderName}
	exposed := []string{requestid.HeaderName}
	creds, maxAge := true, 600
	if s.config != nil {
		if len(s.config.Web.CORS.AllowedMethods) > 0 {
			methods = s.config.Web.CORS.AllowedMethods
		}
		if len(s.config.Web.CORS.AllowedHeaders) > 0 {
			headers = s.config.Web.CORS.AllowedHeaders
		}
		if len(s.config.Web.CORS.ExposedHeaders) > 0 {
			exposed = s.config.Web.CORS.ExposedHeaders
		}
		creds = s.config.Web.CORS.AllowCredentials
		if s.config.Web.CORS.MaxAge > 0 {
			maxAge = s.config.Web.CORS.MaxAge
		}
	}
	handler := securityMiddleware(mux)
	handler = requestIDMiddleware(handler)
	handler = requestSizeLimitMiddleware(maxBodyBytes)(handler)
	handler = corsMiddleware(allowedOrigins, methods, headers, exposed, creds, maxAge)(handler)
	if s.config != nil && s.config.Web.Auth.Enabled {
		handler = s.adminAuthMiddleware()(handler)
		logger.Infof("Web auth enabled: admin token authentication active")
	} else if bindAddr := s.bindAddr(); bindAddr != "127.0.0.1" && bindAddr != "localhost" {
		logger.Fatalf("FATAL: Admin auth is DISABLED and server is bound to %s (non-loopback). Refusing to start to prevent unauthenticated access. Set web.auth.enabled=true or bind to loopback.", bindAddr)
	}
	handler = metricsMiddleware(handler)
	handler = auditMiddleware(handler)
	return handler
}

func (s *Server) buildServerRootHandler(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isWS := strings.Contains(r.Header.Get("Connection"), "Upgrade") && strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		if isWS && r.URL.Path == "/api/v1/ws" {
			if s.adminToken() != "" && !s.isPublicPath(r.URL.Path) {
				token := s.getSessionToken(r)
				if token == "" {
					token = r.Header.Get("X-Admin-Token")
				}
				if token == "" {
					token = extractBearerToken(r.Header.Get("Authorization"))
				}
				if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.adminToken())) != 1 {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized: valid admin token required"})
					return
				}
			}
			s.handleWebSocket(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	}
}

func allowedOriginsFromConfig(cfg *config.Config) []string {
	if cfg == nil || len(cfg.Web.CORS.AllowedOrigins) == 0 {
		port := 8448
		if cfg != nil && cfg.Web.Port != 0 {
			port = cfg.Web.Port
		}
		return []string{fmt.Sprintf("http://localhost:%d", port), fmt.Sprintf("http://127.0.0.1:%d", port)}
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
		port := 8448
		if cfg != nil && cfg.Web.Port != 0 {
			port = cfg.Web.Port
		}
		return []string{fmt.Sprintf("http://localhost:%d", port), fmt.Sprintf("http://127.0.0.1:%d", port)}
	}
	return origins
}

func allowedExtensionIDsFromConfig(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	ids := make([]string, 0, len(cfg.Web.CORS.AllowedExtensionIDs))
	for _, id := range cfg.Web.CORS.AllowedExtensionIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// Shutdown 优雅关闭Web服务器
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	logger.Info("Shutting down web server...")

	if s.shutdownCancel != nil {
		s.shutdownCancel()
	}

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("web server shutdown error: %w", err)
	}

	s.shutdownBackgroundServices()
	s.shutdownDatabases()
	s.shutdownChromeProcess()
	s.shutdownWebSocketConnections()

	logger.Info("Web server shutdown completed")
	return nil
}

// shutdownBackgroundServices stops distributed components, scheduler, router, and revocation store.
func (s *Server) shutdownBackgroundServices() {
	if s.distributed != nil && s.distributed.NodeRegistry != nil {
		s.distributed.NodeRegistry.Stop()
	}
	if s.distributed != nil && s.distributed.NodeTaskQueue != nil {
		s.distributed.NodeTaskQueue.Stop()
	}
	if s.scheduler != nil {
		s.scheduler.Stop()
	}
	if s.screenshotRouter != nil {
		s.screenshotRouter.Stop()
	}
	if s.revocationStore != nil {
		s.revocationStore.Stop()
	}
}

// shutdownDatabases closes ICP, history, and user databases.
func (s *Server) shutdownDatabases() {
	if s.tamperApp != nil {
		if err := s.tamperApp.Close(); err != nil {
			logger.Warnf("tamper history DB close error: %v", err)
		}
	}
	if s.icpDB != nil {
		if err := s.icpDB.Close(); err != nil {
			logger.Warnf("ICP result DB close error: %v", err)
		}
	}
	if s.historyDB != nil {
		if err := s.historyDB.Close(); err != nil {
			logger.Warnf("history DB close error: %v", err)
		}
	}
	if s.userDB != nil {
		if err := s.userDB.Close(); err != nil {
			logger.Warnf("user DB close error: %v", err)
		}
	}
	if s.batchDB != nil {
		if err := s.batchDB.Close(); err != nil {
			logger.Warnf("screenshot batch DB close error: %v", err)
		}
	}
}

// shutdownChromeProcess kills the Chrome process if running.
func (s *Server) shutdownChromeProcess() {
	s.chromeCmdMu.Lock()
	defer s.chromeCmdMu.Unlock()
	if s.chromeCmd == nil {
		return
	}
	logger.Info("Shutting down Chrome process...")
	if err := s.chromeCmd.Kill(); err != nil {
		logger.Warnf("Failed to kill Chrome process: %v", err)
	} else if _, err := s.chromeCmd.Wait(); err != nil {
		logger.Warnf("Failed to wait for Chrome process: %v", err)
	}
	s.chromeCmd = nil
}

// shutdownWebSocketConnections closes all active WebSocket connections.
func (s *Server) shutdownWebSocketConnections() {
	s.connManager.mutex.Lock()
	defer s.connManager.mutex.Unlock()
	for id, managed := range s.connManager.connections {
		if err := managed.conn.Close(); err != nil {
			logger.Warnf("Failed to close WebSocket connection %s: %v", id, err)
		}
	}
	s.connManager.connections = make(map[string]*managedConn)
}

// bindAddr returns the configured bind address, defaulting to "127.0.0.1".
func (s *Server) bindAddr() string {
	if s.config != nil && s.config.Web.BindAddress != "" {
		return s.config.Web.BindAddress
	}
	return "127.0.0.1"
}

func (s *Server) cleanupStaleQueries() {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			logger.Errorf("panic in cleanupStaleQueries: %v\n%s", r, buf[:n])
		}
	}()
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.queryMutex.Lock()
			now := time.Now()
			maxAge := 1 * time.Hour
			for id, status := range s.queryStatus {
				if now.Sub(status.StartTime) > maxAge {
					delete(s.queryStatus, id)
				}
			}
			s.queryMutex.Unlock()
		case <-s.shutdownCtx.Done():
			return
		}
	}
}

func (s *Server) cleanupStaleBridgeTokens() {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			logger.Errorf("panic in cleanupStaleBridgeTokens: %v\n%s", r, buf[:n])
		}
	}()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.bridge.mu.Lock()
			now := time.Now().Unix()
			if s.bridge.Tokens != nil {
				for token, exp := range s.bridge.Tokens {
					if exp <= now {
						delete(s.bridge.Tokens, token)
						delete(s.bridge.LastSeen, token)
					}
				}
			}
			if s.bridge.CallbackNonces != nil {
				for key, exp := range s.bridge.CallbackNonces {
					if exp <= now {
						delete(s.bridge.CallbackNonces, key)
					}
				}
			}
			s.bridge.mu.Unlock()
		case <-s.shutdownCtx.Done():
			return
		}
	}
}

func (s *Server) cleanupStaleBatchJobs() {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			logger.Errorf("panic in cleanupStaleBatchJobs: %v\n%s", r, buf[:n])
		}
	}()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.batchJobs != nil {
				s.batchJobs.cleanup(1 * time.Hour)
			}
		case <-s.shutdownCtx.Done():
			return
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	health := map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	}

	_ = json.NewEncoder(w).Encode(health)
}
