// Package pathutil provides utilities for safe file path validation and resolution.
package pathutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesprial/research-dashboard/internal/model"
)

// WithinBase reports whether path equals base or is contained within it.
// Both paths are cleaned before comparison; the check is purely lexical.
func WithinBase(base, path string) bool {
	cleanBase := filepath.Clean(base)
	cleanPath := filepath.Clean(path)
	return cleanPath == cleanBase ||
		strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator))
}

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

	// The lexical check above cannot catch symlinks inside the base directory
	// that point outside it, so verify the fully resolved location too. A
	// path that does not exist yet resolves lexically and is returned as-is;
	// the caller reports not-found.
	realBase, err := filepath.EvalSymlinks(cleanBase)
	if err != nil {
		return "", errors.New("base directory cannot be resolved")
	}
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return resolved, nil
		}
		return "", errors.New("file path cannot be resolved")
	}
	realPrefix := realBase + string(filepath.Separator)
	if realPath != realBase && !strings.HasPrefix(realPath, realPrefix) {
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
