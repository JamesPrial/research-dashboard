# Research Dashboard

A web dashboard for managing [Claude Code](https://docs.anthropic.com/en/docs/claude-code) research jobs. Submit research queries, watch Claude work in real time via SSE streaming, and browse generated reports with cited sources.

## Prerequisites

- **Go 1.22+**
- **Claude Code CLI** (`claude`) installed and available on your PATH, with valid API credentials configured

## Quick Start

### Local

```bash
go build -o research-dashboard .
./research-dashboard
```

Open http://localhost:8420 in your browser.

### Docker

```bash
cp .env.example .env
# Edit .env with your ANTHROPIC_API_KEY
docker compose up
```

Open http://localhost:8420. Research output is persisted to `./research-data/`.

The dashboard expects a working directory (default `~/research`, `/research` in Docker) where Claude will create `research-*` output directories containing reports and source files.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `8420` | Server port |
| `--host` | `0.0.0.0` | Bind address |
| `--cwd` | `~/research` | Working directory for research output |
| `--claude-path` | `claude` | Path to the Claude Code CLI binary |

### Docker Authentication

Two methods are supported:

1. **API Key (recommended)**: Set `ANTHROPIC_API_KEY` in your `.env` file. The Claude CLI picks this up automatically.
2. **OAuth (Pro/Max plan)**: If already logged in on your host, uncomment the `~/.claude` volume mount in `docker-compose.yml` to share your auth session.

## How It Works

1. **Submit a query** from the dashboard sidebar. Pick a model (opus, sonnet, haiku) and hit Start Research.
2. **The server spawns `claude`** as a subprocess with `--output-format stream-json`, streaming structured events back to the browser via Server-Sent Events.
3. **Watch the job live** — the main panel shows assistant messages (rendered as Markdown), tool calls with expandable input/output, and a progress indicator with turn count.
4. **When the job completes**, Claude's output directory (`research-{topic}-{timestamp}/`) is detected automatically. The report and source files become available in the Reader view.
5. **Past runs** are discovered from existing `research-*` directories on disk and listed in the sidebar.

### Web UI

- **Dashboard** (`/`) — Submit queries, monitor active jobs with live streaming, browse past runs. Supports multiple concurrent jobs with toast notifications for background completions. Keyboard shortcut: Ctrl/Cmd+Enter to submit.
- **Reader** (`/reader`) — Read rendered Markdown reports, browse source files (Markdown and HTML), and navigate cited sources with an index table.

## API

All endpoints return JSON unless noted.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/research` | Start a new job. Body: `{"query": "...", "model": "opus", "max_turns": 100}` |
| `GET` | `/research` | List active jobs and past runs |
| `GET` | `/research/{id}` | Job detail with full event log |
| `DELETE` | `/research/{id}` | Cancel a running job |
| `GET` | `/research/{id}/stream` | SSE event stream. Optional `?after=N` cursor. |
| `GET` | `/research/{id}/report` | Raw report.md content (text/plain) |
| `GET` | `/research/{id}/files` | List files in job output directory |
| `GET` | `/research/{id}/files/{path}` | Serve a file from job output |
| `GET` | `/research/past/{dir}/report` | Report from a past run directory |
| `GET` | `/research/past/{dir}/files` | List files in a past run |
| `GET` | `/research/past/{dir}/files/{path}` | Serve a file from a past run |

## Development

```bash
# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run a single package
go test ./internal/parser/ -v

# Run a specific test
go test ./internal/server/ -run Test_HandleStreamResearch -v

# Static analysis
go vet ./...

# Run locally on a different port
go run . --port 9000 --cwd /tmp/research
```

The project has zero external dependencies — only the Go standard library.

### Docker

```bash
# Build and run
docker compose up --build

# Run in background
docker compose up -d

# View logs
docker compose logs -f

# Stop
docker compose down
```
