package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/render"
	"github.com/andyhtran/cct/internal/session"
)

type ExportCmd struct {
	ID                 string `arg:"" help:"Session ID or prefix"`
	Full               bool   `help:"Show everything (no truncation, include tool results)"`
	Short              bool   `help:"Compact output (truncate messages to 500 chars)"`
	Render             bool   `help:"Render with syntax highlighting (styled terminal output)"`
	Output             string `short:"o" help:"Output file (default: stdout)"`
	Role               string `short:"r" help:"Filter by role (comma-separated: user,assistant)" default:"user,assistant"`
	Limit              int    `short:"n" help:"Last N messages (0=all)" default:"0"`
	MaxChars           int    `help:"Truncate conversation text to N chars (0=no limit)" default:"0" name:"max-chars"`
	MaxToolChars       int    `help:"Truncate tool result text to N chars (0=no limit)" default:"2000" name:"max-tool-chars"`
	IncludeToolResults bool   `help:"Include tool result content" name:"include-tool-results"`
	Search             string `short:"s" help:"Filter messages containing this text (case-insensitive)"`
}

func (cmd *ExportCmd) Run(globals *Globals) error {
	match, err := session.FindByPrefixFull(cmd.ID)
	if err != nil {
		return err
	}

	maxChars := cmd.MaxChars
	maxToolChars := cmd.MaxToolChars
	includeToolResults := cmd.IncludeToolResults
	if cmd.Full {
		maxChars = 0
		maxToolChars = 0
		includeToolResults = true
	}
	if cmd.Short {
		maxChars = 500
	}

	roles := parseRoles(cmd.Role)

	if globals.JSON {
		return cmd.exportJSON(match, roles, maxChars, maxToolChars, includeToolResults, cmd.Search)
	}

	if cmd.Render {
		return render.RenderSession(match, render.Options{
			MaxChars:           maxChars,
			MaxToolChars:       maxToolChars,
			IncludeToolResults: includeToolResults,
			Limit:              cmd.Limit,
		})
	}

	md, stats, err := renderMarkdown(match, roles, maxChars, maxToolChars, cmd.Limit, includeToolResults, cmd.Search)
	if err != nil {
		return err
	}

	if cmd.Output != "" {
		err = os.WriteFile(cmd.Output, []byte(md), 0o600)
		if err != nil {
			return err
		}
		printHints(stats)
		return nil
	}

	fmt.Print(md)
	printHints(stats)
	return nil
}

type exportStats struct {
	toolBlocksSkipped int
	messagesTruncated int
}

func printHints(stats exportStats) {
	if os.Getenv("CCT_NO_HINTS") != "" {
		return
	}
	if stats.toolBlocksSkipped > 0 {
		fmt.Fprintf(os.Stderr, "hint: %d tool result(s) skipped (use --include-tool-results or --full to include)\n", stats.toolBlocksSkipped)
	}
	if stats.messagesTruncated > 0 {
		fmt.Fprintf(os.Stderr, "hint: %d message(s) truncated (use --full for complete output)\n", stats.messagesTruncated)
	}
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

func renderMarkdown(s *session.Session, roles map[string]bool, maxChars, maxToolChars, limit int, includeToolResults bool, searchFilter string) (string, exportStats, error) {
	f, err := os.Open(s.FilePath)
	if err != nil {
		return "", exportStats{}, fmt.Errorf("cannot open session file: %w", err)
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

	messages, stats := collectMessages(f, roles, includeToolResults, searchFilter, maxToolChars)

	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	for _, msg := range messages {
		text := msg.text
		if maxChars > 0 && len(text) > maxChars {
			text = output.TruncateWithCount(text, maxChars)
			stats.messagesTruncated++
		}

		if msg.role == "user" {
			b.WriteString("## User\n\n")
		} else {
			b.WriteString("## Assistant\n\n")
		}
		b.WriteString(text)
		b.WriteString("\n\n---\n\n")
	}

	return b.String(), stats, nil
}

type exportMessage struct {
	role      string
	text      string
	timestamp time.Time
}

func collectMessages(r io.Reader, roles map[string]bool, includeToolResults bool, searchFilter string, maxToolChars int) ([]exportMessage, exportStats) {
	scanner := session.NewJSONLScanner(r)
	var messages []exportMessage
	var stats exportStats
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

		text, skipped := extractContent(obj, includeToolResults, maxToolChars)
		stats.toolBlocksSkipped += skipped

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

	return messages, stats
}

func extractContent(obj map[string]any, includeToolResults bool, maxToolChars int) (string, int) {
	msg, ok := obj["message"].(map[string]any)
	if !ok {
		return "", 0
	}
	content := msg["content"]
	if content == nil {
		return "", 0
	}
	if str, ok := content.(string); ok {
		return str, 0
	}
	arr, ok := content.([]any)
	if !ok {
		return "", 0
	}
	var parts []string
	skipped := 0
	for _, item := range arr {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)
		if session.SkipTypes[blockType] {
			continue
		}

		if blockType == "tool_result" && !includeToolResults {
			skipped++
			continue
		}

		var text string
		if blockType == "tool_use" {
			text = render.FormatToolUse(block)
		} else if blockType == "tool_result" {
			text = session.ExtractTextFromContent(item)
		} else if t, ok := block["text"].(string); ok {
			text = t
		}

		if text == "" {
			continue
		}

		if blockType == "tool_result" && maxToolChars > 0 && len(text) > maxToolChars {
			text = output.TruncateWithCount(text, maxToolChars)
		}

		parts = append(parts, text)
	}
	return strings.Join(parts, " "), skipped
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

func (cmd *ExportCmd) exportJSON(s *session.Session, roles map[string]bool, maxChars, maxToolChars int, includeToolResults bool, searchFilter string) error {
	f, err := os.Open(s.FilePath)
	if err != nil {
		return fmt.Errorf("cannot open session file: %w", err)
	}
	defer func() { _ = f.Close() }()

	messages, _ := collectMessages(f, roles, includeToolResults, searchFilter, maxToolChars)

	if cmd.Limit > 0 && len(messages) > cmd.Limit {
		messages = messages[len(messages)-cmd.Limit:]
	}

	var jsonMessages []exportJSONMessage
	for _, msg := range messages {
		text := msg.text
		if maxChars > 0 && len(text) > maxChars {
			text = output.TruncateWithCount(text, maxChars)
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

	var w io.Writer = os.Stdout
	if cmd.Output != "" {
		outFile, err := os.Create(cmd.Output)
		if err != nil {
			return err
		}
		defer func() { _ = outFile.Close() }()
		w = outFile
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
