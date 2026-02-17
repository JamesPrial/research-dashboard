// Package envutil provides utilities for working with process environment variables.
package envutil

import (
	"os"
	"strings"
)

// FilteredEnv returns the current environment with all CLAUDE-prefixed
// variables removed. The comparison is case-sensitive — only uppercase
// "CLAUDE" prefix is stripped.
func FilteredEnv() []string {
	result := make([]string, 0)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CLAUDE") {
			continue
		}
		result = append(result, entry)
	}
	return result
}
