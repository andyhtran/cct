// Package paths provides base directory paths for Claude Code configuration.
package paths

import (
	"os"
	"path/filepath"
)

func ClaudeDir() string {
	return filepath.Join(os.Getenv("HOME"), ".claude")
}

func ProjectsDir() string {
	return filepath.Join(ClaudeDir(), "projects")
}

func CacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "cct")
	}
	return filepath.Join(os.Getenv("HOME"), ".cache", "cct")
}

func IndexPath() string {
	return filepath.Join(CacheDir(), "index.db")
}

func ChangelogCachePath() string {
	return filepath.Join(CacheDir(), "changelog.md")
}

func ChangelogMetaPath() string {
	return filepath.Join(CacheDir(), "changelog.meta.json")
}

// BackupDir holds hard-linked copies of JSONL session files. Mirrors the layout
// of ProjectsDir() so restore is a reverse-link to the original path. Placed
// under CacheDir() so hardlinks work against ProjectsDir() on the default
// same-filesystem case.
func BackupDir() string {
	return filepath.Join(CacheDir(), "backup")
}

// BackupProjectsDir is the root under which <projectDir>/<sessionID>.jsonl
// mirrors ~/.claude/projects.
func BackupProjectsDir() string {
	return filepath.Join(BackupDir(), "projects")
}

// BackupManifestPath records per-session backup state (inode, size, paths).
// Separate from the SQLite index so the manifest survives index corruption.
func BackupManifestPath() string {
	return filepath.Join(BackupDir(), "manifest.json")
}

// ClaudeSkillsDir is the user-global skills directory the Claude Code harness
// scans at session start.
func ClaudeSkillsDir() string {
	return filepath.Join(ClaudeDir(), "skills")
}

// SkillLiveDir is the on-disk extraction of the embedded cct skill content.
// Symlinked from SkillSymlinkPath() on install; safe to delete (regenerated
// on next cct invocation).
func SkillLiveDir() string {
	return filepath.Join(CacheDir(), "skills", "cct")
}

// SkillSymlinkPath is where ~/.claude/skills/cct lives. Always a symlink to
// SkillLiveDir() when managed by cct.
func SkillSymlinkPath() string {
	return filepath.Join(ClaudeSkillsDir(), "cct")
}

// SkillNudgeLastPath stores the unix timestamp of the most recent install
// nudge so we can rate-limit it to once per 24h.
func SkillNudgeLastPath() string {
	return filepath.Join(CacheDir(), "skill-nudge-last")
}

// SkillNudgeDisabledPath, when present, suppresses the install nudge entirely
// (set by `cct skill nudge off`).
func SkillNudgeDisabledPath() string {
	return filepath.Join(CacheDir(), "skill-nudge-disabled")
}
