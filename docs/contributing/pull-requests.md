# Submitting a Pull Request

## Before opening a PR

### Formatting

All Go code must be formatted with `gofmt`. Check and fix before committing:

```bash
gofmt -l .   # list files with issues
gofmt -w .   # fix them
```

The CI will reject PRs with unformatted code.

### Comments

Comments explain why, not what, and a comment block stays within three lines. A pre-commit hook checks every block a commit touches in Go, TypeScript, JavaScript and Svelte files; turn it on once per clone:

```bash
git config core.hooksPath .githooks
```

A block that has outgrown three lines usually says what the code already says. When one truly needs the room, `git commit --no-verify` skips the check.

### Tests

Run the full test suite:

```bash
make test
```

**New functionality must include tests.** PRs that add or change behaviour without corresponding test coverage will not be merged.

### Vet

```bash
CGO_ENABLED=1 go vet ./...
```

### Duplication

`make dupes` runs jscpd over `internal/` and `cmd/` and opens an HTML report of copy-pasted blocks, like PhpStorm's Duplicated Code inspection. It is not a CI gate; run it before extracting a helper to find every copy, and after, to check none are left. Tests are left out because table-driven setup repeats on purpose.

### Screenshots for UI changes

If your PR changes anything you can see, in the web UI, the TUI or the tray, add screenshots to the PR description. Show the affected view before and after the change, and include both light and dark themes when the change touches colours or layout. A short screen recording works better than screenshots for animations or multi-step flows.

A CI check fails when a PR touches `internal/ui/web/src/`, `internal/tui/` or `cmd/lerd-tray/` and its description has no image or video. It reruns when you edit the description. A refactor that changes no visible behaviour can carry the `no-ui-change` label to skip it.

## CI checks

Every PR runs the following checks automatically. All must pass before merging.

| Check | Command |
|-------|---------|
| Build | `go build ./cmd/lerd` |
| Tests | `go test ./...` |
| Race detector | `go test -race ./...` |
| Vet | `go vet ./...` |
| Format | `gofmt -l .` |
| Vulnerabilities | `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` |
| UI tests | `make test-ui` |
| Docs build | `npm run build` in `docs/` |
| Installer tests | `bats tests/installer/installer.bats` |
| Comment blocks | `.githooks/pre-commit origin/main` (or the `release/` branch the pull request targets), the pre-commit hook run over the whole pull request |
| UI screenshots | PR description has an image when UI files change |
| No AI attribution | No commit or PR description credits an AI tool with a `Co-Authored-By` trailer or a "Generated with" footer |

The same checks also run daily on `main`, so a Go advisory published between pushes still turns CI red. A separate daily scan audits the web UI's npm dependencies, the published PHP base images and the pinned host tools.

The Linux jobs restore the tray's `libayatana-appindicator3-dev` headers from an apt cache. That cache can hold a failed install as an empty one, so when the headers are still missing after it the job refreshes the package index and installs them itself. A failure on that step means apt is failing on the runner, not your change, and a rerun later usually clears it.
