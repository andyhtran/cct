package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/andyhtran/cct/internal/skill"
)

type SkillCmd struct {
	Install   SkillInstallCmd   `cmd:"" help:"Install the cct Claude Code skill (creates a symlink at ~/.claude/skills/cct)"`
	Uninstall SkillUninstallCmd `cmd:"" help:"Remove the symlink at ~/.claude/skills/cct (live copy is preserved)"`
	Status    SkillStatusCmd    `cmd:"" help:"Show install state, symlink target, sync state, and nudge state"`
	Nudge     SkillNudgeCmd     `cmd:"" help:"Toggle the install-prompt nudge"`
}

type SkillInstallCmd struct{}

func (cmd *SkillInstallCmd) Run(globals *Globals) error {
	if err := skill.Install(); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	if globals.JSON {
		return jsonEncode(map[string]string{"status": "installed"})
	}
	fmt.Println("Installed cct skill at ~/.claude/skills/cct")
	return nil
}

type SkillUninstallCmd struct{}

func (cmd *SkillUninstallCmd) Run(globals *Globals) error {
	if err := skill.Uninstall(); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	if globals.JSON {
		return jsonEncode(map[string]string{"status": "uninstalled"})
	}
	fmt.Println("Removed cct skill symlink (live copy preserved at ~/.cache/cct/skills/cct)")
	return nil
}

type SkillStatusCmd struct{}

func (cmd *SkillStatusCmd) Run(globals *Globals) error {
	s, err := skill.GetStatus()
	if err != nil {
		return err
	}
	if globals.JSON {
		return jsonEncode(s)
	}

	fmt.Printf("Installed:    %s\n", yesNo(s.Installed))
	fmt.Printf("Symlink:      %s\n", s.SymlinkPath)
	switch {
	case s.SymlinkTarget == "":
		fmt.Println("              (no symlink)")
	case s.OurSymlink:
		fmt.Printf("              → %s\n", s.SymlinkTarget)
	default:
		fmt.Printf("              → %s  (foreign — not managed by cct)\n", s.SymlinkTarget)
	}
	fmt.Printf("Live dir:     %s\n", s.LiveDir)
	switch {
	case s.LiveHash == "":
		fmt.Println("Sync:         not yet extracted (run any cct command to populate)")
	case s.InSync:
		fmt.Println("Sync:         in sync with embedded version")
	default:
		fmt.Println("Sync:         out of date (next cct invocation will resync)")
	}
	nudgeWord := "disabled"
	if s.NudgeEnabled {
		nudgeWord = "enabled"
	}
	fmt.Printf("Nudge:        %s", nudgeWord)
	if s.NudgeLastShown != "" {
		fmt.Printf(" (last shown %s)", s.NudgeLastShown)
	}
	fmt.Println()
	return nil
}

type SkillNudgeCmd struct {
	State string `arg:"" enum:"on,off,status" help:"on, off, or status"`
}

func (cmd *SkillNudgeCmd) Run(globals *Globals) error {
	switch cmd.State {
	case "on":
		if err := skill.SetNudgeEnabled(true); err != nil {
			return err
		}
		if !globals.JSON {
			fmt.Println("Nudge enabled")
		}
	case "off":
		if err := skill.SetNudgeEnabled(false); err != nil {
			return err
		}
		if !globals.JSON {
			fmt.Println("Nudge disabled")
		}
	case "status":
		state := "disabled"
		if skill.NudgeEnabled() {
			state = "enabled"
		}
		if globals.JSON {
			return jsonEncode(map[string]string{"nudge": state})
		}
		fmt.Println(state)
	}
	return nil
}

func jsonEncode(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
