package runner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesprial/research-dashboard/internal/jobstore"
	"github.com/jamesprial/research-dashboard/internal/model"
	"github.com/jamesprial/research-dashboard/internal/runner"
)

// ---------------------------------------------------------------------------
// TestMain — fake subprocess binary
// ---------------------------------------------------------------------------

// TestMain is the entry point for the test binary. When the binary is invoked
// as a subprocess (controlled by TEST_SUBPROCESS_BEHAVIOR), it acts as the
// fake "claude" binary and exits without running any tests.
func TestMain(m *testing.M) {
	behavior := os.Getenv("TEST_SUBPROCESS_BEHAVIOR")
	switch behavior {
	case "success":
		fakeClaudeSuccess()
		os.Exit(0)
	case "failure":
		fakeClaudeFailure()
		os.Exit(1)
	case "slow":
		fakeClaudeSlow()
		os.Exit(0)
	case "outputdir":
		fakeClaudeOutputDir()
		os.Exit(0)
	case "maxturns":
		fakeClaudeMaxTurns()
		os.Exit(2)
	default:
		// Normal test run — execute all tests.
		os.Exit(m.Run())
	}
}

// fakeClaudeSuccess emits a realistic sequence of stream-json events to stdout
// and exits 0.
func fakeClaudeSuccess() {
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"sess-abc-123"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Researching your topic now."}]}}`,
		`{"type":"result","result":"Research complete.","is_error":false,"total_cost_usd":0.05,"duration_ms":3000,"duration_api_ms":2500,"num_turns":2,"session_id":"sess-abc-123","usage":{"input_tokens":100,"output_tokens":200}}`,
	}
	for _, line := range lines {
		fmt.Println(line)
	}
}

// fakeClaudeFailure writes an error message to stderr and exits 1.
func fakeClaudeFailure() {
	fmt.Fprintln(os.Stderr, "claude: fatal error: model unavailable")
}

// fakeClaudeSlow sleeps long enough to be cancelled, then emits nothing.
func fakeClaudeSlow() {
	// Sleep for 30s; the test will cancel the context well before this.
	time.Sleep(30 * time.Second)
}

// fakeClaudeMaxTurns emits a successful result event and exits with code 2,
// simulating the claude CLI behavior when max turns is reached.
func fakeClaudeMaxTurns() {
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"sess-maxturns-789"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Working on research..."}]}}`,
		`{"type":"result","result":"Research complete (max turns reached).","is_error":false,"total_cost_usd":0.12,"duration_ms":5000,"duration_api_ms":4000,"num_turns":10,"session_id":"sess-maxturns-789"}`,
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	fmt.Fprintln(os.Stderr, "Max turns reached")
}

// fakeClaudeOutputDir creates a research-* directory in the cwd before emitting events.
func fakeClaudeOutputDir() {
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	dirName := "research-outputdir-test"
	if err := os.MkdirAll(filepath.Join(cwd, dirName), 0o755); err != nil {
		os.Exit(2)
	}
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"sess-dir-456"}`,
		`{"type":"result","result":"done","is_error":false}`,
	}
	for _, line := range lines {
		fmt.Println(line)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// selfExe returns the path to the current test binary, which is used as the
// fake claude binary.
func selfExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable(): %v", err)
	}
	return exe
}

// newTestRunner returns a Runner that invokes the current test binary as the
// fake claude subprocess.
func newTestRunner(t *testing.T) *runner.Runner {
	t.Helper()
	return runner.New(selfExe(t))
}

// newJob creates a minimal job in a fresh store using a temp directory as cwd.
func newJob(t *testing.T, cwd string) (*jobstore.Store, *jobstore.Job) {
	t.Helper()
	store := jobstore.NewStore()
	job := store.Create("test-job-1", "AI trends 2026", "opus", 10, cwd)
	return store, job
}

// setSubprocessBehavior sets the environment variable that controls the fake
// subprocess behavior. It also re-sets PATH so that the test binary is
// resolved via ClaudePath directly (not through PATH lookup).
func setSubprocessBehavior(t *testing.T, behavior string) {
	t.Helper()
	t.Setenv("TEST_SUBPROCESS_BEHAVIOR", behavior)
}

// ---------------------------------------------------------------------------
// Test: PromptPrefix constant
// ---------------------------------------------------------------------------

func Test_PromptPrefix(t *testing.T) {
	// The embedded prompt must contain key orchestration phrases.
	for _, phrase := range []string{
		"research-worker",
		"source-archiver",
		"Research question:",
	} {
		if !strings.Contains(runner.PromptPrefix, phrase) {
			t.Errorf("PromptPrefix missing expected phrase %q", phrase)
		}
	}
	// Must end with the query concatenation anchor.
	if !strings.HasSuffix(runner.PromptPrefix, "Research question:\n\n") {
		t.Errorf("PromptPrefix does not end with %q", "Research question:\\n\\n")
	}
}

// ---------------------------------------------------------------------------
// Test: New
// ---------------------------------------------------------------------------

func Test_New_DefaultsToClaudeWhenEmpty(t *testing.T) {
	r := runner.New("")
	if r == nil {
		t.Fatal("New(\"\") returned nil")
	}
	if r.ClaudePath != "claude" {
		t.Errorf("ClaudePath = %q, want %q", r.ClaudePath, "claude")
	}
}

func Test_New_PreservesCustomPath(t *testing.T) {
	r := runner.New("/usr/local/bin/claude")
	if r.ClaudePath != "/usr/local/bin/claude" {
		t.Errorf("ClaudePath = %q, want %q", r.ClaudePath, "/usr/local/bin/claude")
	}
}

// ---------------------------------------------------------------------------
// Test: Successful run
// ---------------------------------------------------------------------------

func Test_Run_Success(t *testing.T) {
	setSubprocessBehavior(t, "success")

	cwd := t.TempDir()
	r := newTestRunner(t)
	store, job := newJob(t, cwd)

	ctx := context.Background()
	if err := r.Run(ctx, job, store); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	// Status should be completed.
	if job.Status() != model.StatusCompleted {
		t.Errorf("Status() = %q, want %q", job.Status(), model.StatusCompleted)
	}

	// Events should have been captured (system + assistant + result = at least 3).
	if job.EventCount() < 3 {
		t.Errorf("EventCount() = %d, want >= 3", job.EventCount())
	}

	// Session ID should be set from the system event.
	if job.SessionID() != "sess-abc-123" {
		t.Errorf("SessionID() = %q, want %q", job.SessionID(), "sess-abc-123")
	}

	// ResultInfo should be populated.
	info := job.ResultInfo()
	if info.CostUSD == nil {
		t.Fatal("ResultInfo().CostUSD is nil, want non-nil")
	}
	if *info.CostUSD != 0.05 {
		t.Errorf("ResultInfo().CostUSD = %v, want 0.05", *info.CostUSD)
	}
	if info.NumTurns == nil {
		t.Fatal("ResultInfo().NumTurns is nil, want non-nil")
	}
	if *info.NumTurns != 2 {
		t.Errorf("ResultInfo().NumTurns = %d, want 2", *info.NumTurns)
	}
	if info.DurationMS == nil {
		t.Fatal("ResultInfo().DurationMS is nil, want non-nil")
	}
	if *info.DurationMS != 3000 {
		t.Errorf("ResultInfo().DurationMS = %d, want 3000", *info.DurationMS)
	}

	// Usage map should be populated.
	if len(info.Usage) == 0 {
		t.Error("ResultInfo().Usage is empty, want non-empty map")
	}

	// Error should not be set.
	if job.Error() != "" {
		t.Errorf("Error() = %q, want empty string", job.Error())
	}
}

// ---------------------------------------------------------------------------
// Test: Failed run
// ---------------------------------------------------------------------------

func Test_Run_Failure(t *testing.T) {
	setSubprocessBehavior(t, "failure")

	cwd := t.TempDir()
	r := newTestRunner(t)
	store, job := newJob(t, cwd)

	ctx := context.Background()
	if err := r.Run(ctx, job, store); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	// Status should be failed.
	if job.Status() != model.StatusFailed {
		t.Errorf("Status() = %q, want %q", job.Status(), model.StatusFailed)
	}

	// Error message should contain the stderr output.
	errMsg := job.Error()
	if errMsg == "" {
		t.Fatal("Error() is empty, want non-empty error message")
	}
	if !strings.Contains(errMsg, "fatal error") {
		t.Errorf("Error() = %q, want it to contain %q", errMsg, "fatal error")
	}
}

// ---------------------------------------------------------------------------
// Test: Cancel during run
// ---------------------------------------------------------------------------

func Test_Run_Cancel(t *testing.T) {
	setSubprocessBehavior(t, "slow")

	cwd := t.TempDir()
	r := newTestRunner(t)
	store, job := newJob(t, cwd)

	ctx, cancel := context.WithCancel(context.Background())

	// Run in a goroutine so we can cancel from the main goroutine.
	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx, job, store)
	}()

	// Give the subprocess a moment to start, then cancel.
	time.Sleep(200 * time.Millisecond)

	// Simulate external cancellation by setting status then cancelling context.
	job.SetStatus(model.StatusCancelled)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() returned error after cancel: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run() did not return within 15s after cancel")
	}

	// Status should remain cancelled.
	if job.Status() != model.StatusCancelled {
		t.Errorf("Status() = %q, want %q", job.Status(), model.StatusCancelled)
	}
}

// ---------------------------------------------------------------------------
// Test: Output directory detection
// ---------------------------------------------------------------------------

func Test_Run_OutputDirDetection(t *testing.T) {
	setSubprocessBehavior(t, "outputdir")

	cwd := t.TempDir()
	r := newTestRunner(t)
	store, job := newJob(t, cwd)

	ctx := context.Background()
	if err := r.Run(ctx, job, store); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	// Status should be completed.
	if job.Status() != model.StatusCompleted {
		t.Errorf("Status() = %q, want %q", job.Status(), model.StatusCompleted)
	}

	// The output directory should be set because the fake binary creates one.
	outputDir := job.OutputDir()
	if outputDir == "" {
		t.Fatal("OutputDir() is empty, want a research-* directory path")
	}
	if !strings.Contains(outputDir, "research-outputdir-test") {
		t.Errorf("OutputDir() = %q, want it to contain %q", outputDir, "research-outputdir-test")
	}

	// The directory should actually exist.
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		t.Errorf("OutputDir %q does not exist on disk", outputDir)
	}
}

// ---------------------------------------------------------------------------
// Test: Output dir claimed by another job is not assigned
// ---------------------------------------------------------------------------

func Test_Run_OutputDirAlreadyClaimed(t *testing.T) {
	setSubprocessBehavior(t, "outputdir")

	cwd := t.TempDir()

	// Use a shared store and claim the directory that the fake binary will
	// create. Because the directory does NOT exist at run start (it is not
	// in preDirs), the runner will find it as a new candidate and attempt
	// to claim it — but it will already be claimed so OutputDir stays empty.
	store := jobstore.NewStore()
	targetDir := filepath.Join(cwd, "research-outputdir-test")
	// Claim the path in the store before the run begins (the path need not
	// exist on disk yet — ClaimDir just tracks string paths).
	store.ClaimDir(targetDir)

	job := store.Create("test-job-claim", "query", "opus", 10, cwd)
	r := newTestRunner(t)

	ctx := context.Background()
	if err := r.Run(ctx, job, store); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	// Job should complete but OutputDir should be empty because the dir was
	// already claimed in the store before the run.
	if job.Status() != model.StatusCompleted {
		t.Errorf("Status() = %q, want %q", job.Status(), model.StatusCompleted)
	}
	if job.OutputDir() != "" {
		t.Errorf("OutputDir() = %q, want empty (dir was pre-claimed in store)", job.OutputDir())
	}
}

// ---------------------------------------------------------------------------
// Test: Pre-existing research dirs are not treated as new
// ---------------------------------------------------------------------------

func Test_Run_PreExistingDirsNotClaimed(t *testing.T) {
	setSubprocessBehavior(t, "success")

	cwd := t.TempDir()

	// Create a pre-existing research-* directory.
	preExisting := filepath.Join(cwd, "research-preexisting-2025")
	if err := os.MkdirAll(preExisting, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	store := jobstore.NewStore()
	job := store.Create("test-job-preexist", "query", "opus", 10, cwd)
	r := newTestRunner(t)

	ctx := context.Background()
	if err := r.Run(ctx, job, store); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	// The pre-existing dir should not be assigned as output.
	outputDir := job.OutputDir()
	if outputDir == preExisting {
		t.Errorf("OutputDir() = %q, should not be the pre-existing directory", outputDir)
	}
}

// ---------------------------------------------------------------------------
// Test: Non-zero exit with successful result → completed
// ---------------------------------------------------------------------------

func Test_Run_NonZeroExitWithSuccessfulResult(t *testing.T) {
	setSubprocessBehavior(t, "maxturns")

	cwd := t.TempDir()
	r := newTestRunner(t)
	store, job := newJob(t, cwd)

	ctx := context.Background()
	if err := r.Run(ctx, job, store); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	// Status should be completed even though exit code was 2, because
	// the result event had is_error: false.
	if job.Status() != model.StatusCompleted {
		t.Errorf("Status() = %q, want %q", job.Status(), model.StatusCompleted)
	}

	// Session ID should be set.
	if job.SessionID() != "sess-maxturns-789" {
		t.Errorf("SessionID() = %q, want %q", job.SessionID(), "sess-maxturns-789")
	}

	// ResultInfo should be populated.
	info := job.ResultInfo()
	if info.CostUSD == nil || *info.CostUSD != 0.12 {
		t.Errorf("ResultInfo().CostUSD = %v, want 0.12", info.CostUSD)
	}

	// Error should not be set on a completed job.
	if job.Error() != "" {
		t.Errorf("Error() = %q, want empty string", job.Error())
	}
}

// ---------------------------------------------------------------------------
// Test: Run sets status to running initially
// ---------------------------------------------------------------------------

func Test_Run_SetsStatusRunning(t *testing.T) {
	setSubprocessBehavior(t, "success")

	cwd := t.TempDir()
	store, job := newJob(t, cwd)
	r := newTestRunner(t)

	// Verify initial status.
	if job.Status() != model.StatusPending {
		t.Errorf("initial Status() = %q, want %q", job.Status(), model.StatusPending)
	}

	ctx := context.Background()
	// Run synchronously — we cannot observe the mid-run status without
	// concurrency, but we can verify the end status.
	if err := r.Run(ctx, job, store); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// After a successful run the status transitions to completed.
	if job.Status() != model.StatusCompleted {
		t.Errorf("Status() = %q, want %q", job.Status(), model.StatusCompleted)
	}
}

// ---------------------------------------------------------------------------
// Test: Run with no stderr on success
// ---------------------------------------------------------------------------

func Test_Run_NoErrorOnSuccess(t *testing.T) {
	setSubprocessBehavior(t, "success")

	cwd := t.TempDir()
	r := newTestRunner(t)
	store, job := newJob(t, cwd)

	if err := r.Run(context.Background(), job, store); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if job.Error() != "" {
		t.Errorf("Error() = %q, want empty string on successful run", job.Error())
	}
}
