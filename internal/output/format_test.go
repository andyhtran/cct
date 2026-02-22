package output

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"zero duration", 0, "0m"},
		{"minutes", 30 * time.Minute, "30m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 3 * 24 * time.Hour, "3d"},
		{"weeks", 14 * 24 * time.Hour, "2w"},
		{"zero time", -1, "?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input time.Time
			if tt.age == -1 {
				input = time.Time{}
			} else {
				input = time.Now().Add(-tt.age)
			}
			got := FormatAge(input)
			if got != tt.want {
				t.Errorf("FormatAge(%v) = %q, want %q", tt.age, got, tt.want)
			}
		})
	}

	t.Run("old date falls back to month day", func(t *testing.T) {
		old := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		got := FormatAge(old)
		if got != "Jun 15" {
			t.Errorf("FormatAge(2025-06-15) = %q, want %q", got, "Jun 15")
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"hello world this is long", 10, "hello w..."},
		{"has\nnewlines\nin it", 30, "has newlines in it"},
		{"has\rcarriage\rreturns", 30, "hascarriagereturns"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Truncate(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

func TestHighlightKeyword_splits_around_match(t *testing.T) {
	origColor := colorEnabled
	colorEnabled = true
	defer func() { colorEnabled = origColor }()

	got := HighlightKeyword("hello world", "world")
	// Prefix "hello " should be dimmed, keyword "world" should be bold.
	if !strings.Contains(got, ansiDim+"hello "+ansiReset) {
		t.Errorf("expected dimmed prefix in %q", got)
	}
	if !strings.Contains(got, ansiBold+"world"+ansiReset) {
		t.Errorf("expected bold keyword in %q", got)
	}
}

func TestHighlightKeyword_no_match_dims_entire_string(t *testing.T) {
	origColor := colorEnabled
	colorEnabled = true
	defer func() { colorEnabled = origColor }()

	got := HighlightKeyword("hello world", "zzz")
	want := ansiDim + "hello world" + ansiReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func ExampleExtractSnippet() {
	text := "The quick brown fox jumps over the lazy dog"
	snippet := ExtractSnippet(text, "fox", 30)
	fmt.Println(snippet)
	// Output:
	// ...ick brown fox jumps over...
}

func TestExtractSnippet(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog"

	tests := []struct {
		name    string
		keyword string
		width   int
		want    string
	}{
		{"centers around keyword", "fox", 30, "...ick brown fox jumps over..."},
		{"keyword not found truncates from start", "zzz", 30, "The quick brown fox jumps o..."},
		{"keyword at start", "the", 20, "The quick brown f..."},
		{"keyword at end", "dog", 30, "... the lazy dog"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSnippet(text, tt.keyword, tt.width)
			if got != tt.want {
				t.Errorf("ExtractSnippet(%q, %q, %d) = %q, want %q", text, tt.keyword, tt.width, got, tt.want)
			}
		})
	}
}
