package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jamesprial/research-dashboard/internal/jobstore"
	"github.com/jamesprial/research-dashboard/internal/model"
	"github.com/jamesprial/research-dashboard/internal/server"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestServer constructs a server with a MapFS, a temp dir as cwd, and a
// nil runner (no subprocesses started in tests).
func newTestServer(t *testing.T) (*server.Server, *jobstore.Store, string) {
	t.Helper()

	staticFS := fstest.MapFS{
		"dashboard.html": &fstest.MapFile{Data: []byte("<html>dashboard</html>")},
		"reader.html":    &fstest.MapFile{Data: []byte("<html>reader</html>")},
		"shared.js":      &fstest.MapFile{Data: []byte("console.log('shared');")},
	}

	store := jobstore.NewStore()
	cwd := t.TempDir()
	ctx := context.Background()

	srv := server.New(store, nil, staticFS, cwd, ctx)
	return srv, store, cwd
}

func doRequest(t *testing.T, srv http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// Page handlers
// ---------------------------------------------------------------------------

func Test_HandleDashboard_ServesHTML(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := doRequest(t, srv, http.MethodGet, "/", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rr.Body.String(), "dashboard") {
		t.Errorf("body does not contain 'dashboard': %q", rr.Body.String())
	}
}

func Test_HandleReader_ServesHTML(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := doRequest(t, srv, http.MethodGet, "/reader", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rr.Body.String(), "reader") {
		t.Errorf("body does not contain 'reader': %q", rr.Body.String())
	}
}

func Test_HandleStatic_ServesFile(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := doRequest(t, srv, http.MethodGet, "/static/shared.js", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "shared") {
		t.Errorf("body does not contain 'shared': %q", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /research
// ---------------------------------------------------------------------------

func Test_HandleStartResearch_ValidBody_Returns201(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body := `{"query":"test topic","model":"opus","max_turns":10}`
	rr := doRequest(t, srv, http.MethodPost, "/research", body)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var status model.JobStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if status.ID == "" {
		t.Error("ID is empty, want non-empty")
	}
	if status.Query != "test topic" {
		t.Errorf("Query = %q, want %q", status.Query, "test topic")
	}
	if status.Status != model.StatusPending {
		t.Errorf("Status = %q, want %q", status.Status, model.StatusPending)
	}
}

func Test_HandleStartResearch_EmptyQuery_Returns400(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body := `{"query":"","model":"opus","max_turns":10}`
	rr := doRequest(t, srv, http.MethodPost, "/research", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error field in response")
	}
}

func Test_HandleStartResearch_InvalidModel_Returns400(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body := `{"query":"valid query","model":"unknown-model","max_turns":10}`
	rr := doRequest(t, srv, http.MethodPost, "/research", body)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error field in response")
	}
}

func Test_HandleStartResearch_InvalidJSON_Returns400(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := doRequest(t, srv, http.MethodPost, "/research", "not-json{{{")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// GET /research
// ---------------------------------------------------------------------------

func Test_HandleListResearch_ReturnsJobList(t *testing.T) {
	srv, store, cwd := newTestServer(t)

	// Create a past-run directory so PastRuns returns something.
	pastDir := filepath.Join(cwd, "research-past-run-20240101")
	if err := os.MkdirAll(pastDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a job in the store.
	_ = store.Create("list-test-id", "query", "opus", 10, cwd)

	rr := doRequest(t, srv, http.MethodGet, "/research", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var list model.JobList
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if list.Active == nil {
		t.Error("Active is nil, want non-nil slice")
	}
	if list.Past == nil {
		t.Error("Past is nil, want non-nil slice")
	}
	if len(list.Active) != 1 {
		t.Errorf("len(Active) = %d, want 1", len(list.Active))
	}
	if len(list.Past) != 1 {
		t.Errorf("len(Past) = %d, want 1", len(list.Past))
	}
}

func Test_HandleListResearch_EmptyStore_ReturnsEmptyArrays(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := doRequest(t, srv, http.MethodGet, "/research", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var list model.JobList
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(list.Active) != 0 {
		t.Errorf("len(Active) = %d, want 0", len(list.Active))
	}
	if len(list.Past) != 0 {
		t.Errorf("len(Past) = %d, want 0", len(list.Past))
	}
}

// ---------------------------------------------------------------------------
// GET /research/{id}
// ---------------------------------------------------------------------------

func Test_HandleGetResearch_ExistingJob_ReturnsDetail(t *testing.T) {
	srv, store, cwd := newTestServer(t)
	_ = store.Create("get-test-id", "detail query", "sonnet", 50, cwd)

	rr := doRequest(t, srv, http.MethodGet, "/research/get-test-id", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var detail model.JobDetail
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if detail.ID != "get-test-id" {
		t.Errorf("ID = %q, want %q", detail.ID, "get-test-id")
	}
	if detail.Query != "detail query" {
		t.Errorf("Query = %q, want %q", detail.Query, "detail query")
	}
	if detail.Events == nil {
		t.Error("Events is nil, want non-nil slice")
	}
}

func Test_HandleGetResearch_NonExistent_Returns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := doRequest(t, srv, http.MethodGet, "/research/does-not-exist", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error field in response")
	}
}

// ---------------------------------------------------------------------------
// DELETE /research/{id}
// ---------------------------------------------------------------------------

func Test_HandleCancelResearch_ExistingJob_CancelsAndReturns200(t *testing.T) {
	srv, store, cwd := newTestServer(t)
	_ = store.Create("cancel-test-id", "query", "opus", 10, cwd)

	rr := doRequest(t, srv, http.MethodDelete, "/research/cancel-test-id", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var status model.JobStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if status.Status != model.StatusCancelled {
		t.Errorf("Status = %q, want %q", status.Status, model.StatusCancelled)
	}

	// Verify the store also reflects cancellation.
	job, ok := store.Get("cancel-test-id")
	if !ok {
		t.Fatal("job not found in store after cancel")
	}
	if job.Status() != model.StatusCancelled {
		t.Errorf("job.Status() = %q, want %q", job.Status(), model.StatusCancelled)
	}
}

func Test_HandleCancelResearch_NonExistent_Returns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := doRequest(t, srv, http.MethodDelete, "/research/no-such-job", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// GET /research/past/{dir}/report
// ---------------------------------------------------------------------------

func Test_HandleGetPastReport_ExistingReport_ReturnsContent(t *testing.T) {
	srv, _, cwd := newTestServer(t)

	dirName := "research-past-20240101"
	pastDir := filepath.Join(cwd, dirName)
	if err := os.MkdirAll(pastDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportContent := "# My Research Report\n\nFindings here."
	if err := os.WriteFile(filepath.Join(pastDir, "report.md"), []byte(reportContent), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := doRequest(t, srv, http.MethodGet, "/research/past/"+dirName+"/report", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if rr.Body.String() != reportContent {
		t.Errorf("body = %q, want %q", rr.Body.String(), reportContent)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

func Test_HandleGetPastReport_NoReport_Returns404(t *testing.T) {
	srv, _, cwd := newTestServer(t)

	dirName := "research-no-report-20240101"
	pastDir := filepath.Join(cwd, dirName)
	if err := os.MkdirAll(pastDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rr := doRequest(t, srv, http.MethodGet, "/research/past/"+dirName+"/report", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func Test_HandleGetPastReport_InvalidDirName_Returns400(t *testing.T) {
	srv, _, _ := newTestServer(t)

	// Name does not start with "research-".
	rr := doRequest(t, srv, http.MethodGet, "/research/past/evil-dir/report", "")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error field in response")
	}
}

// ---------------------------------------------------------------------------
// GET /research/past/{dir}/files
// ---------------------------------------------------------------------------

func Test_HandleListPastFiles_ReturnsFileList(t *testing.T) {
	srv, _, cwd := newTestServer(t)

	dirName := "research-files-20240101"
	pastDir := filepath.Join(cwd, dirName)
	if err := os.MkdirAll(pastDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pastDir, "report.md"), []byte("# Report"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pastDir, "notes.md"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := doRequest(t, srv, http.MethodGet, "/research/past/"+dirName+"/files", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp model.FileListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.DirName != dirName {
		t.Errorf("DirName = %q, want %q", resp.DirName, dirName)
	}
	if len(resp.Files) != 2 {
		t.Errorf("len(Files) = %d, want 2", len(resp.Files))
	}
	if resp.Sources == nil {
		t.Error("Sources is nil, want non-nil slice")
	}
}

func Test_HandleListPastFiles_InvalidDirName_Returns400(t *testing.T) {
	srv, _, _ := newTestServer(t)

	rr := doRequest(t, srv, http.MethodGet, "/research/past/not-research-dir/files", "")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// GET /research/past/{dir}/files/{path}
// ---------------------------------------------------------------------------

func Test_HandleGetPastFile_ServesSingleFile(t *testing.T) {
	srv, _, cwd := newTestServer(t)

	dirName := "research-getfile-20240101"
	pastDir := filepath.Join(cwd, dirName)
	if err := os.MkdirAll(pastDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fileContent := "# Report content"
	if err := os.WriteFile(filepath.Join(pastDir, "report.md"), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := doRequest(t, srv, http.MethodGet, "/research/past/"+dirName+"/files/report.md", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Report content") {
		t.Errorf("body does not contain file content: %q", rr.Body.String())
	}
}

func Test_HandleGetPastFile_PathTraversalRejected(t *testing.T) {
	srv, _, _ := newTestServer(t)

	// Attempt path traversal via encoded ..
	rr := doRequest(t, srv, http.MethodGet, "/research/past/research-x-20240101/files/..%2F..%2Fetc%2Fpasswd", "")

	// Should return 400 (bad path) not 200.
	if rr.Code == http.StatusOK {
		t.Errorf("status = %d, want non-200 for path traversal attempt", rr.Code)
	}
}

func Test_HandleGetPastFile_InvalidDirName_Returns400(t *testing.T) {
	srv, _, _ := newTestServer(t)

	rr := doRequest(t, srv, http.MethodGet, "/research/past/malicious-dir/files/report.md", "")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// GET /research/{id}/report
// ---------------------------------------------------------------------------

func Test_HandleGetReport_ExistingReport_ReturnsContent(t *testing.T) {
	srv, store, cwd := newTestServer(t)

	outputDir := filepath.Join(cwd, "research-output-20240101")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportContent := "# Job Report"
	if err := os.WriteFile(filepath.Join(outputDir, "report.md"), []byte(reportContent), 0o644); err != nil {
		t.Fatal(err)
	}

	job := store.Create("report-job-id", "query", "opus", 10, cwd)
	job.SetOutputDir(outputDir)

	rr := doRequest(t, srv, http.MethodGet, "/research/report-job-id/report", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if rr.Body.String() != reportContent {
		t.Errorf("body = %q, want %q", rr.Body.String(), reportContent)
	}
}

func Test_HandleGetReport_NoOutputDir_Returns404(t *testing.T) {
	srv, store, cwd := newTestServer(t)
	_ = store.Create("no-output-job", "query", "opus", 10, cwd)

	rr := doRequest(t, srv, http.MethodGet, "/research/no-output-job/report", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func Test_HandleGetReport_NonExistentJob_Returns404(t *testing.T) {
	srv, _, _ := newTestServer(t)

	rr := doRequest(t, srv, http.MethodGet, "/research/ghost-job/report", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// GET /research/{id}/files
// ---------------------------------------------------------------------------

func Test_HandleListJobFiles_NoOutputDir_Returns404(t *testing.T) {
	srv, store, cwd := newTestServer(t)
	_ = store.Create("no-files-job", "query", "opus", 10, cwd)

	rr := doRequest(t, srv, http.MethodGet, "/research/no-files-job/files", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func Test_HandleListJobFiles_WithOutputDir_ReturnsList(t *testing.T) {
	srv, store, cwd := newTestServer(t)

	outputDir := filepath.Join(cwd, "research-job-files-20240101")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "report.md"), []byte("report"), 0o644); err != nil {
		t.Fatal(err)
	}

	job := store.Create("files-job-id", "query", "opus", 10, cwd)
	job.SetOutputDir(outputDir)

	rr := doRequest(t, srv, http.MethodGet, "/research/files-job-id/files", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp model.FileListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Files) != 1 {
		t.Errorf("len(Files) = %d, want 1", len(resp.Files))
	}
	if resp.Files[0].Name != "report.md" {
		t.Errorf("Files[0].Name = %q, want %q", resp.Files[0].Name, "report.md")
	}
}

// ---------------------------------------------------------------------------
// Sources subdirectory and source index
// ---------------------------------------------------------------------------

func Test_HandleListPastFiles_WithSources_IncludesSourceEntries(t *testing.T) {
	srv, _, cwd := newTestServer(t)

	dirName := "research-sources-20240101"
	pastDir := filepath.Join(cwd, dirName)
	sourcesDir := filepath.Join(pastDir, "sources")
	if err := os.MkdirAll(sourcesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pastDir, "report.md"), []byte("report"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcesDir, "source1.md"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexContent := "# Source Index"
	if err := os.WriteFile(filepath.Join(sourcesDir, "index.md"), []byte(indexContent), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := doRequest(t, srv, http.MethodGet, "/research/past/"+dirName+"/files", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp model.FileListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// sources should include source1.md and index.md
	if len(resp.Sources) < 1 {
		t.Errorf("len(Sources) = %d, want >= 1", len(resp.Sources))
	}
	if resp.SourceIndex == nil {
		t.Error("SourceIndex is nil, want non-nil")
	} else if *resp.SourceIndex != indexContent {
		t.Errorf("SourceIndex = %q, want %q", *resp.SourceIndex, indexContent)
	}
}

// ---------------------------------------------------------------------------
// GET /research/{id}/stream  (SSE)
// ---------------------------------------------------------------------------

func Test_HandleStreamResearch_NonExistentJob_Returns404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rr := doRequest(t, srv, http.MethodGet, "/research/no-such-job/stream", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func Test_HandleStreamResearch_CompletedJob_EmitsEventsAndDone(t *testing.T) {
	srv, store, cwd := newTestServer(t)

	job := store.Create("sse-test", "query", "opus", 10, cwd)
	job.AddEvent(model.ParsedEvent{Index: 0, Type: model.EventTypeSystem, Text: "init"})
	job.AddEvent(model.ParsedEvent{Index: 1, Type: model.EventTypeAssistant, Subtype: model.SubtypeText, Text: "hello"})
	job.AddEvent(model.ParsedEvent{Index: 2, Type: model.EventTypeResult, Text: "done"})
	job.SetStatus(model.StatusCompleted)
	job.SetOutputDir(filepath.Join(cwd, "output"))

	// Use a context with timeout so the SSE handler's ticker fires and exits.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/research/sse-test/stream", nil)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Verify SSE headers.
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	body := rr.Body.String()

	// Should contain data lines for the events.
	dataCount := strings.Count(body, "data: ")
	if dataCount < 3 {
		t.Errorf("expected at least 3 data: lines, got %d; body:\n%s", dataCount, body)
	}

	// Should contain the done sentinel.
	if !strings.Contains(body, "event: done") {
		t.Errorf("body missing 'event: done' sentinel; body:\n%s", body)
	}
}

func Test_HandleStreamResearch_AfterParam_SetsCursor(t *testing.T) {
	srv, store, cwd := newTestServer(t)

	job := store.Create("sse-after", "query", "opus", 10, cwd)
	job.AddEvent(model.ParsedEvent{Index: 0, Type: model.EventTypeSystem, Text: "init"})
	job.AddEvent(model.ParsedEvent{Index: 1, Type: model.EventTypeAssistant, Subtype: model.SubtypeText, Text: "hello"})
	job.AddEvent(model.ParsedEvent{Index: 2, Type: model.EventTypeResult, Text: "done"})
	job.SetStatus(model.StatusCompleted)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/research/sse-after/stream?after=2", nil)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	body := rr.Body.String()

	// With after=2, only event at index 2 should be emitted (plus done sentinel).
	// The data lines should contain the result event but NOT the first two.
	dataCount := strings.Count(body, "data: ")
	// 1 event data line + 1 done data line = 2
	if dataCount != 2 {
		t.Errorf("expected 2 data: lines with after=2, got %d; body:\n%s", dataCount, body)
	}
}

// ---------------------------------------------------------------------------
// GET /research/{id}/files/{path...}  (active job file serving)
// ---------------------------------------------------------------------------

func Test_HandleGetJobFile_ServesFile(t *testing.T) {
	srv, store, cwd := newTestServer(t)

	outputDir := filepath.Join(cwd, "research-jobfile-20240101")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "file content here"
	if err := os.WriteFile(filepath.Join(outputDir, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	job := store.Create("file-job", "query", "opus", 10, cwd)
	job.SetOutputDir(outputDir)

	rr := doRequest(t, srv, http.MethodGet, "/research/file-job/files/notes.txt", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), content) {
		t.Errorf("body does not contain file content: %q", rr.Body.String())
	}
}

func Test_HandleGetJobFile_PathTraversal_Returns400(t *testing.T) {
	srv, store, cwd := newTestServer(t)

	outputDir := filepath.Join(cwd, "research-traversal-20240101")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	job := store.Create("traversal-job", "query", "opus", 10, cwd)
	job.SetOutputDir(outputDir)

	rr := doRequest(t, srv, http.MethodGet, "/research/traversal-job/files/..%2F..%2Fetc%2Fpasswd", "")

	if rr.Code == http.StatusOK {
		t.Errorf("status = %d, want non-200 for path traversal attempt", rr.Code)
	}
}

func Test_HandleGetJobFile_NoOutputDir_Returns404(t *testing.T) {
	srv, store, cwd := newTestServer(t)
	_ = store.Create("no-dir-job", "query", "opus", 10, cwd)

	rr := doRequest(t, srv, http.MethodGet, "/research/no-dir-job/files/report.md", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func Test_HandleGetJobFile_NonExistentJob_Returns404(t *testing.T) {
	srv, _, _ := newTestServer(t)

	rr := doRequest(t, srv, http.MethodGet, "/research/ghost-job/files/report.md", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}
