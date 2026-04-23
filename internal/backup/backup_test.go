//go:build darwin || linux

package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeHome struct {
	projects     string
	backupRoot   string
	manifestPath string
}

func setupFakeHome(t *testing.T) fakeHome {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	fh := fakeHome{
		projects:     filepath.Join(home, ".claude", "projects"),
		backupRoot:   filepath.Join(home, ".cache", "cct", "backup", "projects"),
		manifestPath: filepath.Join(home, ".cache", "cct", "backup", "manifest.json"),
	}
	if err := os.MkdirAll(fh.projects, 0o755); err != nil {
		t.Fatal(err)
	}
	return fh
}

// writeQuietJSONL creates a JSONL file and backdates its mtime so the sweep's
// active-file guard doesn't skip it. 20 minutes is well past MinQuietPeriod.
func writeQuietJSONL(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-20 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestSweep_FreshFiles(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath

	proj := filepath.Join(projects, "-Users-test-proj")
	writeQuietJSONL(t, filepath.Join(proj, "aaaa1111-2222-3333-4444-555555555555.jsonl"), `{"hello":"world"}`)
	writeQuietJSONL(t, filepath.Join(proj, "bbbb1111-2222-3333-4444-555555555555.jsonl"), `{"second":"session"}`)

	result, err := SweepAt(projects, backupRoot, manifestPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Linked != 2 {
		t.Fatalf("want 2 linked, got %d (%+v)", result.Linked, result)
	}

	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("want 2 manifest entries, got %d", len(manifest.Entries))
	}
	for id, e := range manifest.Entries {
		if _, err := os.Stat(e.BackupPath); err != nil {
			t.Errorf("%s: backup file missing: %v", id, err)
		}
		if e.CopyMode != CopyModeHardlink {
			t.Errorf("%s: want hardlink, got %s", id, e.CopyMode)
		}
		srcStat, _ := os.Stat(e.SourcePath)
		dstStat, _ := os.Stat(e.BackupPath)
		if inodeOf(srcStat) != inodeOf(dstStat) {
			t.Errorf("%s: inodes differ — not hard linked", id)
		}
	}
}

func TestSweep_Idempotent(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	writeQuietJSONL(t, filepath.Join(proj, "aaaa1111-2222-3333-4444-555555555555.jsonl"), `{"hello":"world"}`)

	if _, err := SweepAt(projects, backupRoot, manifestPath, Options{}); err != nil {
		t.Fatal(err)
	}
	result, err := SweepAt(projects, backupRoot, manifestPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Linked != 0 || result.Copied != 0 || result.Relinked != 0 {
		t.Fatalf("second sweep should do nothing, got %+v", result)
	}
	if result.Unchanged != 1 {
		t.Fatalf("want 1 unchanged, got %d", result.Unchanged)
	}
}

func TestSweep_ActiveFileSkipped(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	path := filepath.Join(proj, "aaaa1111-2222-3333-4444-555555555555.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"live":"write"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := SweepAt(projects, backupRoot, manifestPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SkippedActive != 1 {
		t.Fatalf("want 1 skipped-active, got %+v", result)
	}
	if result.Linked != 0 {
		t.Fatalf("want 0 linked for active file, got %d", result.Linked)
	}

	result, err = SweepAt(projects, backupRoot, manifestPath, Options{IncludeActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Linked != 1 {
		t.Fatalf("want 1 linked with IncludeActive, got %+v", result)
	}
}

func TestSweep_OrphanDetection(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	source := filepath.Join(proj, "aaaa1111-2222-3333-4444-555555555555.jsonl")
	writeQuietJSONL(t, source, `{"doomed":true}`)

	if _, err := SweepAt(projects, backupRoot, manifestPath, Options{}); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}

	result, err := SweepAt(projects, backupRoot, manifestPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Orphaned != 1 {
		t.Fatalf("want 1 orphan, got %+v", result)
	}

	manifest, _ := LoadManifest(manifestPath)
	if len(manifest.Entries) != 1 {
		t.Fatalf("entry should survive deletion, got %d", len(manifest.Entries))
	}
	for _, e := range manifest.Entries {
		if e.SourceDeletedAt.IsZero() {
			t.Error("SourceDeletedAt should be set on first orphan-detecting sweep")
		}
		if _, err := os.Stat(e.BackupPath); err != nil {
			t.Errorf("backup file should survive: %v", err)
		}
	}
}

// A --no-agents sweep should not flag existing agent manifest entries as
// orphaned — they're deliberately out of scope for this sweep, not missing.
func TestSweep_NoAgentsPreservesAgentEntries(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	parentID := "aaaa1111-2222-3333-4444-555555555555"
	mainSource := filepath.Join(proj, parentID+".jsonl")
	agentSource := filepath.Join(proj, parentID, "subagents", "agent-bbbb2222-3333-4444-5555-666666666666.jsonl")
	writeQuietJSONL(t, mainSource, `{"main":true}`)
	writeQuietJSONL(t, agentSource, `{"agent":true}`)

	if _, err := SweepAt(projects, backupRoot, manifestPath, Options{IncludeAgents: true}); err != nil {
		t.Fatal(err)
	}

	result, err := SweepAt(projects, backupRoot, manifestPath, Options{IncludeAgents: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Orphaned != 0 {
		t.Fatalf("agent entries should be out-of-scope, not orphaned; got %+v", result)
	}

	manifest, _ := LoadManifest(manifestPath)
	agentID := "agent-bbbb2222-3333-4444-5555-666666666666"
	entry, ok := manifest.Entries[agentID]
	if !ok {
		t.Fatalf("agent entry missing from manifest")
	}
	if !entry.SourceDeletedAt.IsZero() {
		t.Errorf("agent SourceDeletedAt should stay zero on --no-agents sweep, got %v", entry.SourceDeletedAt)
	}
}

// An entry whose SourceDeletedAt was set (e.g. by a prior false-orphan sweep)
// must clear once the sweep sees the source is alive with the expected
// inode+size — otherwise the stale timestamp sticks around forever.
func TestSweep_UnchangedClearsStaleSourceDeletedAt(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	source := filepath.Join(proj, "aaaa1111-2222-3333-4444-555555555555.jsonl")
	writeQuietJSONL(t, source, `{"alive":true}`)

	if _, err := SweepAt(projects, backupRoot, manifestPath, Options{}); err != nil {
		t.Fatal(err)
	}

	// Seed a stale deletion marker directly in the manifest.
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	id := "aaaa1111-2222-3333-4444-555555555555"
	entry := manifest.Entries[id]
	entry.SourceDeletedAt = time.Now().Add(-time.Hour)
	manifest.Entries[id] = entry
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}

	result, err := SweepAt(projects, backupRoot, manifestPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Unchanged != 1 {
		t.Fatalf("want 1 unchanged, got %+v", result)
	}

	manifest, _ = LoadManifest(manifestPath)
	if got := manifest.Entries[id].SourceDeletedAt; !got.IsZero() {
		t.Errorf("stale SourceDeletedAt should clear on unchanged sweep, got %v", got)
	}
}

func TestSweep_InodeChange(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	source := filepath.Join(proj, "aaaa1111-2222-3333-4444-555555555555.jsonl")
	writeQuietJSONL(t, source, `{"first":"inode"}`)
	if _, err := SweepAt(projects, backupRoot, manifestPath, Options{}); err != nil {
		t.Fatal(err)
	}

	// Force a new inode by rm+recreate (simulates write-temp-then-rename).
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	writeQuietJSONL(t, source, `{"new":"inode"}`)

	result, err := SweepAt(projects, backupRoot, manifestPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Relinked != 1 {
		t.Fatalf("want 1 relinked on inode change, got %+v", result)
	}

	manifest, _ := LoadManifest(manifestPath)
	for _, e := range manifest.Entries {
		srcStat, _ := os.Stat(e.SourcePath)
		dstStat, _ := os.Stat(e.BackupPath)
		if inodeOf(srcStat) != inodeOf(dstStat) {
			t.Error("relink should re-establish shared inode")
		}
	}
}

func TestRestore_DryRun(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	id := "aaaa1111-2222-3333-4444-555555555555"
	source := filepath.Join(proj, id+".jsonl")
	writeQuietJSONL(t, source, `{"backmeup":true}`)
	if _, err := SweepAt(projects, backupRoot, manifestPath, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}

	result, err := RestoreAt(manifestPath, RestoreOptions{SessionIDs: []string{id}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 1 {
		t.Fatalf("want 1 reported in dry-run, got %+v", result)
	}
	if _, err := os.Stat(source); err == nil {
		t.Error("dry-run should not have created source file")
	}
}

func TestRestore_Writes(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	id := "aaaa1111-2222-3333-4444-555555555555"
	source := filepath.Join(proj, id+".jsonl")
	writeQuietJSONL(t, source, `{"content":"restored"}`)
	if _, err := SweepAt(projects, backupRoot, manifestPath, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}

	result, err := RestoreAt(manifestPath, RestoreOptions{SessionIDs: []string{id}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 1 {
		t.Fatalf("want 1 restored, got %+v", result)
	}
	if _, err := os.Stat(source); err != nil {
		t.Errorf("source should exist after restore: %v", err)
	}

	manifest, _ := LoadManifest(manifestPath)
	for _, e := range manifest.Entries {
		if !e.SourceDeletedAt.IsZero() {
			t.Error("SourceDeletedAt should clear after restore")
		}
	}
}

func TestManifest_CorruptedFile(t *testing.T) {
	manifestPath := setupFakeHome(t).manifestPath
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(manifestPath)
	if err == nil {
		t.Fatal("want parse error, got nil")
	}
	if m == nil {
		t.Fatal("manifest should be non-nil even on parse error")
	}
	if len(m.Entries) != 0 {
		t.Errorf("corrupted manifest should yield empty entries, got %d", len(m.Entries))
	}
}

func TestManifest_MissingFile(t *testing.T) {
	manifestPath := setupFakeHome(t).manifestPath
	m, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(m.Entries) != 0 {
		t.Error("want empty entries for missing manifest")
	}
}

func TestDiscoverBackupSources(t *testing.T) {
	fh := setupFakeHome(t)
	projects, backupRoot, manifestPath := fh.projects, fh.backupRoot, fh.manifestPath
	proj := filepath.Join(projects, "-Users-test-proj")
	writeQuietJSONL(t, filepath.Join(proj, "aaaa1111-2222-3333-4444-555555555555.jsonl"), `{"x":1}`)
	writeQuietJSONL(t, filepath.Join(proj, "bbbb1111-2222-3333-4444-555555555555.jsonl"), `{"x":2}`)
	if _, err := SweepAt(projects, backupRoot, manifestPath, Options{}); err != nil {
		t.Fatal(err)
	}

	sources := DiscoverBackupSources()
	if len(sources) != 2 {
		t.Fatalf("want 2 backup sources, got %d", len(sources))
	}
}
