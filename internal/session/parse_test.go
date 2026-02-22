package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFastExtractType(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"user", `{"type":"user","message":{"role":"user"}}`, "user"},
		{"assistant", `{"type":"assistant","message":{"role":"assistant"}}`, "assistant"},
		{"empty", `{}`, ""},
		{"no type", `{"message":"hello"}`, ""},
		{"file-history-snapshot", `{"type":"file-history-snapshot","snapshot":{}}`, "file-history-snapshot"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FastExtractType([]byte(tt.line))
			if got != tt.want {
				t.Errorf("FastExtractType(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestFastExtractTimestamp(t *testing.T) {
	t.Run("valid timestamp", func(t *testing.T) {
		line := `{"type":"assistant","timestamp":"2026-01-15T10:30:00Z","message":{}}`
		got := FastExtractTimestamp([]byte(line))
		if got.IsZero() {
			t.Error("expected non-zero time")
		}
		if got.Year() != 2026 || got.Month() != 1 || got.Day() != 15 {
			t.Errorf("unexpected time: %v", got)
		}
	})

	t.Run("no timestamp", func(t *testing.T) {
		line := `{"type":"user","message":{}}`
		got := FastExtractTimestamp([]byte(line))
		if !got.IsZero() {
			t.Errorf("expected zero time, got %v", got)
		}
	})
}

func TestExtractTextFromContent(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		got := ExtractTextFromContent("hello world")
		if got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("nil content", func(t *testing.T) {
		got := ExtractTextFromContent(nil)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("array content with text blocks", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "text", "text": "hello"},
			map[string]any{"type": "text", "text": "world"},
		}
		got := ExtractTextFromContent(content)
		if got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("array content skips thinking/tool_use", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "thinking", "text": "hmm"},
			map[string]any{"type": "text", "text": "visible"},
			map[string]any{"type": "tool_use", "text": "hidden"},
		}
		got := ExtractTextFromContent(content)
		if got != "visible" {
			t.Errorf("got %q, want %q", got, "visible")
		}
	})
}

func TestExtractMetadata(t *testing.T) {
	// Create a temp JSONL file
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session-id.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hello world"},"cwd":"/Users/test/project","gitBranch":"main","sessionId":"abc-123","timestamp":"2026-01-15T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hi there!"}]},"timestamp":"2026-01-15T10:00:05Z"}`,
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	s := ExtractMetadata(path)
	if s == nil {
		t.Fatal("expected non-nil session")
	}

	if s.ID != "abc-123" {
		t.Errorf("ID = %q, want %q", s.ID, "abc-123")
	}
	if s.ProjectPath != "/Users/test/project" {
		t.Errorf("ProjectPath = %q, want %q", s.ProjectPath, "/Users/test/project")
	}
	if s.ProjectName != "project" {
		t.Errorf("ProjectName = %q, want %q", s.ProjectName, "project")
	}
	if s.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want %q", s.GitBranch, "main")
	}
	if s.FirstPrompt != "hello world" {
		t.Errorf("FirstPrompt = %q, want %q", s.FirstPrompt, "hello world")
	}
	if s.Created.Year() != 2026 {
		t.Errorf("Created year = %d, want 2026", s.Created.Year())
	}
}

func TestParseFullSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-full-id.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"first prompt"},"cwd":"/test","gitBranch":"dev","sessionId":"full-123","timestamp":"2026-02-01T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"response 1"}]},"timestamp":"2026-02-01T08:00:05Z"}`,
		`{"type":"user","message":{"role":"user","content":"second prompt"},"timestamp":"2026-02-01T08:01:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"response 2"}]},"timestamp":"2026-02-01T08:01:05Z"}`,
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	s := ParseFullSession(path)
	if s == nil {
		t.Fatal("expected non-nil session")
	}

	if s.MessageCount != 4 {
		t.Errorf("MessageCount = %d, want 4", s.MessageCount)
	}
	if s.FirstPrompt != "first prompt" {
		t.Errorf("FirstPrompt = %q, want %q", s.FirstPrompt, "first prompt")
	}
	if s.ID != "full-123" {
		t.Errorf("ID = %q, want %q", s.ID, "full-123")
	}
}

func TestShortID(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"abcdefgh-1234-5678", "abcdefgh"},
		{"short", "short"},
		{"12345678", "12345678"},
		{"", ""},
	}

	for _, tt := range tests {
		got := ShortID(tt.id)
		if got != tt.want {
			t.Errorf("ShortID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestExtractUserMetadata(t *testing.T) {
	t.Run("populates all fields from complete message", func(t *testing.T) {
		s := &Session{ID: "file-id", ShortID: "file-id"}
		obj := map[string]any{
			"cwd":       "/Users/test/myproject",
			"gitBranch": "feature-x",
			"sessionId": "real-session-id-1234",
			"timestamp": "2026-03-01T09:00:00Z",
			"message":   map[string]any{"role": "user", "content": "implement auth"},
		}

		complete := extractUserMetadata(s, obj)
		if !complete {
			t.Error("expected complete=true when ProjectPath and FirstPrompt are set")
		}
		if s.ProjectPath != "/Users/test/myproject" {
			t.Errorf("ProjectPath = %q, want %q", s.ProjectPath, "/Users/test/myproject")
		}
		if s.ProjectName != "myproject" {
			t.Errorf("ProjectName = %q, want %q", s.ProjectName, "myproject")
		}
		if s.GitBranch != "feature-x" {
			t.Errorf("GitBranch = %q, want %q", s.GitBranch, "feature-x")
		}
		if s.ID != "real-session-id-1234" {
			t.Errorf("ID = %q, want %q", s.ID, "real-session-id-1234")
		}
		if s.FirstPrompt != "implement auth" {
			t.Errorf("FirstPrompt = %q, want %q", s.FirstPrompt, "implement auth")
		}
		if s.Created.IsZero() {
			t.Error("expected Created to be set")
		}
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		s := &Session{
			ID:          "original-id",
			ShortID:     "original",
			ProjectPath: "/existing/path",
			ProjectName: "path",
			FirstPrompt: "existing prompt",
			Created:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		obj := map[string]any{
			"cwd":       "/new/path",
			"sessionId": "new-id",
			"timestamp": "2026-06-01T00:00:00Z",
			"message":   map[string]any{"role": "user", "content": "new prompt"},
		}

		complete := extractUserMetadata(s, obj)
		if !complete {
			t.Error("expected complete=true")
		}
		if s.ProjectPath != "/existing/path" {
			t.Errorf("ProjectPath changed to %q", s.ProjectPath)
		}
		if s.ID != "original-id" {
			t.Errorf("ID changed to %q", s.ID)
		}
		if s.FirstPrompt != "existing prompt" {
			t.Errorf("FirstPrompt changed to %q", s.FirstPrompt)
		}
		if s.Created.Year() != 2026 || s.Created.Month() != 1 {
			t.Errorf("Created changed to %v", s.Created)
		}
	})

	t.Run("returns false when metadata is incomplete", func(t *testing.T) {
		s := &Session{}
		obj := map[string]any{
			"cwd":     "/some/path",
			"message": map[string]any{"role": "user", "content": []any{}},
		}

		complete := extractUserMetadata(s, obj)
		if complete {
			t.Error("expected complete=false when FirstPrompt is empty")
		}
		if s.ProjectPath != "/some/path" {
			t.Errorf("ProjectPath = %q, want %q", s.ProjectPath, "/some/path")
		}
	})
}

func TestParseTimestamp(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		obj := map[string]any{"timestamp": "2026-01-15T10:30:00Z"}
		got := ParseTimestamp(obj)
		if got.IsZero() {
			t.Error("expected non-zero time")
		}
		want := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("missing", func(t *testing.T) {
		obj := map[string]any{"other": "value"}
		got := ParseTimestamp(obj)
		if !got.IsZero() {
			t.Errorf("expected zero time, got %v", got)
		}
	})
}
