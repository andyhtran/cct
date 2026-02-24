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
	Search      SearchCmd    `cmd:"" help:"Search session content"`
	Info        InfoCmd      `cmd:"" help:"Show session metadata and first prompt"`
	Resume      ResumeCmd    `cmd:"" help:"Resume a session (auto-switches directory)"`
	Export      ExportCmd    `cmd:"" help:"Export session messages (with filtering)"`
	Plans       PlansCmd     `cmd:"" help:"Browse and search plans"`
	Stats       StatsCmd     `cmd:"" help:"Session statistics"`
	Changelog   ChangelogCmd `cmd:"" aliases:"log" help:"Show Claude Code changelog"`
	VersionInfo VersionCmd   `cmd:"" name:"version" help:"Show version information"`
	Schema      SchemaCmd    `cmd:"" help:"Show CLI schema as JSON (for tooling)"`
}

type Globals struct {
	JSON bool `help:"Output as JSON" name:"json"`
}

func Run(version string) int {
	appVersion = version

	var cli CLI
	k, err := kong.New(&cli,
		kong.Name("cct"),
		kong.Description("Claude Code utility CLI"),
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
