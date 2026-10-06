# Submitting a Pull Request

## Before opening a PR

### Formatting

All Go code must be formatted with `gofmt`. Check and fix before committing:

```bash
gofmt -l .   # list files with issues
gofmt -w .   # fix them
```

The CI will reject PRs with unformatted code.

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
| UI screenshots | PR description has an image when UI files change |
