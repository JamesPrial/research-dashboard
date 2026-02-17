package model_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jamesprial/research-dashboard/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

// ---------------------------------------------------------------------------
// Enum: Status
// ---------------------------------------------------------------------------

func Test_Status_Constants(t *testing.T) {
	tests := []struct {
		name     string
		constant model.Status
		want     string
	}{
		{"StatusPending", model.StatusPending, "pending"},
		{"StatusRunning", model.StatusRunning, "running"},
		{"StatusCompleted", model.StatusCompleted, "completed"},
		{"StatusFailed", model.StatusFailed, "failed"},
		{"StatusCancelled", model.StatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, string(tt.constant), tt.want)
			}
		})
	}
}

func Test_Status_IsTypedString(t *testing.T) {
	// Verify Status is its own type, not a raw string alias that can be
	// freely assigned from string without conversion.
	s := model.StatusPending
	_ = s // compile-time proof it is assignable
	if reflect.TypeOf(s).Kind() != reflect.String {
		t.Errorf("Status underlying kind = %v, want string", reflect.TypeOf(s).Kind())
	}
	if reflect.TypeOf(s).Name() != "Status" {
		t.Errorf("Status type name = %q, want %q", reflect.TypeOf(s).Name(), "Status")
	}
}

// ---------------------------------------------------------------------------
// Enum: EventType
// ---------------------------------------------------------------------------

func Test_EventType_Constants(t *testing.T) {
	tests := []struct {
		name     string
		constant model.EventType
		want     string
	}{
		{"EventTypeSystem", model.EventTypeSystem, "system"},
		{"EventTypeAssistant", model.EventTypeAssistant, "assistant"},
		{"EventTypeUser", model.EventTypeUser, "user"},
		{"EventTypeResult", model.EventTypeResult, "result"},
		{"EventTypeRaw", model.EventTypeRaw, "raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, string(tt.constant), tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Enum: EventSubtype
// ---------------------------------------------------------------------------

func Test_EventSubtype_Constants(t *testing.T) {
	tests := []struct {
		name     string
		constant model.EventSubtype
		want     string
	}{
		{"SubtypeText", model.SubtypeText, "text"},
		{"SubtypeToolUse", model.SubtypeToolUse, "tool_use"},
		{"SubtypeToolResult", model.SubtypeToolResult, "tool_result"},
		{"SubtypeTextDelta", model.SubtypeTextDelta, "text_delta"},
		{"SubtypeToolStart", model.SubtypeToolStart, "tool_start"},
		{"SubtypeEmpty", model.SubtypeEmpty, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, string(tt.constant), tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Enum: ModelName
// ---------------------------------------------------------------------------

func Test_ModelName_Constants(t *testing.T) {
	tests := []struct {
		name     string
		constant model.ModelName
		want     string
	}{
		{"ModelOpus", model.ModelOpus, "opus"},
		{"ModelSonnet", model.ModelSonnet, "sonnet"},
		{"ModelHaiku", model.ModelHaiku, "haiku"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, string(tt.constant), tt.want)
			}
		})
	}
}

func Test_ValidModel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"opus is valid", "opus", true},
		{"sonnet is valid", "sonnet", true},
		{"haiku is valid", "haiku", true},
		{"gpt4 is invalid", "gpt4", false},
		{"empty string is invalid", "", false},
		{"uppercase Opus is invalid", "Opus", false},
		{"random string is invalid", "foobar", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.ValidModel(tt.input)
			if got != tt.want {
				t.Errorf("ValidModel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Enum: FileType
// ---------------------------------------------------------------------------

func Test_FileType_Constants(t *testing.T) {
	tests := []struct {
		name     string
		constant model.FileType
		want     string
	}{
		{"FileTypeMD", model.FileTypeMD, "md"},
		{"FileTypeHTML", model.FileTypeHTML, "html"},
		{"FileTypeOther", model.FileTypeOther, "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, string(tt.constant), tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Struct: ParsedEvent — JSON serialization
// ---------------------------------------------------------------------------

func Test_ParsedEvent_JSON_Serialization(t *testing.T) {
	tests := []struct {
		name           string
		event          model.ParsedEvent
		wantKeys       []string // keys that MUST be present
		wantAbsentKeys []string // keys that MUST NOT be present
		wantExact      string   // if non-empty, expect exact JSON match
	}{
		{
			name:           "minimal event",
			event:          model.ParsedEvent{Index: 0, Type: model.EventTypeSystem},
			wantKeys:       []string{"index", "type"},
			wantAbsentKeys: []string{"subtype", "text", "tool_name", "tool_input", "tool_result", "is_error", "raw"},
			wantExact:      `{"index":0,"type":"system"}`,
		},
		{
			name: "text event",
			event: model.ParsedEvent{
				Index:   1,
				Type:    model.EventTypeAssistant,
				Subtype: model.SubtypeText,
				Text:    "hello",
			},
			wantKeys:       []string{"index", "type", "subtype", "text"},
			wantAbsentKeys: []string{"tool_name", "tool_input", "tool_result", "is_error", "raw"},
		},
		{
			name: "tool_use event",
			event: model.ParsedEvent{
				Index:     2,
				Type:      model.EventTypeAssistant,
				Subtype:   model.SubtypeToolUse,
				ToolName:  "WebSearch",
				ToolInput: map[string]any{"query": "test"},
			},
			wantKeys:       []string{"index", "type", "subtype", "tool_name", "tool_input"},
			wantAbsentKeys: []string{"text", "tool_result", "is_error", "raw"},
		},
		{
			name:           "omit empty fields",
			event:          model.ParsedEvent{Index: 0, Type: model.EventTypeRaw},
			wantKeys:       []string{"index", "type"},
			wantAbsentKeys: []string{"subtype", "text", "tool_name", "tool_input", "tool_result", "is_error", "raw"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			if tt.wantExact != "" {
				if string(data) != tt.wantExact {
					t.Errorf("json.Marshal() = %s, want %s", string(data), tt.wantExact)
				}
			}

			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			for _, k := range tt.wantKeys {
				if _, ok := m[k]; !ok {
					t.Errorf("expected key %q in JSON, got keys: %v", k, keys(m))
				}
			}
			for _, k := range tt.wantAbsentKeys {
				if _, ok := m[k]; ok {
					t.Errorf("unexpected key %q in JSON output: %s", k, string(data))
				}
			}
		})
	}
}

func Test_ParsedEvent_JSON_Roundtrip(t *testing.T) {
	original := model.ParsedEvent{
		Index:      3,
		Type:       model.EventTypeAssistant,
		Subtype:    model.SubtypeToolUse,
		Text:       "some text",
		ToolName:   "Bash",
		ToolInput:  map[string]any{"command": "ls"},
		ToolResult: "file.txt",
		IsError:    true,
		Raw:        map[string]any{"extra": "data"},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded model.ParsedEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Index != original.Index {
		t.Errorf("Index = %d, want %d", decoded.Index, original.Index)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, original.Type)
	}
	if decoded.Subtype != original.Subtype {
		t.Errorf("Subtype = %q, want %q", decoded.Subtype, original.Subtype)
	}
	if decoded.Text != original.Text {
		t.Errorf("Text = %q, want %q", decoded.Text, original.Text)
	}
	if decoded.ToolName != original.ToolName {
		t.Errorf("ToolName = %q, want %q", decoded.ToolName, original.ToolName)
	}
	if decoded.ToolResult != original.ToolResult {
		t.Errorf("ToolResult = %q, want %q", decoded.ToolResult, original.ToolResult)
	}
	if decoded.IsError != original.IsError {
		t.Errorf("IsError = %v, want %v", decoded.IsError, original.IsError)
	}
}

// ---------------------------------------------------------------------------
// Struct: ResultStats — JSON serialization
// ---------------------------------------------------------------------------

func Test_ResultStats_JSON_Serialization(t *testing.T) {
	tests := []struct {
		name           string
		stats          model.ResultStats
		wantJSON       string   // exact match if non-empty
		wantKeys       []string // keys that MUST be present
		wantAbsentKeys []string // keys that MUST NOT be present
	}{
		{
			name:           "all nil produces empty object",
			stats:          model.ResultStats{},
			wantJSON:       `{}`,
			wantAbsentKeys: []string{"cost_usd", "duration_ms", "duration_api_ms", "num_turns", "session_id", "usage"},
		},
		{
			name: "all populated",
			stats: model.ResultStats{
				CostUSD:       ptr(1.5),
				DurationMS:    ptr(5000),
				DurationAPIMS: ptr(4000),
				NumTurns:      ptr(10),
				SessionID:     ptr("sess-123"),
				Usage:         map[string]any{"input_tokens": float64(100)},
			},
			wantKeys: []string{"cost_usd", "duration_ms", "duration_api_ms", "num_turns", "session_id", "usage"},
		},
		{
			name: "partial - only cost_usd",
			stats: model.ResultStats{
				CostUSD: ptr(0.5),
			},
			wantKeys:       []string{"cost_usd"},
			wantAbsentKeys: []string{"duration_ms", "duration_api_ms", "num_turns", "session_id", "usage"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.stats)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			if tt.wantJSON != "" {
				if string(data) != tt.wantJSON {
					t.Errorf("json.Marshal() = %s, want %s", string(data), tt.wantJSON)
				}
			}

			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			for _, k := range tt.wantKeys {
				if _, ok := m[k]; !ok {
					t.Errorf("expected key %q in JSON, got: %s", k, string(data))
				}
			}
			for _, k := range tt.wantAbsentKeys {
				if _, ok := m[k]; ok {
					t.Errorf("unexpected key %q in JSON: %s", k, string(data))
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Struct: ResearchRequest — Validate()
// ---------------------------------------------------------------------------

func Test_ResearchRequest_Validate(t *testing.T) {
	tests := []struct {
		name       string
		req        model.ResearchRequest
		wantErr    bool
		errContain string // substring expected in error message
	}{
		{
			name:    "valid minimal",
			req:     model.ResearchRequest{Query: "test", Model: model.ModelOpus, MaxTurns: 100},
			wantErr: false,
		},
		{
			name:       "empty query",
			req:        model.ResearchRequest{Query: "", Model: model.ModelOpus, MaxTurns: 100},
			wantErr:    true,
			errContain: "query is required",
		},
		{
			name:       "invalid model",
			req:        model.ResearchRequest{Query: "test", Model: "gpt4", MaxTurns: 100},
			wantErr:    true,
			errContain: "invalid model",
		},
		{
			name:       "zero max_turns",
			req:        model.ResearchRequest{Query: "test", Model: model.ModelOpus, MaxTurns: 0},
			wantErr:    true,
			errContain: "max_turns must be positive",
		},
		{
			name:       "negative max_turns",
			req:        model.ResearchRequest{Query: "test", Model: model.ModelOpus, MaxTurns: -1},
			wantErr:    true,
			errContain: "max_turns must be positive",
		},
		{
			name:    "valid with cwd",
			req:     model.ResearchRequest{Query: "test", Model: model.ModelOpus, MaxTurns: 50, CWD: ptr("/tmp")},
			wantErr: false,
		},
		{
			name:    "valid with sonnet model",
			req:     model.ResearchRequest{Query: "research AI", Model: model.ModelSonnet, MaxTurns: 10},
			wantErr: false,
		},
		{
			name:    "valid with haiku model",
			req:     model.ResearchRequest{Query: "quick lookup", Model: model.ModelHaiku, MaxTurns: 5},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() error = nil, want error containing %q", tt.errContain)
				}
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errContain)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func Test_ResearchRequest_JSON_Deserialization_Defaults(t *testing.T) {
	tests := []struct {
		name         string
		jsonInput    string
		wantModel    model.ModelName
		wantMaxTurns int
		wantQuery    string
	}{
		{
			name:         "defaults applied for missing model and max_turns",
			jsonInput:    `{"query":"test"}`,
			wantModel:    model.ModelOpus,
			wantMaxTurns: 100,
			wantQuery:    "test",
		},
		{
			name:         "explicit values preserved",
			jsonInput:    `{"query":"test","model":"sonnet","max_turns":50}`,
			wantModel:    model.ModelSonnet,
			wantMaxTurns: 50,
			wantQuery:    "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req model.ResearchRequest
			if err := json.Unmarshal([]byte(tt.jsonInput), &req); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			// Apply defaults — the spec says defaults should be applied.
			// The struct may use a custom UnmarshalJSON or a separate method.
			// We test the final state after unmarshalling.
			if req.Query != tt.wantQuery {
				t.Errorf("Query = %q, want %q", req.Query, tt.wantQuery)
			}
			if req.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", req.Model, tt.wantModel)
			}
			if req.MaxTurns != tt.wantMaxTurns {
				t.Errorf("MaxTurns = %d, want %d", req.MaxTurns, tt.wantMaxTurns)
			}
		})
	}
}

func Test_ResearchRequest_JSON_Roundtrip(t *testing.T) {
	original := model.ResearchRequest{
		Query:    "research something",
		Model:    model.ModelSonnet,
		MaxTurns: 75,
		CWD:      ptr("/home/user"),
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded model.ResearchRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Query != original.Query {
		t.Errorf("Query = %q, want %q", decoded.Query, original.Query)
	}
	if decoded.Model != original.Model {
		t.Errorf("Model = %q, want %q", decoded.Model, original.Model)
	}
	if decoded.MaxTurns != original.MaxTurns {
		t.Errorf("MaxTurns = %d, want %d", decoded.MaxTurns, original.MaxTurns)
	}
	if decoded.CWD == nil || *decoded.CWD != *original.CWD {
		t.Errorf("CWD = %v, want %v", decoded.CWD, original.CWD)
	}
}

func Test_ResearchRequest_JSON_CWD_Omitempty(t *testing.T) {
	req := model.ResearchRequest{Query: "q", Model: model.ModelOpus, MaxTurns: 10}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := m["cwd"]; ok {
		t.Errorf("cwd should be omitted when nil, got JSON: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// Struct: JobStatus — JSON
// ---------------------------------------------------------------------------

func Test_JobStatus_JSON_Roundtrip(t *testing.T) {
	original := model.JobStatus{
		ID:          "job-abc-123",
		Query:       "test query",
		Model:       model.ModelSonnet,
		Status:      model.StatusRunning,
		CreatedAt:   "2026-02-17T10:00:00Z",
		OutputDir:   ptr("/output/dir"),
		OutputLines: 42,
		NumTurns:    5,
		MaxTurns:    100,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded model.JobStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Query != original.Query {
		t.Errorf("Query = %q, want %q", decoded.Query, original.Query)
	}
	if decoded.Model != original.Model {
		t.Errorf("Model = %q, want %q", decoded.Model, original.Model)
	}
	if decoded.Status != original.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
	}
	if decoded.CreatedAt != original.CreatedAt {
		t.Errorf("CreatedAt = %q, want %q", decoded.CreatedAt, original.CreatedAt)
	}
	if decoded.OutputDir == nil || *decoded.OutputDir != *original.OutputDir {
		t.Errorf("OutputDir = %v, want %v", decoded.OutputDir, original.OutputDir)
	}
	if decoded.OutputLines != original.OutputLines {
		t.Errorf("OutputLines = %d, want %d", decoded.OutputLines, original.OutputLines)
	}
	if decoded.NumTurns != original.NumTurns {
		t.Errorf("NumTurns = %d, want %d", decoded.NumTurns, original.NumTurns)
	}
	if decoded.MaxTurns != original.MaxTurns {
		t.Errorf("MaxTurns = %d, want %d", decoded.MaxTurns, original.MaxTurns)
	}
}

func Test_JobStatus_OutputDir_Omitempty(t *testing.T) {
	js := model.JobStatus{
		ID:        "job-1",
		Query:     "q",
		Model:     model.ModelOpus,
		Status:    model.StatusPending,
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	data, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := m["output_dir"]; ok {
		t.Errorf("output_dir should be omitted when nil, got JSON: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// Struct: JobDetail — JSON
// ---------------------------------------------------------------------------

func Test_JobDetail_JSON_Serialization(t *testing.T) {
	detail := model.JobDetail{
		JobStatus: model.JobStatus{
			ID:          "job-detail-1",
			Query:       "detail query",
			Model:       model.ModelHaiku,
			Status:      model.StatusCompleted,
			CreatedAt:   "2026-02-17T12:00:00Z",
			OutputDir:   ptr("/output"),
			OutputLines: 100,
			NumTurns:    10,
			MaxTurns:    50,
		},
		Events:    []map[string]any{{"type": "system", "text": "init"}},
		SessionID: ptr("sess-xyz"),
		ResultInfo: &model.ResultStats{
			CostUSD:    ptr(1.25),
			DurationMS: ptr(3000),
		},
		Error: ptr("something went wrong"),
	}

	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify embedded JobStatus fields are at top level
	expectedKeys := []string{
		"id", "query", "model", "status", "created_at",
		"output_dir", "output_lines", "num_turns", "max_turns",
		"events", "session_id", "result_info", "error",
	}
	for _, k := range expectedKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("expected key %q in JSON, got keys: %v", k, keys(m))
		}
	}
}

func Test_JobDetail_EmptyEvents_SerializesAsArray(t *testing.T) {
	detail := model.JobDetail{
		JobStatus: model.JobStatus{
			ID:        "job-empty-events",
			Query:     "q",
			Model:     model.ModelOpus,
			Status:    model.StatusPending,
			CreatedAt: "2026-01-01T00:00:00Z",
		},
		Events: []map[string]any{},
	}

	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Verify events is [] not null
	if !strings.Contains(string(data), `"events":[]`) {
		t.Errorf("empty events should serialize as [], got: %s", string(data))
	}
}

func Test_JobDetail_OmitemptyFields(t *testing.T) {
	detail := model.JobDetail{
		JobStatus: model.JobStatus{
			ID:        "job-omit",
			Query:     "q",
			Model:     model.ModelOpus,
			Status:    model.StatusPending,
			CreatedAt: "2026-01-01T00:00:00Z",
		},
		Events: []map[string]any{},
	}

	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	for _, k := range []string{"session_id", "result_info", "error"} {
		if _, ok := m[k]; ok {
			t.Errorf("key %q should be omitted when nil, got JSON: %s", k, string(data))
		}
	}
}

// ---------------------------------------------------------------------------
// Struct: PastRun — JSON
// ---------------------------------------------------------------------------

func Test_PastRun_JSON_Roundtrip(t *testing.T) {
	original := model.PastRun{
		Dir:       "/data/runs/run-1",
		Name:      "run-1",
		HasReport: true,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded model.PastRun
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Dir != original.Dir {
		t.Errorf("Dir = %q, want %q", decoded.Dir, original.Dir)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.HasReport != original.HasReport {
		t.Errorf("HasReport = %v, want %v", decoded.HasReport, original.HasReport)
	}
}

// ---------------------------------------------------------------------------
// Struct: JobList — JSON (empty lists as [])
// ---------------------------------------------------------------------------

func Test_JobList_EmptyLists_SerializeAsArrays(t *testing.T) {
	jl := model.JobList{
		Active: []model.JobStatus{},
		Past:   []model.PastRun{},
	}
	data, err := json.Marshal(jl)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"active":[]`) {
		t.Errorf("empty active should serialize as [], got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"past":[]`) {
		t.Errorf("empty past should serialize as [], got: %s", jsonStr)
	}
}

func Test_JobList_JSON_Roundtrip(t *testing.T) {
	original := model.JobList{
		Active: []model.JobStatus{
			{
				ID:        "job-1",
				Query:     "q1",
				Model:     model.ModelOpus,
				Status:    model.StatusRunning,
				CreatedAt: "2026-02-17T10:00:00Z",
				MaxTurns:  100,
			},
		},
		Past: []model.PastRun{
			{Dir: "/runs/old", Name: "old", HasReport: true},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded model.JobList
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if len(decoded.Active) != 1 {
		t.Fatalf("Active length = %d, want 1", len(decoded.Active))
	}
	if decoded.Active[0].ID != "job-1" {
		t.Errorf("Active[0].ID = %q, want %q", decoded.Active[0].ID, "job-1")
	}
	if len(decoded.Past) != 1 {
		t.Fatalf("Past length = %d, want 1", len(decoded.Past))
	}
	if decoded.Past[0].Dir != "/runs/old" {
		t.Errorf("Past[0].Dir = %q, want %q", decoded.Past[0].Dir, "/runs/old")
	}
}

// ---------------------------------------------------------------------------
// Struct: FileEntry — JSON
// ---------------------------------------------------------------------------

func Test_FileEntry_JSON_Roundtrip(t *testing.T) {
	original := model.FileEntry{
		Name: "report.md",
		Path: "/output/report.md",
		Size: 4096,
		Type: model.FileTypeMD,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded model.FileEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Path != original.Path {
		t.Errorf("Path = %q, want %q", decoded.Path, original.Path)
	}
	if decoded.Size != original.Size {
		t.Errorf("Size = %d, want %d", decoded.Size, original.Size)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, original.Type)
	}
}

// ---------------------------------------------------------------------------
// Struct: FileListResponse — JSON (empty lists as [])
// ---------------------------------------------------------------------------

func Test_FileListResponse_EmptyLists_SerializeAsArrays(t *testing.T) {
	flr := model.FileListResponse{
		DirName: "output",
		Files:   []model.FileEntry{},
		Sources: []model.FileEntry{},
	}
	data, err := json.Marshal(flr)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"files":[]`) {
		t.Errorf("empty files should serialize as [], got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"sources":[]`) {
		t.Errorf("empty sources should serialize as [], got: %s", jsonStr)
	}
}

func Test_FileListResponse_SourceIndex_Omitempty(t *testing.T) {
	flr := model.FileListResponse{
		DirName: "output",
		Files:   []model.FileEntry{},
		Sources: []model.FileEntry{},
	}
	data, err := json.Marshal(flr)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := m["source_index"]; ok {
		t.Errorf("source_index should be omitted when nil, got JSON: %s", string(data))
	}
}

func Test_FileListResponse_JSON_Roundtrip(t *testing.T) {
	original := model.FileListResponse{
		DirName: "my-output",
		Files: []model.FileEntry{
			{Name: "report.md", Path: "/out/report.md", Size: 1024, Type: model.FileTypeMD},
		},
		Sources: []model.FileEntry{
			{Name: "page.html", Path: "/out/page.html", Size: 2048, Type: model.FileTypeHTML},
		},
		SourceIndex: ptr("index.html"),
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded model.FileListResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.DirName != original.DirName {
		t.Errorf("DirName = %q, want %q", decoded.DirName, original.DirName)
	}
	if len(decoded.Files) != 1 {
		t.Fatalf("Files length = %d, want 1", len(decoded.Files))
	}
	if decoded.Files[0].Name != "report.md" {
		t.Errorf("Files[0].Name = %q, want %q", decoded.Files[0].Name, "report.md")
	}
	if len(decoded.Sources) != 1 {
		t.Fatalf("Sources length = %d, want 1", len(decoded.Sources))
	}
	if decoded.SourceIndex == nil || *decoded.SourceIndex != "index.html" {
		t.Errorf("SourceIndex = %v, want %q", decoded.SourceIndex, "index.html")
	}
}

// ---------------------------------------------------------------------------
// Function: EventToDict
// ---------------------------------------------------------------------------

func Test_EventToDict(t *testing.T) {
	tests := []struct {
		name           string
		event          model.ParsedEvent
		wantKeys       []string
		wantAbsentKeys []string
		wantValues     map[string]any // specific key->value checks
	}{
		{
			name: "system event",
			event: model.ParsedEvent{
				Index: 0,
				Type:  model.EventTypeSystem,
				Text:  "init",
			},
			wantKeys:       []string{"index", "type", "text"},
			wantAbsentKeys: []string{"subtype", "tool_name", "tool_input", "tool_result", "is_error"},
			wantValues: map[string]any{
				"index": 0,
				"type":  "system",
				"text":  "init",
			},
		},
		{
			name: "assistant text",
			event: model.ParsedEvent{
				Index:   1,
				Type:    model.EventTypeAssistant,
				Subtype: model.SubtypeText,
				Text:    "hello",
			},
			wantKeys:       []string{"index", "type", "subtype", "text"},
			wantAbsentKeys: []string{"tool_name", "tool_input", "tool_result", "is_error"},
			wantValues: map[string]any{
				"subtype": "text",
				"text":    "hello",
			},
		},
		{
			name: "tool_use",
			event: model.ParsedEvent{
				Index:     2,
				Type:      model.EventTypeAssistant,
				Subtype:   model.SubtypeToolUse,
				ToolName:  "Web",
				ToolInput: map[string]any{"url": "https://example.com"},
			},
			wantKeys:       []string{"index", "type", "subtype", "tool_name", "tool_input"},
			wantAbsentKeys: []string{"text", "tool_result", "is_error"},
			wantValues: map[string]any{
				"tool_name": "Web",
			},
		},
		{
			name: "tool_result no error",
			event: model.ParsedEvent{
				Index:      3,
				Type:       model.EventTypeUser,
				Subtype:    model.SubtypeToolResult,
				ToolResult: "data",
				IsError:    false,
			},
			wantKeys:       []string{"index", "type", "subtype", "tool_result"},
			wantAbsentKeys: []string{"is_error"},
			wantValues: map[string]any{
				"tool_result": "data",
			},
		},
		{
			name: "tool_result with error",
			event: model.ParsedEvent{
				Index:      4,
				Type:       model.EventTypeUser,
				Subtype:    model.SubtypeToolResult,
				ToolResult: "err",
				IsError:    true,
			},
			wantKeys: []string{"index", "type", "subtype", "tool_result", "is_error"},
			wantValues: map[string]any{
				"is_error": true,
			},
		},
		{
			name: "result with stats from Raw",
			event: model.ParsedEvent{
				Index: 5,
				Type:  model.EventTypeResult,
				Text:  "done",
				Raw: map[string]any{
					"total_cost_usd":  1.5,
					"duration_ms":     float64(5000),
					"duration_api_ms": float64(4000),
					"num_turns":       float64(10),
					"session_id":      "abc",
					"usage":           map[string]any{"input": float64(100)},
				},
			},
			wantKeys: []string{"index", "type", "text", "cost_usd", "duration_ms", "duration_api_ms", "num_turns", "session_id", "usage"},
			wantValues: map[string]any{
				"cost_usd": 1.5,
			},
		},
		{
			name: "result with cost_usd key",
			event: model.ParsedEvent{
				Index: 6,
				Type:  model.EventTypeResult,
				Raw: map[string]any{
					"cost_usd": 0.5,
				},
			},
			wantKeys: []string{"cost_usd"},
			wantValues: map[string]any{
				"cost_usd": 0.5,
			},
		},
		{
			name: "result prefers total_cost_usd over cost_usd",
			event: model.ParsedEvent{
				Index: 7,
				Type:  model.EventTypeResult,
				Raw: map[string]any{
					"total_cost_usd": 2.0,
					"cost_usd":       1.0,
				},
			},
			wantValues: map[string]any{
				"cost_usd": 2.0,
			},
		},
		{
			name: "empty fields omitted",
			event: model.ParsedEvent{
				Index: 0,
				Type:  model.EventTypeRaw,
			},
			wantKeys:       []string{"index", "type"},
			wantAbsentKeys: []string{"subtype", "text", "tool_name", "tool_input", "tool_result", "is_error", "cost_usd", "duration_ms", "duration_api_ms", "num_turns", "session_id", "usage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.EventToDict(tt.event)

			for _, k := range tt.wantKeys {
				if _, ok := got[k]; !ok {
					t.Errorf("expected key %q in result, got keys: %v", k, keys(got))
				}
			}

			for _, k := range tt.wantAbsentKeys {
				if _, ok := got[k]; ok {
					t.Errorf("unexpected key %q in result map: %v", k, got)
				}
			}

			for k, want := range tt.wantValues {
				gotVal, ok := got[k]
				if !ok {
					t.Errorf("expected key %q with value %v, but key missing", k, want)
					continue
				}
				if !valuesEqual(gotVal, want) {
					t.Errorf("key %q = %v (%T), want %v (%T)", k, gotVal, gotVal, want, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// keys returns the keys of a map for diagnostic output.
func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// valuesEqual compares two values, handling numeric type coercion
// (int vs float64) which is common with JSON/map[string]any.
func valuesEqual(a, b any) bool {
	// Handle numeric comparison across types
	af, aIsFloat := toFloat64(a)
	bf, bIsFloat := toFloat64(b)
	if aIsFloat && bIsFloat {
		return af == bf
	}
	return reflect.DeepEqual(a, b)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}
