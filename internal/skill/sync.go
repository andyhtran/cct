package skill

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/andyhtran/cct/internal/paths"
	"github.com/andyhtran/cct/skills"
)

// versionMarkerFile is written into the live dir alongside the extracted
// content. Contents are the hex-encoded sha256 of the embedded tree at time
// of extraction. Used by Sync to skip work when nothing changed.
const versionMarkerFile = ".cct-skill-version"

// Sync extracts the embedded skill content to paths.SkillLiveDir() if the
// version marker doesn't match the current binary's embedded content. Safe to
// call on every cct invocation — costs one stat + small read in the steady
// state.
func Sync() error {
	liveDir := paths.SkillLiveDir()
	wantHash, err := embeddedHash()
	if err != nil {
		return err
	}

	markerPath := filepath.Join(liveDir, versionMarkerFile)
	if cur, err := os.ReadFile(markerPath); err == nil && string(cur) == wantHash {
		return nil
	}

	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		return err
	}

	// Wipe existing managed contents before re-extracting. Live dir is
	// cct-owned (under our cache dir), so blowing it away is safe.
	entries, _ := os.ReadDir(liveDir)
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(liveDir, e.Name())); err != nil {
			return err
		}
	}

	err = fs.WalkDir(skills.FS, skills.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(skills.Root, p)
		if err != nil {
			return err
		}
		target := filepath.Join(liveDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := skills.FS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		return err
	}

	return os.WriteFile(markerPath, []byte(wantHash), 0o644)
}

// SyncQuiet runs Sync and swallows any error. Used in the root command's
// pre-run hook so a transient sync failure (disk full, race) never blocks the
// user's actual command.
func SyncQuiet() {
	_ = Sync()
}
