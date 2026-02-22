package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

type ExportCmd struct {
	ID     string `arg:"" help:"Session ID or prefix"`
	Full   bool   `help:"Include full assistant responses"`
	Output string `short:"o" help:"Output file (default: stdout)"`
}

func (cmd *ExportCmd) Run(globals *Globals) error {
	match, err := session.FindByPrefixFull(cmd.ID)
	if err != nil {
		return err
	}

	md, err := renderMarkdown(match, cmd.Full)
	if err != nil {
		return err
	}

	if cmd.Output != "" {
		return os.WriteFile(cmd.Output, []byte(md), 0o600)
	}
	fmt.Print(md)
	return nil
}

func renderMarkdown(s *session.Session, full bool) (string, error) {
	f, err := os.Open(s.FilePath)
	if err != nil {
		return "", fmt.Errorf("cannot open session file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var b strings.Builder

	fmt.Fprintf(&b, "# Session %s\n\n", s.ShortID)
	if s.ProjectPath != "" {
		fmt.Fprintf(&b, "- **Project**: %s\n", s.ProjectPath)
	}
	if s.GitBranch != "" {
		fmt.Fprintf(&b, "- **Branch**: %s\n", s.GitBranch)
	}
	if !s.Created.IsZero() {
		fmt.Fprintf(&b, "- **Created**: %s\n", s.Created.Local().Format("2006-01-02 15:04:05"))
	}
	fmt.Fprintf(&b, "- **Messages**: %d\n", s.MessageCount)
	b.WriteString("\n---\n\n")

	scanner := session.NewJSONLScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		lineType := session.FastExtractType(line)

		if lineType != "user" && lineType != "assistant" {
			continue
		}

		var obj map[string]any
		if json.Unmarshal(line, &obj) != nil {
			continue
		}

		text := session.ExtractPromptText(obj)
		if text == "" {
			continue
		}

		if lineType == "user" {
			b.WriteString("## User\n\n")
			b.WriteString(text)
			b.WriteString("\n\n---\n\n")
		} else {
			b.WriteString("## Assistant\n\n")
			if full {
				b.WriteString(text)
			} else {
				b.WriteString(output.Truncate(text, 200))
			}
			b.WriteString("\n\n---\n\n")
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading session: %w", err)
	}

	return b.String(), nil
}
