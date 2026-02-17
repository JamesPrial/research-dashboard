package jobstore_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jamesprial/research-dashboard/internal/jobstore"
	"github.com/jamesprial/research-dashboard/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

// makeEvents returns n ParsedEvents with sequential indices and the given type/subtype.
func makeEvents(n int, typ model.EventType, sub model.EventSubtype) []model.ParsedEvent {
	evts := make([]model.ParsedEvent, n)
	for i := range evts {
		evts[i] = model.ParsedEvent{
			Index:   i,
			Type:    typ,
			Subtype: sub,
			Text:    "text",
		}
	}
	return evts
}

// ---------------------------------------------------------------------------
// NewStore
// ---------------------------------------------------------------------------

func Test_NewStore_ReturnsNonNil(t *testing.T) {
	s := jobstore.NewStore()
	if s == nil {
		t.Fatal("NewStore() returned nil, want non-nil *Store")
	}
}

// ---------------------------------------------------------------------------
// Store.Create
// ---------------------------------------------------------------------------

func Test_Store_Create(t *testing.T) {
	t.Run("returns non-nil Job with matching fields", func(t *testing.T) {
		s := jobstore.NewStore()
		j := s.Create("abc", "test", "opus", 100, "/tmp")
		if j == nil {
			t.Fatal("Create() returned nil")
		}
	})

	t.Run("initial status is pending", func(t *testing.T) {
		s := jobstore.NewStore()
		j := s.Create("abc", "test", "opus", 100, "/tmp")
		if j.Status() != model.StatusPending {
			t.Errorf("Status() = %q, want %q", j.Status(), model.StatusPending)
		}
	})

	t.Run("initial events is zero", func(t *testing.T) {
		s := jobstore.NewStore()
		j := s.Create("abc", "test", "opus", 100, "/tmp")
		if j.EventCount() != 0 {
			t.Errorf("EventCount() = %d, want 0", j.EventCount())
		}
	})

	t.Run("created_at is non-empty ISO 8601 string", func(t *testing.T) {
		s := jobstore.NewStore()
		j := s.Create("abc", "test", "opus", 100, "/tmp")
		status := j.ToStatus()
		if status.CreatedAt == "" {
			t.Fatal("CreatedAt is empty, want non-empty ISO 8601 string")
		}
		// Verify it parses as a valid time.
		_, err := time.Parse(time.RFC3339, status.CreatedAt)
		if err != nil {
			t.Errorf("CreatedAt %q is not valid RFC3339: %v", status.CreatedAt, err)
		}
	})
}

// ---------------------------------------------------------------------------
// Store.Get
// ---------------------------------------------------------------------------

func Test_Store_Get(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(s *jobstore.Store)
		getID     string
		wantFound bool
	}{
		{
			name: "existing job returns job and true",
			setup: func(s *jobstore.Store) {
				_ = s.Create("job-1", "query", "opus", 50, "/tmp")
			},
			getID:     "job-1",
			wantFound: true,
		},
		{
			name:      "non-existent returns nil and false",
			setup:     func(s *jobstore.Store) {},
			getID:     "nonexistent",
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := jobstore.NewStore()
			tt.setup(s)
			got, ok := s.Get(tt.getID)
			if ok != tt.wantFound {
				t.Errorf("Get(%q) ok = %v, want %v", tt.getID, ok, tt.wantFound)
			}
			if tt.wantFound && got == nil {
				t.Errorf("Get(%q) returned nil job, want non-nil", tt.getID)
			}
			if !tt.wantFound && got != nil {
				t.Errorf("Get(%q) returned non-nil job, want nil", tt.getID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Store.List
// ---------------------------------------------------------------------------

func Test_Store_List(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(s *jobstore.Store)
		wantCount int
	}{
		{
			name:      "empty store returns empty slice",
			setup:     func(s *jobstore.Store) {},
			wantCount: 0,
		},
		{
			name: "one job returns slice with 1 element",
			setup: func(s *jobstore.Store) {
				_ = s.Create("j1", "q1", "opus", 10, "/tmp")
			},
			wantCount: 1,
		},
		{
			name: "multiple jobs returns correct count",
			setup: func(s *jobstore.Store) {
				_ = s.Create("j1", "q1", "opus", 10, "/tmp")
				_ = s.Create("j2", "q2", "sonnet", 20, "/tmp")
				_ = s.Create("j3", "q3", "haiku", 30, "/tmp")
			},
			wantCount: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := jobstore.NewStore()
			tt.setup(s)
			got := s.List()
			if got == nil {
				t.Fatal("List() returned nil, want non-nil slice")
			}
			if len(got) != tt.wantCount {
				t.Errorf("List() returned %d items, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Store.Delete
// ---------------------------------------------------------------------------

func Test_Store_Delete(t *testing.T) {
	t.Run("delete existing job makes Get return false", func(t *testing.T) {
		s := jobstore.NewStore()
		_ = s.Create("del-1", "query", "opus", 10, "/tmp")
		s.Delete("del-1")
		_, ok := s.Get("del-1")
		if ok {
			t.Error("Get() returned true after Delete(), want false")
		}
	})

	t.Run("delete non-existent does not panic", func(t *testing.T) {
		s := jobstore.NewStore()
		// Should not panic.
		s.Delete("does-not-exist")
	})
}

// ---------------------------------------------------------------------------
// Store.CleanupExpired
// ---------------------------------------------------------------------------

func Test_Store_CleanupExpired(t *testing.T) {
	maxAge := 1 * time.Hour

	tests := []struct {
		name         string
		status       model.Status
		createdAgo   time.Duration // how far in the past the job was created
		wantSurvives bool
	}{
		{
			name:         "expired completed job is removed",
			status:       model.StatusCompleted,
			createdAgo:   2 * time.Hour,
			wantSurvives: false,
		},
		{
			name:         "expired failed job is removed",
			status:       model.StatusFailed,
			createdAgo:   2 * time.Hour,
			wantSurvives: false,
		},
		{
			name:         "expired cancelled job is removed",
			status:       model.StatusCancelled,
			createdAgo:   2 * time.Hour,
			wantSurvives: false,
		},
		{
			name:         "expired running job is NOT removed",
			status:       model.StatusRunning,
			createdAgo:   2 * time.Hour,
			wantSurvives: true,
		},
		{
			name:         "expired pending job is NOT removed",
			status:       model.StatusPending,
			createdAgo:   2 * time.Hour,
			wantSurvives: true,
		},
		{
			name:         "fresh completed job is NOT removed",
			status:       model.StatusCompleted,
			createdAgo:   10 * time.Minute,
			wantSurvives: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := jobstore.NewStore()
			j := s.Create(tt.name, "query", "opus", 10, "/tmp")
			j.SetStatus(tt.status)
			// Backdating the created_at: set it to a time in the past.
			// We use SetCreatedAt to manipulate the timestamp for testing.
			j.SetCreatedAt(time.Now().Add(-tt.createdAgo))

			s.CleanupExpired(maxAge)

			_, ok := s.Get(tt.name)
			if ok != tt.wantSurvives {
				t.Errorf("after CleanupExpired(%v): Get() found=%v, wantSurvives=%v", maxAge, ok, tt.wantSurvives)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Store.PastRuns
// ---------------------------------------------------------------------------

func Test_Store_PastRuns(t *testing.T) {
	t.Run("no research dirs returns empty slice", func(t *testing.T) {
		s := jobstore.NewStore()
		dir := t.TempDir()
		got := s.PastRuns(dir)
		if got == nil {
			t.Fatal("PastRuns() returned nil, want non-nil slice")
		}
		if len(got) != 0 {
			t.Errorf("PastRuns() returned %d items, want 0", len(got))
		}
	})

	t.Run("one research dir with report", func(t *testing.T) {
		s := jobstore.NewStore()
		dir := t.TempDir()
		researchDir := filepath.Join(dir, "research-test-20240101")
		if err := os.MkdirAll(researchDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(researchDir, "report.md"), []byte("# Report"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := s.PastRuns(dir)
		if len(got) != 1 {
			t.Fatalf("PastRuns() returned %d items, want 1", len(got))
		}
		if !got[0].HasReport {
			t.Error("HasReport = false, want true")
		}
		if got[0].Name != "research-test-20240101" {
			t.Errorf("Name = %q, want %q", got[0].Name, "research-test-20240101")
		}
	})

	t.Run("one research dir without report", func(t *testing.T) {
		s := jobstore.NewStore()
		dir := t.TempDir()
		researchDir := filepath.Join(dir, "research-test-20240101")
		if err := os.MkdirAll(researchDir, 0o755); err != nil {
			t.Fatal(err)
		}

		got := s.PastRuns(dir)
		if len(got) != 1 {
			t.Fatalf("PastRuns() returned %d items, want 1", len(got))
		}
		if got[0].HasReport {
			t.Error("HasReport = true, want false")
		}
	})

	t.Run("multiple dirs sorted by name descending", func(t *testing.T) {
		s := jobstore.NewStore()
		dir := t.TempDir()
		names := []string{
			"research-aaa-20240101",
			"research-ccc-20240103",
			"research-bbb-20240102",
		}
		for _, name := range names {
			if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
		}

		got := s.PastRuns(dir)
		if len(got) != 3 {
			t.Fatalf("PastRuns() returned %d items, want 3", len(got))
		}

		// Verify sorted descending by name.
		gotNames := make([]string, len(got))
		for i, pr := range got {
			gotNames[i] = pr.Name
		}
		if !sort.SliceIsSorted(gotNames, func(i, j int) bool {
			return gotNames[i] > gotNames[j]
		}) {
			t.Errorf("PastRuns() not sorted descending: %v", gotNames)
		}
	})

	t.Run("non-research dirs ignored", func(t *testing.T) {
		s := jobstore.NewStore()
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "output-data"), 0o755); err != nil {
			t.Fatal(err)
		}

		got := s.PastRuns(dir)
		if len(got) != 0 {
			t.Errorf("PastRuns() returned %d items, want 0 (non-research dir should be ignored)", len(got))
		}
	})

	t.Run("files ignored", func(t *testing.T) {
		s := jobstore.NewStore()
		dir := t.TempDir()
		// Create a file (not a directory) with a research-like name.
		if err := os.WriteFile(filepath.Join(dir, "research-fake"), []byte("not a dir"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := s.PastRuns(dir)
		if len(got) != 0 {
			t.Errorf("PastRuns() returned %d items, want 0 (file should be ignored)", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// Job.AddEvent / Job.EventCount
// ---------------------------------------------------------------------------

func Test_Job_AddEvent(t *testing.T) {
	t.Run("add one event", func(t *testing.T) {
		s := jobstore.NewStore()
		j := s.Create("evt-1", "query", "opus", 10, "/tmp")
		j.AddEvent(model.ParsedEvent{Index: 0, Type: model.EventTypeSystem})
		if j.EventCount() != 1 {
			t.Errorf("EventCount() = %d, want 1", j.EventCount())
		}
	})

	t.Run("add multiple events", func(t *testing.T) {
		s := jobstore.NewStore()
		j := s.Create("evt-2", "query", "opus", 10, "/tmp")
		for _, evt := range makeEvents(3, model.EventTypeSystem, model.SubtypeEmpty) {
			j.AddEvent(evt)
		}
		if j.EventCount() != 3 {
			t.Errorf("EventCount() = %d, want 3", j.EventCount())
		}
	})
}

// ---------------------------------------------------------------------------
// Job.EventsSince
// ---------------------------------------------------------------------------

func Test_Job_EventsSince(t *testing.T) {
	tests := []struct {
		name      string
		numEvents int
		cursor    int
		wantCount int
	}{
		{
			name:      "all events from cursor 0",
			numEvents: 3,
			cursor:    0,
			wantCount: 3,
		},
		{
			name:      "from middle",
			numEvents: 3,
			cursor:    1,
			wantCount: 2,
		},
		{
			name:      "from end",
			numEvents: 3,
			cursor:    3,
			wantCount: 0,
		},
		{
			name:      "cursor beyond total",
			numEvents: 3,
			cursor:    10,
			wantCount: 0,
		},
		{
			name:      "cursor 0 empty job",
			numEvents: 0,
			cursor:    0,
			wantCount: 0,
		},
		{
			name:      "negative cursor -1 returns all events",
			numEvents: 3,
			cursor:    -1,
			wantCount: 3,
		},
		{
			name:      "negative cursor -100 returns all events",
			numEvents: 3,
			cursor:    -100,
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := jobstore.NewStore()
			j := s.Create("es-"+tt.name, "query", "opus", 10, "/tmp")
			for _, evt := range makeEvents(tt.numEvents, model.EventTypeAssistant, model.SubtypeText) {
				j.AddEvent(evt)
			}

			got := j.EventsSince(tt.cursor)
			if len(got) != tt.wantCount {
				t.Errorf("EventsSince(%d) returned %d events, want %d", tt.cursor, len(got), tt.wantCount)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Job.SetStatus / Job.Status
// ---------------------------------------------------------------------------

func Test_Job_SetStatus(t *testing.T) {
	tests := []struct {
		name   string
		status model.Status
	}{
		{"set running", model.StatusRunning},
		{"set completed", model.StatusCompleted},
		{"set failed", model.StatusFailed},
		{"set cancelled", model.StatusCancelled},
		{"set pending", model.StatusPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := jobstore.NewStore()
			j := s.Create("status-"+tt.name, "query", "opus", 10, "/tmp")
			j.SetStatus(tt.status)
			if j.Status() != tt.status {
				t.Errorf("Status() = %q, want %q", j.Status(), tt.status)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Job.SetOutputDir / Job.OutputDir
// ---------------------------------------------------------------------------

func Test_Job_SetOutputDir(t *testing.T) {
	s := jobstore.NewStore()
	j := s.Create("od-1", "query", "opus", 10, "/tmp")
	j.SetOutputDir("/output/research-123")
	if j.OutputDir() != "/output/research-123" {
		t.Errorf("OutputDir() = %q, want %q", j.OutputDir(), "/output/research-123")
	}
}

// ---------------------------------------------------------------------------
// Job.SetError / Job.Error
// ---------------------------------------------------------------------------

func Test_Job_SetError(t *testing.T) {
	s := jobstore.NewStore()
	j := s.Create("err-1", "query", "opus", 10, "/tmp")
	j.SetError("something broke")
	if j.Error() != "something broke" {
		t.Errorf("Error() = %q, want %q", j.Error(), "something broke")
	}
}

// ---------------------------------------------------------------------------
// Job.SetSessionID / Job.SessionID
// ---------------------------------------------------------------------------

func Test_Job_SetSessionID(t *testing.T) {
	s := jobstore.NewStore()
	j := s.Create("sid-1", "query", "opus", 10, "/tmp")
	j.SetSessionID("sess-abc-123")
	if j.SessionID() != "sess-abc-123" {
		t.Errorf("SessionID() = %q, want %q", j.SessionID(), "sess-abc-123")
	}
}

// ---------------------------------------------------------------------------
// Job.SetResultInfo / Job.ResultInfo
// ---------------------------------------------------------------------------

func Test_Job_SetResultInfo(t *testing.T) {
	s := jobstore.NewStore()
	j := s.Create("ri-1", "query", "opus", 10, "/tmp")
	info := model.ResultStats{
		CostUSD:    ptr(1.5),
		DurationMS: ptr(5000),
		NumTurns:   ptr(10),
	}
	j.SetResultInfo(info)
	got := j.ResultInfo()
	if got.CostUSD == nil || *got.CostUSD != 1.5 {
		t.Errorf("ResultInfo().CostUSD = %v, want 1.5", got.CostUSD)
	}
	if got.DurationMS == nil || *got.DurationMS != 5000 {
		t.Errorf("ResultInfo().DurationMS = %v, want 5000", got.DurationMS)
	}
	if got.NumTurns == nil || *got.NumTurns != 10 {
		t.Errorf("ResultInfo().NumTurns = %v, want 10", got.NumTurns)
	}
}

// ---------------------------------------------------------------------------
// Job.NumTurns
// ---------------------------------------------------------------------------

func Test_Job_NumTurns(t *testing.T) {
	tests := []struct {
		name      string
		events    []model.ParsedEvent
		wantTurns int
	}{
		{
			name:      "no events",
			events:    nil,
			wantTurns: 0,
		},
		{
			name: "only system events",
			events: []model.ParsedEvent{
				{Index: 0, Type: model.EventTypeSystem},
				{Index: 1, Type: model.EventTypeSystem},
			},
			wantTurns: 0,
		},
		{
			name: "assistant text events",
			events: []model.ParsedEvent{
				{Index: 0, Type: model.EventTypeAssistant, Subtype: model.SubtypeText},
				{Index: 1, Type: model.EventTypeAssistant, Subtype: model.SubtypeText},
			},
			wantTurns: 2,
		},
		{
			name: "mixed events only counts assistant/text",
			events: []model.ParsedEvent{
				{Index: 0, Type: model.EventTypeSystem},
				{Index: 1, Type: model.EventTypeAssistant, Subtype: model.SubtypeText},
				{Index: 2, Type: model.EventTypeAssistant, Subtype: model.SubtypeText},
				{Index: 3, Type: model.EventTypeAssistant, Subtype: model.SubtypeToolUse},
				{Index: 4, Type: model.EventTypeResult},
			},
			wantTurns: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := jobstore.NewStore()
			j := s.Create("nt-"+tt.name, "query", "opus", 10, "/tmp")
			for _, evt := range tt.events {
				j.AddEvent(evt)
			}
			if j.NumTurns() != tt.wantTurns {
				t.Errorf("NumTurns() = %d, want %d", j.NumTurns(), tt.wantTurns)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Job.ToStatus
// ---------------------------------------------------------------------------

func Test_Job_ToStatus(t *testing.T) {
	t.Run("pending job with no events", func(t *testing.T) {
		s := jobstore.NewStore()
		j := s.Create("ts-1", "test query", "opus", 100, "/tmp")
		st := j.ToStatus()

		if st.ID != "ts-1" {
			t.Errorf("ID = %q, want %q", st.ID, "ts-1")
		}
		if st.Query != "test query" {
			t.Errorf("Query = %q, want %q", st.Query, "test query")
		}
		if string(st.Model) != "opus" {
			t.Errorf("Model = %q, want %q", st.Model, "opus")
		}
		if st.Status != model.StatusPending {
			t.Errorf("Status = %q, want %q", st.Status, model.StatusPending)
		}
		if st.NumTurns != 0 {
			t.Errorf("NumTurns = %d, want 0", st.NumTurns)
		}
		if st.OutputLines != 0 {
			t.Errorf("OutputLines = %d, want 0", st.OutputLines)
		}
		if st.MaxTurns != 100 {
			t.Errorf("MaxTurns = %d, want 100", st.MaxTurns)
		}
	})

	t.Run("running job with events", func(t *testing.T) {
		s := jobstore.NewStore()
		j := s.Create("ts-2", "query2", "sonnet", 50, "/tmp")
		j.SetStatus(model.StatusRunning)

		// Add mixed events: 1 system, 2 assistant/text, 1 tool_use.
		j.AddEvent(model.ParsedEvent{Index: 0, Type: model.EventTypeSystem})
		j.AddEvent(model.ParsedEvent{Index: 1, Type: model.EventTypeAssistant, Subtype: model.SubtypeText})
		j.AddEvent(model.ParsedEvent{Index: 2, Type: model.EventTypeAssistant, Subtype: model.SubtypeText})
		j.AddEvent(model.ParsedEvent{Index: 3, Type: model.EventTypeAssistant, Subtype: model.SubtypeToolUse})

		st := j.ToStatus()

		if st.Status != model.StatusRunning {
			t.Errorf("Status = %q, want %q", st.Status, model.StatusRunning)
		}
		// num_turns counts only assistant/text events.
		if st.NumTurns != 2 {
			t.Errorf("NumTurns = %d, want 2", st.NumTurns)
		}
		// output_lines = total event count.
		if st.OutputLines != 4 {
			t.Errorf("OutputLines = %d, want 4", st.OutputLines)
		}
	})
}

// ---------------------------------------------------------------------------
// Job.ToDetail
// ---------------------------------------------------------------------------

func Test_Job_ToDetail(t *testing.T) {
	s := jobstore.NewStore()
	j := s.Create("td-1", "detail query", "haiku", 25, "/tmp")
	j.SetStatus(model.StatusCompleted)
	j.SetError("oops")
	j.SetSessionID("sess-detail")
	j.SetResultInfo(model.ResultStats{CostUSD: ptr(0.5)})
	j.SetOutputDir("/output/detail")

	j.AddEvent(model.ParsedEvent{Index: 0, Type: model.EventTypeSystem, Text: "init"})
	j.AddEvent(model.ParsedEvent{Index: 1, Type: model.EventTypeAssistant, Subtype: model.SubtypeText, Text: "hello"})

	detail := j.ToDetail()

	// Verify JobStatus fields.
	if detail.ID != "td-1" {
		t.Errorf("ID = %q, want %q", detail.ID, "td-1")
	}
	if detail.Query != "detail query" {
		t.Errorf("Query = %q, want %q", detail.Query, "detail query")
	}
	if string(detail.Model) != "haiku" {
		t.Errorf("Model = %q, want %q", detail.Model, "haiku")
	}
	if detail.Status != model.StatusCompleted {
		t.Errorf("Status = %q, want %q", detail.Status, model.StatusCompleted)
	}
	if detail.MaxTurns != 25 {
		t.Errorf("MaxTurns = %d, want 25", detail.MaxTurns)
	}

	// Verify events.
	if detail.Events == nil {
		t.Fatal("Events is nil, want non-nil slice")
	}
	if len(detail.Events) != 2 {
		t.Errorf("len(Events) = %d, want 2", len(detail.Events))
	}

	// Verify error.
	if detail.Error == nil {
		t.Fatal("Error is nil, want non-nil")
	}
	if *detail.Error != "oops" {
		t.Errorf("Error = %q, want %q", *detail.Error, "oops")
	}

	// Verify session_id.
	if detail.SessionID == nil {
		t.Fatal("SessionID is nil, want non-nil")
	}
	if *detail.SessionID != "sess-detail" {
		t.Errorf("SessionID = %q, want %q", *detail.SessionID, "sess-detail")
	}

	// Verify result_info.
	if detail.ResultInfo == nil {
		t.Fatal("ResultInfo is nil, want non-nil")
	}
	if detail.ResultInfo.CostUSD == nil || *detail.ResultInfo.CostUSD != 0.5 {
		t.Errorf("ResultInfo.CostUSD = %v, want 0.5", detail.ResultInfo.CostUSD)
	}
}

// ---------------------------------------------------------------------------
// Store.ClaimDir / Store.ReleaseDir
// ---------------------------------------------------------------------------

func Test_Store_ClaimDir(t *testing.T) {
	t.Run("claim unclaimed dir returns true", func(t *testing.T) {
		s := jobstore.NewStore()
		if !s.ClaimDir("/tmp/research-test") {
			t.Error("ClaimDir() = false, want true for unclaimed dir")
		}
	})

	t.Run("claim already claimed dir returns false", func(t *testing.T) {
		s := jobstore.NewStore()
		s.ClaimDir("/tmp/research-test")
		if s.ClaimDir("/tmp/research-test") {
			t.Error("ClaimDir() = true, want false for already claimed dir")
		}
	})
}

func Test_Store_ReleaseDir(t *testing.T) {
	t.Run("release then claim succeeds", func(t *testing.T) {
		s := jobstore.NewStore()
		s.ClaimDir("/tmp/research-test")
		s.ReleaseDir("/tmp/research-test")
		if !s.ClaimDir("/tmp/research-test") {
			t.Error("ClaimDir() = false after ReleaseDir(), want true")
		}
	})
}

// ---------------------------------------------------------------------------
// Concurrency Tests
// ---------------------------------------------------------------------------

func Test_Concurrent_AddEvent(t *testing.T) {
	s := jobstore.NewStore()
	j := s.Create("concurrent-add", "query", "opus", 200, "/tmp")

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			j.AddEvent(model.ParsedEvent{
				Index:   idx,
				Type:    model.EventTypeAssistant,
				Subtype: model.SubtypeText,
				Text:    "concurrent event",
			})
		}(i)
	}

	wg.Wait()

	if j.EventCount() != numGoroutines {
		t.Errorf("EventCount() = %d, want %d after concurrent adds", j.EventCount(), numGoroutines)
	}
}

func Test_Concurrent_Create_And_Get(t *testing.T) {
	s := jobstore.NewStore()

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Launch creators.
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-%d", idx)
			_ = s.Create(id, "query", "opus", 10, "/tmp")
		}(i)
	}

	// Launch getters concurrently.
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-%d", idx)
			// It is fine if the job does not exist yet; we just verify no race.
			_, _ = s.Get(id)
		}(i)
	}

	wg.Wait()
}

func Test_Concurrent_EventsSince_During_AddEvent(t *testing.T) {
	s := jobstore.NewStore()
	j := s.Create("concurrent-es", "query", "opus", 200, "/tmp")

	const numEvents = 100
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine: adds events.
	go func() {
		defer wg.Done()
		for i := 0; i < numEvents; i++ {
			j.AddEvent(model.ParsedEvent{
				Index:   i,
				Type:    model.EventTypeAssistant,
				Subtype: model.SubtypeText,
				Text:    "streaming",
			})
		}
	}()

	// Reader goroutine: reads events continuously.
	go func() {
		defer wg.Done()
		cursor := 0
		for cursor < numEvents {
			evts := j.EventsSince(cursor)
			cursor += len(evts)
			// Yield to allow writer to proceed.
			time.Sleep(time.Microsecond)
		}
	}()

	wg.Wait()

	// After both finish, all events should be present.
	if j.EventCount() != numEvents {
		t.Errorf("EventCount() = %d, want %d", j.EventCount(), numEvents)
	}
}
