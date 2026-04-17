package app

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/andyhtran/cct/internal/changelog"
)

type ChangelogCmd struct {
	Version string `arg:"" optional:"" help:"Show specific version (e.g. 2.1.49)"`
	All     bool   `help:"Show full changelog"`
	Recent  int    `help:"Show last N versions" default:"0"`
	Since   string `help:"Only include versions >= this one (e.g. 2.1.80)"`
	Until   string `help:"Only include versions <= this one (e.g. 2.1.112)"`
	Search  string `help:"Case-insensitive regex to grep across all entries; prints matching lines with their version"`
	Refresh bool   `help:"Force-refresh the cached changelog from GitHub before rendering"`
}

// searchHit is the JSON shape for --search --json output.
type searchHit struct {
	Version string `json:"version"`
	Line    string `json:"line"`
}

func (cmd *ChangelogCmd) Run(globals *Globals) error {
	// --refresh forces a network fetch regardless of TTL. On success we print a
	// single-line note to stderr so scripted users can distinguish cache hits
	// from fresh pulls without parsing stdout.
	if cmd.Refresh {
		res, err := changelog.Fetch()
		if err != nil {
			return fmt.Errorf("refresh changelog: %w", err)
		}
		if res.NotModified {
			fmt.Fprintln(os.Stderr, "changelog: up to date (304 Not Modified)")
		} else {
			fmt.Fprintln(os.Stderr, "changelog: refreshed from GitHub")
		}
	} else {
		// Non-forced path: ensure cache is present and reasonably fresh. Errors
		// are only surfaced if we end up with no cache at all — otherwise we
		// fall through and serve whatever we have.
		if _, _, err := changelog.EnsureFresh(changelog.DefaultTTL); err != nil {
			return fmt.Errorf("prepare changelog cache: %w", err)
		}
	}

	entries, err := changelog.ParseChangelog()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("  No changelog entries found.")
		return nil
	}

	// Exact-version lookup is a short-circuit: it ignores all other filters
	// since asking for "2.1.50" should return 2.1.50 unconditionally.
	if cmd.Version != "" {
		for _, e := range entries {
			if e.Version == cmd.Version {
				return renderEntries(globals, []changelog.VersionEntry{e})
			}
		}
		return fmt.Errorf("version %s not found in changelog", cmd.Version)
	}

	filtered, err := applyRange(entries, cmd.Since, cmd.Until)
	if err != nil {
		return err
	}

	if cmd.Search != "" {
		return runSearch(globals, filtered, cmd.Search)
	}

	var selected []changelog.VersionEntry
	switch {
	case cmd.All:
		selected = filtered
	case cmd.Recent > 0:
		n := cmd.Recent
		if n > len(filtered) {
			n = len(filtered)
		}
		selected = filtered[:n]
	default:
		if len(filtered) > 0 {
			selected = filtered[:1]
		}
	}

	return renderEntries(globals, selected)
}

func renderEntries(globals *Globals, selected []changelog.VersionEntry) error {
	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(selected)
	}

	for i, e := range selected {
		fmt.Printf("  ## %s\n\n", e.Version)
		for _, line := range strings.Split(e.Content, "\n") {
			if line == "" {
				fmt.Println()
			} else {
				fmt.Printf("  %s\n", line)
			}
		}
		if i < len(selected)-1 {
			fmt.Println()
		}
	}
	return nil
}

// runSearch compiles pattern as a case-insensitive regex and prints every line
// across `entries` that matches, tagged with its version. Version-only matches
// (e.g. searching for "2.1.111") aren't promoted to full-entry output — the
// caller can use `cct changelog <version>` for that.
func runSearch(globals *Globals, entries []changelog.VersionEntry, pattern string) error {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Errorf("invalid search regex: %w", err)
	}

	var hits []searchHit
	for _, e := range entries {
		for _, line := range strings.Split(e.Content, "\n") {
			if line == "" {
				continue
			}
			if re.MatchString(line) {
				hits = append(hits, searchHit{Version: e.Version, Line: strings.TrimSpace(line)})
			}
		}
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(hits)
	}

	if len(hits) == 0 {
		fmt.Printf("  No matches for %q\n", pattern)
		return nil
	}
	for _, h := range hits {
		fmt.Printf("  %s  %s\n", h.Version, h.Line)
	}
	return nil
}

// applyRange filters `entries` (newest-first) to the inclusive [since, until]
// window. Either bound may be empty to leave that side unbounded. Versions
// that fail to parse are dropped from the window check — they sort as
// "outside any bound", which is the safest default.
func applyRange(entries []changelog.VersionEntry, since, until string) ([]changelog.VersionEntry, error) {
	if since == "" && until == "" {
		return entries, nil
	}
	lo, err := parseVersion(since)
	if err != nil {
		return nil, fmt.Errorf("--since: %w", err)
	}
	hi, err := parseVersion(until)
	if err != nil {
		return nil, fmt.Errorf("--until: %w", err)
	}

	var out []changelog.VersionEntry
	for _, e := range entries {
		v, err := parseVersion(e.Version)
		if err != nil {
			continue
		}
		if since != "" && compareVersion(v, lo) < 0 {
			continue
		}
		if until != "" && compareVersion(v, hi) > 0 {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// parseVersion accepts "x.y.z" (any number of dot-separated numeric segments)
// and returns the numeric parts. Empty input returns a nil slice with no error
// so callers can pass empty strings as "no bound set".
func parseVersion(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("not a version %q", s)
		}
		out[i] = n
	}
	return out, nil
}

func compareVersion(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var ai, bi int
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}
