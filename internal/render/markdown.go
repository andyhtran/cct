package render

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/andyhtran/cct/internal/session"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type Options struct {
	MaxChars           int
	MaxToolChars       int
	IncludeToolResults bool
	Limit              int
}

func RenderSession(s *session.Session, opts Options) error {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	f, err := os.Open(s.FilePath)
	if err != nil {
		return fmt.Errorf("cannot open session file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header := renderHeader(s)
	rendered, err := renderer.Render(header)
	if err != nil {
		return err
	}
	fmt.Print(rendered)

	messages := parseMessages(f, opts)

	if opts.Limit > 0 && len(messages) > opts.Limit {
		messages = messages[len(messages)-opts.Limit:]
	}

	userStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true)
	assistantStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("5")).
		Bold(true)
	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	for _, msg := range messages {
		var header string
		if msg.role == "user" {
			header = userStyle.Render("▌ User")
		} else {
			header = assistantStyle.Render("▌ Assistant")
		}
		fmt.Println(header)
		fmt.Println()

		rendered, err := renderer.Render(msg.text)
		if err != nil {
			fmt.Print(msg.text)
		} else {
			fmt.Print(rendered)
		}
		fmt.Println(separatorStyle.Render(strings.Repeat("─", 80)))
		fmt.Println()
	}

	return nil
}

func renderHeader(s *session.Session) string {
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
	b.WriteString("\n---\n")
	return b.String()
}

type message struct {
	role string
	text string
}

func parseMessages(r *os.File, opts Options) []message {
	scanner := session.NewJSONLScanner(r)
	var messages []message

	roles := map[string]bool{"user": true, "assistant": true}

	for scanner.Scan() {
		line := scanner.Bytes()
		lineType := session.FastExtractType(line)

		if !roles[lineType] {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}

		text := extractContent(obj, opts.IncludeToolResults, opts.MaxToolChars)
		if text == "" {
			continue
		}

		if opts.MaxChars > 0 && len(text) > opts.MaxChars {
			text = text[:opts.MaxChars] + fmt.Sprintf("\n\n... (%d chars truncated)", len(text)-opts.MaxChars)
		}

		messages = append(messages, message{role: lineType, text: text})
	}

	return messages
}

func extractContent(obj map[string]any, includeToolResults bool, maxToolChars int) string {
	msg, ok := obj["message"].(map[string]any)
	if !ok {
		return ""
	}
	content := msg["content"]
	if content == nil {
		return ""
	}
	if str, ok := content.(string); ok {
		return str
	}
	arr, ok := content.([]any)
	if !ok {
		return ""
	}

	var parts []string
	for _, item := range arr {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)

		if blockType == "thinking" || blockType == "redacted_thinking" {
			continue
		}

		isToolBlock := blockType == "tool_result" || blockType == "tool_use"

		if isToolBlock && !includeToolResults {
			continue
		}

		var text string
		if blockType == "tool_use" {
			text = formatToolUse(block)
		} else if blockType == "tool_result" {
			text = session.ExtractTextFromContent(item)
			if maxToolChars > 0 && len(text) > maxToolChars {
				text = text[:maxToolChars] + fmt.Sprintf("\n... (%d chars truncated)", len(text)-maxToolChars)
			}
		} else if t, ok := block["text"].(string); ok {
			text = t
		}

		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func formatToolUse(block map[string]any) string {
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]any)

	var desc string
	if d, ok := input["description"].(string); ok && d != "" {
		desc = d
	} else if cmd, ok := input["command"].(string); ok && cmd != "" {
		desc = cmd
	} else if path, ok := input["file_path"].(string); ok && path != "" {
		desc = path
	}

	if desc != "" {
		return fmt.Sprintf("**%s**: %s", name, desc)
	}
	return fmt.Sprintf("**%s**", name)
}
