package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// handleCDPStatus 检测CDP端口状态
func (s *Server) handleCDPStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
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
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Debugf("failed to encode CDP status response: %v", err)
	}
}

// handleCDPConnect 启动Chrome并连接CDP
func (s *Server) handleCDPConnect(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireTrustedRequest(w, r, allowedOriginsFromConfig(s.config)) {
		return
	}

	// Allow persisting proxy/cookie parameters submitted from the UI
	// before attempting to start or reuse CDP.
	s.applyCookiesFromRequest(r)

	baseURL := s.resolveCDPURL()
	ctx := r.Context()
	if ok, info, _ := s.checkCDPStatus(ctx, baseURL); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"online":  true,
			"url":     baseURL,
			"version": info,
			"message": "CDP already online",
		}); err != nil {
			logger.Debugf("failed to encode CDP connect response: %v", err)
		}
		return
	}

	if err := s.startCDPChrome(baseURL); err != nil {
		_, checked := s.resolveChromePathWithDiagnostics()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"success":             false,
			"error":               err.Error(),
			"chrome_path_checked": checked,
		}); err != nil {
			logger.Debugf("failed to encode CDP connect error response: %v", err)
		}
		return
	}

	online, info, err := s.waitForCDP(ctx, baseURL, 5*time.Second)
	if online {
		s.updateCDPConfig(baseURL)
	}

	w.Header().Set("Content-Type", "application/json")
	if online {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"online":  true,
			"url":     baseURL,
			"version": info,
			"message": "CDP connected",
		}); err != nil {
			logger.Debugf("failed to encode CDP connect success response: %v", err)
		}
		return
	}

	msg := "CDP not available"
	if err != nil {
		msg = err.Error()
	}
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"online":  false,
		"url":     baseURL,
		"error":   msg,
	}); err != nil {
		logger.Debugf("failed to encode CDP not available response: %v", err)
	}
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

	chromePath, checked := s.resolveChromePathWithDiagnostics()
	if chromePath == "" {
		if len(checked) > 0 {
			return fmt.Errorf("chrome path not configured; set screenshot.chrome_path or UNIMAP_CHROME_PATH (checked: %s)", strings.Join(checked, ", "))
		}
		return fmt.Errorf("chrome path not configured; set screenshot.chrome_path or UNIMAP_CHROME_PATH")
	}

	port := resolveCDPPort(baseURL)
	debugAddr := "127.0.0.1"
	if s.config != nil {
		if addr := strings.TrimSpace(s.config.Screenshot.ChromeRemoteDebugAddress); addr != "" {
			debugAddr = addr
		}
	}
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		fmt.Sprintf("--remote-debugging-address=%s", debugAddr),
		"--no-first-run",
		"--no-default-browser-check",
	}

	userDataConfigured := false

	if s.config != nil {
		if dir := strings.TrimSpace(s.config.Screenshot.ChromeUserDataDir); dir != "" {
			args = append(args, "--user-data-dir="+dir)
			userDataConfigured = true
		}
		if profile := strings.TrimSpace(s.config.Screenshot.ChromeProfileDir); profile != "" {
			args = append(args, "--profile-directory="+profile)
		}
		if proxy := strings.TrimSpace(s.config.Screenshot.ProxyServer); proxy != "" {
			args = append(args, "--proxy-server="+proxy)
		} else if proxyEnv := strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PROXY_SERVER")); proxyEnv != "" {
			args = append(args, "--proxy-server="+proxyEnv)
		}
		if s.config.Screenshot.Headless != nil && *s.config.Screenshot.Headless {
			args = append(args, "--headless=new")
		}
	}

	// 若未配置用户目录，使用独立目录启动，避免参数被已运行的浏览器实例吞掉。
	if !userDataConfigured {
		fallbackUserDataDir := filepath.Join(os.TempDir(), "unimap-cdp-profile")
		if err := os.MkdirAll(fallbackUserDataDir, 0755); err == nil {
			args = append(args, "--user-data-dir="+fallbackUserDataDir)
		}
	}

	cmd := exec.Command(chromePath, args...)
	if err := cmd.Start(); err != nil {
		return err
	}

	// Start a goroutine to wait for the process to prevent zombie processes
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.Debugf("Chrome process exited: %v", err)
		}
	}()

	s.chromeCmd = cmd.Process
	return nil
}

func (s *Server) resolveChromePath() string {
	path, _ := s.resolveChromePathWithDiagnostics()
	return path
}

func (s *Server) resolveChromePathWithDiagnostics() (string, []string) {
	checked := make([]string, 0, 16)

	if s.config != nil {
		if raw := strings.TrimSpace(s.config.Screenshot.ChromePath); raw != "" {
			checked = append(checked, "config:screenshot.chrome_path")
			return raw, checked
		}
		checked = append(checked, "config:screenshot.chrome_path(empty)")
	}
	if env := strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PATH")); env != "" {
		checked = append(checked, "env:UNIMAP_CHROME_PATH")
		return env, checked
	}
	checked = append(checked, "env:UNIMAP_CHROME_PATH(empty)")

	if runtime.GOOS == "windows" {
		for _, regPath := range []string{
			`HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\App Paths\\chrome.exe`,
			`HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\App Paths\\chrome.exe`,
			`HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\App Paths\\msedge.exe`,
			`HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\App Paths\\msedge.exe`,
		} {
			p := strings.TrimSpace(queryWindowsAppPath(regPath))
			if p == "" {
				checked = append(checked, "registry:"+regPath+"(empty)")
				continue
			}
			checked = append(checked, "registry:"+regPath)
			if _, err := os.Stat(p); err == nil {
				return p, checked
			}
			checked = append(checked, "registry-target-missing:"+p)
		}

		windowsCandidates := []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		}
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			windowsCandidates = append(windowsCandidates,
				filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(localAppData, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
		for _, p := range windowsCandidates {
			checked = append(checked, "path:"+p)
			if _, err := os.Stat(p); err == nil {
				return p, checked
			}
		}
	}

	if runtime.GOOS == "darwin" {
		for _, p := range []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		} {
			checked = append(checked, "path:"+p)
			if _, err := os.Stat(p); err == nil {
				return p, checked
			}
		}
		if homeDir, err := os.UserHomeDir(); err == nil {
			userChrome := filepath.Join(homeDir, "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome")
			checked = append(checked, "path:"+userChrome)
			if _, err := os.Stat(userChrome); err == nil {
				return userChrome, checked
			}
		}
	}

	if runtime.GOOS == "linux" {
		for _, p := range []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/google-chrome-beta",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/usr/bin/microsoft-edge",
			"/usr/bin/microsoft-edge-stable",
			"/opt/google/chrome/chrome",
		} {
			checked = append(checked, "path:"+p)
			if _, err := os.Stat(p); err == nil {
				return p, checked
			}
		}
	}

	for _, name := range []string{"chrome", "chrome.exe", "msedge", "msedge.exe", "chromium", "chromium.exe"} {
		checked = append(checked, "lookpath:"+name)
		if path, err := exec.LookPath(name); err == nil {
			return path, checked
		}
	}
	return "", checked
}

func queryWindowsAppPath(regPath string) string {
	cmd := exec.Command("reg", "query", regPath, "/ve")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "REG_SZ") {
			continue
		}
		parts := strings.SplitN(line, "REG_SZ", 2)
		if len(parts) != 2 {
			continue
		}
		return strings.TrimSpace(parts[1])
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
