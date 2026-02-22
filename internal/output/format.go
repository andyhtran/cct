package output

import (
	"fmt"
	"strings"
	"time"
)

func FormatAge(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return t.Format("Jan 2")
	}
}

func Truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen-3]) + "..."
	}
	return s
}

// HighlightKeyword returns the string with the first occurrence of keyword
// highlighted (bold) and the surrounding text dimmed. Case-insensitive match.
func HighlightKeyword(s, keyword string) string {
	if !colorEnabled || keyword == "" {
		return Dim(s)
	}
	sLower := strings.ToLower(s)
	keyLower := strings.ToLower(keyword)
	idx := strings.Index(sLower, keyLower)
	if idx < 0 {
		return Dim(s)
	}
	before := s[:idx]
	matched := s[idx : idx+len(keyword)]
	after := s[idx+len(keyword):]
	return Dim(before) + Bold(matched) + Dim(after)
}

// ExtractSnippet returns a snippet of text centered around the keyword match.
// The returned string is guaranteed to be at most width runes, including any
// "..." prefix/suffix markers.
func ExtractSnippet(text, keyLower string, width int) string {
	textLower := strings.ToLower(text)
	idx := strings.Index(textLower, keyLower)
	if idx < 0 {
		return Truncate(text, width)
	}
	runes := []rune(text)
	runeIdx := len([]rune(text[:idx]))

	textWidth := width
	hasPrefix := false
	hasSuffix := false

	start := runeIdx - width/3
	if start < 0 {
		start = 0
	} else if start > 0 {
		hasPrefix = true
		textWidth -= 3
	}

	end := start + textWidth
	if end >= len(runes) {
		end = len(runes)
	} else {
		hasSuffix = true
		end -= 3
		if end < start {
			end = start
		}
	}

	snippet := string(runes[start:end])
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	if hasPrefix {
		snippet = "..." + snippet
	}
	if hasSuffix {
		snippet += "..."
	}
	return snippet
}
