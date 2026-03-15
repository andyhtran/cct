//go:build darwin || linux

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andyhtran/cct/internal/index"
)

// setupFixtures creates a fake ~/.claude tree with session, plan, and changelog
// data. It returns the temp HOME directory. The caller must use t.Setenv("HOME", ...)
// before calling any functions that read from ClaudeDir().
func setupFixtures(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	claudeDir := filepath.Join(home, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects")
	plansDir := filepath.Join(claudeDir, "plans")
	cacheDir := filepath.Join(claudeDir, "cache")

	projDir := filepath.Join(projectsDir, "-Users-test-myproject")
	for _, d := range []string{projDir, plansDir, cacheDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	sessionLines := []string{
		`{"type":"user","message":{"role":"user","content":"fix the database bug"},"cwd":"/Users/test/myproject","gitBranch":"main","sessionId":"abcd1234-5678-9abc-def0-111111111111","timestamp":"2026-02-01T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix the database bug."}]},"timestamp":"2026-02-01T08:00:05Z"}`,
		`{"type":"user","message":{"role":"user","content":"now add tests"},"timestamp":"2026-02-01T08:01:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done, tests added."}]},"timestamp":"2026-02-01T08:01:05Z"}`,
	}

	sessionPath := filepath.Join(projDir, "abcd1234-5678-9abc-def0-111111111111.jsonl")
	writeLines(t, sessionPath, sessionLines)

	planContent := "# Refactor Auth\n\nThis plan covers the authentication refactor.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "auth-refactor.md"), []byte(planContent), 0o644); err != nil {
		t.Fatal(err)
	}

	changelogContent := "# Changelog\n\n## 2.1.50\n\n- Added feature A\n- Fixed bug B\n\n## 2.1.49\n\n- Added feature C\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "changelog.md"), []byte(changelogContent), 0o644); err != nil {
		t.Fatal(err)
	}

	return home
}

func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestListCmd_JSON(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &ListCmd{Limit: 10}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var sessions []map[string]any
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0]["project_name"] != "myproject" {
		t.Errorf("project_name = %v, want myproject", sessions[0]["project_name"])
	}
}

func TestDefaultCmd_JSON(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &DefaultCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var sessions []map[string]any
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestSearchCmd_JSON(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &SearchCmd{Query: "database"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var results []map[string]any
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 search result")
	}
	if _, ok := results[0]["id"]; !ok {
		t.Fatal("expected id field in result (flattened session)")
	}
}

func TestSearchCmd_NoResults(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: false}
	cmd := &SearchCmd{Query: "zzz_nonexistent_zzz"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	if out == "" {
		t.Fatal("expected some output for no-results case")
	}
}

func TestInfoCmd_JSON(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &InfoCmd{ID: "abcd1234"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var info map[string]any
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if info["project_name"] != "myproject" {
		t.Errorf("project_name = %v, want myproject", info["project_name"])
	}
	mc, ok := info["message_count"].(float64)
	if !ok || mc != 4 {
		t.Errorf("message_count = %v, want 4", info["message_count"])
	}
}

func TestStatsCmd_JSON(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &StatsCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var stats map[string]any
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	total, ok := stats["total_sessions"].(float64)
	if !ok || total != 1 {
		t.Errorf("total_sessions = %v, want 1", stats["total_sessions"])
	}
}

func TestChangelogCmd_JSON(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &ChangelogCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (latest), got %d", len(entries))
	}
	if entries[0]["version"] != "2.1.50" {
		t.Errorf("version = %v, want 2.1.50", entries[0]["version"])
	}
}

func TestChangelogCmd_All(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &ChangelogCmd{All: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestPlansListCmd_JSON(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &PlansListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var plans []map[string]any
	if err := json.Unmarshal([]byte(out), &plans); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0]["name"] != "auth-refactor" {
		t.Errorf("name = %v, want auth-refactor", plans[0]["name"])
	}
	if plans[0]["title"] != "Refactor Auth" {
		t.Errorf("title = %v, want Refactor Auth", plans[0]["title"])
	}
}

func TestPlansSearchCmd_JSON(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: true}
	cmd := &PlansSearchCmd{Query: "authentication"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var matches []map[string]any
	if err := json.Unmarshal([]byte(out), &matches); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestExportCmd(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{}
	cmd := &ExportCmd{ID: "abcd1234", Role: "user,assistant"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	if out == "" {
		t.Fatal("expected markdown output")
	}
	if !bytes.Contains([]byte(out), []byte("# Session")) {
		t.Error("expected markdown header")
	}
	if !bytes.Contains([]byte(out), []byte("## User")) {
		t.Error("expected user section")
	}
	if !bytes.Contains([]byte(out), []byte("## Assistant")) {
		t.Error("expected assistant section")
	}
}

func TestExportCmd_NoTruncationByDefault(t *testing.T) {
	home := setupFixtures(t)

	// Create a session with a long assistant message (>500 chars).
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	longText := strings.Repeat("word ", 200) // 1000 chars
	sessionLines := []string{
		`{"type":"user","message":{"role":"user","content":"tell me something long"},"cwd":"/Users/test/myproject","sessionId":"long1234-5678-9abc-def0-222222222222","timestamp":"2026-03-01T10:00:00Z"}`,
		fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"%s"}]},"timestamp":"2026-03-01T10:00:05Z"}`, longText),
	}
	writeLines(t, filepath.Join(projDir, "long1234-5678-9abc-def0-222222222222.jsonl"), sessionLines)

	globals := &Globals{}
	cmd := &ExportCmd{ID: "long1234", Role: "user,assistant"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, longText) {
		t.Error("expected full message text without truncation by default")
	}
}

func TestExportCmd_Short(t *testing.T) {
	home := setupFixtures(t)

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	longText := strings.Repeat("word ", 200)
	sessionLines := []string{
		`{"type":"user","message":{"role":"user","content":"tell me something long"},"cwd":"/Users/test/myproject","sessionId":"short123-5678-9abc-def0-333333333333","timestamp":"2026-03-01T10:00:00Z"}`,
		fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"%s"}]},"timestamp":"2026-03-01T10:00:05Z"}`, longText),
	}
	writeLines(t, filepath.Join(projDir, "short123-5678-9abc-def0-333333333333.jsonl"), sessionLines)

	globals := &Globals{}
	cmd := &ExportCmd{ID: "short123", Role: "user,assistant", Short: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	if strings.Contains(out, longText) {
		t.Error("expected truncated output with --short flag")
	}
	if !strings.Contains(out, "[+") {
		t.Error("expected truncation count indicator")
	}
}

func TestExportCmd_ToolResultTruncation(t *testing.T) {
	home := setupFixtures(t)

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	longToolOutput := strings.Repeat("line of tool output ", 150) // 3000 chars
	sessionLines := []string{
		fmt.Sprintf(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"%s"},{"type":"text","text":"read something"}]},"cwd":"/Users/test/myproject","sessionId":"tool1234-5678-9abc-def0-444444444444","timestamp":"2026-03-01T10:00:00Z"}`, longToolOutput),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Here is the file content."}]},"timestamp":"2026-03-01T10:00:05Z"}`,
	}
	writeLines(t, filepath.Join(projDir, "tool1234-5678-9abc-def0-444444444444.jsonl"), sessionLines)

	globals := &Globals{}
	cmd := &ExportCmd{ID: "tool1234", Role: "user,assistant", IncludeToolResults: true, MaxToolChars: 100}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	// Conversation text should be untruncated.
	if !strings.Contains(out, "read something") {
		t.Error("conversation text should not be truncated")
	}
	// Tool result block should be truncated with indicator.
	if !strings.Contains(out, "[+") {
		t.Error("expected tool result truncation with count indicator")
	}
	if strings.Contains(out, longToolOutput) {
		t.Error("tool result output should be truncated")
	}
}

func TestExportCmd_FullIncludesToolResults(t *testing.T) {
	home := setupFixtures(t)

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	sessionLines := []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"file contents here"},{"type":"text","text":"check this"}]},"cwd":"/Users/test/myproject","sessionId":"full1234-5678-9abc-def0-555555555555","timestamp":"2026-03-01T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Got it."}]},"timestamp":"2026-03-01T10:00:05Z"}`,
	}
	writeLines(t, filepath.Join(projDir, "full1234-5678-9abc-def0-555555555555.jsonl"), sessionLines)

	globals := &Globals{}
	cmd := &ExportCmd{ID: "full1234", Role: "user,assistant", Full: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, "file contents here") {
		t.Error("--full should include tool result content")
	}
}

func TestExportCmd_HintOnSkippedToolBlocks(t *testing.T) {
	home := setupFixtures(t)

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	sessionLines := []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"big output"},{"type":"text","text":"check this"}]},"cwd":"/Users/test/myproject","sessionId":"hint1234-5678-9abc-def0-666666666666","timestamp":"2026-03-01T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done."},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]},"timestamp":"2026-03-01T10:00:05Z"}`,
	}
	writeLines(t, filepath.Join(projDir, "hint1234-5678-9abc-def0-666666666666.jsonl"), sessionLines)

	// Capture stderr for hints.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	globals := &Globals{}
	cmd := &ExportCmd{ID: "hint1234", Role: "user,assistant"}

	captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	hints := buf.String()

	if !strings.Contains(hints, "tool block(s) skipped") {
		t.Errorf("expected hint about skipped tool blocks, got: %q", hints)
	}
	if !strings.Contains(hints, "--include-tool-results") {
		t.Errorf("expected hint to mention --include-tool-results flag, got: %q", hints)
	}
}

func TestExportCmd_ToFile(t *testing.T) {
	setupFixtures(t)

	outDir := t.TempDir()
	outFile := filepath.Join(outDir, "export.md")

	globals := &Globals{}
	cmd := &ExportCmd{ID: "abcd1234", Output: outFile, Role: "user,assistant"}

	if err := cmd.Run(globals); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty export file")
	}
}

func TestResumeCmd_DryRun(t *testing.T) {
	home := setupFixtures(t)

	projDir := filepath.Join(home, "Users", "test", "myproject")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	globals := &Globals{}
	cmd := &ResumeCmd{ID: "abcd1234", DryRun: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	if out == "" {
		t.Fatal("expected dry-run output")
	}
	if !bytes.Contains([]byte(out), []byte("claude --resume")) {
		t.Errorf("dry-run output missing 'claude --resume': %q", out)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "''"},
		{"simple", "'simple'"},
		{"has space", "'has space'"},
		{"it's", `'it'\''s'`},
		{"it's a 'test'", `'it'\''s a '\''test'\'''`},
		{"$HOME", "'$HOME'"},
		{"`cmd`", "'`cmd`'"},
		{"line\nnewline", "'line\nnewline'"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatSyncResult(t *testing.T) {
	tests := []struct {
		name   string
		result *index.SyncResult
		want   string
	}{
		{
			"up to date with sessions",
			&index.SyncResult{Unchanged: 100},
			"Already up to date (100 sessions)",
		},
		{
			"up to date empty index",
			&index.SyncResult{},
			"Already up to date",
		},
		{
			"only new",
			&index.SyncResult{Added: 3, Unchanged: 97},
			"Synced 3 new (97 unchanged)",
		},
		{
			"only updated",
			&index.SyncResult{Updated: 2, Unchanged: 98},
			"Synced 2 updated (98 unchanged)",
		},
		{
			"new and updated",
			&index.SyncResult{Added: 3, Updated: 2, Unchanged: 95},
			"Synced 3 new, 2 updated (95 unchanged)",
		},
		{
			"all types",
			&index.SyncResult{Added: 1, Updated: 2, Deleted: 3, Unchanged: 94},
			"Synced 1 new, 2 updated, 3 deleted (94 unchanged)",
		},
		{
			"changes with zero unchanged",
			&index.SyncResult{Added: 5},
			"Synced 5 new",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSyncResult(tt.result)
			if got != tt.want {
				t.Errorf("formatSyncResult() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSearchCmd_ProjectNotFound(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: false}
	cmd := &SearchCmd{Query: "database", Project: "nonexistent_xyz"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, `No project matching "nonexistent_xyz"`) {
		t.Errorf("expected 'No project matching' message, got: %q", out)
	}
}

func TestSearchCmd_ProjectExistsNoQueryMatch(t *testing.T) {
	setupFixtures(t)

	globals := &Globals{JSON: false}
	cmd := &SearchCmd{Query: "zzz_impossible_term_zzz", Project: "myproject"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, `No sessions matching`) || !strings.Contains(out, `in project "myproject"`) {
		t.Errorf("expected 'No sessions matching ... in project' message, got: %q", out)
	}
}

func TestExitError(t *testing.T) {
	err := &ExitError{Code: 42}
	if err.Error() != "exit status 42" {
		t.Errorf("Error() = %q, want %q", err.Error(), "exit status 42")
	}
}
