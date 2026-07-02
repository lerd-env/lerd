# CLAUDE.md — agent guide for the lerd codebase

You are a coding agent working on **lerd**: an open-source, Herd-like local PHP
development environment for Linux and macOS (Windows via WSL2, beta). It runs
Nginx, PHP-FPM, and services as rootless Podman containers, and ships a built-in
Svelte web UI, a TUI, a CLI, and an MCP server. No Docker, no sudo, no system
pollution.

This file is loaded into every session. Read it before you touch anything, then
follow it exactly. It overrides your defaults. When a rule here conflicts with a
habit, the rule wins.

---

## 1. The three design laws

Almost every mistake an agent makes here is a violation of one of these. Check
your change against all three before writing code.

1. **Store-first, never hardcode.** lerd ships only the default stack. Every
   framework lives in `lerd-frameworks/` and every service in `lerd-services/`,
   as versioned YAML. If a change adds a framework, a service, a worker, an env
   wiring, a doctor check, a custom command, or a proxy, it belongs in a store
   YAML file, **not** in Go. New workers go in the framework store YAML — never
   in hardcoded Go, and never add a merger that backfills them. Copy the closest
   existing YAML; those files are the schema of record.

2. **Framework-agnostic.** No feature may know the name "Laravel" (or Symfony,
   WordPress…) in Go. Behaviour is driven by the YAML definition. If you find
   yourself branching on a framework name in Go, the logic is in the wrong layer
   — move it into the store as declarative data.

3. **Env belongs to sites, not workers.** All environment variables live in the
   site's `.env`. Workers never declare env vars. Service presets inject their
   host/port/credentials into the site `.env` via the framework's `env.services`
   mapping.

---

## 2. Where things live

```
cmd/lerd            CLI + long-running lerd-ui/watcher entrypoints
cmd/lerd-tray       system tray (CGO, libayatana-appindicator)
internal/           all Go application code, one package per concern:
  podman/           quadlet generation, container lifecycle
  nginx/ certs/ dns/  site serving, TLS, .test resolution
  services/ serviceops/  service-preset engine + operations
  store/ registry/  fetch + cache the lerd-frameworks / lerd-services stores
  siteops/ siteinfo/ grouping/  site linking, groups, worktrees
  worker*/ idle/    workers, self-heal, idle-suspend
  sitedoctor/       framework-agnostic health checks
  mcp/              MCP server (tools AI assistants call)
  tui/              terminal dashboard
  ui/web/           Svelte web UI (built + //go:embed'd into the binary)
lerd-frameworks/    local checkout of the framework store for dev; submit to lerd-env/frameworks
lerd-services/      local checkout of the service store for dev; submit to lerd-env/services
docs/               mkdocs site published to lerd.sh; docs/contributing has the human guide
tests/installer/    bats tests for install.sh
```

The web UI is Svelte under `internal/ui/web/`, built to `dist/` and embedded via
`//go:embed`. The Go binary is self-contained; `make build` builds the UI first.

---

## 3. The contribution lifecycle — follow every step, in order

This is the full life cycle of a change. Do not skip steps. Do not reorder them.

### Step 0 — Open an issue first
Before writing code, an issue should exist for the work. Frame it as future work
(what will be done), not as already done. One issue per unit of work. Do not
create GitHub issues, comments, or PRs without explicit approval — draft the text,
show it, and wait. (See §7.)

### Step 1 — Understand before you build
Read the surrounding package and the closest existing example. Match its naming,
comment density, and idioms. Decide which layer the change belongs to using the
three laws in §1. If it's store data, you're editing YAML, not Go.

### Step 2 — Write the test first (TDD)
New functionality **must** include tests, and behaviour changes must update them.
A PR without corresponding test coverage will not merge. Write the failing test,
then make it pass. Keep tests out of `/tmp` — those get wiped; use a repo fixture.

### Step 3 — Implement (DRY, KISS)
Reuse existing patterns and helpers; do not duplicate. In the web UI, extract
shared markup into components from the start — never copy markup between views.
Pick the simplest design that satisfies the issue. Keep code comments minimal:
explain only what isn't self-evident, and keep any comment block to 2-3 lines at most.

### Step 4 — Document it
Update the relevant page under `docs/` for every feature or behaviour change,
**before** committing. If the change touches what AI assistants can do, update the
MCP skills and guidelines alongside it.

### Step 5 — Run the full local gate (see §4)
Build, test, vet, gofmt, UI tests, and installer tests must all pass locally —
not just at release time. Then install the local build and smoke-test the real
app; don't rely on tests alone for anything with a runtime surface.

### Step 6 — Commit only when asked (see §6)
Do not commit, push, or open a PR automatically. Wait for an explicit go-ahead,
and pause at phase boundaries of a staged change so a human can smoke-test in the
browser before the next phase.

### Step 7 — Open the PR the lerd way (see §7)

### Step 8 — Release is a separate, maintainer-only lifecycle (see §8)

---

## 4. The local verification gate

Run this before every commit — the exact checks CI runs, plus install + smoke.
There is a preflight skill that automates it: prefer `/lerd-preflight`.

```bash
make build-ui                         # Svelte → dist/ (only if UI changed)
CGO_ENABLED=1 go build ./cmd/lerd     # build
CGO_ENABLED=1 go test ./...           # tests
CGO_ENABLED=1 go vet ./...            # vet
test -z "$(gofmt -l .)"               # format (gofmt -w . to fix)
make test-ui                          # Vitest, if you touched the UI
bats tests/installer/installer.bats   # if you touched install.sh
```

Then install the local build and restart the UI so you can drive it for real:

```bash
make install    # → ~/.local/bin/lerd (+ lerd-tray), restarts lerd-ui/watcher
```

Never install to `/usr/local/bin`; lerd installs to `~/.local/bin/lerd`. Never run
`sudo` from a tool call — if a step needs privilege, ask the human to run it.

For UI iteration: `cd internal/ui/web && npm run dev` (Vite on :5173, proxies
`/api/*` to a running `lerd-ui` on :7073). Run `lerd start` first so the backend is up.

---

## 5. Adding to the stores (the common case)

- **A service** (database, cache, search, admin dashboard): add
  `services/<name>.yaml` in the **lerd-env/services** repo. Copy the closest
  existing preset. There is a skill: `/lerd-add-service`.
- **A framework or a new major version**: add
  `frameworks/<name>/<version>.yaml` in the **lerd-env/frameworks** repo. Copy
  the closest existing definition. Workers, env wiring, doctor checks, custom
  commands, tinker, and logs all live here. There is a skill: `/lerd-add-framework`.

The `lerd-frameworks/` and `lerd-services/` directories in this repo are local
checkouts for development; contributions are submitted to the `lerd-env/frameworks`
and `lerd-env/services` repos. A store change ships to every install within ~24h
with no binary release and no Go code. That is the point — resist the urge to
"just add it in Go."

---

## 6. Commit conventions

- **Only commit when explicitly asked.** No automatic commit/push/PR. Many small
  branches are worse than one coherent one — ask before each git step.
- **Branch off `main`**; never commit straight to `main`.
- Conventional-commit style subject (`feat:`, `fix:`, `docs:`…). Body is prose
  paragraphs that read like a human wrote them, single-line paragraphs (no column
  wrapping), no robotic bullet lists.
- **No `Co-Authored-By` trailer. No "Generated with…" footer.** Ever.
- **No em dashes** in any commit or PR text — use commas or rewrite.
- Stage files by explicit path. **Never `git add -A` or `git add .`** — the repo
  root carries long-lived untracked entries that get swept in. `git status` first.
- Never mention incidental cleanup (renames, test hygiene) in the message; keep it
  about the change. Don't write prose about tests, TDD, or coverage.

---

## 7. Pull request conventions

- **Nothing hits GitHub without per-action approval.** Creating, editing, closing,
  reopening, merging, or commenting on any issue or PR requires explicit sign-off
  each time. Draft it, show it, wait. There is a skill: `/lerd-open-pr`.
- PR body is human prose. **Do not** add: a Test plan, a Verified/Tested section,
  a checklist (`- [ ]` / `- [x]`), a "Notes for reviewers" section, or file:line
  citations. We own the project; there is no external reviewer to address.
- Issue linking: feature PRs use `Closes #N` (auto-close on merge). Bug-report
  issues use `Refs #N` and stay open until the stable release ships — **except**
  security issues, which close once the fix merges to main.
- PR/issue comment style: casual plain prose, no markdown, no bullets, no hyphens,
  commas instead. Don't open replies with boilerplate ("Pulled it down and…").
- After pushing, return — don't sit waiting on CI; failures get flagged.

---

## 8. Release lifecycle (maintainer-only, do not self-initiate)

Only touch these when explicitly cutting a release:

- Update **both** `CHANGELOG.md` (a symlink to `docs/changelog.md`) and `README.md`
  every release. The changelog is a **release-time artifact** — never add
  `[Unreleased]` or stray "Changed" entries mid-build.
- A normal version adds a **new** section above the previous one; never overwrite
  an existing section. Only beta headings get replaced in place. On release,
  rename `[Unreleased]` to the version heading and leave no empty placeholder.
- Update the MCP skills and guidelines to match the release.
- Create and push the git **tag only after** the release PR merges into `main`.

---

## 9. Scope guards

- **Dashboard clutter is a hard line.** Don't add empty cards; hide empty widgets
  or fold them into a related card. Prefer icons over letter-initial badges.
- **TUI scope is informative + reversible quick actions only.** Destructive
  commands (migrate, remove, reinstall, anything `--purge`/`--reset-data`) stay
  CLI-only.
- **Container shell stays isolated.** Ship lerd-controlled zsh + starship; never
  bind-mount host shell config into containers.
- **Don't flip a default** without user demand behind it. No complaints means the
  default works.
- Treat "can we do X?" as a question ("is X needed?"), not an instruction to build
  X. Answer first.
```
