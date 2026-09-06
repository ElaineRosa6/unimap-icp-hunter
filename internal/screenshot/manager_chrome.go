package screenshot

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/chromedp/chromedp"

	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/utils"
)

// engineStorageHosts maps a browser engine to the site host whose same-origin
// Web Storage (and cookies) the browser session must receive. Built once to
// avoid rebuilding the map on every page load.
var engineStorageHosts = map[string]string{
	"fofa": "fofa.info", "hunter": "hunter.qianxin.com", "zoomeye": "zoomeye.org",
	"quake": "quake.360.net", "shodan": "shodan.io", "censys": "censys.io", "daydaymap": "daydaymap.com",
}

// loadPageContent is a CDP-only helper used by cookie/session validation.
func (m *Manager) loadPageContent(ctx context.Context, targetURL string, cookies []Cookie, title *string, html *string) error {
	loadedTitle, loadedHTML, err := m.loadBrowserPage(ctx, targetURL, cookies, "")
	if title != nil {
		*title = loadedTitle
	}
	if html != nil {
		*html = loadedHTML
	}
	return err
}

// LoadPage renders a URL through the guarded browser path. It implements the
// page-loader seam consumed by browser-backed tamper detection.
func (m *Manager) LoadPage(ctx context.Context, targetURL string) (string, string, error) {
	return m.loadBrowserPage(ctx, targetURL, nil, "")
}

// LoadPageWithProxy is the request-proxy variant of LoadPage.
func (m *Manager) LoadPageWithProxy(ctx context.Context, targetURL, proxy string) (string, string, error) {
	return m.loadBrowserPage(ctx, targetURL, nil, proxy)
}

func (m *Manager) loadBrowserPage(ctx context.Context, targetURL string, cookies []Cookie, proxy string) (string, string, error) {
	session, err := m.newGuardedBrowserSession(ctx, targetURL, proxy)
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	var title, html string
	actions := []chromedp.Action{}

	// Apply configured or Bridge-synchronized cookies before the first
	// navigation in both local and attached-CDP modes. Attached Chrome may use
	// a separate profile and must not be assumed to share the UI session.
	if len(cookies) > 0 {
		actions = append(actions, setCookieActions(cookies, targetURL)...)
		actions = append(actions, chromedp.Navigate(targetURL))
	} else {
		actions = append(actions, chromedp.Navigate(targetURL))
	}

	actions = append(actions,
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(m.waitTime),
		chromedp.Title(&title),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	)

	if err := session.Run(actions...); err != nil {
		return title, html, err
	}
	if storage := m.browserStorageForURL(targetURL); len(storage.Local) > 0 || len(storage.Session) > 0 {
		if err := applyBrowserStorage(session, storage, targetURL); err != nil {
			return title, html, fmt.Errorf("apply browser storage: %w", err)
		}
		if err := session.Run(chromedp.Sleep(m.waitTime), chromedp.Title(&title), chromedp.OuterHTML("html", &html, chromedp.ByQuery)); err != nil {
			return title, html, err
		}
	}
	return title, html, nil
}

func (m *Manager) browserStorageForURL(rawURL string) BrowserStorage {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return BrowserStorage{}
	}
	host := strings.ToLower(parsed.Hostname())
	for engine, candidateHost := range engineStorageHosts {
		if host == candidateHost || strings.HasSuffix(host, "."+candidateHost) {
			return m.GetBrowserStorage(engine)
		}
	}
	return BrowserStorage{}
}

func (m *Manager) buildExecAllocatorOptions(proxyOverride string) []chromedp.ExecAllocatorOption {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", m.headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WindowSize(m.windowWidth, m.windowHeight),
	)
	if m.noSandbox {
		opts = append(opts, chromedp.Flag("no-sandbox", true), chromedp.Flag("disable-setuid-sandbox", true))
	}

	if m.userDataDir != "" {
		opts = append(opts, chromedp.UserDataDir(m.userDataDir))
	}
	if m.profileDir != "" {
		opts = append(opts, chromedp.Flag("profile-directory", m.profileDir))
	}

	proxyServer := strings.TrimSpace(proxyOverride)
	if proxyServer == "" {
		proxyServer = strings.TrimSpace(m.proxyServer)
	}
	if proxyServer == "" {
		proxyServer = strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PROXY_SERVER"))
	}
	if proxyServer != "" {
		opts = append(opts,
			chromedp.Flag("proxy-server", proxyServer),
			chromedp.Flag("proxy-bypass-list", "<-loopback>"),
		)
		logger.Infof("Chrome proxy enabled: %s", proxyServer)
	}

	chromePath, _ := ResolveChromePath(m.chromePath)
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	if userData := os.Getenv("UNIMAP_CHROME_USER_DATA_DIR"); userData != "" && m.userDataDir == "" {
		opts = append(opts, chromedp.UserDataDir(userData))
	}
	if profileDir := os.Getenv("UNIMAP_CHROME_PROFILE_DIR"); profileDir != "" && m.profileDir == "" {
		opts = append(opts, chromedp.Flag("profile-directory", profileDir))
	}

	return opts
}

// ResolveChromePath resolves the Chrome/Chromium executable used by the
// managed CDP runtime. An explicit configured path is authoritative: a typo
// must be reported instead of silently launching a different system browser.
func ResolveChromePath(configured string) (string, error) {
	if path := strings.TrimSpace(configured); path != "" {
		if info, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("configured chrome path %q is unavailable: %w", path, err)
		} else if info.IsDir() {
			return "", fmt.Errorf("configured chrome path %q is a directory", path)
		} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("configured chrome path %q is not executable", path)
		}
		return path, nil
	}

	if path := strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PATH")); path != "" {
		if info, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("UNIMAP_CHROME_PATH %q is unavailable: %w", path, err)
		} else if info.IsDir() {
			return "", fmt.Errorf("UNIMAP_CHROME_PATH %q is a directory", path)
		} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("UNIMAP_CHROME_PATH %q is not executable", path)
		}
		return path, nil
	}

	if path := findChromePath(); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("chrome not found; install Chrome/Chromium or configure screenshot.chrome_path or UNIMAP_CHROME_PATH")
}

// findChromePath 自动查找Chrome路径
func findChromePath() string {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates,
				filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(localAppData, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
		if homeDir, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates,
				filepath.Join(homeDir, "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
			)
		}
	case "linux":
		candidates = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/google-chrome-beta",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/usr/bin/microsoft-edge",
			"/usr/bin/microsoft-edge-stable",
			"/opt/google/chrome/chrome",
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			logger.Infof("Found Chrome at: %s", path)
			return path
		}
	}

	return ""
}

func (m *Manager) newAllocator(ctx context.Context) (context.Context, context.CancelFunc, error) {
	return m.newAllocatorWithProxy(ctx, "")
}

func (m *Manager) newAllocatorWithProxy(ctx context.Context, proxyOverride string) (context.Context, context.CancelFunc, error) {
	// 检查是否配置了远程调试URL
	remoteURL := strings.TrimSpace(m.remoteDebugURL)
	if remoteURL == "" {
		remoteURL = strings.TrimSpace(os.Getenv("UNIMAP_CHROME_REMOTE_DEBUG_URL"))
	}

	// 如果配置了远程调试URL，先尝试连接，失败则回退到本地启动
	if remoteURL != "" {
		// 测试远程调试端口是否可用
		if isRemoteDebuggerAvailable(remoteURL) {
			release, err := m.acquireBrowserSlot(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("wait for browser session: %w", err)
			}
			logger.Infof("Using remote Chrome debugger at: %s", remoteURL)
			allocCtx, cancel := chromedp.NewRemoteAllocator(ctx, remoteURL)
			return allocCtx, combineCancel(cancel, release), nil
		}
		logger.Warnf("Remote Chrome debugger not available at %s, falling back to local Chrome", remoteURL)
	}

	if err := m.validateLocalChromeDataDir(); err != nil {
		return nil, nil, err
	}
	chromePath, err := ResolveChromePath(m.chromePath)
	if err != nil {
		return nil, nil, err
	}
	release, err := m.acquireBrowserSlot(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("wait for browser session: %w", err)
	}
	opts := m.buildExecAllocatorOptions(proxyOverride)

	logger.Infof("Starting Chrome with options, chrome path: %s", chromePath)
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	return allocCtx, combineCancel(cancel, release), nil
}

func (m *Manager) acquireBrowserSlot(ctx context.Context) (context.CancelFunc, error) {
	if m == nil || m.sessionSlots == nil {
		return func() {}, nil
	}
	select {
	case m.sessionSlots <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-m.sessionSlots })
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func combineCancel(cancel, release context.CancelFunc) context.CancelFunc {
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			release()
		})
	}
}

// NewAllocator exposes the browser allocator so other browser-driven features
// can share the same Chrome/CDP bootstrap strategy as screenshots.
func (m *Manager) NewAllocator(ctx context.Context) (context.Context, context.CancelFunc, error) {
	return m.newAllocator(ctx)
}

// NewAllocatorWithProxy creates a browser allocator with request-level proxy override.
func (m *Manager) NewAllocatorWithProxy(ctx context.Context, proxy string) (context.Context, context.CancelFunc, error) {
	return m.newAllocatorWithProxy(ctx, proxy)
}

// isRemoteDebuggerAvailable 检查远程调试端口是否可用
func isRemoteDebuggerAvailable(remoteURL string) bool {
	client := utils.FastHTTPClient()
	resp, err := client.Get(remoteURL + "/json/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// validateLocalChromeDataDir catches the default-directory restriction before
// starting a browser. Chrome 136+ ignores CDP switches for this directory even
// when a different profile-directory is selected. Never migrate user sessions.
func (m *Manager) validateLocalChromeDataDir() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	dir := strings.TrimSpace(m.userDataDir)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("UNIMAP_CHROME_USER_DATA_DIR"))
	}
	if dir == "" {
		return nil
	} // chromedp allocates a fresh temporary directory.
	canonical := func(path string) string {
		path, err := filepath.Abs(path)
		if err != nil {
			return ""
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		return filepath.Clean(path)
	}
	candidates := []string{}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		candidates = append(candidates, filepath.Join(local, "Google", "Chrome", "User Data"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "AppData", "Local", "Google", "Chrome", "User Data"))
	}
	for _, candidate := range candidates {
		if strings.EqualFold(canonical(dir), canonical(candidate)) {
			return fmt.Errorf("Chrome CDP requires a non-default user data directory; selecting another profile in the default Chrome directory is insufficient")
		}
	}
	return nil
}
