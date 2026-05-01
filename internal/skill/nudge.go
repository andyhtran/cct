package skill

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/andyhtran/cct/internal/paths"
)

// nudgeInterval rate-limits the install prompt so it isn't shown on every
// invocation. 24h is the sweet spot — visible enough to be remembered, sparse
// enough not to be noise.
const nudgeInterval = 24 * time.Hour

// MaybeNudge prints a one-line install hint to w when the skill isn't
// installed, the user hasn't silenced the nudge, and >24h have passed since
// the last one. Best-effort: any I/O error is swallowed.
//
// stderr is the right destination — keeps stdout clean for `--json` pipelines
// while still surfacing the hint to both terminal users and agents (Claude
// Code's Bash tool captures both streams).
func MaybeNudge(w io.Writer) {
	if ours, err := IsOurSymlink(); err == nil && ours {
		return
	}
	if _, err := os.Stat(paths.SkillNudgeDisabledPath()); err == nil {
		return
	}

	lastPath := paths.SkillNudgeLastPath()
	if b, err := os.ReadFile(lastPath); err == nil {
		if ts, parseErr := strconv.ParseInt(string(b), 10, 64); parseErr == nil {
			if time.Since(time.Unix(ts, 0)) < nudgeInterval {
				return
			}
		}
	}

	_, _ = fmt.Fprintln(w, "tip: cct ships a Claude Code skill so agents auto-discover this tool.")
	_, _ = fmt.Fprintln(w, "     run `cct skill install` to enable, or `cct skill nudge off` to silence.")

	_ = os.MkdirAll(filepath.Dir(lastPath), 0o755)
	_ = os.WriteFile(lastPath, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o644)
}

// SetNudgeEnabled toggles the persistent disable flag. enabled=true removes
// the flag file; enabled=false creates it.
func SetNudgeEnabled(enabled bool) error {
	p := paths.SkillNudgeDisabledPath()
	if enabled {
		err := os.Remove(p)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte{}, 0o644)
}

// NudgeEnabled reports whether the install nudge is currently enabled (i.e.
// the user has not run `cct skill nudge off`).
func NudgeEnabled() bool {
	_, err := os.Stat(paths.SkillNudgeDisabledPath())
	return errors.Is(err, os.ErrNotExist)
}
