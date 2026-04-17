# Changelog

All notable changes to this project will be documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `changelog --refresh`: force-fetch the latest upstream CHANGELOG.md from the claude-code GitHub repo
- `changelog --search <regex>`: case-insensitive grep across all entries, printing `version  matching-line` (or a `[{version, line}]` array with `--json`)
- `changelog --since <version>` / `--until <version>`: filter to an inclusive version window

### Changed

- `changelog` now caches a mirror of the upstream CHANGELOG.md at `~/.cache/cct/changelog.md` (or `$XDG_CACHE_HOME/cct/`), auto-refreshed every 6h via a conditional GET with ETag. Previously it read `~/.claude/cache/changelog.md`, which Claude Code refreshes on its own schedule and could be weeks stale.
- `changelog` help text now includes examples for the common lookup flows

## [1.2.1] - 2026-04-13

### Added

- `plans list`: now a first-class visible subcommand (default limit 15, `-p` filter, `--json`)
- `plans view`: interactive TUI for viewing plan markdown with scrollable viewport and vim-style navigation
- `plans export`: dump plan markdown to stdout, with `--render` for glamour-styled output and `-o` for file output
- `export`: show `tool_use` blocks (tool name + summary) by default, without needing `--include-tool-results`

### Changed

- `plans list` / `plans search` hint lines now show `export` and `export --render` on separate lines for easy triple-click copying

## [1.1.0] - 2026-03-15

### Changed

- `search --json`: session fields are now top-level instead of nested under a `session` key — `.id`, `.project_name`, `.created` work directly in jq
- `plans search --json`: plan fields are now top-level instead of nested under a `plan` key
- `search --help`: now lists available JSON fields and shows a jq example

## [1.0.0] - 2026-03-15

### Added

- FTS5 full-text search index for session history — searches now use SQLite's FTS5 engine for faster, more accurate full-text matching

## [0.6.0] - 2026-03-13

### Added

- `view`: interactive TUI for viewing session history with scrollable viewport, vim-style navigation (j/k/g/G/q), colored user/assistant messages, and inline tool call display
- `export --render`: styled terminal output with syntax highlighting via glamour, colored role headers, works with pipes

## [0.5.0] - 2026-03-08

### Added

- `export`: `--short` flag for compact output (truncates messages to 500 chars)
- `export`: `--max-tool-chars` flag to control tool result truncation separately
- `export`: contextual hints on stderr when tool blocks are skipped or messages are truncated, suggesting relevant flags
- `CCT_NO_HINTS` environment variable to suppress stderr hints

### Changed

- `export`: default output is now full (no truncation) — conversation text is shown complete
- `export`: truncation uses `[+N chars]` indicator so users know content was cut
- `export`: tool results (with `--include-tool-results`) are truncated independently via `--max-tool-chars` (default 2000) while conversation text stays full
- `export`: `--full` now also includes tool results (acts as "show everything")

### Fixed

- `resume`: recreate missing project directory instead of failing — allows resuming sessions for review even if the project was deleted or renamed

## [0.4.0] - 2026-03-06

### Added

- `search`: multi-term AND matching — `cct search "Read search.go"` finds sessions containing all terms
- `search`: `-C/--context` flag adds extra characters to snippet width for more surrounding context
- `search`: `-s/--session` flag scopes search to a single session
- `search`: `--no-agents` flag excludes sub-agent sessions
- `search`: tool_use blocks are now searchable — file paths, bash commands, grep patterns, and URLs from tool invocations are indexed
- `search`: matches show source labels (`[a:Read]`, `[a:Bash]`) to distinguish tool invocations from conversation text
- `search`: role labels (`[u]`/`[a]`) on all match snippets
- `list`/`stats`: `--agents` flag to include sub-agent sessions

### Fixed

- ShortID preserves full agent session IDs to avoid collisions between sub-agents sharing a parent
- Session IDs derived from filename, not from `sessionId` field which could point to parent session

## [0.3.0] - 2026-02-24

### Added

- `cct schema` command for machine-readable CLI introspection
- `search`: `-n/--limit`, `-a/--all`, `-m/--max-matches` flags for bounded output
- `plans list`: `-p/--project`, `-n/--limit`, `-a/--all` flags
- `plans search`: `-n/--limit`, `-a/--all` flags
- `export`: `--role`, `--limit`, `--search`, `--max-chars`, `--include-tool-results` flags

### Changed

- Default `--max-chars` in export increased from 200 to 500
- Error messages show short hint instead of full usage

### Fixed

- FastExtractType now correctly identifies assistant messages

## [0.2.0] - 2026-02-22

### Fixed

- Search and export now find text inside tool result content blocks (AskUserQuestion answers, file reads, command outputs, sub-agent responses)

### Changed

- Replaced flat content extraction with recursive design that follows nested `text` and `content` fields, with an explicit skip list for non-text types

## [0.1.0] - 2026-02-22

### Added

- `cct list` — browse Claude Code sessions, sorted by recency
- `cct search` — full-text search across conversation content
- `cct info` — view session metadata (project, branch, timestamps, message count)
- `cct resume` — resume a session with automatic directory switching
- `cct export` — export sessions to markdown
- `cct plans` — browse, search, and copy Claude Code plan files
- `cct changelog` — view Claude Code release notes
- `cct stats` — aggregate usage statistics across projects
- `cct version` — show cct and Claude Code versions
- `--json` flag on all commands for machine-readable output
