---
name: cct
description: Search and recall Claude Code session history via the cct CLI. Use ONLY when the user asks about previous sessions — what was discussed, what was done in a project, a decision/plan from an earlier conversation, or session statistics. Covers cct search, cct export, cct info, cct list, cct stats, cct backup, cct changelog. Do not trigger proactively — wait for the user to reference past sessions.
---

# cct

`cct` is the canonical way to query the user's local Claude Code session history. Sessions live as JSONL files under `~/.claude/projects/`; cct indexes them into a SQLite FTS5 database and adds backup, export, and a TUI on top.

## The core loop

**Recall content from a past discussion:** search → export.
**Inspect recent activity:** list → info.

- `cct search <query>` — full-text search across session content (most-used)
- `cct export <id>` — export full session as markdown or JSON
- `cct info <id>` — metadata + first prompt for one session
- `cct list` — recent sessions, newest first

Session IDs accept a short prefix (first 8 chars of the UUID) — no need to type the full one.

## Quickstart

```
cct search "login bug" -p myproject             # find a past discussion in one project
cct export <short-id> > out.md                  # dump the conversation as markdown
cct list -p MyProject --limit 20                # recent activity in a project
cct info <short-id>                             # first prompt + metadata
```

`-p` is short for `--project`. `--no-agents` excludes sub-agent sessions on both `list` and `search`. `-n` is short for `--limit`.

## Common workflows

The order below reflects real usage frequency.

**1. Search by topic, then export the winner.**
This is the dominant pattern. The user remembers a technical concept and wants the conversation back.

```
cct search "auth flow" -p myproject --json | jq -r '.[].short_id'
cct export <short-id> > recall.md
```

Use `--project` (`-p`) to scope. FTS5 phrase queries: quote multi-word phrases. See `references/search-syntax.md` for operators (OR, NOT, hyphen handling).

**2. Recent activity in a project, filtered by date.**
cct doesn't have a date flag — filter via jq on the `modified` field.

```
cct list -p myproject --limit 40 --no-agents --json \
  | jq -r '.[] | select(.modified[:10] == "2025-04-21") | .short_id'
```

**3. Inspect a single session.**
`cct info <id>` for metadata + first prompt. `cct export <id>` for the full conversation. There is **no `cct show`**.

**4. Recover a deleted session.**
Claude Code occasionally cleans up old sessions. `cct backup status` shows what's archived locally; `cct backup restore <id>` brings it back.

**5. Look up Claude Code release notes.**
`cct changelog` (alias `cct log`) fetches upstream CHANGELOG.md, cached 6h. `cct changelog --search "disable|opt.?out"` greps across entries.

## Programmatic inspection (JSON + jq)

`--json` is the dominant inspection mode for agents. Stable schemas — pipe to jq.

**Top projects from stats.**
```
cct stats --json | jq '.top_projects[] | {name, sessions}'
```
Schema: `total_sessions`, `unique_projects`, `sessions_this_week`, `sessions_this_month`, `top_projects[].{name,sessions}`, `recent_projects[].{name,last_used}`, `agent_types[].{type,count}`.

**Pluck short IDs from a search.**
```
cct search "rate limiter" -p myproject --json | jq -r '.[].short_id'
```

**Filter list by date and select fields.**
```
cct list -p myproject --limit 50 --no-agents --json \
  | jq -r '.[] | "\(.short_id) \(.modified[:10]) \(.first_prompt[:80])"'
```

Common fields on list/search results: `id`, `short_id`, `project_name`, `project_path`, `created`, `modified`, `first_prompt`, `git_branch`, `message_count`, `is_agent`. Search adds `matches[].{role,snippet,source}` and `score`.

## When to use cct vs. ad-hoc Bash

**Always prefer cct over manual filesystem operations on `~/.claude/projects/`.**

- Don't `grep -r ~/.claude/projects/` — use `cct search`. Faster (FTS5) and JSONL-aware.
- Don't `ls -lt ~/.claude/projects/` — use `cct list`. Filenames are UUIDs; cct surfaces project + first prompt + modified time.
- Don't `cat *.jsonl | jq ...` for ad-hoc inspection — use `cct info` or `cct export`.
- **There is no `cct show`.** Use `cct info <id>` for metadata, `cct export <id>` for full content. (Common mistake.)

If `cct search` genuinely misses something, falling back to grep is fine — but try cct first.

## Indexing

cct keeps a SQLite FTS5 index at `~/.cache/cct/index.db`. It auto-syncs on every search/list. Rarely needed: `cct index rebuild` (wipe + re-index), `cct index status` (counts).

## Troubleshooting

- **Empty search when content clearly exists** → `cct index sync` forces a fresh scan.
- **Session not found by ID** → `cct backup status`; `cct backup restore <id>` to recover.
- **Stats JSON field name wrong** → schema is in the JSON section above; don't guess (`top_projects`, not `topProjects`).

## Full reference

- `references/commands.md` — every command, every flag, every alias, JSON schemas
- `references/search-syntax.md` — FTS5 quoting, operators, special characters
