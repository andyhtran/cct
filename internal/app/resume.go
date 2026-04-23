package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/andyhtran/cct/internal/backup"
	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/paths"
	"github.com/andyhtran/cct/internal/session"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// checkBackupOnly warns when the session was resolved through the backup
// mirror because the live JSONL is gone. DiscoverFilesWithBackups lets live
// paths win, so a FilePath under BackupProjectsDir means live is (or was)
// missing — but the index can lag behind a just-completed restore, so we
// re-stat the canonical source path before nudging. Manifest load errors
// collapse to "not in backup" — a parse failure should never block the user
// from trying `claude --resume` directly. Any stat error other than
// ErrNotExist also falls through: resume isn't the place to surface
// permission or FS issues.
func checkBackupOnly(match *session.Session) error {
	if !strings.HasPrefix(match.FilePath, paths.BackupProjectsDir()) {
		return nil
	}
	manifest, _ := backup.LoadManifest(paths.BackupManifestPath())
	entry, ok := manifest.Entries[match.ID]
	if !ok {
		return nil
	}
	if _, err := os.Stat(entry.SourcePath); !os.IsNotExist(err) {
		return nil
	}
	fmt.Fprintf(os.Stderr,
		"Session %s is preserved in your cct backup but missing from\n"+
			"~/.claude/projects/. Restore it first:\n\n"+
			"    cct backup restore %s\n",
		match.ShortID, match.ID)
	return &ExitError{Code: 1}
}

type ResumeCmd struct {
	ID     string `arg:"" help:"Session ID or prefix"`
	DryRun bool   `help:"Print command instead of executing" name:"dry-run"`
}

func (cmd *ResumeCmd) Run(globals *Globals) error {
	match, err := session.FindByPrefixFull(cmd.ID)
	if err != nil {
		return err
	}

	if err := checkBackupOnly(match); err != nil {
		return err
	}

	if cmd.DryRun {
		if match.ProjectPath != "" {
			fmt.Printf("cd %s && claude --resume %s\n", shellQuote(match.ProjectPath), match.ID)
		} else {
			fmt.Printf("claude --resume %s\n", match.ID)
		}
		return nil
	}

	dir := match.ProjectPath
	if dir != "" {
		if _, err := os.Stat(dir); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("cannot access project directory: %s: %w", dir, err)
			}
			if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
				return fmt.Errorf("project directory missing and could not be created: %s: %w", dir, mkErr)
			}
			fmt.Fprintf(os.Stderr, "Created missing directory: %s\n", dir)
		}
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}

	// Pre-launch context (visible in scrollback after claude exits).
	fmt.Println()
	fmt.Printf("  Resuming session %s\n", output.Dim(match.ShortID))
	if match.CustomTitle != "" {
		fmt.Printf("  Title:    %s\n", output.Bold(match.CustomTitle))
	}
	if match.ProjectName != "" {
		fmt.Printf("  Project:  %s", output.Bold(match.ProjectName))
		if match.GitBranch != "" {
			fmt.Printf("  (%s)", match.GitBranch)
		}
		fmt.Println()
	}
	if match.FirstPrompt != "" {
		fmt.Printf("  Prompt:   %s\n", output.Dim(output.Truncate(match.FirstPrompt, 60)))
	}
	fmt.Println()

	// Ignore signals that claude's TUI handles.
	signal.Ignore(syscall.SIGINT, syscall.SIGQUIT)

	c := exec.Command(claudePath, "--resume", match.ID)
	if dir != "" {
		c.Dir = dir
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	runErr := c.Run()

	signal.Reset(syscall.SIGINT, syscall.SIGQUIT)

	fmt.Println()
	fmt.Printf("  %s\n", output.Cyan(fmt.Sprintf("cct resume %s", match.ShortID)))
	fmt.Println()

	if runErr != nil {
		var execErr *exec.ExitError
		if errors.As(runErr, &execErr) {
			return &ExitError{Code: execErr.ExitCode()}
		}
		return runErr
	}
	return nil
}
