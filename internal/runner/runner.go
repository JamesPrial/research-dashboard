// Package runner manages the subprocess lifecycle for research jobs executed
// via the claude CLI.
package runner

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jamesprial/research-dashboard/internal/envutil"
	"github.com/jamesprial/research-dashboard/internal/jobstore"
	"github.com/jamesprial/research-dashboard/internal/model"
	"github.com/jamesprial/research-dashboard/internal/parser"
)

// PromptPrefix is prepended to every research query before it is passed to
// the claude CLI subprocess. It contains the full multi-agent research
// orchestration prompt embedded from prompt.md.
//
//go:embed prompt.md
var PromptPrefix string

// Runner manages the lifecycle of claude CLI subprocesses for research jobs.
type Runner struct {
	// ClaudePath is the path to the claude binary. When empty, "claude" is
	// used, which relies on the PATH environment variable.
	ClaudePath string
}

// DirClaimer atomically claims a research output directory.
type DirClaimer interface {
	ClaimDir(dir string) bool
}

// New creates a Runner. If claudePath is empty, the binary name defaults to
// "claude".
func New(claudePath string) *Runner {
	if claudePath == "" {
		claudePath = "claude"
	}
	return &Runner{ClaudePath: claudePath}
}

// Run executes a research job as a subprocess. It:
//  1. Sets job status to running
//  2. Snapshots existing research-* directories in job's cwd
//  3. Builds and starts the claude command
//  4. Reads stdout line-by-line, parsing via parser.ParseStreamLine
//  5. Appends events to job, captures session_id and result_info
//  6. After exit: diffs dirs to find new output, claims it via store
//  7. Sets final status (completed/failed) and error if any
func (r *Runner) Run(ctx context.Context, job *jobstore.Job, store *jobstore.Store) error {
	job.SetStatus(model.StatusRunning)

	cwd := job.CWD()

	// Snapshot existing research-* directories before the subprocess runs.
	preDirs := researchDirs(cwd)
	slog.Debug("runner: pre-run directory snapshot", "job_id", job.ID(), "count", len(preDirs))

	// Build the command arguments.
	query := fmt.Sprintf("%s%s", PromptPrefix, job.Query())
	args := []string{
		"-p",
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
		"--model", job.Model(),
		"--max-turns", fmt.Sprintf("%d", job.MaxTurns()),
		query,
	}

	// Pass the API key via CLI flag if available. The env var is already set
	// via FilteredEnv(), but some Claude CLI versions in Docker do not read
	// it reliably.
	if apiKey := envutil.ResolvedAPIKey(); apiKey != "" {
		args = append([]string{"--api-key", apiKey}, args...)
	}

	slog.Debug("runner: starting subprocess", "job_id", job.ID(), "claude_path", r.ClaudePath, "cwd", cwd, "model", job.Model())

	cmd := exec.CommandContext(ctx, r.ClaudePath, args...)

	// Send SIGTERM on context cancellation, then wait up to 10s for exit.
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Signal(syscall.SIGTERM)
		}
		return nil
	}
	cmd.WaitDelay = 10 * time.Second

	// Use a filtered environment (strips CLAUDE_* variables).
	cmd.Env = envutil.FilteredEnv()
	cmd.Dir = cwd

	// Capture stderr separately so we can report it on failure.
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("runner: failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("runner: failed to start claude subprocess: %w", err)
	}

	slog.Info("runner: subprocess started", "job_id", job.ID(), "pid", cmd.Process.Pid)

	// Read stdout line-by-line with a 512 KB scanner buffer.
	var counter atomic.Int64
	scanner := bufio.NewScanner(stdout)
	const bufSize = 512 * 1024
	scanner.Buffer(make([]byte, bufSize), bufSize)

	// Track whether we received a result event indicating successful completion.
	// The CLI may exit non-zero (e.g. exit code 2 for max turns reached) even
	// when the research completed successfully and a result was emitted.
	gotResult := false
	resultIsError := false

	for scanner.Scan() {
		line := scanner.Text()
		events := parser.ParseStreamLine(line, &counter)
		for _, evt := range events {
			job.AddEvent(evt)

			// Capture session_id from system events.
			if evt.Type == model.EventTypeSystem && evt.Raw != nil {
				if sid, ok := evt.Raw["session_id"]; ok {
					if sidStr, ok := sid.(string); ok && sidStr != "" {
						job.SetSessionID(sidStr)
						slog.Debug("runner: captured session_id", "job_id", job.ID(), "session_id", sidStr)
					}
				}
			}

			// Capture result stats from result events.
			if evt.Type == model.EventTypeResult {
				gotResult = true
				resultIsError = evt.IsError
				slog.Debug("runner: received result event", "job_id", job.ID(), "is_error", evt.IsError)
				if evt.Raw != nil {
					stats := extractResultStats(evt.Raw)
					job.SetResultInfo(stats)
				}
			}
		}
		if len(events) > 0 {
			slog.Debug("runner: parsed events", "job_id", job.ID(), "count", len(events))
		}
	}

	if scanErr := scanner.Err(); scanErr != nil && scanErr != io.EOF {
		slog.Warn("runner: scanner error reading stdout", "job_id", job.ID(), "err", scanErr)
	}

	// Wait for the subprocess to exit.
	waitErr := cmd.Wait()

	// If the job was cancelled externally, respect that and return cleanly.
	if job.Status() == model.StatusCancelled {
		slog.Info("runner: job was cancelled", "job_id", job.ID())
		return nil
	}

	// Determine exit status.
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Non-exit error (e.g. SIGTERM with WaitDelay timeout).
			exitCode = -1
		}
	}

	// Detect new output directory produced by the subprocess.
	postDirs := researchDirs(cwd)
	slog.Debug("runner: post-run directory snapshot", "job_id", job.ID(), "count", len(postDirs))
	if newDir := detectNewOutputDir(preDirs, postDirs, store); newDir != "" {
		// detectNewOutputDir already claimed the directory atomically.
		job.SetOutputDir(newDir)
		slog.Info("runner: claimed output dir", "job_id", job.ID(), "dir", newDir)
	}

	// A job is considered successful if:
	//   - the subprocess exited cleanly (exit code 0), OR
	//   - we received a result event that was not an error (the CLI may exit
	//     non-zero for non-fatal reasons, e.g. exit code 2 for max turns reached).
	if exitCode == 0 || (gotResult && !resultIsError) {
		job.SetStatus(model.StatusCompleted)
		if exitCode != 0 {
			slog.Info("runner: job completed (non-zero exit ignored, got successful result)",
				"job_id", job.ID(), "exit_code", exitCode)
		} else {
			slog.Info("runner: job completed", "job_id", job.ID())
		}
	} else {
		job.SetStatus(model.StatusFailed)
		errMsg := strings.TrimSpace(stderrBuf.String())
		if errMsg == "" {
			errMsg = fmt.Sprintf("subprocess exited with code %d", exitCode)
		}
		job.SetError(errMsg)
		slog.Error("runner: job failed", "job_id", job.ID(), "exit_code", exitCode, "stderr", errMsg)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// researchDirs returns a map of absolute directory paths for all "research-*"
// subdirectories inside dir. Non-existent directories and read errors are
// silently ignored.
func researchDirs(dir string) map[string]time.Time {
	result := make(map[string]time.Time)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), model.ResearchDirPrefix) {
			continue
		}
		absPath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result[absPath] = info.ModTime()
	}
	return result
}

// detectNewOutputDir diffs pre-run and post-run research-* directory snapshots,
// atomically claims the newest unclaimed new directory via the store, and
// returns its path. Returns an empty string if no new unclaimed directory
// is found.
func detectNewOutputDir(pre, post map[string]time.Time, claimer DirClaimer) string {
	// Collect directories that are new (present after run but not before).
	var candidates []string
	for path := range post {
		if _, existed := pre[path]; !existed {
			candidates = append(candidates, path)
		}
	}
	if len(candidates) == 0 {
		return ""
	}

	// Sort newest first by mtime.
	sort.Slice(candidates, func(i, j int) bool {
		return post[candidates[i]].After(post[candidates[j]])
	})

	// Claim the first directory that has not yet been claimed by another job.
	// store.ClaimDir is the atomic test-and-set for ownership.
	for _, path := range candidates {
		if claimer.ClaimDir(path) {
			return path
		}
		// Already claimed by another job — try the next candidate.
	}
	return ""
}

// extractResultStats converts a raw result event map into a model.ResultStats.
func extractResultStats(raw map[string]any) model.ResultStats {
	var stats model.ResultStats

	// cost_usd: prefer total_cost_usd, fall back to cost_usd.
	if v, ok := raw["total_cost_usd"]; ok {
		if f := toFloat64(v); f != nil {
			stats.CostUSD = f
		}
	} else if v, ok := raw["cost_usd"]; ok {
		if f := toFloat64(v); f != nil {
			stats.CostUSD = f
		}
	}

	if v, ok := raw["duration_ms"]; ok {
		if i := toInt(v); i != nil {
			stats.DurationMS = i
		}
	}

	if v, ok := raw["duration_api_ms"]; ok {
		if i := toInt(v); i != nil {
			stats.DurationAPIMS = i
		}
	}

	if v, ok := raw["num_turns"]; ok {
		if i := toInt(v); i != nil {
			stats.NumTurns = i
		}
	}

	if v, ok := raw["session_id"]; ok {
		if s, ok := v.(string); ok && s != "" {
			stats.SessionID = &s
		}
	}

	if v, ok := raw["usage"]; ok {
		if usageMap, ok := v.(map[string]any); ok && len(usageMap) > 0 {
			stats.Usage = usageMap
		}
	}

	return stats
}

// toFloat64 attempts to convert a JSON-decoded numeric value to *float64.
// JSON numbers decode as float64 when unmarshalled into an any.
func toFloat64(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	}
	return nil
}

// toInt attempts to convert a JSON-decoded numeric value to *int.
func toInt(v any) *int {
	switch n := v.(type) {
	case float64:
		i := int(n)
		return &i
	case int:
		return &n
	case int64:
		i := int(n)
		return &i
	}
	return nil
}
