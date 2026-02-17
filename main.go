package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jamesprial/research-dashboard/internal/jobstore"
	"github.com/jamesprial/research-dashboard/internal/runner"
	"github.com/jamesprial/research-dashboard/internal/server"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed research-config/agents/*
var researchConfigFS embed.FS

type config struct {
	port       int
	host       string
	cwd        string
	claudePath string
}

func defaultConfig() config {
	home, _ := os.UserHomeDir()
	return config{
		port:       8420,
		host:       "0.0.0.0",
		cwd:        filepath.Join(home, "research"),
		claudePath: "claude",
	}
}

func main() {
	cfg := defaultConfig()

	flag.IntVar(&cfg.port, "port", cfg.port, "server port")
	flag.StringVar(&cfg.host, "host", cfg.host, "server host")
	flag.StringVar(&cfg.cwd, "cwd", cfg.cwd, "working directory for research runs")
	flag.StringVar(&cfg.claudePath, "claude-path", cfg.claudePath, "path to the claude binary")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// run starts the HTTP server and blocks until ctx is cancelled.
// If addrCh is non-nil, the bound address is sent after the listener starts
// (used by tests with port 0).
func run(ctx context.Context, cfg config, addrCh chan<- string) error {
	// Validate cwd exists.
	info, err := os.Stat(cfg.cwd)
	if err != nil {
		return fmt.Errorf("cwd %q: %w", cfg.cwd, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("cwd %q is not a directory", cfg.cwd)
	}

	// Write embedded agent configs to {cwd}/.claude/agents/.
	if err := ensureResearchConfig(cfg.cwd); err != nil {
		return fmt.Errorf("research config: %w", err)
	}

	// Strip the "static/" prefix from the embedded FS.
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("embedded static fs: %w", err)
	}

	store := jobstore.NewStore()
	r := runner.New(cfg.claudePath)
	srv := server.New(store, r, staticFS, cfg.cwd, ctx)

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.host, cfg.port),
		Handler: srv,
	}

	// Start periodic cleanup of expired jobs.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				store.CleanupExpired(24 * time.Hour)
			}
		}
	}()

	// Start listener manually so we can report the address for tests.
	ln, err := net.Listen("tcp", httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", httpSrv.Addr, err)
	}

	if addrCh != nil {
		addrCh <- ln.Addr().String()
	}

	slog.Info("server started", "addr", ln.Addr().String(), "cwd", cfg.cwd)

	// Serve in a goroutine so we can wait for shutdown signal.
	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.Serve(ln); err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for context cancellation.
	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	// Return any serve error.
	return <-errCh
}

// ensureResearchConfig writes embedded agent definition files to
// {cwd}/.claude/agents/. Files are always overwritten so that binary
// upgrades propagate updated prompts.
func ensureResearchConfig(cwd string) error {
	agentsDir := filepath.Join(cwd, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", agentsDir, err)
	}

	entries, err := fs.ReadDir(researchConfigFS, "research-config/agents")
	if err != nil {
		return fmt.Errorf("read embedded agents: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(researchConfigFS, "research-config/agents/"+entry.Name())
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", entry.Name(), err)
		}
		dst := filepath.Join(agentsDir, entry.Name())
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		slog.Info("wrote agent config", "path", dst)
	}
	return nil
}
