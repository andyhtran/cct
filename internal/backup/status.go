package backup

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/andyhtran/cct/internal/paths"
	"github.com/andyhtran/cct/internal/session"
)

// SessionStatusCode classifies a single session's relationship between the
// live tree (~/.claude/projects) and the backup manifest.
type SessionStatusCode string

const (
	StatusBackedUp    SessionStatusCode = "backed-up"
	StatusDrifted     SessionStatusCode = "drifted"
	StatusOrphaned    SessionStatusCode = "orphaned"
	StatusNotBackedUp SessionStatusCode = "not-backed-up"
)

// SessionStatus describes one session in the drift report. Exactly one of
// SourcePath/BackupPath may be empty depending on Status.
type SessionStatus struct {
	SessionID       string            `json:"session_id"`
	Status          SessionStatusCode `json:"status"`
	SourcePath      string            `json:"source_path,omitempty"`
	BackupPath      string            `json:"backup_path,omitempty"`
	LiveSize        int64             `json:"live_size,omitempty"`
	BackupSize      int64             `json:"backup_size,omitempty"`
	CopyMode        CopyMode          `json:"copy_mode,omitempty"`
	SourceDeletedAt time.Time         `json:"source_deleted_at,omitempty"`
	Reason          string            `json:"reason,omitempty"`
}

// Status is the drift report returned by BuildStatus. Counts is keyed by
// SessionStatusCode string values so JSON consumers can index it directly.
// LastSweep is the most recent LastVerifiedAt across all manifest entries
// (zero when the manifest is empty). TotalBackupSize sums BackupSize across
// every session in the report — intended for the one-line "Total size" in
// human output.
type Status struct {
	ManifestPath    string          `json:"manifest_path"`
	BackupDir       string          `json:"backup_dir"`
	Counts          map[string]int  `json:"counts"`
	Sessions        []SessionStatus `json:"sessions"`
	LastSweep       time.Time       `json:"last_sweep,omitempty"`
	TotalBackupSize int64           `json:"total_backup_size,omitempty"`
}

// BuildStatus classifies every session ID in the manifest and live tree.
// Production callers use this wrapper; tests inject paths via BuildStatusAt.
func BuildStatus() (*Status, error) {
	return BuildStatusAt(paths.BackupManifestPath(), paths.ProjectsDir(), paths.BackupDir())
}

func BuildStatusAt(manifestPath, sourceRoot, backupDir string) (*Status, error) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	liveByID := make(map[string]string)
	for _, path := range discoverSources(sourceRoot, true) {
		liveByID[session.ExtractIDFromFilename(path)] = path
	}

	status := &Status{
		ManifestPath: manifestPath,
		BackupDir:    backupDir,
		Counts:       map[string]int{},
		Sessions:     []SessionStatus{},
	}

	seen := make(map[string]bool, len(manifest.Entries))
	for id := range manifest.Entries {
		entry := manifest.Entries[id]
		ss := classifyManifestEntry(id, entry, liveByID)
		status.Counts[string(ss.Status)]++
		status.Sessions = append(status.Sessions, ss)
		seen[id] = true
	}
	status.LastSweep = manifest.LastSweep()

	for id, livePath := range liveByID {
		if seen[id] {
			continue
		}
		ss := SessionStatus{
			SessionID:  id,
			Status:     StatusNotBackedUp,
			SourcePath: livePath,
		}
		if info, err := os.Stat(livePath); err == nil {
			ss.LiveSize = info.Size()
		}
		status.Counts[string(ss.Status)]++
		status.Sessions = append(status.Sessions, ss)
	}

	sort.Slice(status.Sessions, func(i, j int) bool {
		a, b := status.Sessions[i], status.Sessions[j]
		if a.Status != b.Status {
			return statusOrder(a.Status) < statusOrder(b.Status)
		}
		return a.SessionID < b.SessionID
	})

	for i := range status.Sessions {
		status.TotalBackupSize += status.Sessions[i].BackupSize
	}

	return status, nil
}

func classifyManifestEntry(id string, entry Entry, liveByID map[string]string) SessionStatus {
	ss := SessionStatus{
		SessionID:  id,
		SourcePath: entry.SourcePath,
		BackupPath: entry.BackupPath,
		CopyMode:   entry.CopyMode,
	}
	if bi, err := os.Stat(entry.BackupPath); err == nil {
		ss.BackupSize = bi.Size()
	}

	livePath, hasLive := liveByID[id]
	if !hasLive {
		ss.Status = StatusOrphaned
		ss.SourceDeletedAt = entry.SourceDeletedAt
		return ss
	}

	ss.SourcePath = livePath
	liveInfo, err := os.Stat(livePath)
	if err != nil {
		ss.Status = StatusOrphaned
		ss.SourceDeletedAt = entry.SourceDeletedAt
		return ss
	}
	ss.LiveSize = liveInfo.Size()

	backupInfo, backupErr := os.Stat(entry.BackupPath)
	if backupErr != nil {
		ss.Status = StatusDrifted
		ss.Reason = "backup file missing"
		return ss
	}

	if entry.CopyMode == CopyModeHardlink {
		liveIno := inodeOf(liveInfo)
		backupIno := inodeOf(backupInfo)
		if liveIno != backupIno {
			ss.Status = StatusDrifted
			ss.Reason = fmt.Sprintf("inode mismatch (live=%d backup=%d)", liveIno, backupIno)
			return ss
		}
		if liveInfo.Size() != backupInfo.Size() {
			// Shared inode with differing sizes shouldn't happen; report it so
			// the operator investigates rather than silently trusting the sweep.
			ss.Status = StatusDrifted
			ss.Reason = fmt.Sprintf("size differs (live=%d backup=%d)", liveInfo.Size(), backupInfo.Size())
			return ss
		}
		ss.Status = StatusBackedUp
		return ss
	}

	// Copy-mode entries can't check inode parity; size equality is the best
	// signal we have that the copy is still current.
	if liveInfo.Size() != backupInfo.Size() {
		ss.Status = StatusDrifted
		ss.Reason = fmt.Sprintf("size differs (live=%d backup=%d)", liveInfo.Size(), backupInfo.Size())
		return ss
	}
	ss.Status = StatusBackedUp
	return ss
}

func statusOrder(s SessionStatusCode) int {
	switch s {
	case StatusDrifted:
		return 0
	case StatusOrphaned:
		return 1
	case StatusNotBackedUp:
		return 2
	case StatusBackedUp:
		return 3
	}
	return 4
}
