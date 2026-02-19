package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jamesprial/research-dashboard/internal/model"
)

// handleStartResearch handles POST /research.
// It decodes and validates the request, creates a new job in the store,
// and launches the runner in a goroutine.
func (s *Server) handleStartResearch(w http.ResponseWriter, r *http.Request) {
	var req model.ResearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.store.CleanupExpired(maxJobAge)

	id := generateID()
	cwd := s.cwd
	if req.CWD != nil {
		cwd = *req.CWD
	}

	job := s.store.Create(id, req.Query, string(req.Model), req.MaxTurns, cwd)
	slog.Debug("job created", "id", id, "model", string(req.Model), "max_turns", req.MaxTurns)

	go func() {
		ctx := context.Background()
		if err := s.runner.Run(ctx, job, s.store); err != nil {
			slog.Error("job failed", "id", id, "err", err)
		}
	}()

	writeJSON(w, http.StatusCreated, job.ToStatus())
}

// handleListResearch handles GET /research.
// It returns the list of active jobs along with past run directories.
func (s *Server) handleListResearch(w http.ResponseWriter, r *http.Request) {
	s.store.CleanupExpired(maxJobAge)

	active := s.store.List()
	past := s.store.PastRuns(s.cwd)

	writeJSON(w, http.StatusOK, model.JobList{
		Active: active,
		Past:   past,
	})
}

// handleGetResearch handles GET /research/{id}.
// It returns the full job detail for the requested job.
func (s *Server) handleGetResearch(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, job.ToDetail())
}

// handleCancelResearch handles DELETE /research/{id}.
// It sets the job status to cancelled and returns the updated status.
func (s *Server) handleCancelResearch(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	job.SetStatus(model.StatusCancelled)
	slog.Debug("job cancelled", "id", r.PathValue("id"))
	writeJSON(w, http.StatusOK, job.ToStatus())
}

// handleGetReport handles GET /research/{id}/report.
// It reads report.md from the job's output directory and returns its contents.
func (s *Server) handleGetReport(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	outputDir := job.OutputDir()
	if outputDir == "" {
		writeError(w, http.StatusNotFound, "no output directory")
		return
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "report.md"))
	if err != nil {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// generateID generates a UUID-like random hex identifier using crypto/rand.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
