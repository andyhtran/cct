package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

var appVersion string

type CLI struct {
	Globals

	Version kong.VersionFlag `short:"v" help:"Show version"`

	Default     DefaultCmd   `cmd:"" default:"noargs" hidden:""`
	List        ListCmd      `cmd:"" help:"List recent sessions"`
	Search      SearchCmd    `cmd:"" help:"Search session content\n\nJSON fields: id, short_id, project_name, project_path, created, modified, first_prompt, git_branch, message_count, matches, score\n\nExample: cct search 'query' --json | jq '.[] | {short_id, project_name, created}'"`
	Info        InfoCmd      `cmd:"" help:"Show session metadata and first prompt"`
	Resume      ResumeCmd    `cmd:"" help:"Resume a session (auto-switches directory)"`
	Export      ExportCmd    `cmd:"" help:"Export session messages (with filtering)"`
	View        ViewCmd      `cmd:"" help:"View session in interactive TUI"`
	Plans       PlansCmd     `cmd:"" help:"Browse and search plans"`
	Stats       StatsCmd     `cmd:"" help:"Session statistics"`
	Changelog   ChangelogCmd `cmd:"" aliases:"log" help:"Show Claude Code changelog\n\nFetches the upstream CHANGELOG.md from the claude-code GitHub repo (cached locally for 6h). Use this to look up recent features, behavior changes, and disable flags.\n\nExamples:\n  cct changelog                              # Latest release only\n  cct changelog 2.1.111                      # A specific version\n  cct changelog --since 2.1.100 --all        # Every change since 2.1.100\n  cct changelog --search 'disable|opt.?out'  # Grep across all entries\n  cct changelog --refresh                    # Force re-fetch from GitHub"`
	VersionInfo VersionCmd   `cmd:"" name:"version" help:"Show version information"`
	Schema      SchemaCmd    `cmd:"" help:"Show CLI schema as JSON (for tooling)"`
	Index       IndexCmd     `cmd:"" help:"Manage search index"`
}

type Globals struct {
	JSON bool `help:"Output as JSON" name:"json"`
}

func Run(version string) int {
	appVersion = version

	var cli CLI
	k, err := kong.New(&cli,
		kong.Name("cct"),
		kong.Description("Claude Code Tools"),
		kong.Vars{"version": "cct " + appVersion},
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cct: %v\n", err)
		return 1
	}

	ctx, err := k.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cct: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run 'cct --help' or 'cct <command> --help' for usage.\n")
		return 1
	}

	err = ctx.Run(&cli.Globals, k)
	if err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
