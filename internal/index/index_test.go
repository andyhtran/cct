//go:build darwin || linux

package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestIndex(t *testing.T) *Index {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionLines := []string{
		`{"type":"user","message":{"role":"user","content":"fix the pre-commit hook and don't forget fmt.Println"},"cwd":"/Users/test/myproject","gitBranch":"main","sessionId":"aaaa1111-2222-3333-4444-555555555555","timestamp":"2026-02-01T08:00:00Z"}`,
		`{"type":"custom-title","customTitle":"fix-precommit","sessionId":"aaaa1111-2222-3333-4444-555555555555"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix the pre-commit hook. It doesn't need fmt.Println here."}]},"timestamp":"2026-02-01T08:00:05Z"}`,
		`{"type":"user","message":{"role":"user","content":"now add tests for the parser"},"timestamp":"2026-02-01T08:01:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done, tests added for the parser."}]},"timestamp":"2026-02-01T08:01:05Z"}`,
	}

	sessionPath := filepath.Join(projDir, "aaaa1111-2222-3333-4444-555555555555.jsonl")
	f, err := os.Create(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range sessionLines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()

	idx, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.ForceSync(true); err != nil {
		t.Fatal(err)
	}

	return idx
}

func TestComputeChanges(t *testing.T) {
	t.Run("all unchanged", func(t *testing.T) {
		current := map[string]fileInfo{
			"/a.jsonl": {modified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), size: 100},
		}
		indexed := map[string]indexedFile{
			"/a.jsonl": {sessionID: "aaa", modifiedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), fileSize: 100},
		}
		toAdd, toUpdate, toDelete := computeChanges(current, indexed)
		if len(toAdd) != 0 || len(toUpdate) != 0 || len(toDelete) != 0 {
			t.Errorf("expected no changes, got add=%d update=%d delete=%d", len(toAdd), len(toUpdate), len(toDelete))
		}
	})

	t.Run("new file", func(t *testing.T) {
		current := map[string]fileInfo{
			"/a.jsonl": {modified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), size: 100},
			"/b.jsonl": {modified: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), size: 200},
		}
		indexed := map[string]indexedFile{
			"/a.jsonl": {sessionID: "aaa", modifiedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), fileSize: 100},
		}
		toAdd, toUpdate, toDelete := computeChanges(current, indexed)
		if len(toAdd) != 1 || toAdd[0] != "/b.jsonl" {
			t.Errorf("expected 1 add (/b.jsonl), got %v", toAdd)
		}
		if len(toUpdate) != 0 || len(toDelete) != 0 {
			t.Errorf("expected no updates/deletes, got update=%d delete=%d", len(toUpdate), len(toDelete))
		}
	})

	t.Run("modified time triggers update", func(t *testing.T) {
		current := map[string]fileInfo{
			"/a.jsonl": {modified: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), size: 100},
		}
		indexed := map[string]indexedFile{
			"/a.jsonl": {sessionID: "aaa", modifiedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), fileSize: 100},
		}
		_, toUpdate, _ := computeChanges(current, indexed)
		if len(toUpdate) != 1 {
			t.Errorf("expected 1 update for modified time, got %d", len(toUpdate))
		}
	})

	t.Run("size change triggers update", func(t *testing.T) {
		ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		current := map[string]fileInfo{
			"/a.jsonl": {modified: ts, size: 200},
		}
		indexed := map[string]indexedFile{
			"/a.jsonl": {sessionID: "aaa", modifiedAt: ts, fileSize: 100},
		}
		_, toUpdate, _ := computeChanges(current, indexed)
		if len(toUpdate) != 1 {
			t.Errorf("expected 1 update for size change, got %d", len(toUpdate))
		}
	})

	t.Run("deleted file", func(t *testing.T) {
		current := map[string]fileInfo{}
		indexed := map[string]indexedFile{
			"/a.jsonl": {sessionID: "aaa", modifiedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), fileSize: 100},
		}
		_, _, toDelete := computeChanges(current, indexed)
		if len(toDelete) != 1 {
			t.Errorf("expected 1 delete, got %d", len(toDelete))
		}
	})

	t.Run("same-second timestamps are unchanged", func(t *testing.T) {
		current := map[string]fileInfo{
			"/a.jsonl": {modified: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), size: 100},
		}
		indexed := map[string]indexedFile{
			"/a.jsonl": {sessionID: "aaa", modifiedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), fileSize: 100},
		}
		toAdd, toUpdate, toDelete := computeChanges(current, indexed)
		if len(toAdd) != 0 || len(toUpdate) != 0 || len(toDelete) != 0 {
			t.Error("same-second timestamps should not trigger an update")
		}
	})
}

func TestSyncResult_UpToDate(t *testing.T) {
	tests := []struct {
		name   string
		result SyncResult
		want   bool
	}{
		{"all zero", SyncResult{}, true},
		{"unchanged only", SyncResult{Unchanged: 100}, true},
		{"has added", SyncResult{Added: 1, Unchanged: 99}, false},
		{"has updated", SyncResult{Updated: 1, Unchanged: 99}, false},
		{"has deleted", SyncResult{Deleted: 1, Unchanged: 99}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.UpToDate(); got != tt.want {
				t.Errorf("UpToDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncIncremental(t *testing.T) {
	idx := setupTestIndex(t)

	result, err := idx.SyncWithProgress(true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpToDate() {
		t.Errorf("second sync should be up to date, got add=%d update=%d delete=%d",
			result.Added, result.Updated, result.Deleted)
	}
	if result.Unchanged != 1 {
		t.Errorf("expected 1 unchanged session, got %d", result.Unchanged)
	}

	home := os.Getenv("HOME")
	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	sessionPath := filepath.Join(projDir, "aaaa1111-2222-3333-4444-555555555555.jsonl")
	f, err := os.OpenFile(sessionPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintln(f, `{"type":"user","message":{"role":"user","content":"one more message"},"timestamp":"2026-02-01T08:02:00Z"}`)
	_ = f.Close()

	result, err = idx.SyncWithProgress(true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 {
		t.Errorf("expected 1 updated after file change, got %d", result.Updated)
	}
	if result.Added != 0 {
		t.Errorf("expected 0 added, got %d", result.Added)
	}
}

func TestProjectExists(t *testing.T) {
	idx := setupTestIndex(t)

	if !idx.ProjectExists("myproject") {
		t.Error("expected ProjectExists to return true for indexed project")
	}
	if idx.ProjectExists("nonexistent_project") {
		t.Error("expected ProjectExists to return false for non-existent project")
	}
}

func TestSanitizeFTSTerm(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"Hello", "hello"},
		{"pre-commit", "pre commit"},
		{"snake_case", "snake case"},
		{"fmt.Println", "fmt println"},
		{"don't", "don t"},
		{"!!!", ""},
		{"", ""},
		{"abc123", "abc123"},
		{"a--b__c..d", "a b c d"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFTSTerm(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFTSTerm(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildFTSQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello*"},
		{"hello world", "hello world*"},
		{"pre-commit", "pre commit*"},
		{"don't", "don t*"},
		{"fmt.Println", "fmt println*"},
		{"!!!", ""},
		{"", ""},
		{"  hello  ", "hello*"},
		{"a b", "a b*"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := buildFTSQuery(tt.input)
			if got != tt.want {
				t.Errorf("buildFTSQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFtsTokens(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello", []string{"hello"}},
		{"hello world", []string{"hello", "world"}},
		{"pre-commit", []string{"pre", "commit"}},
		{"fmt.Println", []string{"fmt", "println"}},
		{"!!!", nil},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ftsTokens(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("ftsTokens(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ftsTokens(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildOrQuery(t *testing.T) {
	tests := []struct {
		tokens []string
		want   string
	}{
		{[]string{"fix"}, "fix*"},
		{[]string{"fix", "bug"}, "fix OR bug*"},
		{[]string{"fmt", "println"}, "fmt OR println*"},
		{nil, ""},
	}

	for _, tt := range tests {
		got := buildOrQuery(tt.tokens)
		if got != tt.want {
			t.Errorf("buildOrQuery(%v) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestSearch_BasicMatch(t *testing.T) {
	idx := setupTestIndex(t)

	results, total, err := idx.Search(SearchOptions{
		Query:         "parser",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'parser'")
	}
	if total == 0 {
		t.Fatal("expected total > 0")
	}
}

func TestSearch_NoResults(t *testing.T) {
	idx := setupTestIndex(t)

	results, total, err := idx.Search(SearchOptions{
		Query:         "zzz_nonexistent_zzz",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
	if total != 0 {
		t.Fatalf("expected total 0, got %d", total)
	}
}

func TestSearch_Apostrophe(t *testing.T) {
	idx := setupTestIndex(t)

	results, _, err := idx.Search(SearchOptions{
		Query:         "doesn't",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatalf("apostrophe query must not crash: %v", err)
	}
	_ = results
}

func TestSearch_HyphenatedTerms(t *testing.T) {
	idx := setupTestIndex(t)

	results, _, err := idx.Search(SearchOptions{
		Query:         "pre-commit",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'pre-commit'")
	}
}

func TestSearch_DottedTerms(t *testing.T) {
	idx := setupTestIndex(t)

	results, _, err := idx.Search(SearchOptions{
		Query:         "fmt.Println",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'fmt.Println'")
	}
}

func TestSearch_ProjectFilter(t *testing.T) {
	idx := setupTestIndex(t)

	results, _, err := idx.Search(SearchOptions{
		Query:         "parser",
		ProjectFilter: "myproject",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results with project filter")
	}
}

func TestSearch_ProjectFilter_NoMatch(t *testing.T) {
	idx := setupTestIndex(t)

	results, total, err := idx.Search(SearchOptions{
		Query:         "parser",
		ProjectFilter: "nonexistent",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for non-matching project filter, got %d", len(results))
	}
	if total != 0 {
		t.Fatalf("expected total 0, got %d", total)
	}
}

func TestSearch_CompoundTermFiltering(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-compound")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Session with literal "pre-commit"
	writeTestSession(t, projDir, "match111-2222-3333-4444-555555555555", []string{
		`{"type":"user","message":{"role":"user","content":"fix the pre-commit hook"},"cwd":"/Users/test/compound","sessionId":"match111-2222-3333-4444-555555555555","timestamp":"2026-02-01T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix the pre-commit hook."}]},"timestamp":"2026-02-01T08:00:05Z"}`,
	})

	// Session with "pre" and "commit" separately (should NOT match "pre-commit")
	writeTestSession(t, projDir, "noise222-2222-3333-4444-555555555555", []string{
		`{"type":"user","message":{"role":"user","content":"please commit the pre existing changes"},"cwd":"/Users/test/compound","sessionId":"noise222-2222-3333-4444-555555555555","timestamp":"2026-02-01T09:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done, I will commit those pre existing changes now."}]},"timestamp":"2026-02-01T09:00:05Z"}`,
	})

	idx, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.ForceSync(true); err != nil {
		t.Fatal(err)
	}

	results, _, err := idx.Search(SearchOptions{
		Query:         "pre-commit",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'pre-commit', got %d", len(results))
	}
	if results[0].ID != "match111-2222-3333-4444-555555555555" {
		t.Errorf("expected matching session, got %s", results[0].ID)
	}
}

func writeTestSession(t *testing.T, projDir, id string, lines []string) {
	t.Helper()
	path := filepath.Join(projDir, id+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()
}

func TestIndex_NestedSubagent_MetaSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-nested")
	parentID := "parent11-2222-3333-4444-555555555555"
	parentDir := filepath.Join(projDir, parentID)
	subagentDir := filepath.Join(parentDir, "subagents")
	if err := os.MkdirAll(subagentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeTestSession(t, projDir, parentID, []string{
		`{"type":"user","message":{"role":"user","content":"parent prompt"},"cwd":"/Users/test/nested","sessionId":"` + parentID + `","timestamp":"2026-02-01T08:00:00Z"}`,
	})

	agentID := "agent-deadbeef01234567"
	writeTestSession(t, subagentDir, agentID, []string{
		`{"type":"user","message":{"role":"user","content":"nested subagent task"},"cwd":"/Users/test/nested","isSidechain":true,"timestamp":"2026-02-01T08:00:10Z"}`,
	})
	sidecarPath := filepath.Join(subagentDir, agentID+".meta.json")
	sidecar := `{"agentType":"Explore","description":"Survey the codebase"}`
	if err := os.WriteFile(sidecarPath, []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.ForceSync(true); err != nil {
		t.Fatal(err)
	}

	var agentType, agentDescription sql.NullString
	var isAgent int
	err = idx.db.QueryRow(
		"SELECT is_agent, agent_type, agent_description FROM sessions WHERE id = ?",
		agentID,
	).Scan(&isAgent, &agentType, &agentDescription)
	if err != nil {
		t.Fatalf("nested subagent row missing: %v", err)
	}
	if isAgent != 1 {
		t.Errorf("is_agent = %d, want 1", isAgent)
	}
	if !agentType.Valid || agentType.String != "Explore" {
		t.Errorf("agent_type = %v, want Explore", agentType)
	}
	if !agentDescription.Valid || agentDescription.String != "Survey the codebase" {
		t.Errorf("agent_description = %v, want %q", agentDescription, "Survey the codebase")
	}
}

func TestSearch_CrossMessageMultiTerm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-cross")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Session where "fix" and "bug" are in different messages
	writeTestSession(t, projDir, "cross111-2222-3333-4444-555555555555", []string{
		`{"type":"user","message":{"role":"user","content":"please fix the login page"},"cwd":"/Users/test/cross","sessionId":"cross111-2222-3333-4444-555555555555","timestamp":"2026-02-01T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Found a bug in the auth handler, fixing now."}]},"timestamp":"2026-02-01T08:00:05Z"}`,
	})

	// Session where neither term appears
	writeTestSession(t, projDir, "unrel222-2222-3333-4444-555555555555", []string{
		`{"type":"user","message":{"role":"user","content":"add documentation for the API"},"cwd":"/Users/test/cross","sessionId":"unrel222-2222-3333-4444-555555555555","timestamp":"2026-02-01T09:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Documentation added."}]},"timestamp":"2026-02-01T09:00:05Z"}`,
	})

	idx, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.ForceSync(true); err != nil {
		t.Fatal(err)
	}

	results, _, err := idx.Search(SearchOptions{
		Query:         "fix bug",
		IncludeAgents: true,
		MaxResults:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for cross-message 'fix bug', got %d", len(results))
	}
	if results[0].ID != "cross111-2222-3333-4444-555555555555" {
		t.Errorf("expected cross-message session, got %s", results[0].ID)
	}
}

func TestSearch_TotalCount(t *testing.T) {
	idx := setupTestIndex(t)

	results, total, err := idx.Search(SearchOptions{
		Query:         "fix",
		IncludeAgents: true,
		MaxResults:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) > 1 {
		t.Fatalf("expected at most 1 result, got %d", len(results))
	}
	if total < len(results) {
		t.Fatalf("total (%d) must be >= len(results) (%d)", total, len(results))
	}
}

func TestOpen_CorruptionRecovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-p")
	cacheDir := filepath.Join(home, ".cache", "cct")
	for _, d := range []string{projDir, cacheDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dbPath := filepath.Join(cacheDir, "index.db")
	if err := os.WriteFile(dbPath, []byte("corrupt garbage data"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Open()
	if err != nil {
		t.Fatalf("Open() should recover from corruption, got: %v", err)
	}
	defer func() { _ = idx.Close() }()

	var version int
	if err := idx.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("expected schema version %d after recovery, got %d", schemaVersion, version)
	}
}

func TestEnsureSchema_FreshDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-p")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	idx, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = idx.Close() }()

	var version int
	if err := idx.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("expected schema version %d, got %d", schemaVersion, version)
	}

	var count int
	if err := idx.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sessions'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Error("sessions table not created")
	}
}

func TestCustomTitle_RoundTrip(t *testing.T) {
	idx := setupTestIndex(t)

	var title sql.NullString
	err := idx.db.QueryRow(
		"SELECT custom_title FROM sessions WHERE id = ?",
		"aaaa1111-2222-3333-4444-555555555555",
	).Scan(&title)
	if err != nil {
		t.Fatal(err)
	}
	if !title.Valid || title.String != "fix-precommit" {
		t.Errorf("custom_title = %v, want fix-precommit", title)
	}

	results, _, err := idx.Search(SearchOptions{Query: "parser", IncludeAgents: true, MaxResults: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected search result")
	}
	if results[0].CustomTitle != "fix-precommit" {
		t.Errorf("search result CustomTitle = %q, want fix-precommit", results[0].CustomTitle)
	}
}

// The index is a derived cache: any stored user_version that doesn't match
// the current schemaVersion should result in a clean rebuild — all tables
// dropped, fresh schema applied, next Sync() backfills from JSONL. This
// test seeds an old-shape DB with stale rows and asserts the rebuild wipes
// them rather than trying to migrate them in place.
func TestEnsureSchema_RebuildsFromOldVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	cacheDir := filepath.Join(home, ".cache", "cct")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed a v6-shaped database that lacks custom_title and contains a
	// stale session row + sync marker that the rebuild must discard.
	dbPath := filepath.Join(cacheDir, "index.db")
	seed, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	oldSchema := `
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL UNIQUE,
			project_dir TEXT NOT NULL,
			project_name TEXT NOT NULL,
			project_path TEXT NOT NULL,
			is_agent INTEGER NOT NULL DEFAULT 0,
			modified_at TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			first_prompt TEXT,
			created_at TEXT,
			git_branch TEXT,
			message_count INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE content_map (
			rowid INTEGER PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			source TEXT,
			byte_offset INTEGER NOT NULL,
			byte_length INTEGER NOT NULL
		);
		CREATE VIRTUAL TABLE content_fts USING fts5(text, content='', contentless_delete=1, tokenize='porter unicode61');
		CREATE TABLE index_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO sessions(id, file_path, project_dir, project_name, project_path, is_agent, modified_at, file_size, message_count)
			VALUES ('stale-id', '/stale/path.jsonl', '/stale', 'stale', '/stale', 0, '2026-01-01T00:00:00Z', 0, 0);
		INSERT INTO index_meta(key, value) VALUES ('last_sync_time', '2026-01-01T00:00:00Z');
		PRAGMA user_version = 6;
	`
	if _, err := seed.Exec(oldSchema); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	idx, err := Open()
	if err != nil {
		t.Fatalf("Open on old DB: %v", err)
	}
	defer func() { _ = idx.Close() }()

	var version int
	if err := idx.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("post-rebuild user_version = %d, want %d", version, schemaVersion)
	}

	// New column is present on the rebuilt table.
	if _, err := idx.db.Exec("SELECT custom_title FROM sessions LIMIT 1"); err != nil {
		t.Errorf("custom_title column missing after rebuild: %v", err)
	}

	// The stale row and stale sync marker must both be gone — the rebuild
	// starts from a blank slate, and the next Sync() will backfill.
	var sessionCount int
	if err := idx.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 0 {
		t.Errorf("sessions table still has %d rows after rebuild, want 0", sessionCount)
	}
	var metaCount int
	if err := idx.db.QueryRow("SELECT COUNT(*) FROM index_meta").Scan(&metaCount); err != nil {
		t.Fatal(err)
	}
	if metaCount != 0 {
		t.Errorf("index_meta still has %d rows after rebuild, want 0", metaCount)
	}
}

// Opening a DB that's already at the current schemaVersion must be a no-op:
// no tables dropped, no rows lost. This pins the "same version = leave it
// alone" side of the rebuild model, which is what protects users from
// paying the rescan cost on every process start.
func TestEnsureSchema_NoRebuildAtCurrentVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	cacheDir := filepath.Join(home, ".cache", "cct")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// First Open creates the schema at the current version.
	idx, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.db.Exec(
		"INSERT INTO index_meta(key, value) VALUES ('last_sync_time', '2026-01-01T00:00:00Z')",
	); err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen: since user_version already matches, ensureSchema should
	// skip the rebuild and preserve the sync marker.
	idx2, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = idx2.Close() }()

	var lastSync string
	if err := idx2.db.QueryRow(
		"SELECT value FROM index_meta WHERE key = 'last_sync_time'",
	).Scan(&lastSync); err != nil {
		t.Fatalf("last_sync_time lost on reopen: %v", err)
	}
	if lastSync != "2026-01-01T00:00:00Z" {
		t.Errorf("last_sync_time = %q, want preserved value", lastSync)
	}
}

func TestRebuildWithProgress(t *testing.T) {
	idx := setupTestIndex(t)

	var buf strings.Builder
	if _, err := idx.RebuildWithProgress(true, &buf); err != nil {
		t.Fatal(err)
	}

	status, err := idx.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.TotalSessions == 0 {
		t.Error("expected sessions after rebuild")
	}
}
