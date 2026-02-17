// Package jobstore provides a thread-safe in-memory store for research jobs.
package jobstore

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/jamesprial/research-dashboard/internal/model"
)

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

// Store is a thread-safe in-memory repository for Job instances.
// It also tracks claimed output directories to prevent duplicate usage.
type Store struct {
	mu          sync.RWMutex
	jobs        map[string]*Job
	claimedDirs map[string]struct{}
}

// NewStore returns a Store with initialized internal maps.
func NewStore() *Store {
	return &Store{
		jobs:        make(map[string]*Job),
		claimedDirs: make(map[string]struct{}),
	}
}

// Create constructs a new Job with the given parameters, registers it in the
// store, and returns a pointer to it. The initial status is pending and
// createdAt is set to the current UTC time.
func (s *Store) Create(id, query, mdl string, maxTurns int, cwd string) *Job {
	j := &Job{
		id:        id,
		query:     query,
		model:     mdl,
		maxTurns:  maxTurns,
		cwd:       cwd,
		status:    model.StatusPending,
		createdAt: time.Now().UTC(),
		events:    []model.ParsedEvent{},
	}

	s.mu.Lock()
	s.jobs[id] = j
	s.mu.Unlock()

	return j
}

// Get returns the Job identified by id and whether it was found.
// The lookup is thread-safe.
func (s *Store) Get(id string) (*Job, bool) {
	s.mu.RLock()
	j, ok := s.jobs[id]
	s.mu.RUnlock()
	return j, ok
}

// List returns a snapshot of all jobs as a slice of model.JobStatus.
// The returned slice is never nil.
func (s *Store) List() []model.JobStatus {
	s.mu.RLock()
	out := make([]model.JobStatus, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j.ToStatus())
	}
	s.mu.RUnlock()
	return out
}

// Delete removes the job with the given id from the store.
// If the id is not found the call is a no-op.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
}

// CleanupExpired removes completed, failed, or cancelled jobs whose age
// exceeds maxAge. Running and pending jobs are never removed regardless of age.
func (s *Store) CleanupExpired(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, j := range s.jobs {
		j.mu.RLock()
		status := j.status
		createdAt := j.createdAt
		j.mu.RUnlock()

		switch status {
		case model.StatusCompleted, model.StatusFailed, model.StatusCancelled:
			if time.Since(createdAt) > maxAge {
				delete(s.jobs, id)
			}
		}
	}
}

// PastRuns scans cwd for subdirectories whose names begin with "research-".
// It returns a slice of model.PastRun entries sorted by name descending.
// Files are ignored; only directories are considered.
func (s *Store) PastRuns(cwd string) []model.PastRun {
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return []model.PastRun{}
	}

	var runs []model.PastRun
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < len("research-") || name[:len("research-")] != "research-" {
			continue
		}
		dir := filepath.Join(cwd, name)
		_, err := os.Stat(filepath.Join(dir, "report.md"))
		hasReport := err == nil

		runs = append(runs, model.PastRun{
			Dir:       dir,
			Name:      name,
			HasReport: hasReport,
		})
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].Name > runs[j].Name
	})

	if runs == nil {
		return []model.PastRun{}
	}
	return runs
}

// ClaimDir attempts to claim the given directory path. It returns true if the
// directory was not previously claimed (and is now claimed), or false if it
// was already claimed by a previous call.
func (s *Store) ClaimDir(dir string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.claimedDirs[dir]; exists {
		return false
	}
	s.claimedDirs[dir] = struct{}{}
	return true
}

// ReleaseDir releases a previously claimed directory so it can be claimed again.
func (s *Store) ReleaseDir(dir string) {
	s.mu.Lock()
	delete(s.claimedDirs, dir)
	s.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Job
// ---------------------------------------------------------------------------

// Job holds all state for a single research job. All field access is
// protected by an internal read-write mutex.
type Job struct {
	mu         sync.RWMutex
	id         string
	query      string
	model      string
	maxTurns   int
	cwd        string
	status     model.Status
	createdAt  time.Time
	events     []model.ParsedEvent
	outputDir  string
	errMsg     string
	sessionID  string
	resultInfo model.ResultStats
}

// ---------------------------------------------------------------------------
// Getters
// ---------------------------------------------------------------------------

// ID returns the job's unique identifier.
func (j *Job) ID() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.id
}

// Query returns the research query string.
func (j *Job) Query() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.query
}

// Model returns the model name string used by this job.
func (j *Job) Model() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.model
}

// MaxTurns returns the maximum number of agent turns allowed.
func (j *Job) MaxTurns() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.maxTurns
}

// CWD returns the working directory for the job.
func (j *Job) CWD() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.cwd
}

// Status returns the current lifecycle status of the job.
func (j *Job) Status() model.Status {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status
}

// OutputDir returns the output directory path, or an empty string if not set.
func (j *Job) OutputDir() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.outputDir
}

// Error returns the error message, or an empty string if none.
func (j *Job) Error() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.errMsg
}

// SessionID returns the agent session ID, or an empty string if not set.
func (j *Job) SessionID() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.sessionID
}

// ResultInfo returns a copy of the job's result statistics.
func (j *Job) ResultInfo() model.ResultStats {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.resultInfo
}

// EventCount returns the number of events recorded for this job.
func (j *Job) EventCount() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return len(j.events)
}

// ---------------------------------------------------------------------------
// Setters
// ---------------------------------------------------------------------------

// SetStatus updates the job's lifecycle status.
func (j *Job) SetStatus(s model.Status) {
	j.mu.Lock()
	j.status = s
	j.mu.Unlock()
}

// SetOutputDir sets the output directory path for the job.
func (j *Job) SetOutputDir(dir string) {
	j.mu.Lock()
	j.outputDir = dir
	j.mu.Unlock()
}

// SetError records an error message on the job.
func (j *Job) SetError(msg string) {
	j.mu.Lock()
	j.errMsg = msg
	j.mu.Unlock()
}

// SetSessionID records the agent session ID on the job.
func (j *Job) SetSessionID(id string) {
	j.mu.Lock()
	j.sessionID = id
	j.mu.Unlock()
}

// SetResultInfo stores cost and performance statistics for the job.
func (j *Job) SetResultInfo(info model.ResultStats) {
	j.mu.Lock()
	j.resultInfo = info
	j.mu.Unlock()
}

// SetCreatedAt overrides the job creation timestamp. Intended for use in
// tests that need to backdate a job to trigger expiration logic.
func (j *Job) SetCreatedAt(t time.Time) {
	j.mu.Lock()
	j.createdAt = t
	j.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Event methods
// ---------------------------------------------------------------------------

// AddEvent appends a parsed event to the job's event log.
func (j *Job) AddEvent(evt model.ParsedEvent) {
	j.mu.Lock()
	j.events = append(j.events, evt)
	j.mu.Unlock()
}

// EventsSince returns a copy of the events slice starting at the given cursor
// index. If cursor is greater than or equal to the number of events, an empty
// slice is returned.
func (j *Job) EventsSince(cursor int) []model.ParsedEvent {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(j.events) {
		return []model.ParsedEvent{}
	}
	subset := j.events[cursor:]
	out := make([]model.ParsedEvent, len(subset))
	copy(out, subset)
	return out
}

// NumTurns counts events whose Type is assistant and Subtype is text.
func (j *Job) NumTurns() int {
	j.mu.RLock()
	defer j.mu.RUnlock()

	count := 0
	for _, evt := range j.events {
		if evt.Type == model.EventTypeAssistant && evt.Subtype == model.SubtypeText {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Conversion methods
// ---------------------------------------------------------------------------

// ToStatus returns a model.JobStatus snapshot of the current job state.
// OutputDir is represented as a *string and is nil when the field is empty.
func (j *Job) ToStatus() model.JobStatus {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var outputDir *string
	if j.outputDir != "" {
		v := j.outputDir
		outputDir = &v
	}

	numTurns := 0
	for _, evt := range j.events {
		if evt.Type == model.EventTypeAssistant && evt.Subtype == model.SubtypeText {
			numTurns++
		}
	}

	return model.JobStatus{
		ID:          j.id,
		Query:       j.query,
		Model:       model.ModelName(j.model),
		Status:      j.status,
		CreatedAt:   j.createdAt.Format(time.RFC3339),
		OutputDir:   outputDir,
		OutputLines: len(j.events),
		NumTurns:    numTurns,
		MaxTurns:    j.maxTurns,
	}
}

// ToDetail returns a model.JobDetail that embeds a JobStatus and includes
// the full event log along with session, error, and result metadata.
// SessionID and Error are represented as *string (nil when empty).
// ResultInfo is a *model.ResultStats and is nil when the field is a zero value.
func (j *Job) ToDetail() model.JobDetail {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var outputDir *string
	if j.outputDir != "" {
		v := j.outputDir
		outputDir = &v
	}

	numTurns := 0
	for _, evt := range j.events {
		if evt.Type == model.EventTypeAssistant && evt.Subtype == model.SubtypeText {
			numTurns++
		}
	}

	status := model.JobStatus{
		ID:          j.id,
		Query:       j.query,
		Model:       model.ModelName(j.model),
		Status:      j.status,
		CreatedAt:   j.createdAt.Format(time.RFC3339),
		OutputDir:   outputDir,
		OutputLines: len(j.events),
		NumTurns:    numTurns,
		MaxTurns:    j.maxTurns,
	}

	// Convert events; always return a non-nil slice so JSON serializes as [].
	events := make([]map[string]any, 0, len(j.events))
	for _, evt := range j.events {
		events = append(events, model.EventToDict(evt))
	}

	var sessionID *string
	if j.sessionID != "" {
		v := j.sessionID
		sessionID = &v
	}

	var errPtr *string
	if j.errMsg != "" {
		v := j.errMsg
		errPtr = &v
	}

	var resultInfo *model.ResultStats
	if !isZeroResultStats(j.resultInfo) {
		ri := j.resultInfo
		resultInfo = &ri
	}

	return model.JobDetail{
		JobStatus:  status,
		Events:     events,
		SessionID:  sessionID,
		ResultInfo: resultInfo,
		Error:      errPtr,
	}
}

// isZeroResultStats returns true when all pointer fields of a ResultStats are
// nil and the Usage map is empty. This is used to decide whether to include
// ResultInfo in the ToDetail response.
func isZeroResultStats(r model.ResultStats) bool {
	return r.CostUSD == nil &&
		r.DurationMS == nil &&
		r.DurationAPIMS == nil &&
		r.NumTurns == nil &&
		r.SessionID == nil &&
		len(r.Usage) == 0
}
