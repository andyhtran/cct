package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/andyhtran/cct/internal/backup"
	"github.com/andyhtran/cct/internal/index"
	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/paths"
)

type IndexCmd struct {
	Sync    IndexSyncCmd    `cmd:"" help:"Sync index with latest sessions"`
	Rebuild IndexRebuildCmd `cmd:"" help:"Rebuild index from scratch"`
	Status  IndexStatusCmd  `cmd:"" help:"Show index status"`
}

type IndexSyncCmd struct {
	NoAgents bool `help:"Exclude sub-agent sessions" name:"no-agents"`
}

func (cmd *IndexSyncCmd) Run(globals *Globals) error {
	idx, err := index.Open()
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = idx.Close() }()

	result, err := idx.SyncWithProgress(!cmd.NoAgents, true, os.Stderr)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if globals.JSON {
		status, err := idx.Status()
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Println(formatSyncResult(result))
	printBackupSyncNudge()
	return nil
}

// printBackupSyncNudge appends one of three best-effort lines to `cct index
// sync` output: "feature exists, you haven't used it" when the manifest is
// missing or empty, "last sweep is stale" when the newest LastVerifiedAt is
// older than StaleSweepThreshold, otherwise silent. Manifest read errors are
// swallowed — the nudge is never allowed to block or fail a sync.
func printBackupSyncNudge() {
	m, err := backup.LoadManifest(paths.BackupManifestPath())
	if err != nil {
		return
	}
	if len(m.Entries) == 0 {
		fmt.Println("cct can preserve session history against upstream cleanup bugs")
		fmt.Println("(issues #41458, #23710, #20992). Run `cct backup sweep` to enable.")
		return
	}
	last := m.LastSweep()
	if last.IsZero() {
		return
	}
	age := time.Since(last)
	if age > backup.StaleSweepThreshold {
		fmt.Printf("Last backup was %d days ago. Run `cct backup sweep` to refresh.\n", int(age.Hours()/24))
	}
}

type IndexRebuildCmd struct {
	NoAgents bool `help:"Exclude sub-agent sessions" name:"no-agents"`
}

func (cmd *IndexRebuildCmd) Run(globals *Globals) error {
	idx, err := index.Open()
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = idx.Close() }()

	result, err := idx.RebuildWithProgress(!cmd.NoAgents, os.Stderr)
	if err != nil {
		return fmt.Errorf("rebuild: %w", err)
	}

	if globals.JSON {
		status, err := idx.Status()
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("Indexed %d sessions\n", result.Added)
	return nil
}

type IndexStatusCmd struct{}

func (cmd *IndexStatusCmd) Run(globals *Globals) error {
	idx, err := index.Open()
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = idx.Close() }()

	status, err := idx.Status()
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("Index: %s\n", status.Path)
	fmt.Printf("Sessions: %d\n", status.TotalSessions)
	fmt.Printf("Messages: %d\n", status.TotalMessages)
	fmt.Printf("Size: %s\n", output.FormatBytes(status.IndexSizeBytes))
	if !status.LastSyncTime.IsZero() {
		fmt.Printf("Last sync: %s\n", output.FormatAge(status.LastSyncTime))
	}
	return nil
}

func formatSyncResult(r *index.SyncResult) string {
	if r.UpToDate() {
		if r.Unchanged > 0 {
			return fmt.Sprintf("Already up to date (%d sessions)", r.Unchanged)
		}
		return "Already up to date"
	}

	var parts []string
	if r.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d new", r.Added))
	}
	if r.Updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", r.Updated))
	}
	if r.Adopted > 0 {
		parts = append(parts, fmt.Sprintf("%d adopted", r.Adopted))
	}
	if r.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", r.Deleted))
	}
	summary := "Synced " + strings.Join(parts, ", ")
	if r.Unchanged > 0 {
		summary += fmt.Sprintf(" (%d unchanged)", r.Unchanged)
	}
	if r.Adopted > 0 {
		summary += "\nRun `cct backup status` to see which sessions live in your backup tree."
	}
	return summary
}
