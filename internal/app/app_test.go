//go:build darwin || linux

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// setupFixtures creates a fake ~/.claude tree with session, plan, and changelog
// data. It returns the temp HOME directory. The caller must use t.Setenv("HOME", ...)
// before calling any functions that read from ClaudeDir().
func setupFixtures(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	cmd := &ExportCmd{ID: "abcd1234"}

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

func TestExportCmd_ToFile(t *testing.T) {
	setupFixtures(t)

	outDir := t.TempDir()
	outFile := filepath.Join(outDir, "export.md")

	globals := &Globals{}
	cmd := &ExportCmd{ID: "abcd1234", Output: outFile}

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

func TestExitError(t *testing.T) {
	err := &ExitError{Code: 42}
	if err.Error() != "exit status 42" {
		t.Errorf("Error() = %q, want %q", err.Error(), "exit status 42")
	}
}
