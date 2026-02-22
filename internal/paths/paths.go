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
