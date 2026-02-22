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
	ProjectPath  string    `json:"project_path"`
	ProjectName  string    `json:"project_name"`
	GitBranch    string    `json:"git_branch"`
	FirstPrompt  string    `json:"first_prompt"`
	FilePath     string    `json:"-"`
	Created      time.Time `json:"created"`
	Modified     time.Time `json:"modified"`
	MessageCount int       `json:"message_count"`
}

type SearchResult struct {
	Session *Session `json:"session"`
	Matches []string `json:"matches"`
}

func ExtractIDFromFilename(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl")
}

func ShortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func FindByPrefix(prefix string) (*Session, error) {
	sessions := ScanAll("", false)
	var matches []*Session
	for _, s := range sessions {
		if s.ID == prefix || strings.HasPrefix(s.ID, prefix) || s.ShortID == prefix {
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

	full := ParseFullSession(s.FilePath)
	if full != nil {
		s.MessageCount = full.MessageCount
		if full.FirstPrompt != "" {
			s.FirstPrompt = full.FirstPrompt
		}
		if full.ProjectPath != "" {
			s.ProjectPath = full.ProjectPath
			s.ProjectName = full.ProjectName
		}
		if full.GitBranch != "" {
			s.GitBranch = full.GitBranch
		}
		if !full.Created.IsZero() {
			s.Created = full.Created
		}
	}

	return s, nil
}
