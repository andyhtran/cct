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
	if match.CustomTitle != "" {
		fmt.Printf("  %s    %s\n", output.Dim("Title:"), output.Bold(match.CustomTitle))
	}
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
	if match.Model != "" {
		fmt.Printf("  %s    %s\n", output.Dim("Model:"), match.Model)
	}
	if match.ContextTokens > 0 {
		window := session.ContextWindow(match.Model)
		pct := float64(match.ContextTokens) / float64(window) * 100
		line := fmt.Sprintf("%s / %s (%.0f%%)",
			formatInt(match.ContextTokens), formatInt(window), pct)
		if match.PeakContextTokens > match.ContextTokens {
			line += fmt.Sprintf("  %s %s", output.Dim("peak"), formatInt(match.PeakContextTokens))
		}
		fmt.Printf("  %s  %s\n", output.Dim("Context:"), line)
	}
	if match.TotalOutputTokens > 0 {
		fmt.Printf("  %s   %s\n", output.Dim("Output:"), formatInt(match.TotalOutputTokens))
	}
	fmt.Printf("  %s   %s\n", output.Dim("Prompt:"), prompt)
	fmt.Println()
	fmt.Printf("  %s\n", output.Cyan(fmt.Sprintf("cct resume %s", match.ShortID)))
	fmt.Println()
	return nil
}

func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b []byte
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	first := len(s) % 3
	if first > 0 {
		b = append(b, s[:first]...)
		if len(s) > first {
			b = append(b, ',')
		}
	}
	for i := first; i < len(s); i += 3 {
		b = append(b, s[i:i+3]...)
		if i+3 < len(s) {
			b = append(b, ',')
		}
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
