package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

type InfoCmd struct {
	ID string `arg:"" help:"Session ID or prefix"`
}

func (cmd *InfoCmd) Run(globals *Globals) error {
	match, err := session.FindByPrefixFull(cmd.ID)
	if err != nil {
		return err
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(match)
	}

	prompt := match.FirstPrompt
	if prompt == "" {
		prompt = "[no prompt]"
	}

	fmt.Println()
	fmt.Printf("  %s  %s\n", output.Dim("Session:"), match.ID)
	if match.ProjectPath != "" {
		fmt.Printf("  %s  %s\n", output.Dim("Project:"), output.Bold(match.ProjectPath))
	}
	if match.GitBranch != "" {
		fmt.Printf("  %s   %s\n", output.Dim("Branch:"), match.GitBranch)
	}
	if !match.Created.IsZero() {
		fmt.Printf("  %s  %s\n", output.Dim("Created:"), match.Created.Local().Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("  %s %s (%s)\n", output.Dim("Modified:"), match.Modified.Local().Format("2006-01-02 15:04:05"), output.FormatAge(match.Modified))
	fmt.Printf("  %s %s\n", output.Dim("Messages:"), fmt.Sprintf("%d", match.MessageCount))
	fmt.Printf("  %s   %s\n", output.Dim("Prompt:"), prompt)
	fmt.Println()
	fmt.Printf("  %s\n", output.Cyan(fmt.Sprintf("cct resume %s", match.ShortID)))
	fmt.Println()
	return nil
}
