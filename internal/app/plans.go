package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/plan"
)

type PlansCmd struct {
	List   PlansListCmd   `cmd:"" default:"1" hidden:""`
	Search PlansSearchCmd `cmd:"" help:"Search plan content"`
	Cp     PlansCpCmd     `cmd:"" help:"Copy a plan to current directory"`
}

type PlansListCmd struct{}

func (cmd *PlansListCmd) Run(globals *Globals) error {
	plans, err := plan.ListPlans()
	if err != nil {
		return err
	}

	if len(plans) == 0 {
		fmt.Println("  No plans found.")
		return nil
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(plans)
	}

	tbl := output.NewTable("",
		output.Flex("NAME", 35, 20),
		output.Fixed("AGE", 6),
		output.Flex("TITLE", 0, 20),
	)

	fmt.Println()
	tbl.PrintHeader()

	for _, p := range plans {
		tbl.Row(
			[]string{
				output.Truncate(p.Name, tbl.ColWidth(0)),
				output.FormatAge(p.Modified),
				output.Truncate(p.Title, tbl.LastColWidth()),
			},
			[]func(string) string{output.Dim, output.Dim, output.Bold},
		)
	}

	if len(plans) > 0 {
		fmt.Println()
		fmt.Printf("  %s\n", output.Cyan(fmt.Sprintf("cct plans cp %s", plans[0].Name)))
	}
	fmt.Println()
	return nil
}

type PlansSearchCmd struct {
	Query string `arg:"" help:"Search query"`
}

func (cmd *PlansSearchCmd) Run(globals *Globals) error {
	tbl := output.NewTable(cmd.Query,
		output.Flex("NAME", 25, 20),
		output.Fixed("AGE", 6),
		output.Flex("TITLE", 20, 15),
		output.Flex("MATCH", 0, 30),
	)

	matches, err := plan.SearchPlans(cmd.Query, tbl.LastColWidth())
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		fmt.Printf("  No plans matching %q\n", cmd.Query)
		return nil
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(matches)
	}

	fmt.Printf("\n  Found %d plan(s) matching %q\n", len(matches), cmd.Query)
	fmt.Println()
	tbl.PrintHeader()

	for _, m := range matches {
		tbl.Row(
			[]string{
				output.Truncate(m.Plan.Name, tbl.ColWidth(0)),
				output.FormatAge(m.Plan.Modified),
				output.Truncate(m.Plan.Title, tbl.ColWidth(2)),
				m.Snippet,
			},
			[]func(string) string{output.Dim, output.Dim, output.Bold, nil},
		)
	}

	if len(matches) > 0 {
		fmt.Println()
		fmt.Printf("  %s\n", output.Cyan(fmt.Sprintf("cct plans cp %s", matches[0].Plan.Name)))
	}
	fmt.Println()
	return nil
}

type PlansCpCmd struct {
	Name string `arg:"" help:"Plan name or partial match"`
	As   string `help:"Rename copied file" name:"as"`
}

func (cmd *PlansCpCmd) Run(globals *Globals) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine current directory: %w", err)
	}

	dest, err := plan.CopyPlan(cmd.Name, cwd, cmd.As)
	if err != nil {
		return err
	}

	fmt.Printf("  Copied to %s\n", dest)
	return nil
}
