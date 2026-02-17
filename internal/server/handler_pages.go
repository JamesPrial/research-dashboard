package server

import (
	"io/fs"
	"net/http"
)

// handleDashboard reads dashboard.html from the embedded static FS and serves it.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.staticFS, "dashboard.html")
	if err != nil {
		http.Error(w, "dashboard not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// handleReader reads reader.html from the embedded static FS and serves it.
func (s *Server) handleReader(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.staticFS, "reader.html")
	if err != nil {
		http.Error(w, "reader not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
