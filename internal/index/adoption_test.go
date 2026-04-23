//go:build darwin || linux

package index

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andyhtran/cct/internal/backup"
	"github.com/andyhtran/cct/internal/paths"
)

// TestFallbackAdoption is the integration test for the whole backup feature:
// a session is indexed, backed up, deleted from the live tree, and the next
// sync must keep the session searchable via the backup path.
func TestFallbackAdoption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "aaaa1111-2222-3333-4444-555555555555"
	sourcePath := filepath.Join(projDir, sessionID+".jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"find me later"},"cwd":"/Users/test/myproject","gitBranch":"main","sessionId":"aaaa1111-2222-3333-4444-555555555555","timestamp":"2026-02-01T08:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"acknowledged"}]},"timestamp":"2026-02-01T08:00:05Z"}`,
	}
	f, err := os.Create(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()
	old := time.Now().Add(-20 * time.Minute)
	if err := os.Chtimes(sourcePath, old, old); err != nil {
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

	// Confirm the session is indexed with the live path.
	var indexedPath string
	if err := idx.db.QueryRow("SELECT file_path FROM sessions WHERE id = ?", sessionID).Scan(&indexedPath); err != nil {
		t.Fatalf("pre-backup row missing: %v", err)
	}
	if indexedPath != sourcePath {
		t.Errorf("want live path, got %s", indexedPath)
	}

	// Back up, then wipe the source (simulates the upstream cleanup bug).
	if _, err := backup.SweepAt(
		paths.ProjectsDir(),
		paths.BackupProjectsDir(),
		paths.BackupManifestPath(),
		backup.Options{},
	); err != nil {
		t.Fatalf("backup sweep: %v", err)
	}
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}

	if err := idx.ForceSync(true); err != nil {
		t.Fatalf("sync after deletion: %v", err)
	}

	// The row must still exist — adoption kept it alive — but now its
	// file_path points at the backup copy.
	if err := idx.db.QueryRow("SELECT file_path FROM sessions WHERE id = ?", sessionID).Scan(&indexedPath); err != nil {
		t.Fatalf("session row should survive after adoption: %v", err)
	}
	if !filepath.IsAbs(indexedPath) {
		t.Errorf("adopted path should be absolute, got %s", indexedPath)
	}
	if _, err := os.Stat(indexedPath); err != nil {
		t.Errorf("adopted file_path should exist on disk: %v", err)
	}
	if indexedPath == sourcePath {
		t.Error("adopted path should be the backup, not the deleted source")
	}
}

// TestAdoptionCounted verifies that a session rescued from backup path is
// reported as adopted (not deleted+added), so users see accurate counts.
func TestAdoptionCounted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "aaaa1111-2222-3333-4444-555555555555"
	sourcePath := filepath.Join(projDir, sessionID+".jsonl")
	contents := `{"type":"user","message":{"role":"user","content":"hi"},"sessionId":"aaaa1111-2222-3333-4444-555555555555","timestamp":"2026-02-01T08:00:00Z"}` + "\n"
	if err := os.WriteFile(sourcePath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-20 * time.Minute)
	_ = os.Chtimes(sourcePath, old, old)

	idx, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	// Initial sync: indexes the live file.
	if _, err := idx.SyncWithProgress(true, true, nil); err != nil {
		t.Fatal(err)
	}

	// Back up then delete live — simulates upstream cleanup.
	if _, err := backup.SweepAt(
		paths.ProjectsDir(),
		paths.BackupProjectsDir(),
		paths.BackupManifestPath(),
		backup.Options{},
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}

	// Next sync should report adoption, not delete+add.
	result, err := idx.SyncWithProgress(true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Adopted != 1 {
		t.Errorf("Adopted = %d, want 1 (%+v)", result.Adopted, result)
	}
	if result.Added != 0 {
		t.Errorf("Added = %d, want 0 (adoption should not count as new) (%+v)", result.Added, result)
	}
	if result.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0 (adoption should not count as delete) (%+v)", result.Deleted, result)
	}
}

// TestLiveReadoption covers the reverse: after adoption, if the source reappears
// (e.g., via `cct backup restore`), the next sync should swap file_path back
// to the live location.
func TestLiveReadoption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	projDir := filepath.Join(home, ".claude", "projects", "-Users-test-myproject")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "aaaa1111-2222-3333-4444-555555555555"
	sourcePath := filepath.Join(projDir, sessionID+".jsonl")
	contents := `{"type":"user","message":{"role":"user","content":"hi"},"sessionId":"aaaa1111-2222-3333-4444-555555555555","timestamp":"2026-02-01T08:00:00Z"}` + "\n"
	if err := os.WriteFile(sourcePath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-20 * time.Minute)
	_ = os.Chtimes(sourcePath, old, old)

	idx, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.ForceSync(true); err != nil {
		t.Fatal(err)
	}
	if _, err := backup.SweepAt(
		paths.ProjectsDir(),
		paths.BackupProjectsDir(),
		paths.BackupManifestPath(),
		backup.Options{},
	); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(sourcePath)
	if err := idx.ForceSync(true); err != nil {
		t.Fatal(err)
	}

	// Verify we're in the adopted state.
	var adoptedPath string
	_ = idx.db.QueryRow("SELECT file_path FROM sessions WHERE id = ?", sessionID).Scan(&adoptedPath)
	if adoptedPath == sourcePath {
		t.Fatal("precondition: expected adoption to have happened")
	}

	// Restore the live source and sync.
	if _, err := backup.RestoreAt(paths.BackupManifestPath(), backup.RestoreOptions{SessionIDs: []string{sessionID}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.ForceSync(true); err != nil {
		t.Fatal(err)
	}

	var finalPath string
	_ = idx.db.QueryRow("SELECT file_path FROM sessions WHERE id = ?", sessionID).Scan(&finalPath)
	if finalPath != sourcePath {
		t.Errorf("after restore, file_path should be live (%s), got %s", sourcePath, finalPath)
	}
}
