package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jamesprial/research-dashboard/internal/model"
)

// handleStreamResearch handles GET /research/{id}/stream.
// It streams job events as Server-Sent Events (SSE). The optional query
// parameter "after" specifies the cursor index to resume from (default 0).
// The connection is held open and polled every 300ms until the job reaches
// a terminal state or either the server or request context is cancelled.
func (s *Server) handleStreamResearch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	after := 0
	if v := r.URL.Query().Get("after"); v != "" {
		after, _ = strconv.Atoi(v)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	rc := http.NewResponseController(w)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	cursor := after
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-r.Context().Done():
			return
		case <-ticker.C:
			events := job.EventsSince(cursor)
			for _, evt := range events {
				data, _ := json.Marshal(model.EventToDict(evt))
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				cursor++
			}
			if len(events) > 0 {
				_ = rc.Flush()
			}

			// Check whether the job has reached a terminal state.
			status := job.Status()
			if status == model.StatusCompleted || status == model.StatusFailed || status == model.StatusCancelled {
				doneData := map[string]string{
					"status":     string(status),
					"output_dir": job.OutputDir(),
				}
				data, _ := json.Marshal(doneData)
				_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
				_ = rc.Flush()
				return
			}
		}
	}
}
