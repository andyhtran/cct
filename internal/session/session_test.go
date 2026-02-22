//go:build darwin || linux

package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func setupSessionFixtures(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessions := []struct {
		id    string
		lines []string
	}{
		{
			id: "aaaa1111-2222-3333-4444-555555555555",
			lines: []string{
				`{"type":"user","message":{"role":"user","content":"first session"},"cwd":"/test/proj","gitBranch":"main","sessionId":"aaaa1111-2222-3333-4444-555555555555","timestamp":"2026-01-10T08:00:00Z"}`,
				`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]},"timestamp":"2026-01-10T08:00:05Z"}`,
			},
		},
		{
			id: "aaaa2222-3333-4444-5555-666666666666",
			lines: []string{
				`{"type":"user","message":{"role":"user","content":"second session"},"cwd":"/test/proj","gitBranch":"dev","sessionId":"aaaa2222-3333-4444-5555-666666666666","timestamp":"2026-01-11T08:00:00Z"}`,
			},
		},
		{
			id: "bbbb1111-2222-3333-4444-555555555555",
			lines: []string{
				`{"type":"user","message":{"role":"user","content":"third session"},"cwd":"/test/proj","gitBranch":"feat","sessionId":"bbbb1111-2222-3333-4444-555555555555","timestamp":"2026-01-12T08:00:00Z"}`,
			},
		},
	}

	for _, s := range sessions {
		path := filepath.Join(projDir, s.id+".jsonl")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range s.lines {
			if _, err := fmt.Fprintln(f, line); err != nil {
				t.Fatal(err)
			}
		}
		_ = f.Close()
	}
}

func TestFindByPrefix_ExactMatch(t *testing.T) {
	setupSessionFixtures(t)

	s, err := FindByPrefix("bbbb1111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "bbbb1111-2222-3333-4444-555555555555" {
		t.Errorf("ID = %q, want bbbb1111-2222-3333-4444-555555555555", s.ID)
	}
}

func TestFindByPrefix_ShortID(t *testing.T) {
	setupSessionFixtures(t)

	s, err := FindByPrefix("bbbb1111")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "bbbb1111-2222-3333-4444-555555555555" {
		t.Errorf("ID = %q, want bbbb1111-2222-3333-4444-555555555555", s.ID)
	}
}

func TestFindByPrefix_NoMatch(t *testing.T) {
	setupSessionFixtures(t)

	_, err := FindByPrefix("zzzz9999")
	if err == nil {
		t.Fatal("expected error for no match")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %q, want ErrNotFound", err)
	}
}

func TestFindByPrefix_MultipleMatches(t *testing.T) {
	setupSessionFixtures(t)

	// "aaaa" matches both aaaa1111 and aaaa2222.
	_, err := FindByPrefix("aaaa")
	if err == nil {
		t.Fatal("expected error for multiple matches")
	}
	if !errors.Is(err, ErrMultipleMatches) {
		t.Errorf("error = %q, want ErrMultipleMatches", err)
	}
}

func TestFindByPrefixFull(t *testing.T) {
	setupSessionFixtures(t)

	s, err := FindByPrefixFull("bbbb1111")
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstPrompt != "third session" {
		t.Errorf("FirstPrompt = %q, want %q", s.FirstPrompt, "third session")
	}
	if s.ProjectPath != "/test/proj" {
		t.Errorf("ProjectPath = %q, want /test/proj", s.ProjectPath)
	}
	if s.GitBranch != "feat" {
		t.Errorf("GitBranch = %q, want feat", s.GitBranch)
	}
}

func TestExtractIDFromFilename(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/foo/bar/abcd-1234.jsonl", "abcd-1234"},
		{"/foo/bar/simple.jsonl", "simple"},
		{"file.jsonl", "file"},
	}
	for _, tt := range tests {
		got := ExtractIDFromFilename(tt.path)
		if got != tt.want {
			t.Errorf("ExtractIDFromFilename(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
