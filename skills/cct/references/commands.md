# cct commands — full reference

Scope: every cct subcommand, its flags, JSON schemas where applicable. SKILL.md covers the happy path; this file is the exhaustive enumeration.

**Related**: [SKILL.md](../SKILL.md), [search-syntax.md](search-syntax.md).

## Global flags

- `--json` — emit JSON to stdout (where supported). Stable schemas; safe for `jq`.
- `-v`, `--version` — show version and exit.

## search — full-text search

```
cct search <query> [-p|--project <name>] [-n|--limit <n>] [--no-agents] [--json]
```

FTS5 query over indexed session content. Default limit 25 (use `-n 0` for unlimited).

**JSON result fields:**
- `id`, `short_id` — full + 8-char UUID prefix
- `is_agent` — true for sub-agent sessions
- `project_name`, `project_path`
- `created`, `modified` (RFC3339)
- `first_prompt`
- `git_branch`
- `message_count`
- `matches[]` — array of `{role, snippet, source?}` objects (snippets contain the matched terms)
- `score` — FTS5 ranking; higher is better

See [search-syntax.md](search-syntax.md) for query operators and special characters.

## export — export messages

```
cct export <session-id> [--format markdown|json] [--filter <expr>]
```

Default format markdown. Accepts short ID prefix (≥8 chars). `--filter` supports message-level expressions (user/assistant/tool_use).

## info — session metadata

```
cct info <session-id> [--json]
```

Prints first prompt, project, git branch, message count, created/modified timestamps.

## list — recent sessions

```
cct list [-p|--project <name>] [-n|--limit <n>] [-a|--all] [--agents|--no-agents] [--json]
```

Newest first by modified time. Default limit 15. `cct list` (no args) shows the 5 most recent.
Sub-agent sessions are excluded by default; `--agents` includes them, `--no-agents` is the explicit form (and works as kong's negation of `--agents`).

**JSON result fields:** same as search minus `matches` and `score`.

## stats — session statistics

```
cct stats [--json]
```

**JSON schema:**
```json
{
  "total_sessions": 1000,
  "unique_projects": 50,
  "sessions_this_week": 100,
  "sessions_this_month": 400,
  "top_projects": [{"name": "<project>", "sessions": 200}],
  "recent_projects": [{"name": "<project>", "last_used": "<rfc3339>"}],
  "agent_types": [{"type": "<agent-type>", "count": 10}]
}
```

Field names are `top_projects` (not `topProjects`), `unique_projects`, `total_sessions` — exact snake_case.

## resume — resume a session

```
cct resume <session-id>
```

Auto-cd to the project directory and resume. Fails if the project dir was moved or deleted. Rarely needed for agent workflows; primarily a human convenience.

## view — interactive TUI

```
cct view
```

Bubbletea TUI. Arrow keys to navigate, `/` to search, `q` to quit. Human-only; not useful for agents.

## changelog — Claude Code release notes

```
cct changelog [<version>] [--since <version>] [--all] [--search <regex>] [--refresh]
```

Alias: `cct log`. Fetches upstream CHANGELOG.md (cached 6h at `~/.cache/cct/changelog.md`).

## index — manage the search index

```
cct index sync       # incremental: re-index modified-since-last-sync sessions
cct index rebuild    # wipe + re-index from scratch
cct index status     # session count, last sync, db size
```

Index lives at `~/.cache/cct/index.db`. Lockfile at `~/.cache/cct/index.db.lock`.

## backup — guard against upstream cleanup

```
cct backup            # default: sweep
cct backup sweep [--no-agents] [--include-active] [--quiet]
cct backup status     # per-session drift report
cct backup restore <session-id>... [--dry-run] [--force]
```

Hard-links session JSONL files to `~/.cache/cct/backup/projects/`. Run `sweep` periodically. `restore` reverse-links a session back into `~/.claude/projects/`.

## skill — manage the cct Claude Code skill

```
cct skill install     # create symlink at ~/.claude/skills/cct
cct skill uninstall   # remove symlink (live copy preserved)
cct skill status      # install state, symlink target, sync state, nudge state
cct skill nudge on|off|status
```

Live copy at `~/.cache/cct/skills/cct/`. Auto-syncs from the embedded version on every cct invocation.

## plans — saved plans (rarely used)

```
cct plans [list|search|show|cp] ...
```

Reads from `~/.claude/plans/`. `cct plans cp <name>` copies a plan into the current dir. Low-frequency command; reach for it only if the user explicitly references "the plan from session X".

## schema — CLI structure as JSON

```
cct schema --json
```

Machine-readable manifest of commands and flags.

## version

```
cct version [--json]
```

Prints cct version and detected Claude Code version.
