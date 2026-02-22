package changelog

import (
	"testing"
)

func TestParseChangelogContent(t *testing.T) {
	content := `# Changelog

## 2.1.50

- Added feature A
- Fixed bug B

## 2.1.49

- Added feature C
- Improved performance

## 2.1.48

- Initial release
`

	entries := parseChangelogContent(content)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	t.Run("first entry", func(t *testing.T) {
		e := entries[0]
		if e.Version != "2.1.50" {
			t.Errorf("Version = %q, want %q", e.Version, "2.1.50")
		}
		if e.Content != "- Added feature A\n- Fixed bug B" {
			t.Errorf("Content = %q", e.Content)
		}
	})

	t.Run("second entry", func(t *testing.T) {
		e := entries[1]
		if e.Version != "2.1.49" {
			t.Errorf("Version = %q, want %q", e.Version, "2.1.49")
		}
	})

	t.Run("third entry", func(t *testing.T) {
		e := entries[2]
		if e.Version != "2.1.48" {
			t.Errorf("Version = %q, want %q", e.Version, "2.1.48")
		}
		if e.Content != "- Initial release" {
			t.Errorf("Content = %q", e.Content)
		}
	})
}

func TestParseChangelogContent_Empty(t *testing.T) {
	entries := parseChangelogContent("")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseChangelogContent_NoVersions(t *testing.T) {
	entries := parseChangelogContent("# Changelog\n\nJust some text\n")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseChangelogContent_SingleVersion(t *testing.T) {
	content := "## 1.0.0\n\n- First release\n- With features\n"
	entries := parseChangelogContent(content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", entries[0].Version, "1.0.0")
	}
}
