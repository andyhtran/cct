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
	Info        InfoCmd      `cmd:"" help:"Show session details"`
	Resume      ResumeCmd    `cmd:"" help:"Resume a session (auto-switches directory)"`
	Export      ExportCmd    `cmd:"" help:"Export session as markdown"`
	Plans       PlansCmd     `cmd:"" help:"Browse and search plans"`
	Stats       StatsCmd     `cmd:"" help:"Session statistics"`
	Changelog   ChangelogCmd `cmd:"" aliases:"log" help:"Show Claude Code changelog"`
	VersionInfo VersionCmd   `cmd:"" name:"version" help:"Show version information"`
}

type Globals struct {
	JSON bool `help:"Output as JSON" name:"json"`
}

func Run(version string) int {
	appVersion = version

	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("cct"),
		kong.Description("Claude Code utility CLI"),
		kong.UsageOnError(),
		kong.Vars{"version": "cct " + appVersion},
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	err := ctx.Run(&cli.Globals)
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
