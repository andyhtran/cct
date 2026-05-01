# cct search syntax

Scope: how `cct search <query>` interprets queries, plus filters, ranking, and snippet behavior.

**Related**: [SKILL.md](../SKILL.md), [commands.md](commands.md) for full flag list.

## Backend

`cct search` runs against a SQLite FTS5 virtual table indexed over all message content under `~/.claude/projects/`. The index lives at `~/.cache/cct/index.db` and auto-syncs incrementally before each search.

## Basic queries

- **Single token**: `cct search kong` — matches any session containing `kong`.
- **Multiple tokens (implicit AND)**: `cct search kong subcommand` — matches sessions containing both tokens (any order, any distance).
- **Phrase**: `cct search "kong subcommand"` — matches the literal phrase.
- **OR**: `cct search 'kong OR cobra'` — FTS5 boolean operators (uppercase) work.
- **NOT**: `cct search 'kong NOT cobra'` — exclude sessions containing the second term.

## Filters

- `--project <name>` — restrict to one project (matches against `project_name` from session metadata).
- `--limit <n>` — cap results (default ~20). Useful with `--json | jq` pipelines.

## JSON output

`cct search <query> --json` emits an array. Per-result fields:

- `id`, `short_id` — full + 8-char UUID prefix
- `project_name`, `project_path`
- `created`, `modified` (RFC3339)
- `first_prompt`
- `git_branch`
- `message_count`
- `matches` — array of snippet strings with `<mark>...</mark>` around the matched terms
- `score` — FTS5 BM25 ranking; lower is better

Example:

```
cct search "kong subcommand" --json | jq '.[] | {short_id, project_name, score}'
```

## Ranking

Default sort is FTS5 BM25 (relevance). Use `--json` if you need to re-sort by `modified` or `created` downstream — cct doesn't currently expose a sort flag.

## What's indexed vs. not

Indexed: user messages, assistant messages, tool_use names + input text.
Not indexed: file-history-snapshot events, custom-title metadata events.

If a search misses something you remember writing, possibilities:
1. Session was added since last sync → `cct index sync`
2. Content was in a tool_use's parameters that aren't indexed → fall back to grep on the raw JSONL

## Special characters

FTS5 treats most punctuation as a token boundary. To search for an identifier with hyphens or underscores, quote the phrase:

- `cct search "claude-code"` ✓
- `cct search claude-code` ✗ (treated as `claude code` — same tokens, different precision)

## See also

- `cct index status` — last sync time, total sessions indexed
- `cct list` — when you want recency, not relevance
