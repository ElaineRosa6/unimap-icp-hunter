package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/logger"
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

	if s.screenshotMgr == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_manager_unavailable", "screenshot manager not initialized", nil)
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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireTrustedRequest(w, r, allowedOriginsFromConfig(s.config)) {
		return
	}

	s.applyCookiesFromRequest(r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (s *Server) applyCookiesFromRequest(r *http.Request) {
	if s.config == nil {
		return
	}
	_ = r.ParseForm()

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
