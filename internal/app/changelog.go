package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/andyhtran/cct/internal/changelog"
)

type ChangelogCmd struct {
	Version string `arg:"" optional:"" help:"Show specific version (e.g. 2.1.49)"`
	All     bool   `help:"Show full changelog"`
	Recent  int    `help:"Show last N versions" default:"0"`
}

func (cmd *ChangelogCmd) Run(globals *Globals) error {
	entries, err := changelog.ParseChangelog()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("  No changelog entries found.")
		return nil
	}

	var selected []changelog.VersionEntry

	switch {
	case cmd.Version != "":
		for _, e := range entries {
			if e.Version == cmd.Version {
				selected = append(selected, e)
				break
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("version %s not found in changelog", cmd.Version)
		}

	case cmd.All:
		selected = entries

	case cmd.Recent > 0:
		n := cmd.Recent
		if n > len(entries) {
			n = len(entries)
		}
		selected = entries[:n]

	default:
		selected = entries[:1]
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(selected)
	}

	for i, e := range selected {
		fmt.Printf("  ## %s\n\n", e.Version)
		for _, line := range strings.Split(e.Content, "\n") {
			if line == "" {
				fmt.Println()
			} else {
				fmt.Printf("  %s\n", line)
			}
		}
		if i < len(selected)-1 {
			fmt.Println()
		}
	}
	return nil
}
