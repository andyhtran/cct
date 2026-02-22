package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

type SearchCmd struct {
	Query   string `arg:"" help:"Search query"`
	Project string `short:"p" help:"Filter by project name"`
}

func (cmd *SearchCmd) Run(globals *Globals) error {
	tbl := output.NewTable(cmd.Query,
		output.Fixed("SESSION", 10),
		output.Flex("PROJECT", 25, 15),
		output.Fixed("AGE", 6),
		output.Flex("MATCH", 0, 30),
	)

	files := session.DiscoverFiles(cmd.Project)
	if !globals.JSON && len(files) > 50 {
		fmt.Fprintf(os.Stderr, "Searching %d sessions...\n", len(files))
	}
	results := session.SearchFiles(files, cmd.Query, tbl.LastColWidth())

	sort.Slice(results, func(i, j int) bool {
		return results[i].Session.Modified.After(results[j].Session.Modified)
	})

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
		for i, m := range r.Matches {
			if i == 0 {
				tbl.Row(
					[]string{s.ShortID, output.Truncate(s.ProjectName, tbl.ColWidth(1)), output.FormatAge(s.Modified), m},
					[]func(string) string{output.Dim, output.Bold, output.Dim, nil},
				)
			} else {
				tbl.Continuation(m)
			}
		}
	}

	fmt.Println()
	n := len(results)
	if n > maxResumeHints {
		n = maxResumeHints
	}
	for _, r := range results[:n] {
		fmt.Printf("  %s\n", output.Cyan(fmt.Sprintf("cct resume %s", r.Session.ShortID)))
	}
	fmt.Println()
	return nil
}
