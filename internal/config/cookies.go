package config

import (
	"encoding/json"
	"strings"
)

// DefaultCookieDomain returns a default cookie domain for the engine web UI.
func DefaultCookieDomain(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "fofa":
		return ".fofa.info"
	case "hunter":
		return ".hunter.qianxin.com"
	case "quake":
		return ".quake.360.cn"
	case "zoomeye":
		return ".zoomeye.org"
	default:
		return ""
	}
}

// ParseCookieHeader parses a Cookie header like "a=b; c=d" into config cookies.
func ParseCookieHeader(raw string, domain string) []Cookie {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ";")
	cookies := make([]Cookie, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		if name == "" {
			continue
		}
		cookies = append(cookies, Cookie{
			Name:     name,
			Value:    value,
			Domain:   domain,
			Path:     "/",
			HTTPOnly: false,
			Secure:   true,
		})
	}

	if len(cookies) == 0 {
		return nil
	}
	return cookies
}

// ParseCookieJSON parses a browser-exported cookie JSON array into config cookies.
func ParseCookieJSON(raw string, defaultDomain string) ([]Cookie, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var items []struct {
		Name     string `json:"name"`
		Value    string `json:"value"`
		Domain   string `json:"domain"`
		Path     string `json:"path"`
		HTTPOnly bool   `json:"httpOnly"`
		Secure   bool   `json:"secure"`
	}

	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}

	cookies := make([]Cookie, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		domain := strings.TrimSpace(item.Domain)
		if domain == "" {
			domain = defaultDomain
		}
		path := strings.TrimSpace(item.Path)
		if path == "" {
			path = "/"
		}
		cookies = append(cookies, Cookie{
			Name:     name,
			Value:    strings.TrimSpace(item.Value),
			Domain:   domain,
			Path:     path,
			HTTPOnly: item.HTTPOnly,
			Secure:   item.Secure,
		})
	}

	if len(cookies) == 0 {
		return nil, nil
	}
	return cookies, nil
}
