# Changelog

All notable changes to this project will be documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `skill` command group: ship a Claude Code skill bundled in the cct binary so agents auto-discover the tool. `cct skill install` creates a symlink at `~/.claude/skills/cct/` pointing at `~/.cache/cct/skills/cct/`; the live copy auto-syncs from the embedded version on every cct invocation, so `brew upgrade cct` keeps the on-disk skill aligned with the binary. Idempotent with conflict detection — refuses to overwrite a foreign file or symlink.
- `skill uninstall`: removes only the symlink, preserves the live copy for fast reinstall.
- `skill status`: reports install state, symlink target, sync state (embedded vs. live hash), and nudge state. `--json` for tooling.
- Install nudge: until the skill is installed, cct prints a one-line hint to stderr (rate-limited to once per 24h). `cct skill nudge on|off|status` controls it.
- Skill content: SKILL.md plus `references/commands.md` and `references/search-syntax.md`. Workflows ordered by real-world frequency from session history; explicit anti-triggers (`grep ~/.claude/projects/`, the non-existent `cct show`); full JSON schemas for `stats`, `list`, and `search`.

## [1.5.1] - 2026-04-23

### Fixed

- Sessions whose first user message exceeded 4 MB (typically from many pasted images) silently lost their metadata — `list`/`info`/`search` would show an empty row with no project, branch, prompt, or message count. The line scanner's 4 MB buffer cap was hit before the seed user message was read, and `scanner.Err()` was intentionally unchecked so the failure was invisible. Replaced the capped `bufio.Scanner` with an unbounded `bufio.Reader.ReadBytes`-based scanner across parse, search, export, render, and TUI; added a 512 MB per-line sanity cap and surface scan errors on stderr.

## [1.5.0] - 2026-04-23

### Added

- `backup` command group: hard-links `~/.claude/projects/**/*.jsonl` into `~/.cache/cct/backup/` so session history survives upstream Claude Code cleanup bugs (issues #41458, #23710, #20992). Uses a manifest (`manifest.json`) that tracks inode, size, and copy mode per session.
- `backup sweep`: idempotent hardlink pass with a 10-minute quiet-period guard against capturing mid-write corruption (`--include-active` to override). Falls back to atomic copy on cross-filesystem (EXDEV) setups. Default action when `cct backup` is run with no subcommand.
- `backup status`: per-session drift report classifying each session as `backed-up`, `drifted` (inode or size mismatch), `orphaned` (live file gone, backup preserved), or `not-backed-up` (live file present, no manifest entry). Prints summary counts and grouped listings; `--json` emits a structured per-session array.
- `backup restore <id> [<id>...]`: reverse-links named backup entries to their original `~/.claude/projects/` paths. Session IDs are required positional args; `--dry-run` previews without writing.
- `index sync`: adopts backup files as secondary sources — sessions deleted from the live tree but preserved in the backup remain searchable, and re-adopt the live path when restored.
- Nested subagent discovery: walks `<projectDir>/<parentID>/subagents/agent-*.jsonl` alongside the flat layout and parses the matching `.meta.json` sidecar for agent type and task description. The task description is indexed into FTS so subagents are searchable by their task title, and agent type + task surface in `info`, `list`, and `stats` output.

## [1.4.0] - 2026-04-22

### Added

- `info <id>`: shows token usage for the session — current context size, percent of the 200k window, model, peak context (when a compaction has reduced it), and lifetime output tokens. Mirrors what Claude Code's `/context` displays, so you can spot sessions near auto-compact without resuming them.
- `info --json`: includes `model`, `context_tokens`, `peak_context_tokens`, and `total_output_tokens` fields

## [1.3.0] - 2026-04-17

### Added

- Custom title support: sessions renamed via Claude Code's `/rename` now surface their title in `list`, `info`, and `resume` output
- `info <title>` / `resume <title>`: look up a session by its exact custom title (case-insensitive) before falling back to UUID prefix matching
- `info --json`: includes a `custom_title` field when the session has been renamed
- `changelog --refresh`: force-fetch the latest upstream CHANGELOG.md from the claude-code GitHub repo
- `changelog --search <regex>`: case-insensitive grep across all entries, printing `version  matching-line` (or a `[{version, line}]` array with `--json`)
- `changelog --since <version>` / `--until <version>`: filter to an inclusive version window

### Changed

- `changelog` now caches a mirror of the upstream CHANGELOG.md at `~/.cache/cct/changelog.md` (or `$XDG_CACHE_HOME/cct/`), auto-refreshed every 6h via a conditional GET with ETag. Previously it read `~/.claude/cache/changelog.md`, which Claude Code refreshes on its own schedule and could be weeks stale.
- `changelog` help text now includes examples for the common lookup flows
- Index schema bumped to v7 with a clean-rebuild upgrade model: any `user_version` mismatch drops all derived tables and repopulates from JSONL on the next `sync`, replacing the incremental migration path

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
