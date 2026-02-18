# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
go build .                          # Build the binary
go test ./...                       # Run all tests
go test -race ./...                 # Run all tests with race detector
go test ./internal/server/ -v       # Run a single package's tests
go test ./internal/parser/ -run Test_ParseStreamLine_Assistant -v  # Run a single test
go vet ./...                        # Static analysis
go run . --port 8421 --cwd /tmp     # Run the server locally
docker compose up --build           # Build and run in Docker
docker compose up -d                # Run in background
```

Flags: `--port` (default 8420), `--host` (default 0.0.0.0), `--cwd` (default ~/research), `--claude-path` (default "claude").

## Architecture

This is a Go rewrite of a Python research dashboard. It manages Claude CLI research jobs as subprocesses, parses their streaming JSON output, and serves a web UI with real-time SSE updates. Zero external dependencies — stdlib only.

### Package Dependency Flow

```
main → server → jobstore → model
                 runner  → parser → model
                           envutil
                 pathutil
```

Embedded resources: `static/*` (web UI), `research-config/agents/*` (agent definitions), `internal/runner/prompt.md` (orchestration prompt).

### Key Packages

**`main`** — Entry point. Embeds `static/*` and `research-config/agents/*` via `//go:embed`, writes agent configs to `{cwd}/.claude/agents/` on startup (always overwrites for upgrades), creates the server, handles signals (SIGINT/SIGTERM), and runs periodic job cleanup.

**`internal/server`** — HTTP server with `ServeMux`. Past-run routes (`/research/past/...`) are intercepted in `ServeHTTP()` before the mux to avoid Go 1.22+ pattern conflicts with `/research/{id}/files/{path...}`. SSE streaming at `/research/{id}/stream` polls every 300ms.

**`internal/runner`** — Spawns `claude` CLI as a subprocess with `--output-format stream-json`. Embeds `prompt.md` (6-phase multi-agent orchestration prompt) as `PromptPrefix` via `//go:embed`. Reads stdout line-by-line through the parser, captures events on the job, diffs `research-*` directories pre/post to discover output, and claims the output dir atomically via `store.ClaimDir()`.

**`internal/jobstore`** — In-memory store with two-level locking: `Store.mu` (RWMutex) protects the jobs map, each `Job.mu` (RWMutex) protects individual job fields. All job field access goes through getter/setter methods.

**`internal/parser`** — Parses Claude's stream-JSON format (line-delimited JSON). Uses an atomic counter for event ordering. Handles event types: system, assistant (text/tool_use), user (tool_result), result, and stream_event envelope. Filters out ping/message_stop.

**`internal/model`** — Domain types. Key enums: `Status` (pending/running/completed/failed/cancelled), `EventType`, `EventSubtype`, `ModelName` (opus/sonnet/haiku). API payloads: `ResearchRequest`, `JobStatus`, `JobDetail`, `JobList`.

**`internal/pathutil`** — Path traversal prevention (`ResolveSafeFile`) and directory name validation (must start with `research-`, no `/`, `\`, `..`).

**`internal/envutil`** — Filters `CLAUDE*` variables from the environment before passing to subprocesses.

### Test Patterns

- All test files use external test packages (`package foo_test`)
- Table-driven tests are the norm
- `runner_test.go` uses a fake subprocess pattern: the test binary re-executes itself with `TEST_SUBPROCESS_BEHAVIOR` env var to simulate claude CLI output
- `server_test.go` uses `httptest.NewRecorder` and `fstest.MapFS` for the static FS; SSE tests use `context.WithTimeout` to let the ticker fire
- `main_test.go` uses port `0` and an `addrCh` channel to discover the bound address

### Research Plugin Source

Agent definitions in `research-config/agents/` and the orchestration prompt in `internal/runner/prompt.md` originate from the `deep-research@prial-plugins` Claude Code plugin (`~/.claude/plugins/cache/prial-plugins/deep-research/`). The binary is self-contained — no external plugin install needed.

### Docker Setup

Multi-stage `Dockerfile`: stage 1 (`golang:1.25-alpine`) compiles a static binary with `CGO_ENABLED=0`, stage 2 (`node:20-slim`) installs the Claude CLI npm package and copies the binary. Runtime uses `node:20-slim` (not alpine) because Claude CLI has native dependencies that require glibc. `docker-compose.yml` mounts `./research-data:/research` for persistent output, reads `ANTHROPIC_API_KEY` from `.env`, and has a commented-out OAuth mount option. `.dockerignore` keeps the build context clean.

### Route Conflict Workaround

Go 1.22+ `ServeMux` panics when patterns are ambiguous. The `/research/past/{dir}/...` routes conflict with `/research/{id}/files/{path...}` because neither is strictly more specific. The fix: past-run requests are handled in `ServeHTTP()` before reaching the mux via prefix check on `/research/past/`, with manual dispatch in `handlePastRuns`.
