package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/andyhtran/cct/internal/changelog"
)

type VersionCmd struct{}

type versionInfo struct {
	CCT    string `json:"cct"`
	Claude string `json:"claude_code"`
}

func (cmd *VersionCmd) Run(globals *Globals) error {
	claudeVer := changelog.DetectClaudeVersion()

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(versionInfo{
			CCT:    appVersion,
			Claude: claudeVer,
		})
	}

	fmt.Printf("  cct %s\n", appVersion)
	fmt.Printf("  Claude Code %s\n", claudeVer)
	return nil
}
