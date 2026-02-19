// Package envutil provides utilities for working with process environment variables.
package envutil

import (
	"os"
	"strings"
)

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

	// MAX_API_KEY takes priority over ANTHROPIC_API_KEY.
	apiKey := os.Getenv("MAX_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey != "" {
		result = append(result, "ANTHROPIC_API_KEY="+apiKey)
	}

	return result
}
