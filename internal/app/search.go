package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

var roleTag = map[string]string{
	"user":      "[u]",
	"assistant": "[a]",
}

func formatMatchRole(m session.Match) string {
	tag := roleTag[m.Role]
	if tag == "" {
		tag = "[?]"
	}
	if m.Source != "" {
		tag = tag[:len(tag)-1] + ":" + m.Source + "]"
	}
	return output.Dim(tag) + " " + m.Snippet
}

type SearchCmd struct {
	Query      string `arg:"" help:"Search query"`
	Project    string `short:"p" help:"Filter by project name"`
	Session    string `short:"s" help:"Search within a specific session (ID or prefix)"`
	Limit      int    `short:"n" help:"Max results (0=no limit)" default:"25"`
	All        bool   `short:"a" help:"Show all results"`
	MaxMatches int    `short:"m" help:"Max matches per session" default:"3"`
	Context    int    `short:"C" help:"Extra context characters for snippets" default:"0"`
	NoAgents   bool   `help:"Exclude sub-agent sessions" name:"no-agents"`
}

func (cmd *SearchCmd) Run(globals *Globals) error {
	tbl := output.NewTable(cmd.Query,
		output.Fixed("SESSION", 16),
		output.Flex("PROJECT", 25, 15),
		output.Fixed("AGE", 6),
		output.Flex("MATCH", 0, 30),
	)

	var files []string
	if cmd.Session != "" {
		s, err := session.FindByPrefix(cmd.Session)
		if err != nil {
			return err
		}
		files = []string{s.FilePath}
	} else {
		files = session.DiscoverFiles(cmd.Project, !cmd.NoAgents)
		if !globals.JSON && len(files) > 50 {
			fmt.Fprintf(os.Stderr, "Searching %d sessions...\n", len(files))
		}
	}
	results := session.SearchFiles(files, cmd.Query, tbl.LastColWidth()+cmd.Context, cmd.MaxMatches)

	sort.Slice(results, func(i, j int) bool {
		return results[i].Session.Modified.After(results[j].Session.Modified)
	})

	if !cmd.All && cmd.Limit > 0 && len(results) > cmd.Limit {
		total := len(results)
		results = results[:cmd.Limit]
		if !globals.JSON {
			fmt.Fprintf(os.Stderr, "Showing %d of %d results (use --all or -n to adjust)\n", cmd.Limit, total)
		}
	}

	if len(results) == 0 {
		fmt.Printf("  No sessions matching %q\n", cmd.Query)
		return nil
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	fmt.Printf("\n  Found %d session(s) matching %q\n", len(results), cmd.Query)
	fmt.Println()
	tbl.PrintHeader()

	for _, r := range results {
		s := r.Session
		projectName := s.ProjectName
		if s.IsAgent {
			projectName += " (agent)"
		}
		for i, m := range r.Matches {
			display := formatMatchRole(m)
			if i == 0 {
				tbl.Row(
					[]string{s.ShortID, output.Truncate(projectName, tbl.ColWidth(1)), output.FormatAge(s.Modified), display},
					[]func(string) string{output.Dim, output.Bold, output.Dim, nil},
				)
			} else {
				tbl.Continuation(display)
			}
		}
	}

	fmt.Println()
	sessions := make([]*session.Session, len(results))
	for i, r := range results {
		sessions[i] = r.Session
	}
	printResumeHints(sessions)
	fmt.Println()
	return nil
}
