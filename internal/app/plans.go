package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/plan"
)

type PlansCmd struct {
	List   PlansListCmd   `cmd:"" default:"1" hidden:""`
	Search PlansSearchCmd `cmd:"" help:"Search plan content"`
	Cp     PlansCpCmd     `cmd:"" help:"Copy a plan to current directory"`
}

type PlansListCmd struct {
	Project string `short:"p" help:"Filter by project name (matches title or name)"`
	Limit   int    `short:"n" help:"Max results (0=no limit)" default:"0"`
	All     bool   `short:"a" help:"Show all results"`
}

func (cmd *PlansListCmd) Run(globals *Globals) error {
	plans, err := plan.ListPlans()
	if err != nil {
		return err
	}

	if cmd.Project != "" {
		projectLower := strings.ToLower(cmd.Project)
		var filtered []plan.Plan
		for _, p := range plans {
			if strings.Contains(strings.ToLower(p.Title), projectLower) ||
				strings.Contains(strings.ToLower(p.Name), projectLower) {
				filtered = append(filtered, p)
			}
		}
		plans = filtered
	}

	if len(plans) == 0 {
		if cmd.Project != "" {
			fmt.Printf("  No plans matching project %q\n", cmd.Project)
		} else {
			fmt.Println("  No plans found.")
		}
		return nil
	}

	if !cmd.All && cmd.Limit > 0 && len(plans) > cmd.Limit {
		total := len(plans)
		plans = plans[:cmd.Limit]
		if !globals.JSON {
			fmt.Fprintf(os.Stderr, "Showing %d of %d plans (use --all or -n to adjust)\n", cmd.Limit, total)
		}
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
	Limit int    `short:"n" help:"Max results (0=no limit)" default:"25"`
	All   bool   `short:"a" help:"Show all results"`
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

	// Apply limit
	if !cmd.All && cmd.Limit > 0 && len(matches) > cmd.Limit {
		total := len(matches)
		matches = matches[:cmd.Limit]
		if !globals.JSON {
			fmt.Fprintf(os.Stderr, "Showing %d of %d results (use --all or -n to adjust)\n", cmd.Limit, total)
		}
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
