// Package backup hardlinks ~/.claude/projects/**/*.jsonl into cct's cache
// directory so session history survives upstream Claude Code cleanup bugs
// (see issues #41458, #23710, #20992).
//
// The sweep uses hardlinks when source and backup live on the same
// filesystem — which is the common case since both are under $HOME. When
// they're not (EXDEV), it falls back to an atomic byte copy and records
// copy_mode: "copy" so later sweeps re-copy on source change instead of
// relying on inode identity.
//
// Active files (modified within MinQuietPeriod) are skipped to avoid
// capturing the mid-write corruption described in issue #20992. Use
// Options.IncludeActive to override.
package backup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/andyhtran/cct/internal/paths"
	"github.com/andyhtran/cct/internal/session"
)

// MinQuietPeriod guards against capturing mid-write corruption when a main
// session and subagents append to the same JSONL concurrently. Files modified
// more recently than this are skipped unless Options.IncludeActive is set.
const MinQuietPeriod = 10 * time.Minute

type Options struct {
	// IncludeAgents controls whether to back up nested subagent files.
	// Default (false here) means the sweep matches `cct index sync`'s
	// default of including them — callers typically pass true.
	IncludeAgents bool

	// IncludeActive disables the MinQuietPeriod skip. Useful when the caller
	// knows sessions are quiescent (e.g., running from a SessionEnd hook).
	IncludeActive bool

	// Progress receives human-readable status lines. Nil silences output.
	Progress io.Writer
}

type SweepResult struct {
	Linked        int     `json:"linked"`         // new hardlinks created
	Copied        int     `json:"copied"`         // files fell back to copy mode
	Relinked      int     `json:"relinked"`       // existing entry had a different inode — link remade
	Unchanged     int     `json:"unchanged"`      // manifest entry already matches source inode+size
	SkippedActive int     `json:"skipped_active"` // source modified within MinQuietPeriod
	SkippedError  int     `json:"skipped_error"`  // stat or link errors on individual files (logged)
	Orphaned      int     `json:"orphaned"`       // manifest entries whose source is now gone
	Errors        []error `json:"errors,omitempty"`
}

func (r *SweepResult) Summary() string {
	var parts []string
	if r.Linked > 0 {
		parts = append(parts, fmt.Sprintf("%d linked", r.Linked))
	}
	if r.Copied > 0 {
		parts = append(parts, fmt.Sprintf("%d copied", r.Copied))
	}
	if r.Relinked > 0 {
		parts = append(parts, fmt.Sprintf("%d relinked", r.Relinked))
	}
	if r.Unchanged > 0 {
		parts = append(parts, fmt.Sprintf("%d unchanged", r.Unchanged))
	}
	if r.SkippedActive > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped (active)", r.SkippedActive))
	}
	if r.Orphaned > 0 {
		parts = append(parts, fmt.Sprintf("%d orphaned", r.Orphaned))
	}
	if r.SkippedError > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", r.SkippedError))
	}
	if len(parts) == 0 {
		return "Nothing to back up"
	}
	return strings.Join(parts, ", ")
}

// DiscoverBackupSources returns paths of every backup file whose manifest
// entry is still present on disk. Used by the index sync to treat backups as
// secondary sources so deleted-from-upstream sessions stay searchable.
// Missing manifest or manifest parse errors yield an empty slice — we'd
// rather skip silently than block indexing.
func DiscoverBackupSources() []string {
	m, err := LoadManifest(paths.BackupManifestPath())
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(m.Entries))
	for id := range m.Entries {
		backupPath := m.Entries[id].BackupPath
		if _, err := os.Stat(backupPath); err != nil {
			continue
		}
		out = append(out, backupPath)
	}
	return out
}

// Sweep walks ~/.claude/projects/ and hardlinks any JSONL files that aren't
// already backed up with the current inode+size. It is idempotent — re-running
// with no source changes is ~free (stat calls only).
func Sweep(opts Options) (*SweepResult, error) {
	return SweepAt(paths.ProjectsDir(), paths.BackupProjectsDir(), paths.BackupManifestPath(), opts)
}

// SweepAt lets tests inject alternate directories without stomping on HOME.
// Production callers use Sweep.
func SweepAt(sourceRoot, backupRoot, manifestPath string, opts Options) (*SweepResult, error) {
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return nil, fmt.Errorf("create manifest dir: %w", err)
	}

	lockPath := manifestPath + ".lock"
	lock, err := acquireLock(lockPath)
	if err != nil {
		return nil, err
	}
	defer lock.release()

	manifest, err := LoadManifest(manifestPath)
	if err != nil && opts.Progress != nil {
		_, _ = fmt.Fprintf(opts.Progress, "warning: %v (continuing with fresh manifest)\n", err)
	}

	files := discoverSources(sourceRoot, opts.IncludeAgents)
	result := &SweepResult{}

	seenIDs := make(map[string]bool, len(files))
	for _, source := range files {
		id := session.ExtractIDFromFilename(source)
		seenIDs[id] = true

		outcome, err := processOne(source, id, backupRoot, manifest, opts)
		if err != nil {
			result.SkippedError++
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", source, err))
			continue
		}
		applyOutcome(result, outcome)
	}

	// An orphan is a manifest entry whose source is no longer in the live
	// tree. We keep the entry and the backup file — that's the whole point
	// of the backup. We only update SourceDeletedAt on the first sweep that
	// notices the gap. Scope the check to the same filter discovery used:
	// agent entries aren't orphaned just because this sweep excluded them.
	now := time.Now()
	for id := range manifest.Entries {
		if seenIDs[id] {
			continue
		}
		if !opts.IncludeAgents && manifest.Entries[id].IsSubagent {
			continue
		}
		result.Orphaned++
		if manifest.Entries[id].SourceDeletedAt.IsZero() {
			e := manifest.Entries[id]
			e.SourceDeletedAt = now
			manifest.Entries[id] = e
		}
	}

	if err := manifest.Save(); err != nil {
		return result, fmt.Errorf("save manifest: %w", err)
	}
	return result, nil
}

// outcome captures what happened for a single source file so applyOutcome can
// tally it into SweepResult without the processOne body growing side-effects.
type outcome int

const (
	outcomeLinked outcome = iota
	outcomeCopied
	outcomeRelinked
	outcomeUnchanged
	outcomeSkippedActive
)

func applyOutcome(r *SweepResult, o outcome) {
	switch o {
	case outcomeLinked:
		r.Linked++
	case outcomeCopied:
		r.Copied++
	case outcomeRelinked:
		r.Relinked++
	case outcomeUnchanged:
		r.Unchanged++
	case outcomeSkippedActive:
		r.SkippedActive++
	}
}

func processOne(source, id, backupRoot string, manifest *Manifest, opts Options) (outcome, error) {
	info, err := os.Stat(source)
	if err != nil {
		return 0, fmt.Errorf("stat: %w", err)
	}

	if !opts.IncludeActive && time.Since(info.ModTime()) < MinQuietPeriod {
		return outcomeSkippedActive, nil
	}

	ino := inodeOf(info)
	size := info.Size()
	now := time.Now()
	backupPath := backupPathFor(source, backupRoot)
	isSubagent := session.IsAgentSession(id)

	existing, present := manifest.Entries[id]

	if present && existing.Inode == ino && existing.Size == size && existing.CopyMode == CopyModeHardlink {
		if _, err := os.Stat(existing.BackupPath); err == nil {
			existing.SourceLastSeenAt = now
			existing.LastVerifiedAt = now
			// Source is demonstrably live — clear any stale deletion marker
			// from a prior orphan-detecting sweep (e.g. a --no-agents sweep
			// that excluded this entry, or a brief disappearance).
			existing.SourceDeletedAt = time.Time{}
			manifest.Entries[id] = existing
			return outcomeUnchanged, nil
		}
	}

	// Stale or missing entry: remove any old backup file, then relink.
	if present {
		_ = os.Remove(existing.BackupPath)
	}

	mode, err := linkOrCopy(source, backupPath)
	if err != nil {
		return 0, err
	}

	entry := Entry{
		SourcePath:       source,
		BackupPath:       backupPath,
		Inode:            ino,
		Size:             size,
		CopyMode:         mode,
		IsSubagent:       isSubagent,
		LastVerifiedAt:   now,
		SourceLastSeenAt: now,
	}
	if present {
		entry.FirstBackedUpAt = existing.FirstBackedUpAt
	}
	if entry.FirstBackedUpAt.IsZero() {
		entry.FirstBackedUpAt = now
	}
	manifest.Entries[id] = entry

	switch {
	case !present && mode == CopyModeHardlink:
		return outcomeLinked, nil
	case !present && mode == CopyModeCopy:
		return outcomeCopied, nil
	default:
		return outcomeRelinked, nil
	}
}

// linkOrCopy attempts os.Link first. EXDEV means cross-filesystem; fall back
// to an atomic temp+rename copy. Returns the mode actually used so the
// manifest can record whether future verification can rely on inode identity.
func linkOrCopy(source, dest string) (CopyMode, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("mkdir backup parent: %w", err)
	}

	if err := os.Link(source, dest); err == nil {
		return CopyModeHardlink, nil
	} else if !isCrossDevice(err) {
		return "", fmt.Errorf("link: %w", err)
	}

	if err := atomicCopy(source, dest); err != nil {
		return "", fmt.Errorf("copy: %w", err)
	}
	return CopyModeCopy, nil
}

func isCrossDevice(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return errors.Is(linkErr.Err, syscall.EXDEV)
	}
	return errors.Is(err, syscall.EXDEV)
}

func atomicCopy(source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".copy-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// backupPathFor mirrors the source layout under backupRoot so restore is a
// straightforward reverse operation. Accepts any path that's a descendant of
// ~/.claude/projects (the normal case) and any path otherwise (for tests).
func backupPathFor(source, backupRoot string) string {
	projectsDir := paths.ProjectsDir()
	if rel, err := filepath.Rel(projectsDir, source); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join(backupRoot, rel)
	}
	return filepath.Join(backupRoot, filepath.Base(source))
}

// inodeOf pulls the inode number from a FileInfo. Linux and macOS both return
// syscall.Stat_t here, so the type assertion is safe on the platforms the
// repo targets.
func inodeOf(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}

// discoverSources reuses session.DiscoverFiles when operating on the real
// ProjectsDir, otherwise walks a custom root (tests only). Subagent layout
// mirrors session.discoverNestedSubagents.
func discoverSources(sourceRoot string, includeAgents bool) []string {
	if sourceRoot == paths.ProjectsDir() {
		return session.DiscoverFiles("", includeAgents)
	}
	return walkCustomRoot(sourceRoot, includeAgents)
}

func walkCustomRoot(root string, includeAgents bool) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectDir := filepath.Join(root, entry.Name())
		children, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}
		for _, c := range children {
			name := c.Name()
			if c.IsDir() {
				if !includeAgents {
					continue
				}
				subDir := filepath.Join(projectDir, name, "subagents")
				sub, err := os.ReadDir(subDir)
				if err != nil {
					continue
				}
				for _, s := range sub {
					sn := s.Name()
					if s.IsDir() || !strings.HasPrefix(sn, "agent-") || !strings.HasSuffix(sn, ".jsonl") {
						continue
					}
					out = append(out, filepath.Join(subDir, sn))
				}
				continue
			}
			if !strings.HasSuffix(name, ".jsonl") || name == "sessions-index.json" {
				continue
			}
			if !includeAgents && strings.HasPrefix(name, "agent-") {
				continue
			}
			out = append(out, filepath.Join(projectDir, name))
		}
	}
	return out
}
