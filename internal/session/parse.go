package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	scanInitBuf = 64 * 1024       // 64 KB initial scanner buffer
	scanMaxBuf  = 4 * 1024 * 1024 // 4 MB max line size (sessions can have large tool outputs)
)

var (
	typePrefix      = []byte(`"type":"`)
	timestampPrefix = []byte(`"timestamp":"`)
	// typeUser and typeAssistant are searched first because these values only
	// appear at the top level. The generic typePrefix search can match nested
	// types like "type":"message" in the message object, which appears before
	// the top-level type in assistant messages.
	typeUser      = []byte(`"type":"user"`)
	typeAssistant = []byte(`"type":"assistant"`)
)

func NewJSONLScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, scanInitBuf), scanMaxBuf)
	return scanner
}

func FastExtractType(line []byte) string {
	// Check for top-level message types first - these values are unique to the
	// top level and avoid confusion with nested types like "type":"message".
	if bytes.Contains(line, typeUser) {
		return "user"
	}
	if bytes.Contains(line, typeAssistant) {
		return "assistant"
	}
	// Fall back to generic extraction for other types
	idx := bytes.Index(line, typePrefix)
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(typePrefix):]
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return string(rest[:end])
}

func FastExtractTimestamp(line []byte) time.Time {
	idx := bytes.Index(line, timestampPrefix)
	if idx < 0 {
		return time.Time{}
	}
	rest := line[idx+len(timestampPrefix):]
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, string(rest[:end]))
	return t
}

func ExtractPromptText(obj map[string]any) string {
	msg, ok := obj["message"].(map[string]any)
	if !ok {
		return ""
	}
	return ExtractTextFromContent(msg["content"])
}

// skipTypes lists content block types that never contain searchable text.
var skipTypes = map[string]bool{
	"thinking":          true,
	"redacted_thinking": true,
	"image":             true,
	"document":          true,
	"tool_use":          true,
}

const maxExtractDepth = 10

// ExtractTextFromContent recursively extracts searchable text from message content.
// Content can be a plain string, an array of content blocks, or a single block object.
// Blocks may nest content via a "content" field (e.g. tool_result blocks).
func ExtractTextFromContent(content any) string {
	return extractText(content, 0)
}

func extractText(content any, depth int) string {
	if depth > maxExtractDepth || content == nil {
		return ""
	}
	if str, ok := content.(string); ok {
		if isBase64Like(str) {
			return ""
		}
		return str
	}
	if obj, ok := content.(map[string]any); ok {
		return extractBlockText(obj, depth)
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
		if s := extractBlockText(block, depth); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func extractBlockText(block map[string]any, depth int) string {
	blockType, _ := block["type"].(string)
	if skipTypes[blockType] {
		return ""
	}
	var parts []string
	if text, ok := block["text"].(string); ok && text != "" && !isBase64Like(text) {
		parts = append(parts, text)
	}
	if c, exists := block["content"]; exists {
		if s := extractText(c, depth+1); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func isBase64Like(s string) bool {
	return len(s) > 1000 && !strings.Contains(s[:1000], " ")
}

func ParseTimestamp(obj map[string]any) time.Time {
	ts, ok := obj["timestamp"].(string)
	if !ok {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, ts)
	return t
}

// extractUserMetadata populates session fields from a parsed user message.
// Returns true when all essential metadata (ProjectPath and FirstPrompt) is populated.
func extractUserMetadata(s *Session, obj map[string]any) bool {
	if s.ProjectPath == "" {
		s.ProjectPath, _ = obj["cwd"].(string)
		s.ProjectName = filepath.Base(s.ProjectPath)
		s.GitBranch, _ = obj["gitBranch"].(string)
		if sid, ok := obj["sessionId"].(string); ok && sid != "" {
			s.ID = sid
			s.ShortID = ShortID(sid)
		}
	}
	if s.FirstPrompt == "" {
		s.FirstPrompt = ExtractPromptText(obj)
	}
	if ts := ParseTimestamp(obj); !ts.IsZero() && s.Created.IsZero() {
		s.Created = ts
	}
	return s.ProjectPath != "" && s.FirstPrompt != ""
}

func ExtractMetadata(path string) *Session  { return parseSession(path, false) }
func ParseFullSession(path string) *Session { return parseSession(path, true) }

// parseSession is the shared implementation for metadata extraction and full parsing.
// When full is true, it counts all messages and reads the entire file.
// When full is false, it returns early once project path and first prompt are found.
func parseSession(path string, full bool) *Session {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil
	}

	s := &Session{
		ID:       ExtractIDFromFilename(path),
		FilePath: path,
		Modified: info.ModTime(),
	}
	s.ShortID = ShortID(s.ID)

	scanner := NewJSONLScanner(f)

	for scanner.Scan() {
		line := scanner.Bytes()
		lineType := FastExtractType(line)

		switch lineType {
		case "user":
			if full {
				s.MessageCount++
			}
			var obj map[string]any
			if json.Unmarshal(line, &obj) != nil {
				continue
			}
			complete := extractUserMetadata(s, obj)
			if !full && complete {
				return s
			}

		case "assistant":
			if full {
				s.MessageCount++
			}
			if ts := FastExtractTimestamp(line); !ts.IsZero() && s.Created.IsZero() {
				s.Created = ts
			}

		case "file-history-snapshot":
			if s.Created.IsZero() {
				var obj map[string]any
				if json.Unmarshal(line, &obj) == nil {
					if snap, ok := obj["snapshot"].(map[string]any); ok {
						if ts := ParseTimestamp(snap); !ts.IsZero() {
							s.Created = ts
						}
					}
				}
			}
		}
	}
	// scanner.Err() intentionally not checked — partial metadata is still useful.

	return s
}
