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

func TestFindByPrefix_CustomTitle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Session with a custom title (set via /rename).
	titleLines := []string{
		`{"type":"user","message":{"role":"user","content":"fix thing"},"cwd":"/test/proj","sessionId":"cafe1111-2222-3333-4444-555555555555","timestamp":"2026-03-01T08:00:00Z"}`,
		`{"type":"custom-title","customTitle":"fix-keyboard-dictation-return","sessionId":"cafe1111-2222-3333-4444-555555555555"}`,
	}
	// A decoy session whose UUID prefix would otherwise collide if the
	// user happened to query "cafe" — here we test title lookup doesn't
	// conflict with ID prefix lookup.
	plainLines := []string{
		`{"type":"user","message":{"role":"user","content":"other"},"cwd":"/test/proj","sessionId":"deaf1111-2222-3333-4444-555555555555","timestamp":"2026-03-02T08:00:00Z"}`,
	}

	for _, s := range []struct {
		id    string
		lines []string
	}{
		{"cafe1111-2222-3333-4444-555555555555", titleLines},
		{"deaf1111-2222-3333-4444-555555555555", plainLines},
	} {
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

	t.Run("exact title match", func(t *testing.T) {
		s, err := FindByPrefix("fix-keyboard-dictation-return")
		if err != nil {
			t.Fatal(err)
		}
		if s.ID != "cafe1111-2222-3333-4444-555555555555" {
			t.Errorf("ID = %q, want cafe1111-...", s.ID)
		}
	})

	t.Run("title match is case-insensitive", func(t *testing.T) {
		s, err := FindByPrefix("FIX-Keyboard-Dictation-RETURN")
		if err != nil {
			t.Fatal(err)
		}
		if s.ID != "cafe1111-2222-3333-4444-555555555555" {
			t.Errorf("ID = %q, want cafe1111-...", s.ID)
		}
	})

	t.Run("unknown title still errors", func(t *testing.T) {
		_, err := FindByPrefix("not-a-real-title")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
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

func TestIsAgentSession(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"agent-12345678", true},
		{"agent-abcdef12", true},
		{"agent-", true},
		{"aaaa1111-2222-3333-4444-555555555555", false},
		{"", false},
		{"agents-not-an-agent", false},
	}
	for _, tt := range tests {
		if got := IsAgentSession(tt.id); got != tt.want {
			t.Errorf("IsAgentSession(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestShortID_Agent(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"agent-12345678", "agent-12345678"},
		{"agent-ab", "agent-ab"},
		{"aaaa1111-2222-3333-4444-555555555555", "aaaa1111"},
		{"short", "short"},
	}
	for _, tt := range tests {
		if got := ShortID(tt.id); got != tt.want {
			t.Errorf("ShortID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestFindByPrefix_Agent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, s := range []struct{ id, content string }{
		{"sess-1111-2222-3333-4444-555555555555", `{"type":"user","message":{"role":"user","content":"regular"},"cwd":"/test"}`},
		{"agent-abcdef12", `{"type":"user","message":{"role":"user","content":"agent task"},"cwd":"/test"}`},
	} {
		path := filepath.Join(projDir, s.id+".jsonl")
		if err := os.WriteFile(path, []byte(s.content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s, err := FindByPrefix("agent-abcdef12")
	if err != nil {
		t.Fatalf("FindByPrefix agent: %v", err)
	}
	if !s.IsAgent {
		t.Error("expected IsAgent=true for agent session")
	}

	s, err = FindByPrefix("sess-1111")
	if err != nil {
		t.Fatalf("FindByPrefix regular: %v", err)
	}
	if s.IsAgent {
		t.Error("expected IsAgent=false for regular session")
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
