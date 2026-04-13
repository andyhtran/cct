# Claude Code — cct

CLI tool for browsing, searching, and managing Claude Code session history.

## Development

- **Build**: `just build`
- **Test**: `just test`
- **Full CI gate locally**: `just ci` (format check + lint + test)
- **Format**: `just fmt` — uses `gofumpt`, not `gofmt`. CI enforces this and will fail on `gofmt`-only formatted code.

## Code style

- Formatter is **gofumpt** (`just fmt` before committing). The CI lint step runs `gofumpt -l .` and fails if any file differs.
- Linter is **golangci-lint v2** (`just lint`).

## PR and merge flow

1. Push feature branch, open PR against `main`
2. CI runs three jobs: `lint`, `test (ubuntu-latest)`, `test (macos-latest)` — all must pass
3. Squash merge into `main`

## Release flow

Releases are fully automated by goreleaser. Pushing a version tag triggers the release workflow, which builds binaries for linux/darwin (amd64/arm64), creates a GitHub Release, and updates the Homebrew formula in `andyhtran/homebrew-tap` automatically.

1. Update `CHANGELOG.md` with the new version and date
2. Commit the changelog update to `main`
3. Tag: `git tag vX.Y.Z && git push origin vX.Y.Z`
4. The `Release` GitHub Action handles everything else — no manual formula or binary work needed

After release, users get it via `brew upgrade cct`.