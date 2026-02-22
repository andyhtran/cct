package output

import (
	"fmt"
	"strings"
)

type ColDef struct {
	Header string
	Width  int // Fixed width in chars (use for AGE, SESSION, BRANCH).
	Pct    int // Percentage of flex space (use for PROJECT, NAME, TITLE).
	Min    int // Minimum width for flex columns.
}

func Fixed(header string, width int) ColDef {
	return ColDef{Header: header, Width: width}
}

func Flex(header string, pct, minWidth int) ColDef {
	return ColDef{Header: header, Pct: pct, Min: minWidth}
}

type Table struct {
	cols    []ColDef
	widths  []int
	keyword string
}

// If keyword is non-empty, the last column uses keyword highlighting.
func NewTable(keyword string, cols ...ColDef) *Table {
	tw := TerminalWidth()
	n := len(cols)
	overhead := n * 2 // indent (2) + inter-column gaps ((n-1)*2) = n*2
	available := tw - overhead

	fixedSum := 0
	for _, c := range cols {
		if c.Width > 0 {
			fixedSum += c.Width
		}
	}
	flexSpace := available - fixedSum
	if flexSpace < 0 {
		flexSpace = 0
	}

	widths := make([]int, n)
	used := 0
	for i, c := range cols {
		if i == n-1 {
			break // last column handled below
		}
		if c.Width > 0 {
			widths[i] = c.Width
		} else {
			w := flexSpace * c.Pct / 100
			if w < c.Min {
				w = c.Min
			}
			widths[i] = w
		}
		used += widths[i]
	}

	// Last column gets all remaining space, minimum 20.
	last := available - used
	if last < 20 {
		last = 20
	}
	widths[n-1] = last

	return &Table{cols: cols, widths: widths, keyword: keyword}
}

func (t *Table) ColWidth(i int) int { return t.widths[i] }

func (t *Table) LastColWidth() int { return t.widths[len(t.widths)-1] }

func (t *Table) PrintHeader() {
	n := len(t.cols)
	var h, s []string
	for i, c := range t.cols {
		if i == n-1 {
			h = append(h, Dim(c.Header))
			s = append(s, Dim(strings.Repeat("-", len(c.Header))))
		} else {
			h = append(h, Pad(c.Header, t.widths[i], Dim))
			s = append(s, Pad(strings.Repeat("-", len(c.Header)), t.widths[i], Dim))
		}
	}
	fmt.Printf("  %s\n", strings.Join(h, "  "))
	fmt.Printf("  %s\n", strings.Join(s, "  "))
}

// Row prints a data row. Non-last columns are padded; the last column uses
// keyword highlighting (if keyword is set) or the provided color function.
func (t *Table) Row(values []string, colors []func(string) string) {
	n := len(t.cols)
	var parts []string
	for i := range t.cols {
		color := Dim
		if i < len(colors) && colors[i] != nil {
			color = colors[i]
		}
		val := ""
		if i < len(values) {
			val = values[i]
		}
		if i == n-1 {
			if t.keyword != "" {
				parts = append(parts, HighlightKeyword(val, t.keyword))
			} else {
				parts = append(parts, color(val))
			}
		} else {
			parts = append(parts, Pad(val, t.widths[i], color))
		}
	}
	fmt.Printf("  %s\n", strings.Join(parts, "  "))
}

// Continuation prints a follow-up row with blank prefix columns and a value
// in the last column only.
func (t *Table) Continuation(lastValue string) {
	n := len(t.cols)
	blanks := make([]string, n)
	colors := make([]func(string) string, n)
	for i := range colors {
		colors[i] = Dim
	}
	blanks[n-1] = lastValue
	t.Row(blanks, colors)
}
