package tui

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/andyhtran/cct/internal/session"
)

type MessageKind int

const (
	KindUser MessageKind = iota
	KindAssistant
	KindToolCall
	KindToolResult
)

type Message struct {
	Kind      MessageKind
	Text      string
	ToolName  string
	ToolInput map[string]any
	Timestamp time.Time
}

func ParseMessages(r io.Reader) []Message {
	scanner := session.NewJSONLScanner(r)
	var messages []Message

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

		ts := session.ParseTimestamp(obj)
		extracted := extractMessages(obj, lineType, ts)
		messages = append(messages, extracted...)
	}

	return messages
}

func extractMessages(obj map[string]any, role string, ts time.Time) []Message {
	msg, ok := obj["message"].(map[string]any)
	if !ok {
		return nil
	}

	content := msg["content"]
	if content == nil {
		return nil
	}

	if str, ok := content.(string); ok {
		kind := KindUser
		if role == "assistant" {
			kind = KindAssistant
		}
		return []Message{{Kind: kind, Text: str, Timestamp: ts}}
	}

	arr, ok := content.([]any)
	if !ok {
		return nil
	}

	var messages []Message
	for _, item := range arr {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}

		blockType, _ := block["type"].(string)

		switch blockType {
		case "text":
			text, _ := block["text"].(string)
			if text == "" {
				continue
			}
			kind := KindUser
			if role == "assistant" {
				kind = KindAssistant
			}
			messages = append(messages, Message{Kind: kind, Text: text, Timestamp: ts})

		case "thinking", "redacted_thinking":
			continue

		case "tool_use":
			name, _ := block["name"].(string)
			input, _ := block["input"].(map[string]any)
			desc := extractToolDescription(input)
			messages = append(messages, Message{
				Kind:      KindToolCall,
				ToolName:  name,
				ToolInput: input,
				Text:      desc,
				Timestamp: ts,
			})

		case "tool_result":
			text := extractToolResultText(block)
			if text != "" {
				messages = append(messages, Message{
					Kind:      KindToolResult,
					Text:      text,
					Timestamp: ts,
				})
			}
		}
	}

	return messages
}

func extractToolDescription(input map[string]any) string {
	if desc, ok := input["description"].(string); ok && desc != "" {
		return desc
	}
	if cmd, ok := input["command"].(string); ok && cmd != "" {
		if len(cmd) > 80 {
			return cmd[:77] + "..."
		}
		return cmd
	}
	if path, ok := input["file_path"].(string); ok && path != "" {
		return path
	}
	if pattern, ok := input["pattern"].(string); ok && pattern != "" {
		return pattern
	}
	return ""
}

func extractToolResultText(block map[string]any) string {
	content := block["content"]
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
		if b, ok := item.(map[string]any); ok {
			if text, ok := b["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}
