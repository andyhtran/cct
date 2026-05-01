package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andyhtran/cct/internal/paths"
)

// Install ensures the live copy is up to date and creates the
// ~/.claude/skills/cct symlink pointing at it. Idempotent when the symlink
// already points at our live dir. Refuses to overwrite a foreign item at the
// destination.
func Install() error {
	if err := Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	symlinkPath := paths.SkillSymlinkPath()
	liveDir := paths.SkillLiveDir()

	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(symlinkPath), err)
	}

	ours, err := IsOurSymlink()
	if err != nil {
		return err
	}
	if ours {
		return nil
	}

	if _, err := os.Lstat(symlinkPath); err == nil {
		return fmt.Errorf("%s already exists and is not managed by cct; remove it manually before retrying", symlinkPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return os.Symlink(liveDir, symlinkPath)
}

// Uninstall removes the symlink only if it's ours. Leaves the live copy in
// place so reinstall is a one-syscall operation.
func Uninstall() error {
	ours, err := IsOurSymlink()
	if err != nil {
		return err
	}
	if !ours {
		return nil
	}
	return os.Remove(paths.SkillSymlinkPath())
}

// IsOurSymlink reports whether ~/.claude/skills/cct is a symlink pointing
// exactly at SkillLiveDir().
func IsOurSymlink() (bool, error) {
	symlinkPath := paths.SkillSymlinkPath()
	info, err := os.Lstat(symlinkPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		return false, err
	}
	return target == paths.SkillLiveDir(), nil
}
