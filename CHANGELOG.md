# Changelog

All notable changes to this project will be documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
