// Package server provides the HTTP server for the research dashboard.
// It wires together the job store, runner, and static file system into
// a set of registered routes served by a single http.ServeMux.
package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jamesprial/research-dashboard/internal/jobstore"
)

// maxJobAge is the duration after which completed, failed, or cancelled jobs
// are eligible for removal during a cleanup pass.
const maxJobAge = 24 * time.Hour

// pastRunPrefix is the URL prefix for past-run routes. These are handled
// outside the mux to avoid Go 1.22+ ServeMux ambiguity with the
// GET /research/{id}/files/{path...} wildcard pattern.
const pastRunPrefix = "/research/past/"

// JobRunner launches a research job subprocess.
type JobRunner interface {
	Run(ctx context.Context, job *jobstore.Job, store *jobstore.Store) error
}

// Server holds dependencies and the HTTP mux.
type Server struct {
	store    *jobstore.Store
	runner   JobRunner
	staticFS fs.FS
	cwd      string
	mux      *http.ServeMux
	ctx      context.Context // server lifetime context for SSE shutdown
}

// New creates a Server, registers all routes, and returns it.
// ctx is used to signal SSE connections to close when the server shuts down.
func New(store *jobstore.Store, runner JobRunner, staticFS fs.FS, cwd string, ctx context.Context) *Server {
	s := &Server{
		store:    store,
		runner:   runner,
		staticFS: staticFS,
		cwd:      cwd,
		ctx:      ctx,
	}
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

// ServeHTTP implements http.Handler. Past-run routes are intercepted here
// before reaching the mux to avoid registration conflicts with the
// /research/{id}/files/{path...} wildcard pattern.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slog.Debug("http request", "method", r.Method, "path", r.URL.Path)
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, pastRunPrefix) {
		s.handlePastRuns(w, r)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// registerRoutes attaches all handler functions to the mux.
func (s *Server) registerRoutes() {
	// Pages
	s.mux.HandleFunc("GET /{$}", s.handleDashboard)
	s.mux.HandleFunc("GET /reader", s.handleReader)

	// Static assets
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(s.staticFS))))

	// Research API
	s.mux.HandleFunc("POST /research", s.handleStartResearch)
	s.mux.HandleFunc("GET /research", s.handleListResearch)
	s.mux.HandleFunc("GET /research/{id}", s.handleGetResearch)
	s.mux.HandleFunc("DELETE /research/{id}", s.handleCancelResearch)
	s.mux.HandleFunc("GET /research/{id}/stream", s.handleStreamResearch)
	s.mux.HandleFunc("GET /research/{id}/report", s.handleGetReport)
	s.mux.HandleFunc("GET /research/{id}/files", s.handleListJobFiles)
	s.mux.HandleFunc("GET /research/{id}/files/{path...}", s.handleGetJobFile)

	// Past runs: handled in ServeHTTP to avoid mux conflict.
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response with the given status code and message.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// lookupJob retrieves a job by path ID, writing a 404 error response if not found.
func (s *Server) lookupJob(w http.ResponseWriter, r *http.Request) (*jobstore.Job, bool) {
	id := r.PathValue("id")
	job, ok := s.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return nil, false
	}
	return job, true
}
