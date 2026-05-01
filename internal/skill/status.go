package skill

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/andyhtran/cct/internal/paths"
)

// Status captures everything `cct skill status` reports.
type Status struct {
	Installed      bool   `json:"installed"`
	SymlinkPath    string `json:"symlink_path"`
	SymlinkTarget  string `json:"symlink_target,omitempty"`
	OurSymlink     bool   `json:"our_symlink"`
	LiveDir        string `json:"live_dir"`
	EmbeddedHash   string `json:"embedded_hash"`
	LiveHash       string `json:"live_hash,omitempty"`
	InSync         bool   `json:"in_sync"`
	NudgeEnabled   bool   `json:"nudge_enabled"`
	NudgeLastShown string `json:"nudge_last_shown,omitempty"`
}

// GetStatus reads disk state and reports installation, sync, and nudge state.
// Best-effort on individual fields: a missing file or unreadable target falls
// back to a zero value rather than erroring the whole call.
func GetStatus() (Status, error) {
	s := Status{
		SymlinkPath:  paths.SkillSymlinkPath(),
		LiveDir:      paths.SkillLiveDir(),
		NudgeEnabled: NudgeEnabled(),
	}
	if h, err := embeddedHash(); err == nil {
		s.EmbeddedHash = h
	}

	if b, err := os.ReadFile(filepath.Join(s.LiveDir, versionMarkerFile)); err == nil {
		s.LiveHash = string(b)
		s.InSync = s.LiveHash == s.EmbeddedHash
	}

	if info, err := os.Lstat(s.SymlinkPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(s.SymlinkPath); err == nil {
			s.SymlinkTarget = target
			s.OurSymlink = target == s.LiveDir
			s.Installed = s.OurSymlink
		}
	}

	if b, err := os.ReadFile(paths.SkillNudgeLastPath()); err == nil {
		if ts, parseErr := strconv.ParseInt(string(b), 10, 64); parseErr == nil {
			s.NudgeLastShown = time.Unix(ts, 0).Format(time.RFC3339)
		}
	}

	return s, nil
}
