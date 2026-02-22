// Package plan provides types and functions for listing, searching, and copying
// Claude Code plan files stored in ~/.claude/plans/.
package plan

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/paths"
)

var (
	ErrNotFound        = errors.New("plan not found")
	ErrMultipleMatches = errors.New("multiple plans match")
)

type Plan struct {
	Name     string    `json:"name"`
	Title    string    `json:"title"`
	Modified time.Time `json:"modified"`
	Path     string    `json:"-"`
}

func PlansDir() string {
	return filepath.Join(paths.ClaudeDir(), "plans")
}

func ListPlans() ([]Plan, error) {
	dir := PlansDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read plans directory: %w", err)
	}

	var plans []Plan
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		title := extractTitle(path)

		plans = append(plans, Plan{
			Name:     name,
			Title:    title,
			Modified: info.ModTime(),
			Path:     path,
		})
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Modified.After(plans[j].Modified)
	})

	return plans, nil
}

func SearchPlans(query string, snippetWidth int) ([]PlanMatch, error) {
	plans, err := ListPlans()
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var matches []PlanMatch

	for _, p := range plans {
		content, err := os.ReadFile(p.Path)
		if err != nil {
			continue
		}
		text := string(content)
		if strings.Contains(strings.ToLower(text), queryLower) {
			snippet := output.ExtractSnippet(text, queryLower, snippetWidth)
			matches = append(matches, PlanMatch{
				Plan:    p,
				Snippet: snippet,
			})
		}
	}

	return matches, nil
}

type PlanMatch struct {
	Plan    Plan   `json:"plan"`
	Snippet string `json:"snippet"`
}

func FindPlan(name string) (*Plan, error) {
	plans, err := ListPlans()
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(name)
	var matches []Plan

	for _, p := range plans {
		if strings.ToLower(p.Name) == nameLower {
			// Exact match
			return &p, nil
		}
		if strings.Contains(strings.ToLower(p.Name), nameLower) {
			matches = append(matches, p)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no plan found matching %q: %w", name, ErrNotFound)
	case 1:
		return &matches[0], nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, "  "+m.Name)
		}
		return nil, fmt.Errorf("multiple plans match %q:\n%s: %w", name, strings.Join(names, "\n"), ErrMultipleMatches)
	}
}

func CopyPlan(name, destDir, rename string) (string, error) {
	p, err := FindPlan(name)
	if err != nil {
		return "", err
	}

	destName := p.Name + ".md"
	if rename != "" {
		if !strings.HasSuffix(rename, ".md") {
			rename += ".md"
		}
		destName = rename
	}

	destPath := filepath.Join(destDir, destName)

	content, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("cannot read plan: %w", err)
	}

	if err := os.WriteFile(destPath, content, 0o644); err != nil {
		return "", fmt.Errorf("cannot write to %s: %w", destPath, err)
	}

	return destPath, nil
}

func extractTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
