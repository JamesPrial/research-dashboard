// Package server — file listing and serving handlers for both active jobs
// and past runs.
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesprial/research-dashboard/internal/model"
	"github.com/jamesprial/research-dashboard/internal/pathutil"
)

// handleListJobFiles handles GET /research/{id}/files.
// It returns a FileListResponse describing the files in the job's output
// directory.
func (s *Server) handleListJobFiles(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	outputDir := job.OutputDir()
	if outputDir == "" {
		writeError(w, http.StatusNotFound, "no output directory")
		return
	}
	resp := buildFileListResponse(outputDir, filepath.Base(outputDir))
	writeJSON(w, http.StatusOK, resp)
}

// handleGetJobFile handles GET /research/{id}/files/{path...}.
// It serves a specific file from the job's output directory with path
// traversal protection.
func (s *Server) handleGetJobFile(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	outputDir := job.OutputDir()
	if outputDir == "" {
		writeError(w, http.StatusNotFound, "no output directory")
		return
	}
	filePath := r.PathValue("path")
	serveFile(w, r, outputDir, filePath)
}

// handlePastRuns is the catch-all dispatcher for GET /research/past/.
// It manually parses the suffix after "/research/past/" to dispatch to the
// appropriate handler, avoiding Go 1.22+ ServeMux ambiguity between
// /research/{id}/files/{path...} and /research/past/{dir}/....
func (s *Server) handlePastRuns(w http.ResponseWriter, r *http.Request) {
	// Strip the prefix to get "<dirName>/<rest...>"
	suffix := strings.TrimPrefix(r.URL.Path, "/research/past/")
	if suffix == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Split into dirName and rest: "dirName/report", "dirName/files", "dirName/files/foo.md"
	dirName, rest, _ := strings.Cut(suffix, "/")
	if dirName == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if err := pathutil.ValidateDirName(dirName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch {
	case rest == "report":
		s.servePastReport(w, r, dirName)
	case rest == "files":
		s.servePastFiles(w, r, dirName)
	case strings.HasPrefix(rest, "files/"):
		filePath := strings.TrimPrefix(rest, "files/")
		s.servePastFile(w, r, dirName, filePath)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// servePastReport reads report.md from the named past-run directory under cwd.
func (s *Server) servePastReport(w http.ResponseWriter, _ *http.Request, dirName string) {
	dir := filepath.Join(s.cwd, dirName)
	data, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// servePastFiles returns a FileListResponse for the named past-run directory.
func (s *Server) servePastFiles(w http.ResponseWriter, _ *http.Request, dirName string) {
	dir := filepath.Join(s.cwd, dirName)
	resp := buildFileListResponse(dir, dirName)
	writeJSON(w, http.StatusOK, resp)
}

// servePastFile serves a specific file from the named past-run directory.
func (s *Server) servePastFile(w http.ResponseWriter, r *http.Request, dirName, filePath string) {
	dir := filepath.Join(s.cwd, dirName)
	serveFile(w, r, dir, filePath)
}

// buildFileListResponse builds a FileListResponse by listing files in dir.
// Files in the "sources" subdirectory are listed separately.
// If dir cannot be read, an empty response is returned without error.
func buildFileListResponse(dir, dirName string) model.FileListResponse {
	resp := model.FileListResponse{
		DirName: dirName,
		Files:   []model.FileEntry{},
		Sources: []model.FileEntry{},
	}

	// List top-level files (skip subdirectories).
	entries, err := os.ReadDir(dir)
	if err != nil {
		return resp
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		resp.Files = append(resp.Files, model.FileEntry{
			Name: entry.Name(),
			Path: entry.Name(),
			Size: info.Size(),
			Type: pathutil.ClassifyFileType(entry.Name()),
		})
	}

	// List files inside the "sources" subdirectory.
	sourcesDir := filepath.Join(dir, "sources")
	sourceEntries, err := os.ReadDir(sourcesDir)
	if err == nil {
		for _, entry := range sourceEntries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			resp.Sources = append(resp.Sources, model.FileEntry{
				Name: entry.Name(),
				Path: filepath.Join("sources", entry.Name()),
				Size: info.Size(),
				Type: pathutil.ClassifyFileType(entry.Name()),
			})
		}
	}

	// Attempt to read the source index file.
	indexPath := filepath.Join(sourcesDir, "index.md")
	if data, err := os.ReadFile(indexPath); err == nil {
		content := string(data)
		resp.SourceIndex = &content
	}

	return resp
}

// serveFile serves a file from baseDir identified by filePath.
// It uses pathutil.ResolveSafeFile to prevent path traversal attacks.
func serveFile(w http.ResponseWriter, r *http.Request, baseDir, filePath string) {
	resolved, err := pathutil.ResolveSafeFile(baseDir, filePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file path")
		return
	}
	http.ServeFile(w, r, resolved)
}
