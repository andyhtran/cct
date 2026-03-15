package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/andyhtran/cct/internal/index"
	"github.com/andyhtran/cct/internal/output"
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
	return nil
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
	fmt.Printf("Size: %s\n", formatBytes(status.IndexSizeBytes))
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
	if r.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", r.Deleted))
	}
	summary := "Synced " + strings.Join(parts, ", ")
	if r.Unchanged > 0 {
		summary += fmt.Sprintf(" (%d unchanged)", r.Unchanged)
	}
	return summary
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
