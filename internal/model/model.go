// Package model provides the core domain types for the research dashboard.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ---------------------------------------------------------------------------
// Typed string enums
// ---------------------------------------------------------------------------

// Status represents the lifecycle state of a research job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// EventType classifies the origin or role of a streaming event.
type EventType string

const (
	EventTypeSystem    EventType = "system"
	EventTypeAssistant EventType = "assistant"
	EventTypeUser      EventType = "user"
	EventTypeResult    EventType = "result"
	EventTypeRaw       EventType = "raw"
)

// EventSubtype provides finer-grained classification within an EventType.
type EventSubtype string

const (
	SubtypeText       EventSubtype = "text"
	SubtypeToolUse    EventSubtype = "tool_use"
	SubtypeToolResult EventSubtype = "tool_result"
	SubtypeTextDelta  EventSubtype = "text_delta"
	SubtypeToolStart  EventSubtype = "tool_start"
	SubtypeEmpty      EventSubtype = ""
)

// ModelName identifies a supported Claude model tier.
type ModelName string

const (
	ModelOpus   ModelName = "opus"
	ModelSonnet ModelName = "sonnet"
	ModelHaiku  ModelName = "haiku"
)

// FileType categorizes output files produced by a research job.
type FileType string

const (
	FileTypeMD    FileType = "md"
	FileTypeHTML  FileType = "html"
	FileTypeOther FileType = "other"
)

// ResearchDirPrefix is the required prefix for research output directory names.
const ResearchDirPrefix = "research-"

// ---------------------------------------------------------------------------
// ValidModel
// ---------------------------------------------------------------------------

// ValidModel returns true if name is one of the supported model identifiers
// ("opus", "sonnet", "haiku"). The comparison is case-sensitive.
func ValidModel(name string) bool {
	switch ModelName(name) {
	case ModelOpus, ModelSonnet, ModelHaiku:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// ParsedEvent
// ---------------------------------------------------------------------------

// ParsedEvent is a structured representation of a single streaming event
// emitted by the research agent.
type ParsedEvent struct {
	Index      int            `json:"index"`
	Type       EventType      `json:"type"`
	Subtype    EventSubtype   `json:"subtype,omitempty"`
	Text       string         `json:"text,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolInput  map[string]any `json:"tool_input,omitempty"`
	ToolResult string         `json:"tool_result,omitempty"`
	IsError    bool           `json:"is_error,omitempty"`
	Raw        map[string]any `json:"raw,omitempty"`
}

// ---------------------------------------------------------------------------
// ResultStats
// ---------------------------------------------------------------------------

// ResultStats captures cost and performance metrics for a completed job.
type ResultStats struct {
	CostUSD       *float64       `json:"cost_usd,omitempty"`
	DurationMS    *int           `json:"duration_ms,omitempty"`
	DurationAPIMS *int           `json:"duration_api_ms,omitempty"`
	NumTurns      *int           `json:"num_turns,omitempty"`
	SessionID     *string        `json:"session_id,omitempty"`
	Usage         map[string]any `json:"usage,omitempty"`
}

// ---------------------------------------------------------------------------
// ResearchRequest
// ---------------------------------------------------------------------------

// ResearchRequest is the input payload for starting a new research job.
type ResearchRequest struct {
	Query    string    `json:"query"`
	Model    ModelName `json:"model"`
	MaxTurns int       `json:"max_turns"`
	CWD      *string   `json:"cwd,omitempty"`
}

// UnmarshalJSON applies default values before decoding the JSON payload so
// that fields omitted from the input retain sensible defaults.
func (r *ResearchRequest) UnmarshalJSON(data []byte) error {
	// Apply defaults first.
	r.Model = ModelOpus
	r.MaxTurns = 100

	// Use an alias type to avoid infinite recursion.
	type alias ResearchRequest
	var a alias
	// Pre-seed alias with the defaults already set on r.
	a.Model = r.Model
	a.MaxTurns = r.MaxTurns

	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("ResearchRequest: %w", err)
	}

	*r = ResearchRequest(a)
	return nil
}

// Validate returns an error if the request is not well-formed.
// It checks that Query is non-empty, Model is a recognised value, and
// MaxTurns is positive.
func (r ResearchRequest) Validate() error {
	if r.Query == "" {
		return errors.New("query is required")
	}
	if !ValidModel(string(r.Model)) {
		return fmt.Errorf("invalid model: %q", r.Model)
	}
	if r.MaxTurns <= 0 {
		return errors.New("max_turns must be positive")
	}
	return nil
}

// ---------------------------------------------------------------------------
// JobStatus
// ---------------------------------------------------------------------------

// JobStatus summarises the current state of a research job.
type JobStatus struct {
	ID          string    `json:"id"`
	Query       string    `json:"query"`
	Model       ModelName `json:"model"`
	Status      Status    `json:"status"`
	CreatedAt   string    `json:"created_at"`
	OutputDir   *string   `json:"output_dir,omitempty"`
	OutputLines int       `json:"output_lines"`
	NumTurns    int       `json:"num_turns"`
	MaxTurns    int       `json:"max_turns"`
}

// ---------------------------------------------------------------------------
// JobDetail
// ---------------------------------------------------------------------------

// JobDetail extends JobStatus with the full event log and result metadata.
type JobDetail struct {
	JobStatus
	Events     []map[string]any `json:"events"`
	SessionID  *string          `json:"session_id,omitempty"`
	ResultInfo *ResultStats     `json:"result_info,omitempty"`
	Error      *string          `json:"error,omitempty"`
}

// MarshalJSON ensures that the Events slice serializes as [] rather than null
// when it is nil or empty.
func (d JobDetail) MarshalJSON() ([]byte, error) {
	// Use an alias to avoid infinite recursion.
	type jobDetailAlias struct {
		JobStatus
		Events     []map[string]any `json:"events"`
		SessionID  *string          `json:"session_id,omitempty"`
		ResultInfo *ResultStats     `json:"result_info,omitempty"`
		Error      *string          `json:"error,omitempty"`
	}

	a := jobDetailAlias{
		JobStatus:  d.JobStatus,
		Events:     nilToEmpty(d.Events),
		SessionID:  d.SessionID,
		ResultInfo: d.ResultInfo,
		Error:      d.Error,
	}
	return json.Marshal(a)
}

// ---------------------------------------------------------------------------
// PastRun
// ---------------------------------------------------------------------------

// PastRun describes a completed research run stored on disk.
type PastRun struct {
	Dir       string `json:"dir"`
	Name      string `json:"name"`
	HasReport bool   `json:"has_report"`
}

// ---------------------------------------------------------------------------
// JobList
// ---------------------------------------------------------------------------

// JobList is the top-level response for listing jobs.
type JobList struct {
	Active []JobStatus `json:"active"`
	Past   []PastRun   `json:"past"`
}

// MarshalJSON ensures Active and Past serialize as [] rather than null when
// nil or empty.
func (jl JobList) MarshalJSON() ([]byte, error) {
	type jobListAlias struct {
		Active []JobStatus `json:"active"`
		Past   []PastRun   `json:"past"`
	}

	return json.Marshal(jobListAlias{
		Active: nilToEmpty(jl.Active),
		Past:   nilToEmpty(jl.Past),
	})
}

// ---------------------------------------------------------------------------
// FileEntry
// ---------------------------------------------------------------------------

// FileEntry describes a single file in a job's output directory.
type FileEntry struct {
	Name string   `json:"name"`
	Path string   `json:"path"`
	Size int64    `json:"size"`
	Type FileType `json:"type"`
}

// ---------------------------------------------------------------------------
// FileListResponse
// ---------------------------------------------------------------------------

// FileListResponse is the payload returned when listing files for a job.
type FileListResponse struct {
	DirName     string      `json:"dir_name"`
	Files       []FileEntry `json:"files"`
	Sources     []FileEntry `json:"sources"`
	SourceIndex *string     `json:"source_index,omitempty"`
}

// MarshalJSON ensures Files and Sources serialize as [] rather than null
// when nil or empty.
func (flr FileListResponse) MarshalJSON() ([]byte, error) {
	type fileListAlias struct {
		DirName     string      `json:"dir_name"`
		Files       []FileEntry `json:"files"`
		Sources     []FileEntry `json:"sources"`
		SourceIndex *string     `json:"source_index,omitempty"`
	}

	return json.Marshal(fileListAlias{
		DirName:     flr.DirName,
		Files:       nilToEmpty(flr.Files),
		Sources:     nilToEmpty(flr.Sources),
		SourceIndex: flr.SourceIndex,
	})
}

// ---------------------------------------------------------------------------
// nilToEmpty
// ---------------------------------------------------------------------------

// nilToEmpty returns s if non-nil, or an initialized empty slice of the same type.
// This ensures JSON serialization produces [] rather than null.
func nilToEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// ---------------------------------------------------------------------------
// EventToDict
// ---------------------------------------------------------------------------

// EventToDict converts a ParsedEvent into a map[string]any suitable for
// inclusion in JSON API responses. Only non-zero fields are included.
// For result-type events, cost and timing statistics are extracted from the
// Raw field and promoted to top-level keys.
func EventToDict(evt ParsedEvent) map[string]any {
	m := map[string]any{
		"index": evt.Index,
		"type":  string(evt.Type),
	}

	if evt.Subtype != SubtypeEmpty {
		m["subtype"] = string(evt.Subtype)
	}
	if evt.Text != "" {
		m["text"] = evt.Text
	}
	if evt.ToolName != "" {
		m["tool_name"] = evt.ToolName
	}
	if evt.ToolInput != nil {
		m["tool_input"] = evt.ToolInput
	}
	if evt.ToolResult != "" {
		m["tool_result"] = evt.ToolResult
	}
	if evt.IsError {
		m["is_error"] = true
	}

	// For result events, extract stats from Raw.
	if evt.Type == EventTypeResult && evt.Raw != nil {
		// cost_usd: prefer total_cost_usd, fall back to cost_usd.
		if v, ok := evt.Raw["total_cost_usd"]; ok {
			m["cost_usd"] = v
		} else if v, ok := evt.Raw["cost_usd"]; ok {
			m["cost_usd"] = v
		}

		if v, ok := evt.Raw["duration_ms"]; ok {
			m["duration_ms"] = v
		}
		if v, ok := evt.Raw["duration_api_ms"]; ok {
			m["duration_api_ms"] = v
		}
		if v, ok := evt.Raw["num_turns"]; ok {
			m["num_turns"] = v
		}
		if v, ok := evt.Raw["session_id"]; ok {
			m["session_id"] = v
		}
		if v, ok := evt.Raw["usage"]; ok {
			if usageMap, ok := v.(map[string]any); ok && len(usageMap) > 0 {
				m["usage"] = v
			}
		}
	}

	return m
}
