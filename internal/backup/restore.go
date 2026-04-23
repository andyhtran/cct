package backup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/andyhtran/cct/internal/paths"
)

type RestoreResult struct {
	Restored int
	Skipped  int // session ID not in manifest, or live file exists and !Force
	Errors   []error
}

type RestoreOptions struct {
	// SessionIDs names the entries to restore. Required — restoring "everything"
	// is not a supported mode; callers must opt in by naming IDs.
	SessionIDs []string

	// DryRun reports what would happen without writing. Required by policy for
	// any command that modifies the live projects dir.
	DryRun bool

	// Force overwrites an existing live file. Default-deny protects against a
	// fat-fingered restore on a still-live session, which would drop any writes
	// since the last sweep.
	Force bool

	Progress io.Writer
}

// ErrNoSessionIDs is returned when RestoreOptions.SessionIDs is empty.
var ErrNoSessionIDs = errors.New("restore: at least one session ID is required")

// Restore hardlinks (or copies) backup files back into their original source
// paths under ~/.claude/projects/. Used after a Claude Code cleanup bug wipes
// a session that cct had backed up.
func Restore(opts RestoreOptions) (*RestoreResult, error) {
	return RestoreAt(paths.BackupManifestPath(), opts)
}

// RestoreAt is the test-injectable variant.
func RestoreAt(manifestPath string, opts RestoreOptions) (*RestoreResult, error) {
	if len(opts.SessionIDs) == 0 {
		return nil, ErrNoSessionIDs
	}

	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	result := &RestoreResult{}
	for _, id := range opts.SessionIDs {
		entry, ok := manifest.Entries[id]
		if !ok {
			result.Skipped++
			if opts.Progress != nil {
				_, _ = fmt.Fprintf(opts.Progress, "skip %s: not in manifest\n", id)
			}
			continue
		}

		// Default-deny when the live file exists. Any stat error other than
		// ErrNotExist (permission, etc.) is also a don't-touch — surface it as
		// a skip rather than silently blowing past and clobbering the file.
		if !opts.Force {
			if _, statErr := os.Stat(entry.SourcePath); statErr == nil {
				result.Skipped++
				if opts.Progress != nil {
					_, _ = fmt.Fprintf(opts.Progress, "skip %s: live file already exists; pass --force to overwrite\n", id)
				}
				continue
			} else if !errors.Is(statErr, fs.ErrNotExist) {
				result.Skipped++
				if opts.Progress != nil {
					_, _ = fmt.Fprintf(opts.Progress, "skip %s: cannot stat live file (%v); pass --force to overwrite\n", id, statErr)
				}
				continue
			}
		}

		if opts.DryRun {
			if opts.Progress != nil {
				_, _ = fmt.Fprintf(opts.Progress, "would restore %s -> %s\n", entry.BackupPath, entry.SourcePath)
			}
			result.Restored++
			continue
		}

		if err := restoreOne(entry); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", id, err))
			continue
		}
		result.Restored++
		if opts.Progress != nil {
			_, _ = fmt.Fprintf(opts.Progress, "restored %s\n", entry.SourcePath)
		}
	}

	if !opts.DryRun && result.Restored > 0 {
		now := time.Now()
		for _, id := range opts.SessionIDs {
			entry, ok := manifest.Entries[id]
			if !ok {
				continue
			}
			entry.LastVerifiedAt = now
			entry.SourceLastSeenAt = now
			entry.SourceDeletedAt = time.Time{}
			manifest.Entries[id] = entry
		}
		if err := manifest.Save(); err != nil {
			return result, fmt.Errorf("save manifest: %w", err)
		}
	}

	return result, nil
}

func restoreOne(entry Entry) error {
	if err := os.MkdirAll(filepath.Dir(entry.SourcePath), 0o755); err != nil {
		return err
	}
	_ = os.Remove(entry.SourcePath)

	// Fall through to copy on any link failure (EXDEV or otherwise) — the user
	// cares about getting their data back, not the mechanism.
	if err := os.Link(entry.BackupPath, entry.SourcePath); err == nil {
		return nil
	}
	return atomicCopy(entry.BackupPath, entry.SourcePath)
}
