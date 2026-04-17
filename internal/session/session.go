// Package session provides types and functions for reading and searching
// Claude Code session JSONL files stored in ~/.claude/projects/.
package session

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrNotFound        = errors.New("session not found")
	ErrMultipleMatches = errors.New("multiple sessions match")
)

type Session struct {
	ID           string    `json:"id"`
	ShortID      string    `json:"short_id"`
	IsAgent      bool      `json:"is_agent"`
	ProjectPath  string    `json:"project_path"`
	ProjectName  string    `json:"project_name"`
	GitBranch    string    `json:"git_branch"`
	FirstPrompt  string    `json:"first_prompt"`
	CustomTitle  string    `json:"custom_title,omitempty"`
	FilePath     string    `json:"-"`
	Created      time.Time `json:"created"`
	Modified     time.Time `json:"modified"`
	MessageCount int       `json:"message_count"`
}

type Match struct {
	Role    string `json:"role"`
	Source  string `json:"source,omitempty"`
	Snippet string `json:"snippet"`
}

type SearchResult struct {
	*Session
	Matches []Match `json:"matches"`
}

func ExtractIDFromFilename(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl")
}

func IsAgentSession(id string) bool {
	return strings.HasPrefix(id, "agent-")
}

func ShortID(id string) string {
	if IsAgentSession(id) {
		return id
	}
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func FindByPrefix(prefix string) (*Session, error) {
	sessions := ScanAll("", false, true)

	// Exact match on full ID or 8-char short ID wins outright.
	for _, s := range sessions {
		if s.ID == prefix || s.ShortID == prefix {
			return s, nil
		}
	}

	// Next: exact custom-title match (case-insensitive). Titles set via
	// Claude Code's /rename are the user-facing identifier, so an exact
	// hit takes precedence over any UUID-prefix collisions below. Skip
	// agent sessions — they don't carry custom titles.
	var titleMatches []*Session
	for _, s := range sessions {
		if s.IsAgent || s.CustomTitle == "" {
			continue
		}
		if strings.EqualFold(s.CustomTitle, prefix) {
			titleMatches = append(titleMatches, s)
		}
	}
	if len(titleMatches) == 1 {
		return titleMatches[0], nil
	}
	if len(titleMatches) > 1 {
		var ids []string
		for _, s := range titleMatches {
			ids = append(ids, fmt.Sprintf("  %s  %s  (%s)", s.ShortID, s.ProjectName, s.CustomTitle))
		}
		return nil, fmt.Errorf("multiple sessions share title %q:\n%s: %w", prefix, strings.Join(ids, "\n"), ErrMultipleMatches)
	}

	// Fall back to UUID prefix match.
	var matches []*Session
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, prefix) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no session found matching %q: %w", prefix, ErrNotFound)
	case 1:
		return matches[0], nil
	default:
		var ids []string
		for _, s := range matches {
			ids = append(ids, fmt.Sprintf("  %s  %s", s.ShortID, s.ProjectName))
		}
		return nil, fmt.Errorf("multiple sessions match %q:\n%s: %w", prefix, strings.Join(ids, "\n"), ErrMultipleMatches)
	}
}

func FindByPrefixFull(prefix string) (*Session, error) {
	s, err := FindByPrefix(prefix)
	if err != nil {
		return nil, err
	}
	if full := ParseFullSession(s.FilePath); full != nil {
		return full, nil
	}
	return s, nil
}
