package envutil_test

import (
	"os"
	"strings"
	"testing"

	"github.com/jamesprial/research-dashboard/internal/envutil"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// envContains returns true if the env slice contains an entry with the given
// key prefix (e.g. "PATH=").
func envContains(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// envContainsExact returns true if the env slice contains the exact string.
func envContainsExact(env []string, entry string) bool {
	for _, e := range env {
		if e == entry {
			return true
		}
	}
	return false
}

// clearCLAUDEVars unsets any pre-existing CLAUDE-prefixed vars and API key
// vars so that tests run in a clean environment.
func clearCLAUDEVars(t *testing.T) {
	t.Helper()
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAUDE") {
			k, _, _ := strings.Cut(e, "=")
			t.Setenv(k, "") // register for cleanup
			_ = os.Unsetenv(k) // actually remove it
		}
	}
	for _, key := range []string{"MAX_API_KEY", "ANTHROPIC_API_KEY"} {
		if _, ok := os.LookupEnv(key); ok {
			t.Setenv(key, "")
			_ = os.Unsetenv(key)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: FilteredEnv
// ---------------------------------------------------------------------------

func Test_FilteredEnv_NoCLAUDEVars(t *testing.T) {
	clearCLAUDEVars(t)

	t.Setenv("TEST_ENVUTIL_PATH", "/usr/bin")
	t.Setenv("TEST_ENVUTIL_HOME", "/home/user")

	result := envutil.FilteredEnv()

	if !envContains(result, "TEST_ENVUTIL_PATH") {
		t.Error("expected TEST_ENVUTIL_PATH to be present in filtered env")
	}
	if !envContains(result, "TEST_ENVUTIL_HOME") {
		t.Error("expected TEST_ENVUTIL_HOME to be present in filtered env")
	}
}

func Test_FilteredEnv_SingleCLAUDEVar(t *testing.T) {
	clearCLAUDEVars(t)

	t.Setenv("CLAUDE_API_KEY", "secret")
	t.Setenv("TEST_ENVUTIL_KEEP", "yes")

	result := envutil.FilteredEnv()

	if envContains(result, "CLAUDE_API_KEY") {
		t.Error("expected CLAUDE_API_KEY to be removed from filtered env")
	}
	if !envContains(result, "TEST_ENVUTIL_KEEP") {
		t.Error("expected TEST_ENVUTIL_KEEP to remain in filtered env")
	}
}

func Test_FilteredEnv_MultipleCLAUDEVars(t *testing.T) {
	clearCLAUDEVars(t)

	t.Setenv("CLAUDE_FOO", "1")
	t.Setenv("CLAUDE_BAR", "2")
	t.Setenv("TEST_ENVUTIL_OTHER", "kept")

	result := envutil.FilteredEnv()

	if envContains(result, "CLAUDE_FOO") {
		t.Error("expected CLAUDE_FOO to be removed from filtered env")
	}
	if envContains(result, "CLAUDE_BAR") {
		t.Error("expected CLAUDE_BAR to be removed from filtered env")
	}
	if !envContains(result, "TEST_ENVUTIL_OTHER") {
		t.Error("expected TEST_ENVUTIL_OTHER to remain in filtered env")
	}
}

func Test_FilteredEnv_MixedVars(t *testing.T) {
	clearCLAUDEVars(t)

	t.Setenv("TEST_ENVUTIL_PATH", "/usr/bin")
	t.Setenv("CLAUDE_KEY", "secret")
	t.Setenv("TEST_ENVUTIL_TERM", "xterm")

	result := envutil.FilteredEnv()

	if !envContains(result, "TEST_ENVUTIL_PATH") {
		t.Error("expected TEST_ENVUTIL_PATH to be present")
	}
	if !envContains(result, "TEST_ENVUTIL_TERM") {
		t.Error("expected TEST_ENVUTIL_TERM to be present")
	}
	if envContains(result, "CLAUDE_KEY") {
		t.Error("expected CLAUDE_KEY to be removed")
	}
}

func Test_FilteredEnv_CLAUDEPrefixOnly(t *testing.T) {
	clearCLAUDEVars(t)

	// A variable named exactly "CLAUDE" (with no suffix) should still be
	// removed because it starts with "CLAUDE".
	t.Setenv("CLAUDE", "something")

	result := envutil.FilteredEnv()

	if envContains(result, "CLAUDE") {
		// Be careful: envContains checks for "CLAUDE=" prefix, which could
		// match "CLAUDE_FOO=". We check for exact entry as well.
		for _, e := range result {
			k, _, _ := strings.Cut(e, "=")
			if k == "CLAUDE" {
				t.Error("expected variable named exactly CLAUDE to be removed")
			}
		}
	}
}

func Test_FilteredEnv_LowercaseClaude_NotRemoved(t *testing.T) {
	clearCLAUDEVars(t)

	// Lowercase "claude_key" should NOT be removed. The filter is
	// case-sensitive, only removing uppercase CLAUDE prefix.
	t.Setenv("claude_key", "x")

	result := envutil.FilteredEnv()

	if !envContains(result, "claude_key") {
		t.Error("expected lowercase claude_key to remain (case-sensitive filter)")
	}
}

func Test_FilteredEnv_EmptyValue_StillRemoved(t *testing.T) {
	clearCLAUDEVars(t)

	// Even with an empty value, CLAUDE_EMPTY= should be removed.
	t.Setenv("CLAUDE_EMPTY", "")

	result := envutil.FilteredEnv()

	if envContainsExact(result, "CLAUDE_EMPTY=") {
		t.Error("expected CLAUDE_EMPTY with empty value to be removed")
	}
	if envContains(result, "CLAUDE_EMPTY") {
		t.Error("expected CLAUDE_EMPTY to be removed from filtered env")
	}
}

func Test_FilteredEnv_ReturnsStringSlice(t *testing.T) {
	clearCLAUDEVars(t)

	result := envutil.FilteredEnv()

	// The result must be a non-nil slice (even if empty, it should not be nil).
	if result == nil {
		t.Fatal("FilteredEnv() returned nil, expected non-nil slice")
	}
}

func Test_FilteredEnv_NoExtraneousEntries(t *testing.T) {
	clearCLAUDEVars(t)

	t.Setenv("CLAUDE_SECRET", "hidden")

	before := os.Environ()
	result := envutil.FilteredEnv()

	// The filtered result should have fewer entries than the full environ
	// (since we added at least one CLAUDE var that should be removed).
	// Count CLAUDE vars in before.
	claudeCount := 0
	for _, e := range before {
		k, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(k, "CLAUDE") {
			claudeCount++
		}
	}

	expectedLen := len(before) - claudeCount
	if len(result) != expectedLen {
		t.Errorf("FilteredEnv() returned %d entries, expected %d (full env has %d, with %d CLAUDE vars)",
			len(result), expectedLen, len(before), claudeCount)
	}
}

// ---------------------------------------------------------------------------
// Table-driven comprehensive test
// ---------------------------------------------------------------------------

func Test_FilteredEnv_Cases(t *testing.T) {
	tests := []struct {
		name        string
		setVars     map[string]string // vars to set before calling FilteredEnv
		wantPresent []string          // keys expected in result
		wantAbsent  []string          // keys expected NOT in result
		wantExact   []string          // exact KEY=VALUE entries expected
	}{
		{
			name:        "no CLAUDE vars set",
			setVars:     map[string]string{"TEST_A": "1", "TEST_B": "2"},
			wantPresent: []string{"TEST_A", "TEST_B"},
			wantAbsent:  nil,
		},
		{
			name:        "single CLAUDE var removed",
			setVars:     map[string]string{"CLAUDE_API_KEY": "secret"},
			wantPresent: nil,
			wantAbsent:  []string{"CLAUDE_API_KEY"},
		},
		{
			name:        "multiple CLAUDE vars removed",
			setVars:     map[string]string{"CLAUDE_FOO": "1", "CLAUDE_BAR": "2"},
			wantPresent: nil,
			wantAbsent:  []string{"CLAUDE_FOO", "CLAUDE_BAR"},
		},
		{
			name:        "mixed: keep non-CLAUDE, remove CLAUDE",
			setVars:     map[string]string{"TEST_PATH": "/bin", "CLAUDE_KEY": "y", "TEST_TERM": "z"},
			wantPresent: []string{"TEST_PATH", "TEST_TERM"},
			wantAbsent:  []string{"CLAUDE_KEY"},
		},
		{
			name:        "CLAUDE with no suffix removed",
			setVars:     map[string]string{"CLAUDE": "something"},
			wantPresent: nil,
			wantAbsent:  []string{"CLAUDE"},
		},
		{
			name:        "lowercase claude not removed",
			setVars:     map[string]string{"claude_key": "x"},
			wantPresent: []string{"claude_key"},
			wantAbsent:  nil,
		},
		{
			name:        "empty value CLAUDE still removed",
			setVars:     map[string]string{"CLAUDE_EMPTY": ""},
			wantPresent: nil,
			wantAbsent:  []string{"CLAUDE_EMPTY"},
		},
		{
			name:        "MAX_API_KEY overrides ANTHROPIC_API_KEY",
			setVars:     map[string]string{"ANTHROPIC_API_KEY": "sk-ant-original", "MAX_API_KEY": "sk-ant-max"},
			wantAbsent:  []string{"MAX_API_KEY"},
			wantExact:   []string{"ANTHROPIC_API_KEY=sk-ant-max"},
		},
		{
			name:        "MAX_API_KEY alone becomes ANTHROPIC_API_KEY",
			setVars:     map[string]string{"MAX_API_KEY": "sk-ant-max-only"},
			wantAbsent:  []string{"MAX_API_KEY"},
			wantExact:   []string{"ANTHROPIC_API_KEY=sk-ant-max-only"},
		},
		{
			name:        "ANTHROPIC_API_KEY passes through when no MAX_API_KEY",
			setVars:     map[string]string{"ANTHROPIC_API_KEY": "sk-ant-original"},
			wantAbsent:  []string{"MAX_API_KEY"},
			wantExact:   []string{"ANTHROPIC_API_KEY=sk-ant-original"},
		},
		{
			name:        "neither API key set means no ANTHROPIC_API_KEY in output",
			setVars:     map[string]string{"TEST_OTHER": "value"},
			wantPresent: []string{"TEST_OTHER"},
			wantAbsent:  []string{"MAX_API_KEY", "ANTHROPIC_API_KEY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearCLAUDEVars(t)

			for k, v := range tt.setVars {
				t.Setenv(k, v)
			}

			result := envutil.FilteredEnv()

			for _, key := range tt.wantPresent {
				if !envContains(result, key) {
					t.Errorf("expected %q to be present in filtered env", key)
				}
			}
			for _, key := range tt.wantAbsent {
				if envContains(result, key) {
					t.Errorf("expected %q to be absent from filtered env", key)
				}
			}
			for _, exact := range tt.wantExact {
				if !envContainsExact(result, exact) {
					t.Errorf("expected exact entry %q in filtered env", exact)
				}
			}
		})
	}
}

func Test_FilteredEnv_NoDuplicateAnthropicKey(t *testing.T) {
	clearCLAUDEVars(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-original")
	t.Setenv("MAX_API_KEY", "sk-ant-max")

	result := envutil.FilteredEnv()

	count := 0
	for _, e := range result {
		k, _, _ := strings.Cut(e, "=")
		if k == "ANTHROPIC_API_KEY" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 ANTHROPIC_API_KEY entry, got %d", count)
	}
}
