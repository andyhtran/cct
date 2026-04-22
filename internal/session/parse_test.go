package session

import (
	"os"
	"path/filepath"
	"strings"
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

	t.Run("skips thinking, extracts tool_use name+input only", func(t *testing.T) {
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

	t.Run("tool_result with string content", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": "database_url=postgres://localhost"},
		}
		got := ExtractTextFromContent(content)
		if got != "database_url=postgres://localhost" {
			t.Errorf("got %q, want %q", got, "database_url=postgres://localhost")
		}
	})

	t.Run("tool_result with array content", func(t *testing.T) {
		content := []any{
			map[string]any{
				"type":        "tool_result",
				"tool_use_id": "tu1",
				"content": []any{
					map[string]any{"type": "text", "text": "agent response"},
					map[string]any{"type": "image", "source": map[string]any{}},
				},
			},
		}
		got := ExtractTextFromContent(content)
		if got != "agent response" {
			t.Errorf("got %q, want %q", got, "agent response")
		}
	})

	t.Run("tool_result with base64-like content", func(t *testing.T) {
		longBase64 := strings.Repeat("AAAA", 300) // 1200 chars, no spaces
		content := []any{
			map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": longBase64},
		}
		got := ExtractTextFromContent(content)
		if got != "" {
			t.Errorf("expected empty for base64-like content, got %d chars", len(got))
		}
	})

	t.Run("skips redacted_thinking and document", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "redacted_thinking", "data": "base64stuff"},
			map[string]any{"type": "document", "source": map[string]any{}},
			map[string]any{"type": "text", "text": "visible"},
		}
		got := ExtractTextFromContent(content)
		if got != "visible" {
			t.Errorf("got %q, want %q", got, "visible")
		}
	})

	t.Run("mixed text and tool_result", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "text", "text": "I found the config"},
			map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": "port=5432"},
		}
		got := ExtractTextFromContent(content)
		if got != "I found the config port=5432" {
			t.Errorf("got %q, want %q", got, "I found the config port=5432")
		}
	})

	t.Run("respects max recursion depth", func(t *testing.T) {
		// Build a structure deeper than maxExtractDepth
		inner := map[string]any{"type": "text", "text": "deep"}
		for i := 0; i < maxExtractDepth+5; i++ {
			inner = map[string]any{"type": "wrapper", "content": inner}
		}
		content := []any{inner}
		got := ExtractTextFromContent(content)
		if got != "" {
			t.Errorf("expected empty beyond max depth, got %q", got)
		}
	})
}

func TestExtractMetadata(t *testing.T) {
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

	if s.ID != "test-session-id" {
		t.Errorf("ID = %q, want %q (file-derived ID must be preserved)", s.ID, "test-session-id")
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

func TestExtractMetadata_CustomTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cust-title.jsonl")

	// custom-title records are written after the user message (Claude
	// sets the title mid-session via /rename) and may be rewritten per
	// turn — the last value must win.
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"draft plan"},"cwd":"/p","gitBranch":"main","sessionId":"sid","timestamp":"2026-03-01T08:00:00Z"}`,
		`{"type":"custom-title","customTitle":"old-title","sessionId":"sid"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]},"timestamp":"2026-03-01T08:00:05Z"}`,
		`{"type":"custom-title","customTitle":"latest-title","sessionId":"sid"}`,
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

	meta := ExtractMetadata(path)
	if meta == nil {
		t.Fatal("ExtractMetadata returned nil")
	}
	if meta.CustomTitle != "latest-title" {
		t.Errorf("metadata CustomTitle = %q, want %q", meta.CustomTitle, "latest-title")
	}
	if meta.FirstPrompt != "draft plan" {
		t.Errorf("metadata FirstPrompt = %q, want %q", meta.FirstPrompt, "draft plan")
	}

	full := ParseFullSession(path)
	if full == nil {
		t.Fatal("ParseFullSession returned nil")
	}
	if full.CustomTitle != "latest-title" {
		t.Errorf("full CustomTitle = %q, want %q", full.CustomTitle, "latest-title")
	}
}

func TestExtractMetadata_NoCustomTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-title.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hi"},"cwd":"/p","gitBranch":"main","sessionId":"sid","timestamp":"2026-03-01T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]},"timestamp":"2026-03-01T08:00:05Z"}`,
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
	if s.CustomTitle != "" {
		t.Errorf("expected empty CustomTitle for session without /rename, got %q", s.CustomTitle)
	}
}

func TestParseFullSession_Usage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage-sess.jsonl")

	// Three assistant turns: peak context is 40_000 (turn 2), then a small
	// compaction drops to 15_000 (turn 3). A synthetic turn at the end must
	// be ignored so it doesn't clobber Model/ContextTokens.
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hi"},"cwd":"/p","sessionId":"s","timestamp":"2026-03-01T00:00:00Z"}`,
		`{"type":"assistant","timestamp":"2026-03-01T00:00:01Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"a"}],"usage":{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":9000,"output_tokens":50}}}`,
		`{"type":"assistant","timestamp":"2026-03-01T00:00:02Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"b"}],"usage":{"input_tokens":5,"cache_creation_input_tokens":2000,"cache_read_input_tokens":37995,"output_tokens":100}}}`,
		`{"type":"assistant","timestamp":"2026-03-01T00:00:03Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"c"}],"usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":15000,"output_tokens":25}}}`,
		`{"type":"assistant","timestamp":"2026-03-01T00:00:04Z","message":{"role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"local"}],"usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}}`,
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

	if s.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want claude-opus-4-7 (synthetic turn must not overwrite)", s.Model)
	}
	if s.ContextTokens != 15_000 {
		t.Errorf("ContextTokens = %d, want 15000 (last real turn)", s.ContextTokens)
	}
	if s.PeakContextTokens != 40_000 {
		t.Errorf("PeakContextTokens = %d, want 40000", s.PeakContextTokens)
	}
	if s.TotalOutputTokens != 175 {
		t.Errorf("TotalOutputTokens = %d, want 175 (50+100+25)", s.TotalOutputTokens)
	}
}

func TestExtractMetadata_NoUsage(t *testing.T) {
	// Metadata-only scan (used by list/search) must NOT populate usage fields,
	// since it skips the JSON unmarshal on assistant lines for speed.
	dir := t.TempDir()
	path := filepath.Join(dir, "meta-only.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hi"},"cwd":"/p","sessionId":"s","timestamp":"2026-03-01T00:00:00Z"}`,
		`{"type":"assistant","timestamp":"2026-03-01T00:00:01Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":10,"cache_creation_input_tokens":100,"cache_read_input_tokens":200,"output_tokens":5}}}`,
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
	if s.ContextTokens != 0 || s.Model != "" {
		t.Errorf("ExtractMetadata must skip usage; got Model=%q ContextTokens=%d", s.Model, s.ContextTokens)
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
	if s.ID != "test-full-id" {
		t.Errorf("ID = %q, want %q (file-derived ID must be preserved)", s.ID, "test-full-id")
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

		complete := ExtractUserMetadata(s, obj)
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
		if s.ID != "file-id" {
			t.Errorf("ID = %q, want %q (sessionId should not override file-derived ID)", s.ID, "file-id")
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

		complete := ExtractUserMetadata(s, obj)
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

	t.Run("sub-agent sessionId does not override file-derived ID", func(t *testing.T) {
		// Sub-agent files (agent-*.jsonl) contain a sessionId pointing to their
		// parent session. The file-derived ID must be preserved to avoid duplicate
		// ID collisions when multiple sub-agents share the same parent.
		s := &Session{ID: "agent-19b8cb-fake-uuid", ShortID: "agent-19"}
		obj := map[string]any{
			"cwd":       "/Users/test/project",
			"sessionId": "c8035fd7-parent-session-id",
			"message":   map[string]any{"role": "user", "content": "sub-agent task"},
		}

		ExtractUserMetadata(s, obj)
		if s.ID != "agent-19b8cb-fake-uuid" {
			t.Errorf("ID = %q, want %q (sub-agent ID must not be replaced by parent sessionId)", s.ID, "agent-19b8cb-fake-uuid")
		}
		if s.ShortID != "agent-19" {
			t.Errorf("ShortID = %q, want %q", s.ShortID, "agent-19")
		}
	})

	t.Run("returns false when metadata is incomplete", func(t *testing.T) {
		s := &Session{}
		obj := map[string]any{
			"cwd":     "/some/path",
			"message": map[string]any{"role": "user", "content": []any{}},
		}

		complete := ExtractUserMetadata(s, obj)
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

func TestOffsetScanner(t *testing.T) {
	lines := "line one\nline two\nline three\n"
	scanner := NewOffsetScanner(strings.NewReader(lines))

	type expected struct {
		text   string
		offset int64
		length int
	}

	want := []expected{
		{"line one", 0, 9},     // "line one\n" = 9 bytes
		{"line two", 9, 9},     // "line two\n" = 9 bytes
		{"line three", 18, 11}, // "line three\n" = 11 bytes
	}

	for i, w := range want {
		if !scanner.Scan() {
			t.Fatalf("line %d: Scan() returned false early", i)
		}
		if got := string(scanner.Bytes()); got != w.text {
			t.Errorf("line %d: Bytes() = %q, want %q", i, got, w.text)
		}
		if got := scanner.Offset(); got != w.offset {
			t.Errorf("line %d: Offset() = %d, want %d", i, got, w.offset)
		}
		if got := scanner.Length(); got != w.length {
			t.Errorf("line %d: Length() = %d, want %d", i, got, w.length)
		}
	}

	if scanner.Scan() {
		t.Error("expected Scan() to return false after last line")
	}
}

func TestOffsetScanner_NoTrailingNewline(t *testing.T) {
	scanner := NewOffsetScanner(strings.NewReader("only line"))

	if !scanner.Scan() {
		t.Fatal("expected Scan() to return true")
	}
	if got := string(scanner.Bytes()); got != "only line" {
		t.Errorf("Bytes() = %q, want %q", got, "only line")
	}
	if got := scanner.Offset(); got != 0 {
		t.Errorf("Offset() = %d, want 0", got)
	}
}
