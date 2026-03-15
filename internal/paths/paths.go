// Package paths provides base directory paths for Claude Code configuration.
package paths

import (
	"os"
	"path/filepath"
)

func ClaudeDir() string {
	return filepath.Join(os.Getenv("HOME"), ".claude")
}

func ProjectsDir() string {
	return filepath.Join(ClaudeDir(), "projects")
}

func CacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "cct")
	}
	return filepath.Join(os.Getenv("HOME"), ".cache", "cct")
}

func IndexPath() string {
	return filepath.Join(CacheDir(), "index.db")
}
