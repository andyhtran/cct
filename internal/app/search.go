package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/andyhtran/cct/internal/index"
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

func makeSearchTable(query string) *output.Table {
	return output.NewTable(query,
		output.Fixed("SESSION", 16),
		output.Flex("PROJECT", 25, 15),
		output.Fixed("AGE", 6),
		output.Flex("MATCH", 0, 30),
	)
}

type SearchCmd struct {
	Query      string `arg:"" help:"Search query"`
	Project    string `short:"p" help:"Filter by project name"`
	Session    string `short:"s" help:"Search within a specific session (ID or prefix)"`
	Limit      int    `short:"n" help:"Max results (0=no limit)" default:"25"`
	All        bool   `short:"a" help:"Show all results"`
	MaxMatches int    `short:"m" help:"Max matches per session" default:"3"`
	Context    int    `short:"C" help:"Extra context characters for snippets" default:"0"`
	Sort       string `help:"Sort order: recency (default), relevance" default:"recency" enum:"recency,relevance"`
	NoAgents   bool   `help:"Exclude sub-agent sessions" name:"no-agents"`
	Sync       bool   `help:"Force index sync before searching"`
}

func (cmd *SearchCmd) Run(globals *Globals) error {
	// Single-session search mode uses streaming (no index needed)
	if cmd.Session != "" {
		return cmd.runSessionSearch(globals)
	}

	idx, err := index.Open()
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = idx.Close() }()

	includeAgents := !cmd.NoAgents

	if cmd.Sync {
		if err := idx.ForceSync(includeAgents); err != nil {
			return fmt.Errorf("sync: %w", err)
		}
	}

	if !globals.JSON {
		if status, err := idx.Status(); err == nil && status.TotalSessions == 0 {
			fmt.Fprintln(os.Stderr, "Building search index...")
		}
	}

	tbl := makeSearchTable(cmd.Query)

	limit := cmd.Limit
	if cmd.All {
		limit = 0
	}

	results, total, err := idx.Search(index.SearchOptions{
		Query:         cmd.Query,
		ProjectFilter: cmd.Project,
		IncludeAgents: includeAgents,
		MaxResults:    limit,
		MaxMatches:    cmd.MaxMatches,
		SnippetWidth:  tbl.LastColWidth() + cmd.Context,
		SortBy:        cmd.Sort,
	})
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		switch {
		case cmd.Project != "" && !idx.ProjectExists(cmd.Project):
			fmt.Printf("  No project matching %q\n", cmd.Project)
		case cmd.Project != "":
			fmt.Printf("  No sessions matching %q in project %q\n", cmd.Query, cmd.Project)
		default:
			fmt.Printf("  No sessions matching %q\n", cmd.Query)
		}
		return nil
	}

	if !globals.JSON && total > len(results) {
		fmt.Fprintf(os.Stderr, "Showing %d of %d results (use --all or -n to adjust)\n", len(results), total)
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	fmt.Printf("\n  Found %d session(s) matching %q\n", total, cmd.Query)
	fmt.Println()
	tbl.PrintHeader()
	printResults(results, tbl)
	fmt.Println()

	sessions := make([]*session.Session, 0, len(results))
	for _, r := range results {
		sessions = append(sessions, r.Session)
	}
	printResumeHints(sessions)
	fmt.Println()
	return nil
}

// runSessionSearch searches within a specific session using streaming (for -s flag)
func (cmd *SearchCmd) runSessionSearch(globals *Globals) error {
	s, err := session.FindByPrefix(cmd.Session)
	if err != nil {
		return err
	}

	tbl := makeSearchTable(cmd.Query)
	results := session.SearchFiles([]string{s.FilePath}, cmd.Query, tbl.LastColWidth()+cmd.Context, cmd.MaxMatches)

	if len(results) == 0 {
		fmt.Printf("  No matches for %q in session %s\n", cmd.Query, s.ShortID)
		return nil
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	fmt.Printf("\n  Found %d match(es) for %q in session %s\n", len(results[0].Matches), cmd.Query, s.ShortID)
	fmt.Println()
	tbl.PrintHeader()

	for _, r := range results {
		printSessionMatches(r.Session, r.Matches, tbl)
	}

	fmt.Println()
	return nil
}

func printResults(results []index.SearchResult, tbl *output.Table) {
	for _, r := range results {
		printSessionMatches(r.Session, r.Matches, tbl)
	}
}

func printSessionMatches(s *session.Session, matches []session.Match, tbl *output.Table) {
	projectName := s.ProjectName
	if s.IsAgent {
		projectName += " (agent)"
	}
	for i, m := range matches {
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
