package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/andyhtran/cct/internal/backup"
	"github.com/andyhtran/cct/internal/output"
)

type BackupCmd struct {
	Sweep   BackupSweepCmd   `cmd:"" default:"withargs" help:"Run the backup sweep (default when no subcommand)"`
	Status  BackupStatusCmd  `cmd:"" help:"Per-session drift report between live tree and backup"`
	Restore BackupRestoreCmd `cmd:"" help:"Restore named session IDs from backup to ~/.claude/projects/"`
}

type BackupSweepCmd struct {
	NoAgents      bool `help:"Exclude sub-agent sessions" name:"no-agents"`
	IncludeActive bool `help:"Back up files even if modified in the last 10 minutes (skips the mid-write corruption guard)" name:"include-active"`
	Quiet         bool `help:"Suppress progress output" name:"quiet"`
}

func (cmd *BackupSweepCmd) Run(globals *Globals) error {
	opts := backup.Options{
		IncludeAgents: !cmd.NoAgents,
		IncludeActive: cmd.IncludeActive,
	}
	if !cmd.Quiet && !globals.JSON {
		opts.Progress = os.Stderr
	}

	result, err := backup.Sweep(opts)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if !cmd.Quiet {
		fmt.Println(result.Summary())
	}
	return nil
}

type BackupStatusCmd struct{}

func (cmd *BackupStatusCmd) Run(globals *Globals) error {
	status, err := backup.BuildStatus()
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("Backup dir:    %s\n", status.BackupDir)
	fmt.Printf("Manifest:      %s\n", status.ManifestPath)
	if status.LastSweep.IsZero() {
		fmt.Println("Last sweep:    never (run `cct backup sweep` to start backing up)")
	} else {
		fmt.Printf("Last sweep:    %s\n", output.FormatAge(status.LastSweep))
		if age := time.Since(status.LastSweep); age > backup.StaleSweepThreshold {
			fmt.Printf("Consider running `cct backup sweep` (last run was %d days ago).\n", int(age.Hours()/24))
		}
	}
	fmt.Printf("Total size:    %s\n", output.FormatBytes(status.TotalBackupSize))
	fmt.Println()
	// Summary in stable order (not map iteration order) so output is testable.
	for _, code := range []backup.SessionStatusCode{
		backup.StatusBackedUp,
		backup.StatusDrifted,
		backup.StatusOrphaned,
		backup.StatusNotBackedUp,
	} {
		fmt.Printf("  %-14s %d\n", string(code), status.Counts[string(code)])
	}
	fmt.Println()

	grouped := make(map[backup.SessionStatusCode][]int)
	for i := range status.Sessions {
		code := status.Sessions[i].Status
		grouped[code] = append(grouped[code], i)
	}
	for _, code := range []backup.SessionStatusCode{
		backup.StatusDrifted,
		backup.StatusOrphaned,
		backup.StatusNotBackedUp,
	} {
		indexes := grouped[code]
		if len(indexes) == 0 {
			continue
		}
		fmt.Printf("%s:\n", code)
		sort.Slice(indexes, func(i, j int) bool {
			return status.Sessions[indexes[i]].SessionID < status.Sessions[indexes[j]].SessionID
		})
		for _, idx := range indexes {
			printStatusLine(&status.Sessions[idx])
		}
		fmt.Println()
	}
	return nil
}

func printStatusLine(s *backup.SessionStatus) {
	path := s.SourcePath
	if path == "" {
		path = s.BackupPath
	}
	if s.Reason != "" {
		fmt.Printf("  %s  %s  (%s)\n", s.SessionID, path, s.Reason)
		return
	}
	fmt.Printf("  %s  %s\n", s.SessionID, path)
}

type BackupRestoreCmd struct {
	SessionIDs []string `arg:"" name:"session-id" help:"Session IDs to restore (at least one required)"`
	DryRun     bool     `help:"Report what would be restored without writing" name:"dry-run"`
	Force      bool     `help:"Overwrite the live file when it already exists (default: refuse)" name:"force"`
}

func (cmd *BackupRestoreCmd) Run(globals *Globals) error {
	opts := backup.RestoreOptions{
		SessionIDs: cmd.SessionIDs,
		DryRun:     cmd.DryRun,
		Force:      cmd.Force,
	}
	if !globals.JSON {
		opts.Progress = os.Stderr
	}
	result, err := backup.Restore(opts)
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	prefix := "Restored"
	if cmd.DryRun {
		prefix = "Would restore"
	}
	fmt.Printf("%s: %d, skipped: %d, errors: %d\n", prefix, result.Restored, result.Skipped, len(result.Errors))
	for _, e := range result.Errors {
		fmt.Fprintln(os.Stderr, "  error:", e)
	}
	return nil
}
