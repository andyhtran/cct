package output

import (
	"fmt"
	"testing"
)

func ExampleNewTable() {
	tbl := NewTable("",
		Fixed("ID", 10),
		Fixed("AGE", 6),
		Flex("PROMPT", 0, 20),
	)
	fmt.Println(tbl.ColWidth(0)) // fixed column width
	fmt.Println(tbl.ColWidth(1)) // fixed column width
	// Output:
	// 10
	// 6
}

func TestNewTable_fixed_columns_get_exact_width(t *testing.T) {
	origColor := colorEnabled
	colorEnabled = false
	defer func() { colorEnabled = origColor }()

	tbl := NewTable("",
		Fixed("SESSION", 10),
		Fixed("AGE", 6),
		Flex("PROMPT", 0, 20),
	)

	if tbl.ColWidth(0) != 10 {
		t.Errorf("fixed col 0 width = %d, want 10", tbl.ColWidth(0))
	}
	if tbl.ColWidth(1) != 6 {
		t.Errorf("fixed col 1 width = %d, want 6", tbl.ColWidth(1))
	}
	// Last column gets all remaining space, at least 20.
	if tbl.LastColWidth() < 20 {
		t.Errorf("last col width = %d, want >= 20", tbl.LastColWidth())
	}
}

func TestNewTable_flex_columns_respect_minimum_width(t *testing.T) {
	origColor := colorEnabled
	colorEnabled = false
	defer func() { colorEnabled = origColor }()

	tbl := NewTable("",
		Flex("NAME", 50, 10),
		Flex("TITLE", 0, 20),
	)

	if tbl.ColWidth(0) < 10 {
		t.Errorf("flex col 0 width = %d, want >= min 10", tbl.ColWidth(0))
	}
	if tbl.LastColWidth() < 20 {
		t.Errorf("last col width = %d, want >= min 20", tbl.LastColWidth())
	}
}

func TestNewTable_widths_sum_to_terminal_width(t *testing.T) {
	origColor := colorEnabled
	colorEnabled = false
	defer func() { colorEnabled = origColor }()

	cols := []ColDef{
		Fixed("A", 8),
		Fixed("B", 8),
		Flex("C", 0, 10),
	}
	tbl := NewTable("", cols...)

	tw := TerminalWidth()
	n := len(cols)
	overhead := n * 2 // indent + inter-column gaps

	total := 0
	for i := range cols {
		total += tbl.ColWidth(i)
	}
	// total + overhead should equal terminal width (last col absorbs remainder).
	if total+overhead != tw {
		t.Errorf("column widths sum to %d + %d overhead = %d, want terminal width %d",
			total, overhead, total+overhead, tw)
	}
}
