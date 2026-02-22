// Package changelog provides parsing for Claude Code's changelog and version detection.
package changelog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/andyhtran/cct/internal/paths"
)

type VersionEntry struct {
	Version string `json:"version"`
	Content string `json:"content"`
}

func ChangelogPath() string {
	return filepath.Join(paths.ClaudeDir(), "cache", "changelog.md")
}

func ParseChangelog() ([]VersionEntry, error) {
	data, err := os.ReadFile(ChangelogPath())
	if err != nil {
		return nil, fmt.Errorf("cannot read changelog: %w", err)
	}

	return parseChangelogContent(string(data)), nil
}

func parseChangelogContent(text string) []VersionEntry {
	var entries []VersionEntry
	lines := strings.Split(text, "\n")

	var current *VersionEntry
	var contentLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if current != nil {
				current.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
				entries = append(entries, *current)
			}
			version := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			current = &VersionEntry{Version: version}
			contentLines = nil
		} else if current != nil {
			contentLines = append(contentLines, line)
		}
	}

	if current != nil {
		current.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
		entries = append(entries, *current)
	}

	return entries
}

func DetectClaudeVersion() string {
	cmd := exec.Command("claude", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
