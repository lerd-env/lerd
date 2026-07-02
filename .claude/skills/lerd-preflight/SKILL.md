---
name: lerd-preflight
description: Run the full local verification gate for lerd before committing — build, test, vet, gofmt, UI tests, installer tests, then install the local build and smoke-test. Use before every commit or PR, and whenever asked to "verify", "check CI locally", or "make sure it's green".
---

# lerd preflight

Run the exact checks CI runs, locally, plus install + smoke. Stop at the first
failure, report it plainly with the output, and fix before proceeding. Do not
report "green" unless every step below actually passed.

## 1. Format (fastest signal)

```bash
gofmt -l .            # must print nothing
```

If it lists files, run `gofmt -w .`, then re-check.

## 2. Build the UI only if it changed

If anything under `internal/ui/web/` changed:

```bash
make build-ui
make test-ui          # Vitest
```

Skip both if you only touched Go — the embedded `dist/` is reused.

## 3. Go build, test, vet

```bash
CGO_ENABLED=1 go build ./cmd/lerd
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go vet ./...
```

On macOS or a host without libayatana-appindicator, use the nogui path instead:
`CGO_ENABLED=0 go build -tags nogui ./cmd/lerd` (and `go test -tags nogui ./...`).

## 4. Installer tests, if install.sh changed

```bash
bats tests/installer/installer.bats
```

## 5. Install and smoke-test

```bash
make install          # → ~/.local/bin/lerd, restarts lerd-ui/watcher
```

Then actually drive the affected surface — a CLI command, a UI page, an MCP call —
and observe the behaviour. A change with a runtime surface is not verified by
tests alone. For UI-only work, `make install` restarts `lerd-ui`; reload the
dashboard and exercise the changed view.

## Rules

- Never `sudo`. If a step needs privilege, ask the human to run it.
- Never install anywhere but `~/.local/bin/lerd`.
- If a step is skipped (e.g. UI tests because the UI was untouched), say so
  explicitly rather than implying full coverage.
