package web

import (
	"net/http"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

func (s *Server) renderTemplate(w http.ResponseWriter, status int, name string, data interface{}) bool {
	if s == nil || s.templates == nil {
		http.Error(w, http.StatusText(status), status)
		return false
	}

	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		logger.Errorf("failed to render template %s: %v", name, err)
		http.Error(w, http.StatusText(status), status)
		return false
	}

	return true
}
