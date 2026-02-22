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
)

func NewJSONLScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, scanInitBuf), scanMaxBuf)
	return scanner
}

func FastExtractType(line []byte) string {
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

// Content can be either a plain string or an array of content blocks.
func ExtractTextFromContent(content any) string {
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
		if blockType == "thinking" || blockType == "image" || blockType == "tool_use" {
			continue
		}
		if text, ok := block["text"].(string); ok && text != "" {
			if len(text) > 1000 && !strings.Contains(text[:1000], " ") {
				continue // Skip base64-like data
			}
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
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
