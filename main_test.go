package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test_Run_StartsAndShutdown(t *testing.T) {
	cfg := config{
		port:       0, // OS picks a free port
		host:       "127.0.0.1",
		cwd:        t.TempDir(),
		claudePath: "claude",
		logLevel:   "info",
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
		logLevel:   "info",
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
		logLevel:   "info",
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
	if cfg.logLevel != "info" {
		t.Errorf("logLevel = %q, want %q", cfg.logLevel, "info")
	}
}

func Test_DefaultConfig_LogLevelEnvVar(t *testing.T) {
	t.Run("LOG_LEVEL env var is respected", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "debug")
		cfg := defaultConfig()
		if cfg.logLevel != "debug" {
			t.Errorf("logLevel = %q, want %q", cfg.logLevel, "debug")
		}
	})

	t.Run("empty LOG_LEVEL falls back to info", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "")
		cfg := defaultConfig()
		if cfg.logLevel != "info" {
			t.Errorf("logLevel = %q, want %q", cfg.logLevel, "info")
		}
	})

	t.Run("invalid LOG_LEVEL is passed through and caught by run", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "bogus")
		cfg := defaultConfig()
		if cfg.logLevel != "bogus" {
			t.Errorf("logLevel = %q, want %q", cfg.logLevel, "bogus")
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := run(ctx, cfg, nil)
		if err == nil {
			t.Error("run() returned nil, want error for invalid LOG_LEVEL")
		}
	})
}

func Test_Run_InvalidLogLevel_ReturnsError(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
	}{
		{
			name:     "completely invalid string",
			logLevel: "invalid",
		},
		{
			name:     "unknown level verbose",
			logLevel: "verbose",
		},
		{
			name:     "numeric string",
			logLevel: "42",
		},
		{
			name:     "empty string",
			logLevel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{
				port:       0,
				host:       "127.0.0.1",
				cwd:        t.TempDir(),
				claudePath: "claude",
				logLevel:   tt.logLevel,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// run() should return an error before starting the server,
			// so we call it synchronously with nil addrCh.
			err := run(ctx, cfg, nil)
			if err == nil {
				t.Errorf("run() returned nil, want error for invalid log level %q", tt.logLevel)
				return
			}

			// The error message should mention the invalid log level value.
			errMsg := err.Error()
			if !strings.Contains(errMsg, "log level") {
				t.Errorf("error %q does not mention 'log level'", errMsg)
			}
		})
	}
}

func Test_Run_ValidLogLevels(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
	}{
		{
			name:     "debug level",
			logLevel: "debug",
		},
		{
			name:     "warn level",
			logLevel: "warn",
		},
		{
			name:     "error level",
			logLevel: "error",
		},
		{
			name:     "uppercase DEBUG",
			logLevel: "DEBUG",
		},
		{
			name:     "mixed case Info",
			logLevel: "Info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{
				port:       0,
				host:       "127.0.0.1",
				cwd:        t.TempDir(),
				claudePath: "claude",
				logLevel:   tt.logLevel,
			}

			ctx, cancel := context.WithCancel(context.Background())

			errCh := make(chan error, 1)
			addrCh := make(chan string, 1)
			go func() {
				errCh <- run(ctx, cfg, addrCh)
			}()

			// The server should start successfully with a valid log level.
			select {
			case <-addrCh:
				// Success: server started, log level was accepted.
			case err := <-errCh:
				t.Fatalf("run() returned early with error for log level %q: %v", tt.logLevel, err)
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for server to start")
			}

			// Clean shutdown.
			cancel()
			select {
			case <-errCh:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for run() to exit")
			}
		})
	}
}
