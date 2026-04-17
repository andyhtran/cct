package changelog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/andyhtran/cct/internal/paths"
)

// ChangelogURL is the raw CHANGELOG.md on the claude-code main branch.
// Using `raw.githubusercontent.com` avoids HTML parsing and gets the exact
// file contents Claude Code ships.
const ChangelogURL = "https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md"

// DefaultTTL is how long a cached changelog is considered fresh before an
// auto-refresh triggers. Long enough to avoid hammering GitHub on every cct
// run, short enough that day-to-day "what's new" lookups stay accurate.
const DefaultTTL = 6 * time.Hour

// httpTimeout bounds the initial connect + full-body read. The file is ~230KB
// so a generous ceiling covers slow networks without hanging the CLI.
const httpTimeout = 15 * time.Second

// Meta is the sidecar persisted alongside the cached changelog body.
// Stored as JSON at paths.ChangelogMetaPath().
type Meta struct {
	SourceURL string    `json:"source_url"`
	ETag      string    `json:"etag,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// ReadMeta loads the sidecar. Missing file returns a zero Meta with nil error
// so callers can treat "no cache yet" as a normal first-run state.
func ReadMeta() (Meta, error) {
	var m Meta
	data, err := os.ReadFile(paths.ChangelogMetaPath())
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse changelog meta: %w", err)
	}
	return m, nil
}

func writeMeta(m Meta) error {
	if err := os.MkdirAll(filepath.Dir(paths.ChangelogMetaPath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(paths.ChangelogMetaPath(), data)
}

// writeAtomic writes via a temp file in the same directory, then renames.
// Prevents a half-written cache if the process is killed mid-download.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// FetchResult describes the outcome of a Fetch call. NotModified is true when
// the server returned 304 in response to our If-None-Match; body is unchanged.
type FetchResult struct {
	NotModified bool
	Meta        Meta
	// Bytes is the new body only when NotModified is false.
	Bytes []byte
}

// Fetch downloads CHANGELOG.md from GitHub and writes body + meta to the cct
// cache. If a prior ETag exists, it's sent via If-None-Match so GitHub can
// return 304 without transferring the body. The cache file is only rewritten
// on a 200 response.
func Fetch() (FetchResult, error) {
	prev, _ := ReadMeta()

	req, err := http.NewRequest(http.MethodGet, ChangelogURL, nil)
	if err != nil {
		return FetchResult{}, err
	}
	if prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch changelog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotModified:
		// Refresh FetchedAt so TTL checks treat the cache as fresh even when
		// upstream hasn't changed.
		prev.FetchedAt = time.Now().UTC()
		if err := writeMeta(prev); err != nil {
			return FetchResult{}, err
		}
		return FetchResult{NotModified: true, Meta: prev}, nil

	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return FetchResult{}, fmt.Errorf("read changelog body: %w", err)
		}
		if err := writeAtomic(paths.ChangelogCachePath(), body); err != nil {
			return FetchResult{}, err
		}
		m := Meta{
			SourceURL: ChangelogURL,
			ETag:      resp.Header.Get("ETag"),
			FetchedAt: time.Now().UTC(),
		}
		if err := writeMeta(m); err != nil {
			return FetchResult{}, err
		}
		return FetchResult{Meta: m, Bytes: body}, nil

	default:
		return FetchResult{}, fmt.Errorf("fetch changelog: unexpected status %s", resp.Status)
	}
}

// EnsureFresh fetches only if the local cache is missing or older than ttl.
// Returns whether a network call happened, and the meta describing the cache
// after the call. Never returns stale data silently — a fetch failure on a
// missing cache is surfaced to the caller, while a fetch failure on an
// existing cache is swallowed (we keep serving the stale copy).
func EnsureFresh(ttl time.Duration) (fetched bool, meta Meta, err error) {
	meta, _ = ReadMeta()

	_, statErr := os.Stat(paths.ChangelogCachePath())
	haveBody := statErr == nil

	// Treat the body as fresh when EITHER the TTL hasn't expired OR the meta
	// sidecar is absent. The no-meta case covers two real scenarios: a
	// hand-seeded cache (tests, pre-populated fixtures) and the transitional
	// state between versions of cct that didn't write meta.
	if haveBody && (meta.FetchedAt.IsZero() || time.Since(meta.FetchedAt) < ttl) {
		return false, meta, nil
	}

	res, fetchErr := Fetch()
	if fetchErr != nil {
		if haveBody {
			// Keep serving stale cache — report nothing fetched, no error.
			return false, meta, nil
		}
		return false, meta, fetchErr
	}
	return true, res.Meta, nil
}
