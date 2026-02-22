package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

type DefaultCmd struct{}

func (cmd *DefaultCmd) Run(globals *Globals) error {
	return listSessions(globals, "", 5, false, true)
}

type ListCmd struct {
	Project string `short:"p" help:"Filter by project name"`
	Limit   int    `short:"n" help:"Max results" default:"15"`
	All     bool   `short:"a" help:"Show all results"`
}

func (cmd *ListCmd) Run(globals *Globals) error {
	return listSessions(globals, cmd.Project, cmd.Limit, cmd.All, false)
}

func listSessions(globals *Globals, project string, limit int, showAll, compact bool) error {
	sessions := session.ScanAll(project, false)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})

	if !showAll && limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	if len(sessions) == 0 {
		fmt.Println("  No sessions found.")
		return nil
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}

	printSessionTable(sessions, compact)
	return nil
}

const maxResumeHints = 3

func printSessionTable(sessions []*session.Session, compact bool) {
	tbl := output.NewTable("",
		output.Fixed("SESSION", 10),
		output.Flex("PROJECT", 30, 15),
		output.Fixed("BRANCH", 8),
		output.Fixed("AGE", 6),
		output.Flex("PROMPT", 0, 20),
	)

	fmt.Println()
	tbl.PrintHeader()

	for _, s := range sessions {
		prompt := s.FirstPrompt
		if prompt == "" {
			prompt = "[no prompt]"
		}
		tbl.Row(
			[]string{
				s.ShortID,
				output.Truncate(s.ProjectName, tbl.ColWidth(1)),
				output.Truncate(s.GitBranch, tbl.ColWidth(2)),
				output.FormatAge(s.Modified),
				output.Truncate(prompt, tbl.LastColWidth()),
			},
			[]func(string) string{output.Dim, output.Bold, output.Dim, output.Dim, output.Dim},
		)
	}

	if !compact {
		fmt.Println()
		n := len(sessions)
		if n > maxResumeHints {
			n = maxResumeHints
		}
		for _, s := range sessions[:n] {
			fmt.Printf("  %s\n", output.Cyan(fmt.Sprintf("cct resume %s", s.ShortID)))
		}
	}
	fmt.Println()
}
