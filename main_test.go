package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_Run_StartsAndShutdown(t *testing.T) {
	cfg := config{
		port:       0, // OS picks a free port
		host:       "127.0.0.1",
		cwd:        t.TempDir(),
		claudePath: "claude",
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	addrCh := make(chan string, 1)
	go func() {
		errCh <- run(ctx, cfg, addrCh)
	}()

	// Wait for the server to report its address.
	var addr string
	select {
	case addr = <-addrCh:
	case err := <-errCh:
		t.Fatalf("run() returned early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to start")
	}

	// Verify the server responds.
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Cancel the context to trigger graceful shutdown.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for run() to exit")
	}
}

func Test_Run_InvalidCWD_ReturnsError(t *testing.T) {
	cfg := config{
		port:       0,
		host:       "127.0.0.1",
		cwd:        "/nonexistent/path/that/does/not/exist",
		claudePath: "claude",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := run(ctx, cfg, nil)
	if err == nil {
		t.Error("run() returned nil, want error for nonexistent cwd")
	}
}

func Test_Run_WritesAgentConfigs(t *testing.T) {
	cwd := t.TempDir()
	cfg := config{
		port:       0,
		host:       "127.0.0.1",
		cwd:        cwd,
		claudePath: "claude",
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	addrCh := make(chan string, 1)
	go func() {
		errCh <- run(ctx, cfg, addrCh)
	}()

	select {
	case <-addrCh:
	case err := <-errCh:
		t.Fatalf("run() returned early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to start")
	}

	// Verify agent config files were written to {cwd}/.claude/agents/.
	for _, name := range []string{"research-worker.md", "source-archiver.md"} {
		path := filepath.Join(cwd, ".claude", "agents", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("agent config %s: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("agent config %s is empty", name)
		}
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for run() to exit")
	}
}

func Test_DefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.port != 8420 {
		t.Errorf("port = %d, want 8420", cfg.port)
	}
	if cfg.host != "0.0.0.0" {
		t.Errorf("host = %q, want %q", cfg.host, "0.0.0.0")
	}
	if cfg.claudePath != "claude" {
		t.Errorf("claudePath = %q, want %q", cfg.claudePath, "claude")
	}
	if cfg.cwd == "" {
		t.Error("cwd is empty, want non-empty default")
	}
}
