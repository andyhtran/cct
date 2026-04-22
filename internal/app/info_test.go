package app

import "testing"

func TestFormatInt(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1_000, "1,000"},
		{12_345, "12,345"},
		{200_000, "200,000"},
		{1_234_567, "1,234,567"},
		{-42, "-42"},
		{-1_234, "-1,234"},
	}
	for _, tt := range tests {
		if got := formatInt(tt.in); got != tt.want {
			t.Errorf("formatInt(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
