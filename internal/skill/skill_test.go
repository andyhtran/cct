package skill

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/andyhtran/cct/internal/paths"
)

// setupSkillEnv isolates HOME and XDG_CACHE_HOME for one test so skill state
// can't escape into the developer's real ~/.claude or ~/.cache.
func setupSkillEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")
}

func TestSync_FreshExtraction(t *testing.T) {
	setupSkillEnv(t)

	if err := Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	skillPath := filepath.Join(paths.SkillLiveDir(), "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("SKILL.md missing after sync: %v", err)
	}

	refPath := filepath.Join(paths.SkillLiveDir(), "references", "commands.md")
	if _, err := os.Stat(refPath); err != nil {
		t.Fatalf("references/commands.md missing after sync: %v", err)
	}

	marker, err := os.ReadFile(filepath.Join(paths.SkillLiveDir(), versionMarkerFile))
	if err != nil {
		t.Fatalf("marker missing: %v", err)
	}
	if len(marker) == 0 {
		t.Fatal("marker is empty")
	}
}

func TestSync_IdempotentSkipsRewrite(t *testing.T) {
	setupSkillEnv(t)

	if err := Sync(); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	markerPath := filepath.Join(paths.SkillLiveDir(), versionMarkerFile)
	first, err := os.Stat(markerPath)
	if err != nil {
		t.Fatalf("stat marker: %v", err)
	}

	// Sleep long enough that an mtime change would be observable, then
	// re-sync. With matching hashes Sync must not touch the marker.
	time.Sleep(20 * time.Millisecond)
	if err := Sync(); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	second, err := os.Stat(markerPath)
	if err != nil {
		t.Fatalf("stat marker after second sync: %v", err)
	}
	if !first.ModTime().Equal(second.ModTime()) {
		t.Fatalf("marker rewritten when hashes matched (mtime %v -> %v)", first.ModTime(), second.ModTime())
	}
}

func TestSync_RemovesStaleFilesOnReExtract(t *testing.T) {
	setupSkillEnv(t)

	if err := Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	stale := filepath.Join(paths.SkillLiveDir(), "stale.md")
	if err := os.WriteFile(stale, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Force re-extract by corrupting the marker.
	if err := os.WriteFile(filepath.Join(paths.SkillLiveDir(), versionMarkerFile), []byte("wrong"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Sync(); err != nil {
		t.Fatalf("re-Sync: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file survived re-extract: err=%v", err)
	}
}

func TestInstall_CreatesSymlinkAndIsIdempotent(t *testing.T) {
	setupSkillEnv(t)

	if err := Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	target, err := os.Readlink(paths.SkillSymlinkPath())
	if err != nil {
		t.Fatalf("symlink missing: %v", err)
	}
	if target != paths.SkillLiveDir() {
		t.Fatalf("symlink target = %s, want %s", target, paths.SkillLiveDir())
	}

	if err := Install(); err != nil {
		t.Fatalf("second Install (should be idempotent): %v", err)
	}
}

func TestInstall_RefusesForeignDir(t *testing.T) {
	setupSkillEnv(t)

	// Pre-create a regular directory at the destination — simulates a
	// hand-installed skill that we must not silently overwrite.
	if err := os.MkdirAll(paths.SkillSymlinkPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.SkillSymlinkPath(), "SKILL.md"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Install()
	if err == nil {
		t.Fatal("Install should have refused to overwrite foreign directory")
	}
}

func TestInstall_RefusesForeignSymlink(t *testing.T) {
	setupSkillEnv(t)

	// Symlink pointing somewhere we don't control.
	if err := os.MkdirAll(paths.ClaudeSkillsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp/some-other-skill", paths.SkillSymlinkPath()); err != nil {
		t.Fatal(err)
	}

	err := Install()
	if err == nil {
		t.Fatal("Install should have refused foreign symlink")
	}
}

func TestUninstall_OnlyRemovesOurSymlink(t *testing.T) {
	setupSkillEnv(t)
	if err := os.MkdirAll(paths.ClaudeSkillsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	foreignTarget := "/tmp/not-cct"
	if err := os.Symlink(foreignTarget, paths.SkillSymlinkPath()); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	target, err := os.Readlink(paths.SkillSymlinkPath())
	if err != nil {
		t.Fatalf("foreign symlink should still exist: %v", err)
	}
	if target != foreignTarget {
		t.Fatalf("foreign symlink target changed: %s", target)
	}
}

func TestUninstall_NoOpWhenMissing(t *testing.T) {
	setupSkillEnv(t)
	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall on empty state: %v", err)
	}
}

func TestIsOurSymlink_AllCases(t *testing.T) {
	setupSkillEnv(t)

	if ok, err := IsOurSymlink(); err != nil || ok {
		t.Fatalf("missing path: ok=%v err=%v, want false/nil", ok, err)
	}

	if err := os.MkdirAll(paths.ClaudeSkillsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	regular := paths.SkillSymlinkPath()
	if err := os.WriteFile(regular, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsOurSymlink(); err != nil || ok {
		t.Fatalf("regular file: ok=%v err=%v", ok, err)
	}
	_ = os.Remove(regular)

	if err := os.Symlink("/elsewhere", regular); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsOurSymlink(); err != nil || ok {
		t.Fatalf("foreign symlink: ok=%v err=%v", ok, err)
	}
	_ = os.Remove(regular)

	if err := os.Symlink(paths.SkillLiveDir(), regular); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsOurSymlink(); err != nil || !ok {
		t.Fatalf("our symlink: ok=%v err=%v", ok, err)
	}
}

func TestMaybeNudge_SuppressedWhenInstalled(t *testing.T) {
	setupSkillEnv(t)
	if err := Install(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	MaybeNudge(&buf)
	if buf.Len() != 0 {
		t.Fatalf("nudge fired while installed: %q", buf.String())
	}
}

func TestMaybeNudge_SuppressedWhenDisabled(t *testing.T) {
	setupSkillEnv(t)
	if err := SetNudgeEnabled(false); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	MaybeNudge(&buf)
	if buf.Len() != 0 {
		t.Fatalf("nudge fired while disabled: %q", buf.String())
	}
}

func TestMaybeNudge_RateLimited(t *testing.T) {
	setupSkillEnv(t)

	var first bytes.Buffer
	MaybeNudge(&first)
	if first.Len() == 0 {
		t.Fatal("first nudge should fire")
	}

	var second bytes.Buffer
	MaybeNudge(&second)
	if second.Len() != 0 {
		t.Fatalf("second nudge should be rate-limited: %q", second.String())
	}
}

func TestMaybeNudge_FiresAfterInterval(t *testing.T) {
	setupSkillEnv(t)

	// Backdate the last-shown timestamp to >24h ago.
	if err := os.MkdirAll(filepath.Dir(paths.SkillNudgeLastPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour).Unix()
	if err := os.WriteFile(paths.SkillNudgeLastPath(), []byte(strconv.FormatInt(old, 10)), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	MaybeNudge(&buf)
	if buf.Len() == 0 {
		t.Fatal("nudge should fire after interval elapsed")
	}
}

func TestSetNudgeEnabled_Toggle(t *testing.T) {
	setupSkillEnv(t)

	if !NudgeEnabled() {
		t.Fatal("default state should be enabled")
	}
	if err := SetNudgeEnabled(false); err != nil {
		t.Fatal(err)
	}
	if NudgeEnabled() {
		t.Fatal("after off: still enabled")
	}
	// Idempotent off.
	if err := SetNudgeEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := SetNudgeEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !NudgeEnabled() {
		t.Fatal("after on: still disabled")
	}
	// Idempotent on (file already absent).
	if err := SetNudgeEnabled(true); err != nil {
		t.Fatal(err)
	}
}

func TestGetStatus_NotInstalled(t *testing.T) {
	setupSkillEnv(t)
	s, err := GetStatus()
	if err != nil {
		t.Fatal(err)
	}
	if s.Installed {
		t.Fatal("should report not installed")
	}
	if !s.NudgeEnabled {
		t.Fatal("nudge should default to enabled")
	}
	if s.EmbeddedHash == "" {
		t.Fatal("embedded hash should always be computable")
	}
}

func TestGetStatus_Installed(t *testing.T) {
	setupSkillEnv(t)
	if err := Install(); err != nil {
		t.Fatal(err)
	}
	s, err := GetStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Installed {
		t.Fatal("should report installed")
	}
	if !s.OurSymlink {
		t.Fatal("OurSymlink should be true")
	}
	if !s.InSync {
		t.Fatal("should be in sync")
	}
	if s.SymlinkTarget != paths.SkillLiveDir() {
		t.Fatalf("target = %s, want %s", s.SymlinkTarget, paths.SkillLiveDir())
	}
}
