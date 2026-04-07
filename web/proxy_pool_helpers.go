package web

import "strings"

func (s *Server) selectRequestProxy() string {
	if s == nil || s.proxyPool == nil {
		return ""
	}
	proxy, useProxy := s.proxyPool.Select()
	if !useProxy {
		return ""
	}
	return strings.TrimSpace(proxy)
}

func (s *Server) reportRequestProxy(proxy string, success bool) {
	if s == nil || s.proxyPool == nil || strings.TrimSpace(proxy) == "" {
		return
	}
	s.proxyPool.Report(strings.TrimSpace(proxy), success)
}
