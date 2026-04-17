//go:build darwin || linux

package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/andyhtran/cct/internal/changelog"
)

func TestApplyRange(t *testing.T) {
	entries := []changelog.VersionEntry{
		{Version: "2.1.112", Content: "- newest"},
		{Version: "2.1.111", Content: "- plan naming"},
		{Version: "2.1.80", Content: "- middle"},
		{Version: "2.1.50", Content: "- oldest"},
	}

	tests := []struct {
		name       string
		since      string
		until      string
		wantN      int
		wantFirst  string
		wantLast   string
		wantErrSub string
	}{
		{"no bounds", "", "", 4, "2.1.112", "2.1.50", ""},
		{"since only", "2.1.80", "", 3, "2.1.112", "2.1.80", ""},
		{"until only", "", "2.1.111", 3, "2.1.111", "2.1.50", ""},
		{"both", "2.1.80", "2.1.111", 2, "2.1.111", "2.1.80", ""},
		{"empty window", "2.1.200", "", 0, "", "", ""},
		{"bad since", "abc", "", 0, "", "", "--since"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyRange(entries, tt.since, tt.until)
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("want error containing %q, got %v", tt.wantErrSub, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantN {
				t.Fatalf("got %d entries, want %d", len(got), tt.wantN)
			}
			if tt.wantN > 0 {
				if got[0].Version != tt.wantFirst {
					t.Errorf("first = %q, want %q", got[0].Version, tt.wantFirst)
				}
				if got[len(got)-1].Version != tt.wantLast {
					t.Errorf("last = %q, want %q", got[len(got)-1].Version, tt.wantLast)
				}
			}
		})
	}
}

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		a, b []int
		want int
	}{
		{[]int{2, 1, 111}, []int{2, 1, 80}, 1},
		{[]int{2, 1, 80}, []int{2, 1, 111}, -1},
		{[]int{2, 1, 80}, []int{2, 1, 80}, 0},
		// Shorter version compares as zero-padded: 2.1 == 2.1.0.
		{[]int{2, 1}, []int{2, 1, 0}, 0},
		{[]int{2, 1}, []int{2, 1, 1}, -1},
	}
	for _, tt := range tests {
		got := compareVersion(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareVersion(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestChangelogCmd_Search(t *testing.T) {
	home := setupFixtures(t)
	// Overwrite the seeded fixture with content that has a searchable phrase
	// so this test is self-contained and doesn't depend on the default
	// fixture's exact wording.
	writeChangelog(t, home, `# Changelog

## 2.1.111

- Plan files are now named after your prompt
- Unrelated change

## 2.1.80

- Added a disable flag for the thing
`)

	globals := &Globals{JSON: true}
	cmd := &ChangelogCmd{Search: "disable|plan files"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var hits []map[string]any
	if err := json.Unmarshal([]byte(out), &hits); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d: %v", len(hits), hits)
	}
	versions := map[string]bool{}
	for _, h := range hits {
		v, ok := h["version"].(string)
		if !ok {
			t.Fatalf("hit missing version string: %v", h)
		}
		versions[v] = true
	}
	if !versions["2.1.111"] || !versions["2.1.80"] {
		t.Errorf("expected hits from both versions, got %v", versions)
	}
}

func TestChangelogCmd_SinceUntil(t *testing.T) {
	home := setupFixtures(t)
	writeChangelog(t, home, `# Changelog

## 2.1.112

- a

## 2.1.111

- b

## 2.1.80

- c
`)

	globals := &Globals{JSON: true}
	cmd := &ChangelogCmd{All: true, Since: "2.1.111"}

	out := captureStdout(t, func() {
		if err := cmd.Run(globals); err != nil {
			t.Fatal(err)
		}
	})

	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries (>=2.1.111), got %d", len(entries))
	}
}
