package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// manifestVersion is bumped only when the on-disk format changes in a
// backward-incompatible way. Adding a new optional field does not bump it.
const manifestVersion = 1

// StaleSweepThreshold gates "consider running sweep" hints in human output.
// Presentation-only — JSON consumers decide their own policy from LastSweep.
const StaleSweepThreshold = 30 * 24 * time.Hour

// CopyMode distinguishes hardlink-backed entries (the normal case) from
// entries that fell back to byte-copy because source and backup live on
// different filesystems. A copy-mode entry needs re-copy on source change;
// a hardlink entry tracks source appends automatically.
type CopyMode string

const (
	CopyModeHardlink CopyMode = "hardlink"
	CopyModeCopy     CopyMode = "copy"
)

type Entry struct {
	SourcePath       string    `json:"source_path"`
	BackupPath       string    `json:"backup_path"`
	Inode            uint64    `json:"inode"`
	Size             int64     `json:"size"`
	CopyMode         CopyMode  `json:"copy_mode"`
	IsSubagent       bool      `json:"is_subagent"`
	FirstBackedUpAt  time.Time `json:"first_backed_up_at"`
	LastVerifiedAt   time.Time `json:"last_verified_at"`
	SourceLastSeenAt time.Time `json:"source_last_seen_at"`
	SourceDeletedAt  time.Time `json:"source_deleted_at,omitempty"`
}

type Manifest struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"` // keyed by session ID (UUID)

	path string `json:"-"`
}

func newManifest(path string) *Manifest {
	return &Manifest{
		Version: manifestVersion,
		Entries: map[string]Entry{},
		path:    path,
	}
}

// LoadManifest reads path. A missing file returns an empty manifest, no error.
// A corrupted file returns an empty manifest plus a non-nil error so callers
// can log; the caller can still proceed — the next sweep will rebuild entries
// by walking the backup dir.
func LoadManifest(path string) (*Manifest, error) {
	m := newManifest(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return m, nil
		}
		return m, fmt.Errorf("read manifest %s: %w", path, err)
	}
	if err := json.Unmarshal(data, m); err != nil {
		return newManifest(path), fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
	m.path = path
	return m, nil
}

// Save writes the manifest atomically via temp + rename. The parent directory
// is created if needed.
func (m *Manifest) Save() error {
	if m.path == "" {
		return fmt.Errorf("manifest has no path")
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}

	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
	if m.Version == 0 {
		m.Version = manifestVersion
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".manifest-*.tmp")
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

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, m.path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// LookupBySourcePath scans entries for one matching source path. Linear scan
// is fine at the scale we operate on (~thousands of entries).
func (m *Manifest) LookupBySourcePath(sourcePath string) (Entry, string, bool) {
	for id := range m.Entries {
		if m.Entries[id].SourcePath == sourcePath {
			return m.Entries[id], id, true
		}
	}
	return Entry{}, "", false
}

// LastSweep returns the most recent LastVerifiedAt across all entries, or the
// zero time when the manifest has no entries.
func (m *Manifest) LastSweep() time.Time {
	var latest time.Time
	for id := range m.Entries {
		if t := m.Entries[id].LastVerifiedAt; t.After(latest) {
			latest = t
		}
	}
	return latest
}
