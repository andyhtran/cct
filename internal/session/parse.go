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
	if bytes.Contains(line, typeUser) {
		return "user"
	}
	if bytes.Contains(line, typeAssistant) {
		return "assistant"
	}
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

func ExtractPromptBlocks(obj map[string]any) []ContentBlock {
	msg, ok := obj["message"].(map[string]any)
	if !ok {
		return nil
	}
	return ExtractContentBlocks(msg["content"])
}

// SkipTypes lists content block types that never contain searchable text.
var SkipTypes = map[string]bool{
	"thinking":          true,
	"redacted_thinking": true,
	"image":             true,
	"document":          true,
}

const maxExtractDepth = 10

// ContentBlock holds extracted text from a single content block along with its source.
// Source is empty for regular text blocks, or the tool name for tool_use blocks.
type ContentBlock struct {
	Text   string
	Source string
}

// ExtractTextFromContent recursively extracts searchable text from message content.
// Content can be a plain string, an array of content blocks, or a single block object.
// Blocks may nest content via a "content" field (e.g. tool_result blocks).
func ExtractTextFromContent(content any) string {
	return extractText(content, 0)
}

// ExtractContentBlocks returns individual searchable blocks from message content,
// preserving the source (tool name for tool_use blocks, empty for text).
func ExtractContentBlocks(content any) []ContentBlock {
	return extractBlocks(content, 0)
}

func extractBlocks(content any, depth int) []ContentBlock {
	if depth > maxExtractDepth || content == nil {
		return nil
	}
	if str, ok := content.(string); ok {
		if isBase64Like(str) {
			return nil
		}
		return []ContentBlock{{Text: str}}
	}
	if obj, ok := content.(map[string]any); ok {
		return extractBlockEntries(obj, depth)
	}
	arr, ok := content.([]any)
	if !ok {
		return nil
	}
	var blocks []ContentBlock
	for _, item := range arr {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blocks = append(blocks, extractBlockEntries(block, depth)...)
	}
	return blocks
}

func extractBlockEntries(block map[string]any, depth int) []ContentBlock {
	blockType, _ := block["type"].(string)
	if SkipTypes[blockType] {
		return nil
	}
	if blockType == "tool_use" {
		text := extractToolUseText(block)
		if text == "" {
			return nil
		}
		name, _ := block["name"].(string)
		return []ContentBlock{{Text: text, Source: name}}
	}
	var blocks []ContentBlock
	if text, ok := block["text"].(string); ok && text != "" && !isBase64Like(text) {
		blocks = append(blocks, ContentBlock{Text: text})
	}
	if c, exists := block["content"]; exists {
		blocks = append(blocks, extractBlocks(c, depth+1)...)
	}
	return blocks
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
	if SkipTypes[blockType] {
		return ""
	}
	if blockType == "tool_use" {
		return extractToolUseText(block)
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

func extractToolUseText(block map[string]any) string {
	var parts []string
	if name, ok := block["name"].(string); ok {
		parts = append(parts, name)
	}
	if input, ok := block["input"].(map[string]any); ok {
		for _, v := range input {
			if s, ok := v.(string); ok && s != "" && !isBase64Like(s) {
				parts = append(parts, s)
			}
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

// ExtractUserMetadata populates session fields from a parsed user message.
// Returns true when all essential metadata (ProjectPath and FirstPrompt) is populated.
func ExtractUserMetadata(s *Session, obj map[string]any) bool {
	if s.ProjectPath == "" {
		s.ProjectPath, _ = obj["cwd"].(string)
		s.ProjectName = filepath.Base(s.ProjectPath)
		s.GitBranch, _ = obj["gitBranch"].(string)
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

// OffsetScanner wraps a reader to track byte offsets for each line.
type OffsetScanner struct {
	reader  *bufio.Reader
	offset  int64
	lineLen int
	line    []byte
	err     error
}

// NewOffsetScanner creates a scanner that tracks byte positions.
func NewOffsetScanner(r io.Reader) *OffsetScanner {
	return &OffsetScanner{
		reader: bufio.NewReaderSize(r, scanInitBuf),
	}
}

// Scan advances to the next line, returning true if a line was read.
func (s *OffsetScanner) Scan() bool {
	s.offset += int64(s.lineLen)
	s.line, s.err = s.reader.ReadBytes('\n')
	s.lineLen = len(s.line)
	if s.err != nil && len(s.line) == 0 {
		return false
	}
	return true
}

// Bytes returns the current line (without trailing newline).
func (s *OffsetScanner) Bytes() []byte {
	if len(s.line) > 0 && s.line[len(s.line)-1] == '\n' {
		return s.line[:len(s.line)-1]
	}
	return s.line
}

// Offset returns the byte offset of the current line in the file.
func (s *OffsetScanner) Offset() int64 {
	return s.offset
}

// Length returns the byte length of the current line (including newline).
func (s *OffsetScanner) Length() int {
	return s.lineLen
}

// ReadMessageAtOffset reads a single JSONL line at the given byte offset.
func ReadMessageAtOffset(filePath string, offset int64, length int) (role, source, text string, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", "", "", err
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", "", "", err
	}

	buf := make([]byte, length)
	n, err := io.ReadFull(f, buf)
	if err != nil && n == 0 {
		return "", "", "", err
	}
	buf = buf[:n]

	var obj map[string]any
	if err := json.Unmarshal(buf, &obj); err != nil {
		return "", "", "", err
	}

	role = FastExtractType(buf)
	blocks := ExtractPromptBlocks(obj)
	if len(blocks) > 0 {
		source = blocks[0].Source
		var texts []string
		for _, b := range blocks {
			if b.Text != "" {
				texts = append(texts, b.Text)
			}
		}
		text = strings.Join(texts, " ")
	}

	return role, source, text, nil
}

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
	s.IsAgent = IsAgentSession(s.ID)

	scanner := NewJSONLScanner(f)

	// Metadata-only parse used to early-return on the first complete user
	// message. Claude Code writes "custom-title" records after user messages
	// (the title is set mid-session by /rename), so we scan to EOF to capture
	// the latest title. Parallelised by parallelMap in scan.go.
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
			ExtractUserMetadata(s, obj)

		case "assistant":
			if full {
				s.MessageCount++
			}
			if ts := FastExtractTimestamp(line); !ts.IsZero() && s.Created.IsZero() {
				s.Created = ts
			}

		case "custom-title":
			// Claude rewrites this record every turn; latest value wins.
			var obj map[string]any
			if json.Unmarshal(line, &obj) != nil {
				continue
			}
			if ct, _ := obj["customTitle"].(string); ct != "" {
				s.CustomTitle = ct
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
