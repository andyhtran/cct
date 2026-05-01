# cct — Search and browse your Claude Code sessions

Claude Code sessions are ephemeral. When you need context from yesterday's debugging session or last week's architecture decision, there's no easy way to find it. `cct` makes your session history searchable — for you and for Claude itself.

![cct view and export](docs/demo.png)

> Requires [Claude Code](https://docs.anthropic.com/en/docs/claude-code). macOS and Linux.

## Install

```bash
brew install andyhtran/tap/cct
```

To update:

```bash
brew update && brew upgrade cct
```

<details>
<summary>Build from source</summary>

```bash
git clone https://github.com/andyhtran/cct.git
cd cct && go build -o cct ./cmd/cct
```

</details>

## Finding past sessions

Search across all your conversations:

```bash
cct search "database migration"       # Find sessions mentioning a topic
cct search "auth bug" -p backend      # Filter to a specific project
```

List recent sessions:

```bash
cct                     # Quick view: 5 most recent
cct list -p myproject   # Filter by project name
cct list -a             # Show all sessions
```

## Getting full context

View a session in your terminal:

```bash
cct view <id>           # Interactive TUI viewer
```

Export to markdown:

```bash
cct export <id>           # Truncated output
cct export <id> --full    # Complete conversation
cct export <id> --render  # Syntax-highlighted terminal output
```

> **Why not `claude --resume`?** There are known issues where resumed sessions don't load full context ([#15837](https://github.com/anthropics/claude-code/issues/15837), [#22107](https://github.com/anthropics/claude-code/issues/22107)). Use `cct view` or `cct export` when you need the complete conversation.

## Resuming work

```bash
cct resume <id>         # cd to project dir and run claude --resume
```

## Use with Claude Code agents

`cct skill install` ships a Claude Code skill that the harness auto-loads when you reference past sessions. The skill is embedded in the cct binary and updates with each `brew upgrade cct` — no manual file copying.

```bash
cct skill install     # creates a symlink at ~/.claude/skills/cct
cct skill status      # check install/sync/nudge state
cct skill uninstall   # remove the symlink (live copy preserved)
cct skill nudge off   # silence the install prompt
```

Then prompt naturally:

```
use cct to find sessions where we debugged the auth issue
```

The skill describes canonical workflows (search→export, list→info, JSON+jq pipelines) and explicit anti-patterns so agents prefer cct over `grep ~/.claude/projects/`. Until installed, cct prints a one-line install hint to stderr (rate-limited to once per 24h); run `cct skill nudge off` to silence.

## Preserving session history

Claude Code occasionally wipes session files in `~/.claude/projects/` — see upstream issues [#41458](https://github.com/anthropics/claude-code/issues/41458), [#23710](https://github.com/anthropics/claude-code/issues/23710), and [#20992](https://github.com/anthropics/claude-code/issues/20992). `cct backup` hardlinks your `~/.claude/projects/**/*.jsonl` files into `~/.cache/cct/backup/` so session history survives those cleanups. Hardlinks mean the backup costs near-zero disk (the live file and the backup point at the same inode), and drift is detected if the live file is replaced.

```bash
cct backup sweep     # Hardlink every session file into the backup tree
cct backup status    # Per-session drift report (backed-up / drifted / orphaned)
cct backup restore <id>  # Copy a backup back to ~/.claude/projects/
```

`cct` never modifies `~/.claude/settings.json`. Backup is a manual command — automate it with cron or launchd if you want hands-off.

Backups are per-machine — they track local inodes and absolute paths. Don't sync `~/.cache/cct/` across machines.

## Looking up Claude Code features

`cct changelog` mirrors the upstream [CHANGELOG.md](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md) locally (cached for 6h) and makes it searchable:

```bash
cct changelog                              # Latest release
cct changelog --search "disable|opt.?out"  # Grep for ways to turn things off
cct changelog --search "plan.*name"        # Find mentions of a specific feature
cct changelog --since 2.1.100 --all        # Every change in a version window
cct changelog --refresh                    # Force re-fetch from GitHub
```

Useful when you want to answer "did Claude Code add a flag for X?" or "what changed between versions?" without leaving the terminal.

## Other commands

```bash
cct info <id>    # Session metadata: project, branch, timestamps
cct stats        # Usage statistics across all projects
```

Run `cct --help` for additional commands.

## JSON output

All commands support `--json` for scripting:

```bash
cct search "bug" --json | jq -r '.[].short_id'
```

## How it works

`cct` reads session data from `~/.claude/projects/` (JSONL files). All operations are read-only.

> The Claude Code data format is undocumented and may change between versions.

## License

MIT
