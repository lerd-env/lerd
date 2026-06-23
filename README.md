# Lerd

> Open-source Herd-like local PHP development environment for Linux and macOS,
> with Windows supported via WSL2 (beta). Podman-native, rootless, with a
> built-in Web UI.

[![CI](https://github.com/geodro/lerd/actions/workflows/ci.yml/badge.svg)](https://github.com/geodro/lerd/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/geodro/lerd)](https://github.com/geodro/lerd/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL2%20(beta)-lightgrey)]()
[![Docs](https://img.shields.io/badge/docs-lerd.sh-blue)](https://lerd.sh)
[![Reddit](https://img.shields.io/badge/Reddit-r%2Flerd-ff2d20?logo=reddit)](https://reddit.com/r/lerd)
[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/ej33c5N9s)

![Lerd dashboard tour](docs/assets/screenshots/tour.gif)

Lerd runs Nginx, PHP-FPM, and your services as rootless Podman containers,
designed for PHP developers on Linux and macOS, and on Windows through WSL2 (beta).
No Docker. No sudo. No system pollution. Just `lerd link` and your project
is live at `project.test` with HTTPS.

## Built for Linux PHP developers

If you're a PHP developer on Linux and want frictionless local development — automatic `.test` domains, per-project PHP versions, one-click HTTPS, zero Docker — Lerd is built for you. Works with Laravel, Symfony, WordPress, Drupal, CakePHP, Statamic, and any custom PHP framework.

## Features

- 🌐 **Automatic `.test` domains** with one-command TLS, or [opt out of lerd-managed DNS](https://lerd.sh/features/dns) and use `*.localhost` (no dnsmasq, no system resolver tweak, no sudo for the DNS bits)
- 🐘 **Per-project PHP version** (8.1–8.5, plus a frozen 7.4 / 8.0 legacy tier for hosted-on-the-old-stack projects), switch with one click
- ⚡ **FrankenPHP runtime** per site as an alternative to shared PHP-FPM, with Laravel Octane and Symfony Runtime worker mode
- 📦 **Node.js isolation** per project (Node 22, 24), or **bun** as the JS runtime on the host and, opt-in, inside the container
- 🖥️ **Built-in Web UI** with a dashboard root, sites and services overview dashboards, live widgets, a global Cmd+K command palette, install/remove of PHP and Node versions from the System page, and fourteen dashboard languages (English, German, Spanish, French, Indonesian, Dutch, Portuguese, Turkish, Simplified Chinese, Japanese, Romanian, Italian, Polish, Vietnamese)
- ✏️ **Edit config in the browser** — per-site and global nginx, per-version `php.ini`, `.env` files, and database/service runtime tuning, each validated (`nginx -t` where it applies), with timestamped backups and one-click restore
- 🧪 **Tinker tab** - in-browser PHP REPL per site with autocomplete (project models, composer helpers, PHP built-ins), live `php -l` syntax checking, and a collapsible tree view for `dump()` output. Works on Laravel (`artisan tinker`), Symfony, and any composer-based PHP project
- 🛰️ **Debug window** that intercepts every `dump()` / `dd()` and streams it to the dashboard, TUI (`D` key), MCP, and `lerd dump tail`, scoped per site and per worktree branch, with the original response left clean unless you flip passthrough on. The same window captures SQL queries with N+1 and slow-query detection, plus outgoing mail, rendered views, dispatched events, queued jobs, and outgoing HTTP, across both Laravel and Symfony, with optional opt-in capture of queue-worker activity
- 🔥 **SPX profiler** with one-click on/off, every PHP-FPM request becomes a flame graph viewable in a same-origin Profiler view in the dashboard. No FPM restart, no code changes, and `lerd profile run` profiles a one-shot artisan or CLI command
- 💻 **Terminal dashboard** (`lerd tui`) - btop-style TUI with live status, site detail pane, inline domain and version editing, shell drop-in, log tailing, and filter/sort — the same operations surface as the web UI, for tmux and SSH workflows
- 🗄️ **One-click services**: MySQL, PostgreSQL, Redis, Meilisearch, RustFS, Mailpit, Gotenberg, Stripe Mock, Reverb, soketi, OpenSearch, RedisInsight, Beanstalkd and more. Every default service is a YAML preset you can update, migrate, rollback, or reinstall in place, including a reset-data reinstall that auto-recreates linked sites' databases and buckets. Move a project's database between same-family services with `lerd db:move`
- 🔗 **Site groups** — group related sites so a main owns a base domain and the rest occupy its subdomains, with a shared or separate database per secondary
- 🧱 **Host-proxy sites** — run a Node, Python, Go, or any non-PHP dev server on the host and have nginx serve it at a `.test` domain with HTTPS, git worktrees included
- 🌳 **First-class git worktrees** with auto-detected branch domains, per-worktree PHP/Node versions, optional per-worktree database isolation (clone from main or empty), a per-worktree LAN-share proxy, `env_overrides` templating in `.lerd.yaml` for multi-tenant apps, automatic wildcard cert SANs for `*.branch.site.test`, a built-in Vite dev server worker that runs on the host per branch, and a dashboard modal for adding and removing worktrees without touching the CLI
- ⚒️ **Worker self-heal**, failed queue, schedule, horizon, reverb, and stripe workers are surfaced everywhere (CLI, dashboard banner, TUI, MCP) and recovered with one click or `lerd worker heal`
- 💤 **Idle-suspend**, activity-driven suspension of a site's workers (queue, schedule, horizon, reverb, stripe, Vite) after a configurable idle timeout, resumed on the next request, CLI command, MCP call, or file save, with per-site pinning
- 📋 **Live logs** for PHP-FPM, Queue, Schedule, Reverb, per site
- 🔒 **Rootless & daemonless** - Podman-native, no Docker required, dual-stack IPv4 + IPv6
- 🤖 **MCP server** - let AI assistants (Claude Code, Cursor, JetBrains Junie, Codex CLI, Gemini CLI, GitHub Copilot, Google Antigravity, Windsurf) manage your environment directly
- 🧩 **Framework store** - community definitions for Laravel, Symfony, WordPress, Drupal, CakePHP, Statamic with versioned auto-detection
- ⚡ **Framework-agnostic** workers, env setup, and nginx proxy — driven by YAML definitions, not hardcoded

## AI Integration (MCP)

Lerd ships a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server. Connect it to Claude Code, Cursor, JetBrains Junie, Codex CLI, Gemini CLI, GitHub Copilot, Google Antigravity, Windsurf, or any MCP-compatible AI assistant and manage your dev environment without leaving the chat.

```bash
lerd mcp:enable-global   # register once, works in every project
```

Then just ask:

```
You: set up the project I just cloned
AI:  → site(action: "link")
     → exec(action: "composer", args: ["install"])
     → env(action: "setup")        # detects MySQL + Redis, starts them, creates DB, generates APP_KEY
     → framework(action: "setup")  # storage:link + migrate for Laravel, doctrine:migrations:migrate for Symfony
     ✓  myapp → https://myapp.test ready
```

Eleven grouped tools, each driven by an `action`: `site`, `service`, `db`, `env`, `runtime`, `worker`, `exec`, `framework`, `diag`, `logs`, and `worktree`. Scaffold projects, run migrations, manage services, toggle workers, tail and search logs, enable Xdebug, manage databases and PHP extensions, park directories, switch runtimes between PHP-FPM and FrankenPHP, and more, all from your AI assistant.

📖 [MCP documentation](https://lerd.sh/features/mcp)

## Why Lerd?

|                    | Lerd | DDEV | Lando | Laravel Herd |
|--------------------|------|------|-------|--------------|
| Podman-native      | ✅   | 🟡   | ❌    | ❌           |
| Rootless           | ✅   | ❌   | ❌    | ✅           |
| Web UI             | ✅   | ❌   | ❌    | ✅           |
| Terminal dashboard | ✅   | ❌   | ❌    | ❌           |
| Linux              | ✅   | ✅   | ✅    | ❌           |
| macOS              | ✅   | ✅   | ✅    | ✅           |
| Windows (WSL2)     | 🧪   | ✅   | ✅    | ✅           |
| MCP server         | ✅   | ❌   | ❌    | ✅           |

🟡 DDEV runs on Docker by default and can also use Podman as an alternative runtime; Lerd is built exclusively for rootless Podman.

🧪 Lerd's Windows support runs inside WSL2 and is currently **beta**, see the [Windows (WSL2) guide](https://lerd.sh/getting-started/wsl2).

## Install

### Linux

```bash
curl -fsSL https://lerd.sh/install.sh | bash
```

Update later with:

```bash
lerd update
```

### macOS

```bash
curl -fsSL https://lerd.sh/install.sh | bash
```

Update later with:

```bash
lerd update
```

The installer needs the `podman` CLI; it will offer to `brew install podman` if it's missing.

<details>
<summary>Install via Homebrew instead</summary>

```bash
brew install geodro/lerd/lerd
lerd install
```

Recent Homebrew versions block third-party taps until trusted, so you may need to run `brew trust geodro/lerd` first. Update later with `brew upgrade lerd && lerd install`.

</details>

> [!NOTE]
> See the [installation docs](https://lerd.sh/getting-started/installation) for details.

## Quick Start

```bash
cd my-laravel-project
lerd link
# → https://my-laravel-project.test
```

`lerd install` already starts everything for you on first run, so you can `lerd link` immediately. Day-to-day:

```bash
lerd start          # boot DNS, nginx, PHP-FPM, services, workers, UI
lerd stop           # stop containers and workers (UI and watcher stay up)
lerd quit           # full shutdown including UI, watcher, and tray
lerd autostart enable   # boot lerd on every login
lerd status         # health snapshot
```

See [Start, Stop & Autostart](https://lerd.sh/usage/lifecycle) for the full lifecycle reference.

## Framework Store

Install community framework definitions from [geodro/lerd-frameworks](https://github.com/geodro/lerd-frameworks):

```bash
lerd framework search                   # list all available
lerd framework install symfony          # auto-detects version from composer.lock
lerd framework install drupal@11        # explicit version
lerd framework list --check             # compare local vs store
```

Frameworks auto-detect when you `lerd link` a project. Workers, env setup, nginx proxy, and setup commands are all driven by the framework definition — no hardcoded behavior.

## Documentation

📖 **[lerd.sh](https://lerd.sh)**

- [Requirements](https://lerd.sh/getting-started/requirements)
- [Installation](https://lerd.sh/getting-started/installation)
- [Quick Start](https://lerd.sh/getting-started/quick-start)
- [Start, Stop & Autostart](https://lerd.sh/usage/lifecycle)
- [Frameworks](https://lerd.sh/usage/frameworks)
- [Services](https://lerd.sh/usage/services)
- [Command Reference](https://lerd.sh/reference/commands)

## License

MIT
