package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/screenshot"
)

// handleImportCookieJSON 导入浏览器导出的Cookie JSON
func (s *Server) handleImportCookieJSON(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireTrustedRequest(w, r, allowedOriginsFromConfig(s.config)) {
		return
	}
	if s.config == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "config_not_loaded", "config not loaded", nil)
		return
	}
	if s.currentScreenshotEngine() == "extension" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":      true,
			"cookieHeader": "",
			"engine":       "extension",
			"message":      "extension mode uses browser session; cookie import is optional",
		})
		return
	}

	engine := strings.TrimSpace(r.FormValue("engine"))
	jsonStr := r.FormValue("cookie_json")
	if engine == "" || strings.TrimSpace(jsonStr) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "engine and cookie_json are required", nil)
		return
	}

	cookies, err := config.ParseCookieJSON(jsonStr, config.DefaultCookieDomain(engine))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_cookie_json", "invalid cookie json", err.Error())
		return
	}
	if len(cookies) == 0 {
		writeAPIError(w, http.StatusBadRequest, "empty_cookie_set", "no cookies parsed", nil)
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
		writeAPIError(w, http.StatusBadRequest, "unsupported_engine", "unsupported engine", map[string]string{"engine": engine})
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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if err := validateQueryInput(query); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
		return
	}

	s.applyCookiesFromRequest(r)

	engines := parseEnginesParam(r)
	if len(engines) == 0 {
		engines = s.orchestrator.ListAdapters()
	}
	if len(engines) == 0 {
		writeAPIError(w, http.StatusServiceUnavailable, "no_engines_available", "no engines configured or registered", nil)
		return
	}

	ctx := r.Context()
	results := make(map[string]interface{})
	engineMode := s.currentScreenshotEngine()
	for _, engine := range engines {
		ok, title, hint, err := s.verifyEngineSession(ctx, engineMode, engine, query)
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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireTrustedRequest(w, r, allowedOriginsFromConfig(s.config)) {
		return
	}

	s.applyCookiesFromRequest(r)
	engineMode := s.currentScreenshotEngine()
	if engineMode == "extension" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"engine":  "extension",
			"message": "extension mode uses browser session; cookie injection is skipped",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"engine":  engineMode,
	})
}

func (s *Server) applyCookiesFromRequest(r *http.Request) {
	if s.config == nil {
		return
	}
	_ = r.ParseForm()

	s.configMutex.Lock()
	defer s.configMutex.Unlock()

	engineMode := s.currentScreenshotEngine()

	if engineMode == "extension" {
		changed := false
		if _, present := r.Form["proxy_server"]; present {
			proxy := strings.TrimSpace(r.FormValue("proxy_server"))
			if s.config.Screenshot.ProxyServer != proxy {
				s.config.Screenshot.ProxyServer = proxy
				changed = true
				if s.screenshotMgr != nil {
					s.screenshotMgr.SetProxyServer(proxy)
				}
			}
		}

		if changed && s.configManager != nil {
			if err := s.configManager.Save(); err != nil {
				logger.Warnf("Failed to persist extension proxy config: %v", err)
			}
		}
		logger.Infof("Cookie apply mode=extension_session: skipped cookie injection, proxy update only")
		return
	}

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

	if _, present := r.Form["proxy_server"]; present {
		proxy := strings.TrimSpace(r.FormValue("proxy_server"))
		if s.config.Screenshot.ProxyServer != proxy {
			s.config.Screenshot.ProxyServer = proxy
			changed = true
			if s.screenshotMgr != nil {
				s.screenshotMgr.SetProxyServer(proxy)
			}
		}
	}

	if changed && s.configManager != nil {
		if err := s.configManager.Save(); err != nil {
			logger.Warnf("Failed to persist cookies: %v", err)
		}
	}
	logger.Infof("Cookie apply mode=cdp_cookie_injection: cookie/proxy updates applied")
}

func (s *Server) currentScreenshotEngine() string {
	if s == nil || s.config == nil {
		return "cdp"
	}
	engine := strings.ToLower(strings.TrimSpace(s.config.Screenshot.Engine))
	if engine == "extension" {
		return "extension"
	}
	return "cdp"
}

func (s *Server) verifyEngineSession(ctx context.Context, engineMode, engine, query string) (bool, string, string, error) {
	if engineMode == "extension" {
		if s.bridge.Service == nil {
			return false, "", "extension_not_paired", fmt.Errorf("bridge_unavailable")
		}
		if s.screenshotMgr == nil {
			return false, "", "extension_session_required", fmt.Errorf("screenshot manager not initialized")
		}

		searchURL := strings.TrimSpace(s.screenshotMgr.BuildSearchEngineURL(engine, query))
		if searchURL == "" {
			return false, "", "unsupported engine", fmt.Errorf("unsupported engine: %s", engine)
		}

		result, err := s.bridge.Service.Submit(ctx, screenshot.BridgeTask{
			RequestID:    fmt.Sprintf("verify_%s_%d", strings.ToLower(strings.TrimSpace(engine)), time.Now().UnixNano()),
			URL:          searchURL,
			BatchID:      "cookie_verify",
			WaitStrategy: "load",
		})
		if err != nil {
			return false, "", "extension_session_required", err
		}
		if !result.Success {
			if strings.TrimSpace(result.Error) != "" {
				return false, "", "extension_session_required", fmt.Errorf("%s", result.Error)
			}
			if strings.TrimSpace(result.ErrorCode) != "" {
				return false, "", "extension_session_required", fmt.Errorf("%s", result.ErrorCode)
			}
			return false, "", "extension_session_required", fmt.Errorf("extension verification failed")
		}

		return true, "extension_session_ok", "ok", nil
	}

	if s.screenshotMgr == nil {
		return false, "", "cdp_cookie_missing", fmt.Errorf("screenshot manager not initialized")
	}
	cookies := s.screenshotMgr.GetCookies(engine)
	return s.screenshotMgr.ValidateSearchEngineResult(ctx, engine, query, cookies)
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

// handleCookieLoginStatus returns per-engine login status for the UI.
// GET /api/cookies/login-status?query=...
// Detects: CDP connected, Extension paired, per-engine login wall detection.
func (s *Server) handleCookieLoginStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		query = `protocol="http"`
	}

	// Detect CDP connection status
	cdpConnected := s.screenshotMgr != nil && s.screenshotMgr.RemoteDebugURL() != ""

	// Detect Extension pairing status (non-mock)
	extPaired := s.bridge != nil && s.bridge.Service != nil

	// Check per-engine login status
	engines := []string{"fofa", "hunter", "zoomeye", "quake"}
	results := make([]map[string]interface{}, 0, len(engines))

	if cdpConnected {
		// CDP connected → check each engine by opening its page in the same browser
		for _, engine := range engines {
			status, err := s.screenshotMgr.CheckEngineLoginStatus(r.Context(), engine, query)
			item := map[string]interface{}{
				"engine":     status.Engine,
				"logged_in":  status.LoggedIn,
				"reason":     status.Reason,
				"title":      status.Title,
				"login_url":  status.LoginURL,
				"cdp_connected": cdpConnected,
				"ext_paired": extPaired,
			}
			if status.Error != "" {
				item["error"] = status.Error
			}
			if err != nil {
				item["error"] = err.Error()
			}
			results = append(results, item)
		}
	} else if extPaired {
		// Extension paired → use bridge to open each engine page
		for _, engine := range engines {
			searchURL := ""
			if s.screenshotMgr != nil {
				searchURL = s.screenshotMgr.BuildSearchEngineURL(engine, query)
			}
			loginURL := ""
			if s.screenshotMgr != nil {
				loginURL = s.screenshotMgr.EngineLoginURL(engine)
			}

			item := map[string]interface{}{
				"engine":     engine,
				"cdp_connected": cdpConnected,
				"ext_paired": extPaired,
				"login_url":  loginURL,
			}

			if searchURL != "" {
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				result, err := s.bridge.Service.Submit(ctx, screenshot.BridgeTask{
					RequestID:    fmt.Sprintf("login_check_%s_%d", engine, time.Now().UnixNano()),
					URL:          searchURL,
					BatchID:      "login_status",
					WaitStrategy: "load",
				})
				cancel()

				if err != nil {
					item["logged_in"] = false
					item["reason"] = "bridge_failed"
					item["error"] = err.Error()
				} else if result.Success {
					item["logged_in"] = true
					item["reason"] = "browser_session"
					item["title"] = result.ImagePath
				} else {
					errMsg := strings.TrimSpace(result.Error)
					if errMsg == "" {
						errMsg = strings.TrimSpace(result.ErrorCode)
					}
					item["logged_in"] = false
					item["reason"] = "bridge_check_failed"
					item["error"] = errMsg
				}
			} else {
				item["logged_in"] = false
				item["reason"] = "unsupported_engine"
			}
			results = append(results, item)
		}
	} else {
		// No browser session → just check if cookies are configured
		if s.config != nil {
			s.configMutex.Lock()
			for _, engine := range engines {
				var cookieSet bool
				loginURL := ""
				if s.screenshotMgr != nil {
					loginURL = s.screenshotMgr.EngineLoginURL(engine)
				}
				switch engine {
				case "fofa":
					cookieSet = hasCookies(s.config.Engines.Fofa.Cookies)
				case "hunter":
					cookieSet = hasCookies(s.config.Engines.Hunter.Cookies)
				case "quake":
					cookieSet = hasCookies(s.config.Engines.Quake.Cookies)
				case "zoomeye":
					cookieSet = hasCookies(s.config.Engines.Zoomeye.Cookies)
				}
				reason := "no_session"
				if cookieSet {
					reason = "cookie_configured"
				}
				results = append(results, map[string]interface{}{
					"engine":     engine,
					"logged_in":  false,
					"reason":     reason,
					"login_url":  loginURL,
					"cdp_connected": cdpConnected,
					"ext_paired": extPaired,
				})
			}
			s.configMutex.Unlock()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"cdp_connected": cdpConnected,
		"ext_paired":    extPaired,
		"engines":       results,
	})
}
