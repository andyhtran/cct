# 📋 cct — Claude Code Tools: list, search, resume

A fast, read-only CLI for browsing, searching, and managing your [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions from the terminal.

- Browse and search past conversations
- View session details and export to markdown
- Resume sessions with automatic directory switching
- Manage plans and view changelogs
- Aggregate usage statistics across projects

> Requires [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed. macOS and Linux (amd64/arm64).

## Install

### Option A — Homebrew (Recommended)

```bash
brew install andyhtran/tap/cct
```

### Option B — Build locally

```bash
git clone https://github.com/andyhtran/cct.git
cd cct
go build -o dist/cct ./cmd/cct
./dist/cct --help
```

## Update

```bash
brew update
brew upgrade cct
```

## Commands

### Browse sessions

List your Claude Code sessions, sorted by most recent.

```bash
cct                    # Quick view: 5 most recent sessions
cct list               # List up to 15 most recent sessions
cct list -p myproject  # Filter to sessions whose project name contains "myproject"
cct list -n 50         # Show up to 50 sessions
cct list -a            # Show all sessions (no limit)
```

### Search conversations

Full-text search across message content in your Claude Code sessions.

```bash
cct search "database migration"          # Search all sessions
cct search "auth" -p backend             # Only search sessions in projects matching "backend"
```

The `-p` flag filters by project name (case-insensitive substring match), so `-p backend` matches projects like `my-backend-api` or `backend-service`.

### Session details

Show metadata for a single session: project path, git branch, timestamps, message count, and the first prompt.

```bash
cct info <id>          # Accepts full ID or an 8-char prefix
```

### Resume a session

Open a past session in Claude Code, automatically switching to the original project directory.

```bash
cct resume <id>            # cd into the project dir and run `claude --resume <id>`
cct resume <id> --dry-run  # Print the command without running it
```

### Export session as markdown

Convert a session's conversation into a readable markdown document.

```bash
cct export <id>            # Assistant responses truncated to 200 chars
cct export <id> --full     # Full untruncated assistant responses
cct export <id> -o out.md  # Write to file instead of stdout
```

### Plans

Browse and copy Claude Code plan files stored in `~/.claude/plans/`.

```bash
cct plans                          # List all plans with titles and ages
cct plans search "auth"            # Search within plan file contents
cct plans cp my-plan               # Copy a plan to the current directory
cct plans cp my-plan --as design   # Copy with a custom filename (.md auto-appended)
```

### Changelog

View the Claude Code release changelog (read from `~/.claude/cache/changelog.md`).

```bash
cct changelog            # Show the latest version's release notes
cct changelog 2.1.49     # Show notes for a specific version
cct changelog --recent 5 # Show the last 5 versions
cct changelog --all      # Show the full changelog
```

### Statistics

View aggregate statistics about your Claude Code usage across all projects.

```bash
cct stats       # Total sessions, unique projects, weekly/monthly activity, top projects
```

### Version

```bash
cct version     # Show cct version and the installed Claude Code version
```

## Global flags

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON (supported by all commands) |
| `--version`, `-v` | Print version and exit |
| `--help` | Show help |

## How it works

`cct` reads Claude Code session data from `~/.claude/projects/` (JSONL files), plan files from `~/.claude/plans/`, and changelog from `~/.claude/cache/changelog.md`. All operations are read-only except `plans cp` which copies a file to your current directory.

> **Note:** The Claude Code JSONL data format is undocumented and may change between versions. `cct` is tested against the current format but may need updates when Claude Code changes its storage layout.

## Development

Prerequisites: Go 1.25+, [gofumpt](https://github.com/mvdan/gofumpt), [golangci-lint](https://golangci-lint.run/), [just](https://github.com/casey/just)

```bash
just build        # Build to dist/cct
just test         # Run tests
just cover        # Tests with coverage
just fmt          # Format with gofumpt
just fmt-check    # Check formatting (CI)
just lint         # Run golangci-lint
```

## License

MIT
