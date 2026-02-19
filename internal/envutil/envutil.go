// Package envutil provides utilities for working with process environment variables.
package envutil

import (
	"os"
	"strings"
)

// ResolvedAPIKey returns the API key that should be used for the Claude CLI
// subprocess. MAX_API_KEY takes priority over ANTHROPIC_API_KEY. Returns an
// empty string if neither is set.
func ResolvedAPIKey() string {
	if key := os.Getenv("MAX_API_KEY"); key != "" {
		return key
	}
	return os.Getenv("ANTHROPIC_API_KEY")
}

// FilteredEnv returns the current environment with all CLAUDE-prefixed
// variables removed and API key priority applied. The comparison is
// case-sensitive — only uppercase "CLAUDE" prefix is stripped.
//
// If MAX_API_KEY is set it takes priority over ANTHROPIC_API_KEY.
// The resolved value is passed as ANTHROPIC_API_KEY; MAX_API_KEY is
// never included in the output.
func FilteredEnv() []string {
	result := make([]string, 0)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CLAUDE") {
			continue
		}
		if key == "MAX_API_KEY" || key == "ANTHROPIC_API_KEY" {
			continue
		}
		result = append(result, entry)
	}

	// Append the resolved API key (MAX_API_KEY takes priority).
	if apiKey := ResolvedAPIKey(); apiKey != "" {
		result = append(result, "ANTHROPIC_API_KEY="+apiKey)
	}

	return result
}
