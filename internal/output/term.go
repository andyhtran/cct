// Package output provides terminal formatting, ANSI styling, and table
// rendering for CLI output.
package output

import (
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiCyan  = "\x1b[36m"
)

var colorEnabled = initColor()

func initColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func TerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 100
	}
	return w
}

// Pad left-aligns s to width visible characters, then applies color.
// This avoids the problem where fmt %-Xs counts ANSI bytes as width.
func Pad(s string, width int, color func(string) string) string {
	runes := []rune(s)
	visible := len(runes)
	if visible < width {
		s += strings.Repeat(" ", width-visible)
	}
	return color(s)
}

func Bold(s string) string {
	if !colorEnabled {
		return s
	}
	return ansiBold + s + ansiReset
}

func Dim(s string) string {
	if !colorEnabled {
		return s
	}
	return ansiDim + s + ansiReset
}

func Cyan(s string) string {
	if !colorEnabled {
		return s
	}
	return ansiCyan + s + ansiReset
}
