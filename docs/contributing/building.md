# Building from Source

## Prerequisites

The tray binary requires CGO and `libayatana-appindicator`. See [System Tray: Build requirements](../features/system-tray.md#build-requirements) for per-distro package names.

Go is required to build from source. The released binary has no runtime dependencies.

The installer suite runs under `bats` and shells out to `perl` for the two checks that need a process with no controlling terminal, since `setsid` is util-linux and macOS ships no equivalent. Both are present by default on macOS and on every distro lerd targets.

**Web UI**: the `lerd-ui` dashboard is built from Svelte sources under `internal/ui/web/` and bundled into the Go binary via `//go:embed`. Node.js (20+) and npm are required to rebuild it. `make build` runs `npm install` (once) and `npm run build` automatically before the Go build, so a single `make` command still produces a self-contained binary. If you only change Go code, you can skip the JS build by running `go build` directly against a previously-built `internal/ui/web/dist/` tree.

## Build commands

```bash
make build       # → ./build/lerd  (CGO, with tray support; builds UI first)
make build-nogui # → ./build/lerd-nogui  (no CGO, no tray)
make build-ui    # rebuild only the web UI (internal/ui/web/dist/)
make install     # build + install to ~/.local/bin/lerd
make test        # go test ./...
make test-ui     # run Vitest suite for the web UI
make test-all    # test + test-ui + test-installer (bats)
make clean       # remove ./build/ and internal/ui/web/dist/
```

Tests must isolate lerd's state before they touch it, with `t.Setenv("XDG_CONFIG_HOME", t.TempDir())` and `t.Setenv("XDG_DATA_HOME", t.TempDir())`. Anything writing or deleting a real config file, systemd unit or quadlet panics with the path it tried to touch, because a test that skipped this once removed a developer's lerd-dns quadlet and left the container running under a unit systemd no longer knew about.

On macOS the launchd units in `~/Library/LaunchAgents` follow `HOME` instead, so a test that writes or removes one needs `t.Setenv("HOME", t.TempDir())` on top of the XDG pair, guarded by `runtime.GOOS == "darwin"`. Moving `HOME` on Linux would relocate podman's container storage into the temp dir, which the test then cannot clean up. The guard names whichever var applies when it fires.

Isolating `HOME` moves the plist file but not the launchd domain it is bootstrapped into, which has no per-test equivalent. Starting, stopping or restarting a unit is therefore refused outright under test unless the test installs its own stub in `podman.UnitLifecycle`; the platform manager macOS registers at init counts as the real system, not a stub. A test that needs the lifecycle to run should assign a fake and assert against what it recorded.

## Platform-specific code

Code that behaves differently per OS lives in `_linux.go`, `_darwin.go` or `_windows.go` files behind a function the shared code calls. A file that builds for more than one OS may read `runtime.GOOS` as data, passing it to a lookup or showing it to the user, but may not compare or switch on it. A plain fact about the host, like whether containers run inside a VM or which command opens a URL, goes in `platform.Current`, which each OS sets in its own `caps_<os>.go`. `internal/platform/seams_test.go` fails on any branch like that. CI also cross-builds for macOS on the Linux job, so a per-OS function missing from the darwin side fails there first.

## Cross-compile for arm64

Without tray (no CGO required):

```bash
CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build -tags nogui -o ./build/lerd-arm64 ./cmd/lerd
```

The UI only needs to be built once per source-tree state; the emitted `dist/` is architecture-independent.

## Developing the web UI

```bash
cd internal/ui/web
npm install            # once
npm run dev            # Vite dev server at http://localhost:5173 (proxies /api/* to 7073)
npm run check          # svelte-check + tsc
npm test               # Vitest
```

The Vite dev server proxies `/api/*`, `/icons/*`, `/manifest.webmanifest`, `/sw.js`, and `/offline.html` to a running `lerd-ui` on `:7073`, so you get hot-reload on the Svelte side while the Go backend handles the data. Run `lerd start` (or `make install` once) first so the backend is up.

## Installing a local build

To test a local build end-to-end using the installer:

```bash
make build
bash install.sh --local ./build/lerd
```

This runs the full installer flow (prerequisite checks, PATH setup, `lerd install`) using your locally built binary instead of downloading from GitHub.
