package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

type ExportCmd struct {
	ID                 string `arg:"" help:"Session ID or prefix"`
	Full               bool   `help:"Include full message text (no truncation)"`
	Output             string `short:"o" help:"Output file (default: stdout)"`
	Role               string `short:"r" help:"Filter by role (comma-separated: user,assistant)" default:"user,assistant"`
	Limit              int    `short:"n" help:"Last N messages (0=all)" default:"0"`
	MaxChars           int    `help:"Truncate message text to N chars (0=no limit)" default:"500" name:"max-chars"`
	IncludeToolResults bool   `help:"Include tool result content" name:"include-tool-results"`
	Search             string `short:"s" help:"Filter messages containing this text (case-insensitive)"`
}

func (cmd *ExportCmd) Run(globals *Globals) error {
	match, err := session.FindByPrefixFull(cmd.ID)
	if err != nil {
		return err
	}

	maxChars := cmd.MaxChars
	if cmd.Full {
		maxChars = 0
	}

	roles := parseRoles(cmd.Role)

	if globals.JSON {
		return cmd.exportJSON(match, roles, maxChars, cmd.Search)
	}

	md, err := renderMarkdown(match, roles, maxChars, cmd.Limit, cmd.IncludeToolResults, cmd.Search)
	if err != nil {
		return err
	}

	if cmd.Output != "" {
		return os.WriteFile(cmd.Output, []byte(md), 0o600)
	}
	fmt.Print(md)
	return nil
}

func parseRoles(roleStr string) map[string]bool {
	roles := make(map[string]bool)
	for _, r := range strings.Split(roleStr, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			roles[r] = true
		}
	}
	return roles
}

func renderMarkdown(s *session.Session, roles map[string]bool, maxChars, limit int, includeToolResults bool, searchFilter string) (string, error) {
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

	messages := collectMessages(f, roles, includeToolResults, searchFilter)

	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	for _, msg := range messages {
		text := msg.text
		if maxChars > 0 && len(text) > maxChars {
			text = output.Truncate(text, maxChars)
		}

		if msg.role == "user" {
			b.WriteString("## User\n\n")
		} else {
			b.WriteString("## Assistant\n\n")
		}
		b.WriteString(text)
		b.WriteString("\n\n---\n\n")
	}

	return b.String(), nil
}

type exportMessage struct {
	role      string
	text      string
	timestamp time.Time
}

func collectMessages(r io.Reader, roles map[string]bool, includeToolResults bool, searchFilter string) []exportMessage {
	scanner := session.NewJSONLScanner(r)
	var messages []exportMessage
	searchLower := strings.ToLower(searchFilter)

	for scanner.Scan() {
		line := scanner.Bytes()
		lineType := session.FastExtractType(line)

		if lineType != "user" && lineType != "assistant" {
			continue
		}
		if !roles[lineType] {
			continue
		}

		var obj map[string]any
		if json.Unmarshal(line, &obj) != nil {
			continue
		}

		var text string
		if includeToolResults {
			text = session.ExtractPromptText(obj)
		} else {
			text = extractTextNoToolResults(obj)
		}
		if text == "" {
			continue
		}

		if searchFilter != "" && !strings.Contains(strings.ToLower(text), searchLower) {
			continue
		}

		ts := session.ParseTimestamp(obj)

		messages = append(messages, exportMessage{
			role:      lineType,
			text:      text,
			timestamp: ts,
		})
	}

	return messages
}

// extractTextNoToolResults extracts message text but skips tool_result blocks.
func extractTextNoToolResults(obj map[string]any) string {
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
		// Skip tool_result AND the types already skipped by ExtractTextFromContent
		if blockType == "tool_result" || blockType == "tool_use" || blockType == "thinking" || blockType == "redacted_thinking" || blockType == "image" || blockType == "document" {
			continue
		}
		if text, ok := block["text"].(string); ok && text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

type exportJSONOutput struct {
	Session  *session.Session    `json:"session"`
	Messages []exportJSONMessage `json:"messages"`
}

type exportJSONMessage struct {
	Role      string `json:"role"`
	Timestamp string `json:"timestamp,omitempty"`
	Text      string `json:"text"`
}

func (cmd *ExportCmd) exportJSON(s *session.Session, roles map[string]bool, maxChars int, searchFilter string) error {
	f, err := os.Open(s.FilePath)
	if err != nil {
		return fmt.Errorf("cannot open session file: %w", err)
	}
	defer func() { _ = f.Close() }()

	messages := collectMessages(f, roles, cmd.IncludeToolResults, searchFilter)

	if cmd.Limit > 0 && len(messages) > cmd.Limit {
		messages = messages[len(messages)-cmd.Limit:]
	}

	var jsonMessages []exportJSONMessage
	for _, msg := range messages {
		text := msg.text
		if maxChars > 0 && len(text) > maxChars {
			text = output.Truncate(text, maxChars)
		}
		jm := exportJSONMessage{
			Role: msg.role,
			Text: text,
		}
		if !msg.timestamp.IsZero() {
			jm.Timestamp = msg.timestamp.Format(time.RFC3339)
		}
		jsonMessages = append(jsonMessages, jm)
	}

	out := exportJSONOutput{
		Session:  s,
		Messages: jsonMessages,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if cmd.Output != "" {
		outFile, err := os.Create(cmd.Output)
		if err != nil {
			return err
		}
		defer func() { _ = outFile.Close() }()
		enc = json.NewEncoder(outFile)
		enc.SetIndent("", "  ")
	}

	return enc.Encode(out)
}
