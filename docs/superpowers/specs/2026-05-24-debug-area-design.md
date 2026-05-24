# Debug & Troubleshooting Area — Design

**Status:** Draft  ·  **Date:** 2026-05-24  ·  **Owner:** Gabriel Sousa

## Goal

Give users running `lerd` (Oracle Edition) a single discoverable place to land when something breaks. Two delivery channels:

1. **Markdown in the repo** — versioned with the code, browsable on GitHub, indexable by Google. The README links into it.
2. **A "Debug" page on the dashboard** — same content, but with one-click diagnostic actions that exec real commands against the live install and inline the output.

Both channels source the same content. The markdown is canonical; the dashboard renders it (or links out) and layers actions on top.

## Non-goals

- Not a generic Linux/Podman tutorial. We assume basic familiarity and focus on what's specific to `lerd`'s container/network/quadlet layout.
- Not an exhaustive reference manual. Each topic lists the 3–5 most common issues we've actually seen, not every conceivable failure mode.
- Not English-localised in this iteration. The fork's existing README and CLI strings are pt-BR; the debug area matches that.

## File layout

```
docs/
├── DEBUG.md                  # top-level index, linked from README
└── debug/
    ├── podman.md             # rootless setup, networks, quadlets, restart cascades
    ├── nginx.md              # 502 to FPM, SSL termination, vhost regen
    ├── dns.md                # .localhost vs .test, NSS resolver, IPv6
    ├── php-fpm.md            # image hash mismatch, ext load, php.ini, xdebug
    ├── oracle.md             # ORA-* codes, Instant Client paths, NLS_LANG, gcompat shim
    ├── sites.md              # link/unlink, sites.yaml drift, framework detection
    ├── services.md           # mysql/postgres/redis quadlets, host-port conflicts
    ├── workers.md            # queue/horizon/schedule/reverb (systemd user units)
    └── updates.md            # `lerd update`, -oracle.N versioning, rollback
```

`DEBUG.md` is the entry point. Its top half is a "sintoma → arquivo" lookup table; the bottom half is a one-paragraph teaser per sub-topic with a "veja em detalhe →" link.

Each sub-file has exactly two sections:

1. **Como funciona** — one or two paragraphs of concept, with an ASCII diagram when it clarifies the data flow (e.g. nginx → lerd-php84-fpm → /opt/oracle/instantclient).
2. **Problemas comuns** — repeated blocks of:
   - **🔴 Sintoma:** one-line description ("`nginx 502 Bad Gateway`")
   - **🔍 Diagnóstico:** exact commands to confirm the cause
   - **🟢 Conserto:** exact commands to fix

## README integration

Add a new top-level section in `README.md`:

```markdown
## Debug e troubleshooting

| Sintoma                               | Onde olhar                                |
|---------------------------------------|-------------------------------------------|
| Site retorna 502 Bad Gateway          | [`docs/debug/nginx.md`](docs/debug/nginx.md) + [`docs/debug/php-fpm.md`](docs/debug/php-fpm.md) |
| `.test` ou `.localhost` não resolve   | [`docs/debug/dns.md`](docs/debug/dns.md)  |
| Quadlet falha ao subir                | [`docs/debug/podman.md`](docs/debug/podman.md) |
| `ORA-12541 / ORA-01017 / ORA-12154`   | [`docs/debug/oracle.md`](docs/debug/oracle.md) |
| `lerd update` quebrou                 | [`docs/debug/updates.md`](docs/debug/updates.md) |

**Guia completo:** [`docs/DEBUG.md`](docs/DEBUG.md)
```

5–6 most frequent sintomas in the table, full index in the linked file.

## Dashboard surface

New sidebar entry under the System tab: `Debug` (icon: wrench). Routes at `/#system/debug`.

`DebugDetail.svelte` layout:

- **Quick-action row** at the top: `Executar lerd doctor`, `Checar DNS`, `Listar containers`, `Mostrar últimos logs`. Each runs the corresponding command via a new `POST /api/debug/{action}` endpoint and streams stdout inline. Output is monospace, scrollable, with a "copiar" button.
- **Topic grid** below: 3×3 card grid, one card per sub-file (podman, nginx, dns, php-fpm, oracle, sites, services, workers, updates). Each card shows topic icon, title, one-line teaser, and a "ver guia →" link. Clicking opens the same content rendered inline via `marked` (or links out to the GitHub-rendered version — see Rendering decisions below).
- **Footer**: `Copiar relatório` button — gathers `lerd about`, `lerd doctor`, `podman ps -a`, last 100 lines from `journalctl --user -u 'lerd-*'`, and the contents of `~/.config/lerd/config.yaml` (sensitive keys redacted). Output to clipboard so the user can paste into a GitHub issue. Mirrors `lerd bug-report` but lives in the dashboard.

### Rendering decisions

**Markdown source of truth lives at `docs/debug/*.md`** in the repo. Dashboard does NOT bundle these into the binary via `go:embed` (would bloat the binary and version-skew). Instead the dashboard fetches them at runtime from `https://raw.githubusercontent.com/gabriel-sousa99/lerd/main/docs/debug/<topic>.md`, with the user's currently-installed lerd version's git ref pinned in the URL when possible.

Trade-off: needs internet for in-app rendering. Acceptable because (a) `lerd doctor` and friends still work offline, (b) the GitHub copy is always available at the linked URL even without the dashboard, (c) the dashboard's "copiar relatório" works offline and is the more important offline feature.

### Endpoints

| Verb | Path                          | Returns                                              |
|------|-------------------------------|------------------------------------------------------|
| POST | `/api/debug/doctor`           | `{ok, output}` — stdout of `lerd doctor`             |
| POST | `/api/debug/dns-check`        | `{ok, output}` — stdout of `lerd dns:check`          |
| POST | `/api/debug/containers`       | `{ok, output}` — stdout of `podman ps -a --format='{{.Names}}\t{{.Status}}\t{{.Image}}'` |
| POST | `/api/debug/recent-logs`      | `{ok, output}` — last 100 lines of `journalctl --user -u 'lerd-*' --no-pager` |
| POST | `/api/debug/bug-report`       | `{ok, output}` — same payload as `lerd bug-report`, returned inline for clipboard copy |

All endpoints are read-only with respect to system state. No endpoint takes parameters (avoids command injection surface).

### What this is NOT

The dashboard does NOT execute arbitrary podman/systemctl commands the user types in. That's a separate, future feature. This page is curated: a small set of well-understood diagnostic actions.

## Content tone & shape

- **Pt-BR throughout.** Matches the existing README, install.sh comments, and dashboard strings in this fork.
- **Comando exato primeiro, explicação depois.** The reader is in fire-mode — they need the fix, then the why.
- **Sem placeholders genéricos.** When showing a fix, use the actual real path (e.g. `~/.config/containers/systemd/lerd-php85-fpm.container`), not `<path>/quadlet.container`.
- **Symbol legend** at the top of `DEBUG.md`: 🔴 sintoma · 🔍 diagnóstico · 🟢 conserto · ⚠️ atenção · 💡 dica.

## Architecture / module split

| Concern                          | Lives in                                                      |
|----------------------------------|---------------------------------------------------------------|
| Markdown content                 | `docs/DEBUG.md` + `docs/debug/*.md`                           |
| README link                      | `README.md` (new "Debug e troubleshooting" section)           |
| Dashboard HTTP endpoints         | `internal/ui/debug_api.go` (new file)                         |
| Dashboard route registration     | `internal/ui/server.go` (5 new `mux.HandleFunc` lines)        |
| Dashboard sidebar entry          | `internal/ui/web/src/tabs/SystemTab.svelte` (new ListRow)     |
| Dashboard route dispatch         | `internal/ui/web/src/tabs/SystemDetail.svelte` (new branch)   |
| Dashboard detail page            | `internal/ui/web/src/tabs/system/DebugDetail.svelte` (new)    |
| Markdown fetch + render          | `marked` — added to `internal/ui/web/package.json` (not currently a dep)        |

**Module boundaries**: keep `debug_api.go` independent of the rest of `ui/`. Each handler shells out to the corresponding `lerd` subcommand via `exec.Command(os.Args[0], "doctor")` style — no internal API coupling. This way the dashboard endpoints stay thin and the CLI commands remain the single source of truth for what these diagnostics do.

## Testing

- **Markdown**: no automated tests. Visual review on GitHub render + dashboard render.
- **Endpoints**: smoke test each via `curl` after `lerd install`. No unit tests for these — they're thin shellouts and the underlying CLI commands have their own test coverage.
- **Sidebar link**: manual click-through on `http://lerd.localhost/#system/debug` confirms the page loads, runs `lerd doctor`, and renders at least one topic file inline.

## Out of scope (explicit non-features)

- Localisation. Pt-BR only for this iteration.
- Inline editing of the markdown from the dashboard.
- Authentication / RBAC on `/api/debug/*` — same loopback-only access model as the rest of the dashboard.
- Embedding the markdown in the binary (would couple content shipping to binary releases — not worth it).
- A "fix it for me" auto-remediation button. Each problem gets the exact command, but executing it stays a user action.

## Implementation order

1. Write `docs/DEBUG.md` index + 9 sub-files. Commit and push so the GitHub URLs resolve before the dashboard tries to fetch them.
2. Add README "Debug e troubleshooting" section linking to the new files.
3. Wire `internal/ui/debug_api.go` + register routes.
4. Add `DebugDetail.svelte` + sidebar `ListRow` + `SystemDetail` branch.
5. Build, restart `lerd-ui`, screenshot-validate.
6. Cut a new fork release (`v1.21.2-oracle.4`) bundling this work.

## Open questions

None at this point. User has signed off on:

- Format: markdown in repo + dashboard mirror.
- Scope: both "how X works" + "common problems" per topic.
- Topic list: 9 sub-files as enumerated above.
- Language: pt-BR.
