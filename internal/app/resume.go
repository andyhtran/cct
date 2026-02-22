package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
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
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("project directory no longer exists: %s\nUse --dry-run to see the command", dir)
		}
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}

	// Pre-launch context (visible in scrollback after claude exits).
	fmt.Println()
	fmt.Printf("  Resuming session %s\n", output.Dim(match.ShortID))
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
