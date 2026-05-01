// Package skills exposes the bundled cct Claude Code skill as an embedded FS.
// The cct/ subtree is extracted to ~/.cache/cct/skills/cct/ at runtime by
// internal/skill, and symlinked from ~/.claude/skills/cct/ on install.
package skills

import "embed"

//go:embed cct
var FS embed.FS

// Root is the prefix inside FS where the cct skill lives.
const Root = "cct"
