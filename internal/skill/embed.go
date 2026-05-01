// Package skill manages the on-disk lifecycle of cct's bundled Claude Code
// skill: extracting the embedded content to ~/.cache/cct/skills/cct, creating
// and removing the ~/.claude/skills/cct symlink, and printing the install
// nudge.
package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"sort"

	"github.com/andyhtran/cct/skills"
)

// embeddedHash returns a deterministic sha256 over every file in the embedded
// skill tree (path + null + content + null, files sorted lexicographically).
// Used as a content marker so we only re-extract when the binary's bundled
// version actually differs from what's on disk.
func embeddedHash() (string, error) {
	h := sha256.New()
	var paths []string
	err := fs.WalkDir(skills.FS, skills.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		b, err := skills.FS.ReadFile(p)
		if err != nil {
			return "", err
		}
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
