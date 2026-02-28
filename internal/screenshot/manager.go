package screenshot

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
	"github.com/unimap-icp-hunter/project/internal/logger"
)

// ScreenshotType 截图类型
type ScreenshotType string

const (
	// ScreenshotTypeSearchEngine 搜索引擎结果页面截图
	ScreenshotTypeSearchEngine ScreenshotType = "search-engine-results"
	// ScreenshotTypeTargetWebsite 目标网站截图
	ScreenshotTypeTargetWebsite ScreenshotType = "target-websites"
)

// EngineWebURL 搜索引擎Web界面URL模板
type EngineWebURL struct {
	Name      string
	ResultURL string // 搜索结果页面URL模板
}

// Manager 截图管理器
type Manager struct {
	baseDir        string
	chromePath     string
	cookies        map[string][]Cookie // 各引擎的Cookie
	cookiesMutex   sync.RWMutex
	timeout        time.Duration
	windowWidth    int
	windowHeight   int
	waitTime       time.Duration // 页面加载后等待时间
}

// Cookie Cookie信息
type Cookie struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	HTTPOnly bool
	Secure   bool
}

// Config 截图管理器配置
type Config struct {
	BaseDir     string
	ChromePath  string
	Timeout     time.Duration
	WindowWidth int
	WindowHeight int
	WaitTime    time.Duration
}

// NewManager 创建截图管理器
func NewManager(cfg Config) *Manager {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.WindowWidth == 0 {
		cfg.WindowWidth = 1365
	}
	if cfg.WindowHeight == 0 {
		cfg.WindowHeight = 768
	}
	if cfg.WaitTime == 0 {
		cfg.WaitTime = 500 * time.Millisecond
	}

	return &Manager{
		baseDir:      cfg.BaseDir,
		chromePath:   cfg.ChromePath,
		cookies:      make(map[string][]Cookie),
		timeout:      cfg.Timeout,
		windowWidth:  cfg.WindowWidth,
		windowHeight: cfg.WindowHeight,
		waitTime:     cfg.WaitTime,
	}
}

// SetCookies 设置指定引擎的Cookie
func (m *Manager) SetCookies(engine string, cookies []Cookie) {
	m.cookiesMutex.Lock()
	defer m.cookiesMutex.Unlock()
	m.cookies[strings.ToLower(engine)] = cookies
}

// GetCookies 获取指定引擎的Cookie
func (m *Manager) GetCookies(engine string) []Cookie {
	m.cookiesMutex.RLock()
	defer m.cookiesMutex.RUnlock()
	return m.cookies[strings.ToLower(engine)]
}

// CreateQueryDirectory 创建查询目录结构
// 返回: 查询目录路径, 搜索引擎截图目录, 目标网站截图目录, 错误
func (m *Manager) CreateQueryDirectory(queryID string) (string, string, string, error) {
	// 生成目录名: YYYY-MM-DD-{queryID}
	dateStr := time.Now().Format("2006-01-02")
	dirName := fmt.Sprintf("%s-%s", dateStr, queryID)
	
	queryDir := filepath.Join(m.baseDir, dirName)
	searchEngineDir := filepath.Join(queryDir, string(ScreenshotTypeSearchEngine))
	targetWebsiteDir := filepath.Join(queryDir, string(ScreenshotTypeTargetWebsite))

	// 创建目录
	dirs := []string{queryDir, searchEngineDir, targetWebsiteDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", "", "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return queryDir, searchEngineDir, targetWebsiteDir, nil
}

// CaptureScreenshot 截图指定URL
func (m *Manager) CaptureScreenshot(ctx context.Context, targetURL string, cookies []Cookie) ([]byte, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.WindowSize(m.windowWidth, m.windowHeight),
	)

	// 使用指定的Chrome路径
	if m.chromePath != "" {
		opts = append(opts, chromedp.ExecPath(m.chromePath))
	}

	// 从环境变量获取Chrome路径
	if chromePath := os.Getenv("UNIMAP_CHROME_PATH"); chromePath != "" && m.chromePath == "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// 设置超时
	ctx, cancel = context.WithTimeout(ctx, m.timeout)
	defer cancel()

	var buf []byte
	
	// 构建ChromeDP动作列表
	actions := []chromedp.Action{
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(m.waitTime),
	}

	// 如果有Cookie，先设置Cookie
	if len(cookies) > 0 {
		// 需要先导航到目标域名才能设置Cookie
		actions = append([]chromedp.Action{
			chromedp.Navigate(targetURL),
			chromedp.ActionFunc(func(ctx context.Context) error {
				for _, cookie := range cookies {
					err := network.SetCookie(cookie.Name, cookie.Value).
						WithDomain(cookie.Domain).
						WithPath(cookie.Path).
						WithHTTPOnly(cookie.HTTPOnly).
						WithSecure(cookie.Secure).
						Do(ctx)
					if err != nil {
						logger.Warnf("Failed to set cookie %s: %v", cookie.Name, err)
					}
				}
				return nil
			}),
		}, actions[1:]...) // 替换第一个Navigate
	}

	// 添加截图动作
	actions = append(actions, chromedp.CaptureScreenshot(&buf))

	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}

	return buf, nil
}

// CaptureSearchEngineResult 截图搜索引擎结果页面
func (m *Manager) CaptureSearchEngineResult(ctx context.Context, engine, query string, queryID string) (string, error) {
	// 构建搜索引擎结果页面URL
	searchURL := m.BuildSearchEngineURL(engine, query)
	if searchURL == "" {
		return "", fmt.Errorf("unsupported engine: %s", engine)
	}

	// 创建目录
	_, searchEngineDir, _, err := m.CreateQueryDirectory(queryID)
	if err != nil {
		return "", err
	}

	// 生成文件名
	filename := m.generateSearchEngineFilename(engine, query)
	filepath := filepath.Join(searchEngineDir, filename)

	// 获取该引擎的Cookie
	cookies := m.GetCookies(engine)

	// 截图
	buf, err := m.CaptureScreenshot(ctx, searchURL, cookies)
	if err != nil {
		return "", fmt.Errorf("failed to capture %s result page: %w", engine, err)
	}

	// 保存文件
	if err := os.WriteFile(filepath, buf, 0644); err != nil {
		return "", fmt.Errorf("failed to save screenshot: %w", err)
	}

	logger.Infof("Captured %s result page: %s", engine, filepath)
	return filepath, nil
}

// CaptureTargetWebsite 截图目标网站
func (m *Manager) CaptureTargetWebsite(ctx context.Context, targetURL, ip, port, protocol, queryID string) (string, error) {
	// 构建目标URL
	if targetURL == "" {
		if ip == "" {
			return "", fmt.Errorf("target URL or IP is required")
		}
		proto := "http"
		if protocol != "" {
			proto = strings.ToLower(protocol)
		} else if port == "443" {
			proto = "https"
		}
		if port != "" && port != "80" && port != "443" {
			targetURL = fmt.Sprintf("%s://%s:%s", proto, ip, port)
		} else {
			targetURL = fmt.Sprintf("%s://%s", proto, ip)
		}
	}

	// 确保URL有scheme
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}

	// 创建目录
	_, _, targetWebsiteDir, err := m.CreateQueryDirectory(queryID)
	if err != nil {
		return "", err
	}

	// 生成文件名
	filename := m.generateTargetWebsiteFilename(ip, port, protocol)
	filepath := filepath.Join(targetWebsiteDir, filename)

	// 截图（目标网站不需要Cookie）
	buf, err := m.CaptureScreenshot(ctx, targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to capture target website: %w", err)
	}

	// 保存文件
	if err := os.WriteFile(filepath, buf, 0644); err != nil {
		return "", fmt.Errorf("failed to save screenshot: %w", err)
	}

	logger.Infof("Captured target website: %s", filepath)
	return filepath, nil
}

// BuildSearchEngineURL 构建搜索引擎结果页面URL
func (m *Manager) BuildSearchEngineURL(engine, query string) string {
	// Base64编码查询语句
	b64Query := base64.StdEncoding.EncodeToString([]byte(query))
	
	switch strings.ToLower(engine) {
	case "fofa":
		return fmt.Sprintf("https://fofa.info/result?qbase64=%s", b64Query)
	case "hunter":
		return fmt.Sprintf("https://hunter.qianxin.com/list?searchValue=%s", b64Query)
	case "quake":
		return fmt.Sprintf("https://quake.360.cn/quake/#/searchResult?searchVal=%s", query)
	case "zoomeye":
		return fmt.Sprintf("https://www.zoomeye.org/searchResult?q=%s", query)
	default:
		return ""
	}
}

// generateSearchEngineFilename 生成搜索引擎截图文件名
func (m *Manager) generateSearchEngineFilename(engine, query string) string {
	// 清理查询语句，用于文件名
	cleanQuery := strings.ReplaceAll(query, " ", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, "/", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, "\\", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, ":", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, "*", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, "?", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, "\"", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, "<", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, ">", "_")
	cleanQuery = strings.ReplaceAll(cleanQuery, "|", "_")
	
	// 限制文件名长度
	if len(cleanQuery) > 50 {
		cleanQuery = cleanQuery[:50]
	}
	
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s_%s.png", engine, cleanQuery, timestamp)
}

// generateTargetWebsiteFilename 生成目标网站截图文件名
func (m *Manager) generateTargetWebsiteFilename(ip, port, protocol string) string {
	proto := protocol
	if proto == "" {
		proto = "http"
	}
	
	if port == "" {
		return fmt.Sprintf("%s_%s.png", proto, ip)
	}
	return fmt.Sprintf("%s_%s_%s.png", proto, ip, port)
}

// GetScreenshotDirectory 获取截图根目录
func (m *Manager) GetScreenshotDirectory() string {
	return m.baseDir
}

// SetChromePath 设置Chrome路径
func (m *Manager) SetChromePath(path string) {
	m.chromePath = path
}
