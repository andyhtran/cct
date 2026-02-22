//go:build darwin || linux

package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func setupTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeSessionFile(t *testing.T, dir, id string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for _, line := range lines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscoverFiles(t *testing.T) {
	home := setupTestHome(t)
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	writeSessionFile(t, projDir, "sess-aaa", []string{
		`{"type":"user","message":{"role":"user","content":"hello"},"cwd":"/test","sessionId":"sess-aaa"}`,
	})
	writeSessionFile(t, projDir, "sess-bbb", []string{
		`{"type":"user","message":{"role":"user","content":"world"},"cwd":"/test","sessionId":"sess-bbb"}`,
	})

	proj2Dir := filepath.Join(home, ".claude", "projects", "-Users-test-other")
	writeSessionFile(t, proj2Dir, "sess-ccc", []string{
		`{"type":"user","message":{"role":"user","content":"other"},"cwd":"/test/other","sessionId":"sess-ccc"}`,
	})

	t.Run("no filter", func(t *testing.T) {
		files := DiscoverFiles("")
		if len(files) != 3 {
			t.Errorf("expected 3 files, got %d", len(files))
		}
	})

	t.Run("project filter", func(t *testing.T) {
		files := DiscoverFiles("myproject")
		if len(files) != 2 {
			t.Errorf("expected 2 files for myproject filter, got %d", len(files))
		}
	})

	t.Run("filter no match", func(t *testing.T) {
		files := DiscoverFiles("nonexistent")
		if len(files) != 0 {
			t.Errorf("expected 0 files for nonexistent filter, got %d", len(files))
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		files := DiscoverFiles("MyProject")
		if len(files) != 2 {
			t.Errorf("expected 2 files for case-insensitive filter, got %d", len(files))
		}
	})
}

func TestDiscoverFiles_MissingDir(t *testing.T) {
	home := setupTestHome(t)
	// Don't create the projects directory.
	_ = home

	files := DiscoverFiles("")
	if files != nil {
		t.Errorf("expected nil for missing dir, got %v", files)
	}
}

func TestScanAll(t *testing.T) {
	home := setupTestHome(t)
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-proj")
	writeSessionFile(t, projDir, "scan-001", []string{
		`{"type":"user","message":{"role":"user","content":"first prompt"},"cwd":"/test/proj","gitBranch":"main","sessionId":"scan-001","timestamp":"2026-01-10T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"response"}]},"timestamp":"2026-01-10T08:00:05Z"}`,
	})

	t.Run("quick scan", func(t *testing.T) {
		sessions := ScanAll("", false)
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		s := sessions[0]
		if s.ProjectPath != "/test/proj" {
			t.Errorf("ProjectPath = %q, want /test/proj", s.ProjectPath)
		}
		if s.FirstPrompt != "first prompt" {
			t.Errorf("FirstPrompt = %q, want %q", s.FirstPrompt, "first prompt")
		}
		if s.MessageCount != 0 {
			t.Errorf("MessageCount = %d, want 0 (quick scan)", s.MessageCount)
		}
	})

	t.Run("full parse", func(t *testing.T) {
		sessions := ScanAll("", true)
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		s := sessions[0]
		if s.MessageCount != 2 {
			t.Errorf("MessageCount = %d, want 2", s.MessageCount)
		}
	})

	t.Run("with project filter", func(t *testing.T) {
		sessions := ScanAll("proj", false)
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		sessions = ScanAll("nonexistent", false)
		if len(sessions) != 0 {
			t.Errorf("expected 0 sessions for nonexistent filter, got %d", len(sessions))
		}
	})
}

func TestSearchFiles(t *testing.T) {
	home := setupTestHome(t)
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-proj")

	writeSessionFile(t, projDir, "srch-001", []string{
		`{"type":"user","message":{"role":"user","content":"fix the database migration bug"},"cwd":"/test","sessionId":"srch-001","timestamp":"2026-01-10T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I will fix the database issue."}]},"timestamp":"2026-01-10T08:00:05Z"}`,
	})
	writeSessionFile(t, projDir, "srch-002", []string{
		`{"type":"user","message":{"role":"user","content":"add authentication"},"cwd":"/test","sessionId":"srch-002","timestamp":"2026-01-11T08:00:00Z"}`,
	})

	files := DiscoverFiles("")

	t.Run("keyword found", func(t *testing.T) {
		results := SearchFiles(files, "database", 80)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if len(results[0].Matches) == 0 {
			t.Error("expected at least 1 match snippet")
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		results := SearchFiles(files, "DATABASE", 80)
		if len(results) != 1 {
			t.Errorf("expected 1 result for case-insensitive search, got %d", len(results))
		}
	})

	t.Run("no match", func(t *testing.T) {
		results := SearchFiles(files, "zzz_nothing_zzz", 80)
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})
}

func TestSearchFiles_ToolResult(t *testing.T) {
	home := setupTestHome(t)
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-proj")

	writeSessionFile(t, projDir, "tool-001", []string{
		`{"type":"user","message":{"role":"user","content":"read the config"},"cwd":"/test","sessionId":"tool-001","timestamp":"2026-01-10T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Read","input":{}},{"type":"tool_result","tool_use_id":"tu1","content":"database_url=postgres://localhost"}]},"timestamp":"2026-01-10T08:00:05Z"}`,
	})

	files := DiscoverFiles("")

	t.Run("finds text in tool_result content", func(t *testing.T) {
		results := SearchFiles(files, "postgres", 80)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if len(results[0].Matches) == 0 {
			t.Error("expected at least 1 match snippet")
		}
	})

	t.Run("still skips tool_use", func(t *testing.T) {
		// "Read" only appears inside tool_use which should be skipped
		results := SearchFiles(files, "\"name\":\"Read\"", 80)
		if len(results) != 0 {
			t.Errorf("expected 0 results for tool_use content, got %d", len(results))
		}
	})
}

func TestSearchFiles_MaxMatches(t *testing.T) {
	home := setupTestHome(t)
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-proj")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"keyword here"},"cwd":"/test","sessionId":"srch-max","timestamp":"2026-01-10T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"keyword again"}]},"timestamp":"2026-01-10T08:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":"keyword third"},"timestamp":"2026-01-10T08:00:02Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"keyword fourth"}]},"timestamp":"2026-01-10T08:00:03Z"}`,
		`{"type":"user","message":{"role":"user","content":"keyword fifth"},"timestamp":"2026-01-10T08:00:04Z"}`,
	}
	writeSessionFile(t, projDir, "srch-max", lines)

	files := DiscoverFiles("")
	results := SearchFiles(files, "keyword", 80)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Matches) > 3 {
		t.Errorf("expected at most 3 matches per file, got %d", len(results[0].Matches))
	}
}
