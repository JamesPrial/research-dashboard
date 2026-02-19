// Package pathutil provides utilities for safe file path validation and resolution.
package pathutil

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/jamesprial/research-dashboard/internal/model"
)

// ValidateDirName validates a directory name for use in API paths.
// It rejects names containing /, \, .., or not starting with "research-".
func ValidateDirName(name string) error {
	if name == "" {
		return errors.New("directory name must not be empty")
	}
	if strings.Contains(name, "/") {
		return errors.New("directory name must not contain /")
	}
	if strings.Contains(name, "\\") {
		return errors.New("directory name must not contain \\")
	}
	if strings.Contains(name, "..") {
		return errors.New("directory name must not contain \"..\"")
	}
	if !strings.HasPrefix(name, model.ResearchDirPrefix) {
		return errors.New("directory name must start with \"research-\"")
	}
	return nil
}

// ResolveSafeFile resolves a file path relative to a base directory,
// ensuring the result stays within the base directory.
// Returns the cleaned absolute path or an error if the path escapes
// the base directory or is invalid.
func ResolveSafeFile(baseDir, filePath string) (string, error) {
	if filePath == "" {
		return "", errors.New("file path must not be empty")
	}
	if strings.Contains(filePath, "..") {
		return "", errors.New("file path must not contain \"..\"")
	}
	if filepath.IsAbs(filePath) {
		return "", errors.New("file path must not be absolute")
	}

	cleanBase := filepath.Clean(baseDir)
	resolved := filepath.Clean(filepath.Join(cleanBase, filePath))

	// Verify the resolved path is within the base directory.
	// Add the separator to cleanBase to avoid prefix collisions between
	// sibling directories (e.g. /tmp/base and /tmp/base-other).
	prefix := cleanBase + string(filepath.Separator)
	if resolved != cleanBase && !strings.HasPrefix(resolved, prefix) {
		return "", errors.New("file path escapes base directory")
	}

	return resolved, nil
}

// ClassifyFileType determines the FileType based on the file extension.
// Comparison is case-insensitive. Returns FileTypeMD for .md,
// FileTypeHTML for .html/.htm, and FileTypeOther for everything else.
func ClassifyFileType(name string) model.FileType {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".md":
		return model.FileTypeMD
	case ".html", ".htm":
		return model.FileTypeHTML
	default:
		return model.FileTypeOther
	}
}
