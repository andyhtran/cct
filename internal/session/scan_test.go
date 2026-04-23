//go:build darwin || linux

package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		files := DiscoverFiles("", true)
		if len(files) != 3 {
			t.Errorf("expected 3 files, got %d", len(files))
		}
	})

	t.Run("project filter", func(t *testing.T) {
		files := DiscoverFiles("myproject", true)
		if len(files) != 2 {
			t.Errorf("expected 2 files for myproject filter, got %d", len(files))
		}
	})

	t.Run("filter no match", func(t *testing.T) {
		files := DiscoverFiles("nonexistent", true)
		if len(files) != 0 {
			t.Errorf("expected 0 files for nonexistent filter, got %d", len(files))
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		files := DiscoverFiles("MyProject", true)
		if len(files) != 2 {
			t.Errorf("expected 2 files for case-insensitive filter, got %d", len(files))
		}
	})
}

func TestDiscoverFiles_AgentFiltering(t *testing.T) {
	home := setupTestHome(t)
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	writeSessionFile(t, projDir, "sess-aaa", []string{
		`{"type":"user","message":{"role":"user","content":"hello"},"cwd":"/test"}`,
	})
	writeSessionFile(t, projDir, "agent-12345678", []string{
		`{"type":"user","message":{"role":"user","content":"agent task"},"cwd":"/test"}`,
	})

	t.Run("excludes agents", func(t *testing.T) {
		files := DiscoverFiles("", false)
		if len(files) != 1 {
			t.Errorf("expected 1 file (no agents), got %d", len(files))
		}
	})

	t.Run("includes agents", func(t *testing.T) {
		files := DiscoverFiles("", true)
		if len(files) != 2 {
			t.Errorf("expected 2 files (with agents), got %d", len(files))
		}
	})
}

func writeMetaSidecar(t *testing.T, dir, agentID, agentType, description string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, agentID+".meta.json")
	body := fmt.Sprintf(`{"agentType":%q,"description":%q}`, agentType, description)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFiles_NestedSubagents(t *testing.T) {
	home := setupTestHome(t)
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")

	// Flat parent session.
	writeSessionFile(t, projDir, "parent-aaaa-bbbb-cccc-dddd", []string{
		`{"type":"user","message":{"role":"user","content":"parent prompt"},"cwd":"/test","sessionId":"parent-aaaa-bbbb-cccc-dddd"}`,
	})

	// Nested subagent: <projDir>/<parentID>/subagents/agent-<hex>.jsonl + sidecar.
	parentDir := filepath.Join(projDir, "parent-aaaa-bbbb-cccc-dddd")
	subagentDir := filepath.Join(parentDir, "subagents")
	writeSessionFile(t, subagentDir, "agent-abcdef1234567890", []string{
		`{"type":"user","message":{"role":"user","content":"nested agent prompt"},"cwd":"/test","isSidechain":true}`,
	})
	writeMetaSidecar(t, subagentDir, "agent-abcdef1234567890", "Explore", "Survey something")

	// Sibling tool-results dir should be ignored entirely.
	toolResultsDir := filepath.Join(parentDir, "tool-results")
	if err := os.MkdirAll(toolResultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolResultsDir, "foo.txt"), []byte("not a session"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("includeAgents=true picks up nested agent", func(t *testing.T) {
		files := DiscoverFiles("", true)
		if len(files) != 2 {
			t.Fatalf("expected 2 files (parent + nested agent), got %d: %v", len(files), files)
		}
		var sawNested bool
		for _, p := range files {
			if strings.Contains(p, filepath.Join("subagents", "agent-abcdef1234567890.jsonl")) {
				sawNested = true
			}
			if strings.HasSuffix(p, "foo.txt") {
				t.Errorf("tool-results file leaked into results: %s", p)
			}
		}
		if !sawNested {
			t.Error("nested subagent jsonl not discovered")
		}
	})

	t.Run("includeAgents=false excludes nested agent", func(t *testing.T) {
		files := DiscoverFiles("", false)
		if len(files) != 1 {
			t.Fatalf("expected 1 file (parent only), got %d: %v", len(files), files)
		}
	})

	t.Run("meta sidecar populates AgentType/AgentDescription", func(t *testing.T) {
		nested := filepath.Join(subagentDir, "agent-abcdef1234567890.jsonl")
		s := ExtractMetadata(nested)
		if s == nil {
			t.Fatal("ExtractMetadata returned nil")
		}
		if !s.IsAgent {
			t.Error("IsAgent = false, want true")
		}
		if s.AgentType != "Explore" {
			t.Errorf("AgentType = %q, want Explore", s.AgentType)
		}
		if s.AgentDescription != "Survey something" {
			t.Errorf("AgentDescription = %q, want Survey something", s.AgentDescription)
		}
	})

	t.Run("missing sidecar leaves fields empty", func(t *testing.T) {
		// Write a nested agent without a meta sidecar — should parse cleanly.
		writeSessionFile(t, subagentDir, "agent-nometa9999", []string{
			`{"type":"user","message":{"role":"user","content":"bare"},"cwd":"/test"}`,
		})
		s := ExtractMetadata(filepath.Join(subagentDir, "agent-nometa9999.jsonl"))
		if s == nil {
			t.Fatal("ExtractMetadata returned nil")
		}
		if s.AgentType != "" || s.AgentDescription != "" {
			t.Errorf("expected empty agent fields, got type=%q desc=%q", s.AgentType, s.AgentDescription)
		}
	})
}

func TestDiscoverFilesWithBackups(t *testing.T) {
	home := setupTestHome(t)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	// Live session in projects dir.
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	writeSessionFile(t, projDir, "aaaa1111-2222-3333-4444-555555555555", []string{
		`{"type":"user","message":{"role":"user","content":"live session"}}`,
	})

	// Adopted session: lives only in the backup mirror, not the live tree.
	// Mirrors the state after an upstream cleanup has wiped the source.
	backupProj := filepath.Join(home, ".cache", "cct", "backup", "projects", "-Users-test-myproject")
	writeSessionFile(t, backupProj, "bbbb2222-3333-4444-5555-666666666666", []string{
		`{"type":"user","message":{"role":"user","content":"adopted session"}}`,
	})

	// Dup session: present in both trees. Live path must win in the dedupe.
	writeSessionFile(t, projDir, "cccc3333-4444-5555-6666-777777777777", []string{
		`{"type":"user","message":{"role":"user","content":"live wins"}}`,
	})
	writeSessionFile(t, backupProj, "cccc3333-4444-5555-6666-777777777777", []string{
		`{"type":"user","message":{"role":"user","content":"backup stale"}}`,
	})

	files := DiscoverFilesWithBackups("", true)
	if len(files) != 3 {
		t.Fatalf("expected 3 unique sessions, got %d: %v", len(files), files)
	}

	var sawAdopted, sawDupLive bool
	for _, p := range files {
		if strings.HasSuffix(p, "bbbb2222-3333-4444-5555-666666666666.jsonl") {
			if !strings.Contains(p, ".cache") {
				t.Errorf("adopted session should be the backup path, got %s", p)
			}
			sawAdopted = true
		}
		if strings.HasSuffix(p, "cccc3333-4444-5555-6666-777777777777.jsonl") {
			if strings.Contains(p, ".cache") {
				t.Errorf("duplicated session should resolve to the live path, got %s", p)
			}
			sawDupLive = true
		}
	}
	if !sawAdopted {
		t.Error("adopted (backup-only) session missing from results")
	}
	if !sawDupLive {
		t.Error("duplicated session did not resolve to the live path")
	}
}

func TestDiscoverFiles_MissingDir(t *testing.T) {
	home := setupTestHome(t)
	_ = home

	files := DiscoverFiles("", true)
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
		sessions := ScanAll("", false, true)
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
		sessions := ScanAll("", true, true)
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		s := sessions[0]
		if s.MessageCount != 2 {
			t.Errorf("MessageCount = %d, want 2", s.MessageCount)
		}
	})

	t.Run("with project filter", func(t *testing.T) {
		sessions := ScanAll("proj", false, true)
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		sessions = ScanAll("nonexistent", false, true)
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

	files := DiscoverFiles("", true)

	t.Run("keyword found", func(t *testing.T) {
		results := SearchFiles(files, "database", 80, 3)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if len(results[0].Matches) == 0 {
			t.Error("expected at least 1 match snippet")
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		results := SearchFiles(files, "DATABASE", 80, 3)
		if len(results) != 1 {
			t.Errorf("expected 1 result for case-insensitive search, got %d", len(results))
		}
	})

	t.Run("no match", func(t *testing.T) {
		results := SearchFiles(files, "zzz_nothing_zzz", 80, 3)
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

	files := DiscoverFiles("", true)

	t.Run("finds text in tool_result content", func(t *testing.T) {
		results := SearchFiles(files, "postgres", 80, 3)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if len(results[0].Matches) == 0 {
			t.Error("expected at least 1 match snippet")
		}
	})

	t.Run("raw JSON not searchable", func(t *testing.T) {
		// Raw JSON like "name":"Read" is not in the extracted text (extracted as "Read /path/...")
		results := SearchFiles(files, "\"name\":\"Read\"", 80, 3)
		if len(results) != 0 {
			t.Errorf("expected 0 results for raw JSON, got %d", len(results))
		}
	})
}

func TestSearchFiles_ToolUse(t *testing.T) {
	home := setupTestHome(t)
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-proj")

	writeSessionFile(t, projDir, "tool-use-001", []string{
		`{"type":"user","message":{"role":"user","content":"show me the file"},"cwd":"/test","sessionId":"tool-use-001","timestamp":"2026-01-10T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/path/to/config.go"}},{"type":"tool_result","tool_use_id":"tu1","content":"package config"}]},"timestamp":"2026-01-10T08:00:05Z"}`,
	})

	files := DiscoverFiles("", true)

	t.Run("finds file path in tool_use input", func(t *testing.T) {
		results := SearchFiles(files, "config.go", 80, 3)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if len(results[0].Matches) == 0 {
			t.Fatal("expected at least 1 match snippet")
		}
		m := results[0].Matches[0]
		if m.Source != "Read" {
			t.Errorf("Source = %q, want %q", m.Source, "Read")
		}
	})

	t.Run("finds tool name", func(t *testing.T) {
		results := SearchFiles(files, "Read", 80, 3)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Matches[0].Source != "Read" {
			t.Errorf("Source = %q, want %q", results[0].Matches[0].Source, "Read")
		}
	})

	t.Run("finds bash command", func(t *testing.T) {
		home2 := setupTestHome(t)
		projDir2 := filepath.Join(home2, ".claude", "projects", "-Users-test-proj")
		writeSessionFile(t, projDir2, "tool-use-002", []string{
			`{"type":"user","message":{"role":"user","content":"compile the project"},"cwd":"/test","sessionId":"tool-use-002","timestamp":"2026-01-10T08:00:00Z"}`,
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu2","name":"Bash","input":{"command":"go build ./..."}}]},"timestamp":"2026-01-10T08:00:05Z"}`,
		})
		files2 := DiscoverFiles("", true)
		results := SearchFiles(files2, "go build", 80, 3)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Matches[0].Source != "Bash" {
			t.Errorf("Source = %q, want %q", results[0].Matches[0].Source, "Bash")
		}
	})

	t.Run("text match has empty source", func(t *testing.T) {
		results := SearchFiles(files, "show me the file", 80, 3)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Matches[0].Source != "" {
			t.Errorf("Source = %q, want empty for text match", results[0].Matches[0].Source)
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

	files := DiscoverFiles("", true)
	results := SearchFiles(files, "keyword", 80, 3)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Matches) > 3 {
		t.Errorf("expected at most 3 matches per file, got %d", len(results[0].Matches))
	}
}
