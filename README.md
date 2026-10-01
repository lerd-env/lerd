# Lerd

> Open-source Herd-like local PHP development environment for Linux and macOS,
> with Windows supported via WSL2 (beta). Podman-native, rootless, with a
> built-in Web UI.

[![CI](https://github.com/lerd-env/lerd/actions/workflows/ci.yml/badge.svg)](https://github.com/lerd-env/lerd/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/lerd-env/lerd)](https://github.com/lerd-env/lerd/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL2%20(beta)-lightgrey)]()
[![Docs](https://img.shields.io/badge/docs-lerd.sh-blue)](https://lerd.sh)
[![Reddit](https://img.shields.io/badge/Reddit-r%2Flerd-ff2d20?logo=reddit)](https://reddit.com/r/lerd)
[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/5JK54s7xCC)

https://github.com/user-attachments/assets/707ca4cc-7eef-4285-b7bf-fdeb37f61458

Lerd runs Nginx, PHP-FPM, and your services as rootless [Podman](https://podman.io) containers,
designed for PHP developers on Linux and macOS, and on Windows through WSL2 (beta).
No Docker. No sudo. No system pollution. Just `lerd link` and your project
is live at `project.test` with HTTPS.

## Built for Linux PHP developers

Lerd is built for PHP developers on Linux who want frictionless local development: automatic `.test` domains, per-project PHP versions, one-click HTTPS, zero Docker. Works with Laravel, Symfony, WordPress, Drupal, Magento, CakePHP, Statamic, Yii, and any custom PHP framework.

## Features

### Sites, domains and TLS

- 🌐 **Automatic `.test` domains.** One command gives a project a hostname and TLS that reissues before it expires, with no dnsmasq, no system resolver tweak and no sudo for the DNS bits. You can [opt out of lerd-managed DNS](https://lerd.sh/features/dns) for `*.localhost`, or toggle it later with `dns:enable` / `dns:disable` / `dns:repair`.

- 🔗 **Site groups.** Group related sites so a main site owns a base domain and the rest occupy its subdomains, with a shared or separate database per secondary.

- 🧱 **Node, Python, Rails and more.** `lerd init` recognises a Node, Python, Rails or Rack, Go or Rust project, runs its dev server on the host under lerd's supervision and serves it at a `.test` domain with HTTPS, services and worktrees included, or as a rootless container from a starter `Containerfile.lerd`.

- 🌳 **First-class git worktrees.** Auto-detected branch domains, per-worktree PHP and Node versions, a database picked from the branch's migrations when a script adds it, wildcard cert SANs and a per-branch Vite worker. A bare `git worktree add` from any tool is provisioned automatically.

- 🌍 **Share a site.** On your LAN with a stable port and a QR code, or publicly through ngrok, cloudflared, Expose, Pinggy, serveo or localhost.run. Set a base domain once and every share keeps the same URL between runs, through a tunnel service or the reverse proxy you already run.

- 🎨 **Dev servers on the site's own domain.** A running Vite serves its assets and its hot-reload socket under the site's `.test` hostname instead of advertising `localhost:5173`, so a shared, LAN-opened or worktree page arrives styled. Nothing in the project is edited and nothing is declared per framework.

### PHP, Node and runtimes

- 🐘 **Per-project PHP version.** 8.1 to 8.5, a frozen 7.4 / 8.0 legacy tier and 8.6 as a prerelease nothing picks for you, switched with one click. Custom extensions and Alpine packages are declared once for every image lerd builds, and the ODBC stack ships in all of them.
- ⚡ **PHP on the host, on macOS** (beta). A native runtime runs PHP-FPM, the CLI, composer and the workers on the host, removing the bind-mount boundary a Mac pays on every file PHP reads: a Laravel app with Horizon and Filament went from 62 requests per second to 238. Containers stay the default.

- ⚡ **FrankenPHP runtime.** Per site, as an alternative to shared PHP-FPM, with Laravel Octane and Symfony Runtime worker mode.

- 📱 **NativePHP, desktop and mobile.** The Electron app runs as a host worker and the mobile build reaches an emulator, a simulator or a device. A command whose runtime cannot live in a container is declared as host-only, so `native:run` opens its window instead of dying in the PHP container.

- 📦 **Node.js isolation.** Node 22 or 24 per project, read from `.nvmrc`, `.node-version`, `mise.toml` or `.tool-versions`, through mise (the default for new installs), fnm or an nvm you already have, switchable from the dashboard. Or **bun** as the JS runtime on the host and, opt-in, inside the container.

- 🪄 **No per-framework setup.** Workers, env values and the nginx vhost are configured when you link a project. "Env" means whatever file your framework reads, a `.env`, `wp-config.php`, Magento's `env.php` or Drupal's `settings.php`, written in place, and the worker options you pick are committed to `.lerd.yaml`.

- 🧩 **Framework store.** Laravel, Lumen, Symfony, WordPress, Bedrock, Drupal, Magento, CakePHP, CodeIgniter, Statamic, Winter CMS, Tempest, TYPO3 and Yii, detected per major. Composer packages declare their own workers and the services they need, and a new definition arrives without a lerd release.

### Services and databases

- 🗄️ **One-click services.** MySQL, PostgreSQL, Redis, Meilisearch, RustFS, Mailpit, OpenSearch, Solr, Kafka, LocalStack and more from a store that updates without a lerd release. Admin dashboards open already logged in, and a service page suggests what works alongside it, like SpamAssassin for Mailpit.

- 🔌 **Host tools that reach the container.** `psql`, `mysql`, `pg_dump` and friends run on your host against lerd's engines with no client installed. An IDE's phpstan, php-cs-fixer or phpcs run through the same shims in the project's PHP container, and `lerd cpx` runs any Composer package's command.

- 🧷 **IDE database wiring** for JetBrains. A project gets one data source pointed at its own lerd database on the host port it actually answers on, written on link and refreshed as the project's database changes, leaving every data source lerd doesn't own untouched.

### Debugging and performance

- 🛰️ **Debug window.** Every `dump()` / `dd()`, SQL with N+1 and slow-query detection, mail, views, events, jobs and outgoing HTTP, per site and branch, in the dashboard, TUI, MCP and `lerd dump tail`. `ray()`, the app log, Sentry or Inspector exceptions and outgoing SMS and Slack messages land there too.

- 🔥 **[SPX](https://github.com/NoiseByNorthwest/php-spx) profiler** with one-click on/off. Every PHP-FPM request becomes a flame graph viewable in a same-origin Profiler view in the dashboard, with no FPM restart and no code changes, and `lerd profile run` profiles a one-shot artisan or CLI command.

- 📈 **Request timing analytics.** A durable per-site view of typical and p95 response times, throughput, error rate, and the slowest routes ranked by recent p95 with one-click profiling. Agents get the same signal over MCP with `route_timing` and `optimize_route`.

- 🧪 **Tinker tab.** An in-browser PHP REPL per site with project-aware autocomplete, hover and diagnostics powered by [phpantom_lsp](https://github.com/PHPantom-dev/phpantom_lsp), so your models and Builder chains resolve as you type. Works on Laravel, Symfony and any composer project.

### Interfaces

- 🖥️ **Built-in Web UI.** Sites and services dashboards, live widgets and a Cmd+K palette, in fourteen languages and twelve themes, or your desktop's own colours on Omarchy, Plasma, GNOME and macOS, carried into every embedded admin dashboard. Screen reader friendly, with a streaming mode.

- ✨ **Start a project from the dashboard.** The `+` in Sites scaffolds a project from the framework store or links one you already have, asks what `lerd init` asks, then runs composer and the JS build in the modal. Close the tab mid-install and it picks back up.

- 📚 **The documentation, offline.** Every page ships inside the binary, searchable and rendered in the dashboard, so the one moment you most need the docs, a machine with no internet, is not the moment they stop working. `lerd man` reads the same pages in the terminal.

- 💻 **Terminal dashboard** (`lerd tui`). A btop-style TUI with live status, a site detail pane, inline domain and version editing, a shell drop-in, log tailing, databases with their snapshots and each service's tools and tuning, the same operations surface as the web UI for tmux and SSH.

- 🚀 **Start it without a terminal.** The dashboard brings lerd up from its own banner, streaming the start unit by unit. `lerd install` adds Lerd to the macOS Applications folder and the Linux app list, opening as a chromeless app window, so a cold start is a click. The tray is optional.

- 🧩 **PhpStorm plugin.** [Lerd for PhpStorm](https://plugins.jetbrains.com/plugin/34420-lerd) resolves the project you have open to its site and brings its logs, PHP version, workers, services, query findings and dumps into the IDE, with a path in a stack trace one click from the line that threw it.

- 🪟 **Omarchy bar widget.** [lerd Glance](https://github.com/lerd-env/lerd-omarchy-glance) puts sites, services, workers and container CPU and memory in the Omarchy Quattro bar, quiet until something breaks, so you can see the state of the environment without opening a tab.

- ✏️ **Edit config in the browser.** Per-site and global nginx at server and location scope, `php.ini` with the version's own file beside the shared one, `.env` files, and service runtime tuning, each validated (`nginx -t` where it applies), with timestamped backups and one-click restore.

- 📋 **Live logs** for PHP-FPM, Queue, Schedule and Reverb, per site, rendered in the colour the tool actually emits (artisan, composer, vite, pest) and with a button that hands any log to a real terminal so a long tail survives closing the tab.

- 🔔 **Notifications** for the things worth interrupting you, delivered to open dashboards, to subscribed browsers over Web Push, or to your desktop's native notification daemon. Every one also lands in the dashboard's sidebar bell, which keeps the last 50 with an unread count across reloads.

- 🤖 **MCP server.** Let AI assistants (Claude Code, Cursor, JetBrains Junie, Codex CLI, Gemini CLI, GitHub Copilot, Google Antigravity, Windsurf, OpenCode) manage your environment directly, registered with one command or one switch in the dashboard.

### Health and upkeep

- 🧰 **Environment doctor** (`lerd doctor`). Checks the host lerd itself depends on and repairs what it safely can with `--fix`: missing directories, linger, a missing PHP image, the DNS wiring. Anything needing sudo is printed as a command and never run for you, and `--dry-run` shows it first.

- 🩺 **Site doctor.** Framework-agnostic health checks (env drift, application key, composer and node state, security audits, database presence, PHP version range) plus extra checks for your framework, each with a one-click fix, from the web UI, the TUI, `lerd site:doctor` and MCP.

- ⚒️ **Worker self-heal.** Failed queue, schedule, horizon, reverb and stripe workers are surfaced everywhere (CLI, dashboard banner, TUI, MCP) and recovered with one click or `lerd worker heal`. A worker that keeps failing can be stopped from the same banner rather than restarted into the same wall.

- 💾 **Nothing destructive without a way back.** A `service remove --purge` or a `reinstall --reset-data` snapshots every database first, while the data is still where the engine expects it, so recovery is an ordinary `db:restore -A`. Each engine declares how long it gets to shut down.

- 💤 **Idle-suspend.** Activity-driven suspension of a site's workers (queue, schedule, horizon, reverb, stripe, Vite) after a configurable idle timeout, resumed on the next request, CLI command, MCP call or file save, with per-site pinning.

- 📶 **Nothing downloads behind your back.** Every command that can pull or rebuild an image names it and its size first, read from the registry manifest. The dashboard asks, an assistant over MCP has to come back with your answer, and `--no-pull`, `LERD_OFFLINE=1` and `lerd start --dry-run` cover a metered link.

- 📌 **Pinned host tools.** Composer, mise, fnm and mkcert are pinned behind a published manifest rather than whatever `releases/latest` served that day, so an upstream release cannot break a fresh install, and the System page applies each update on the card that flagged it.

- 🔒 **Rootless and daemonless.** Podman-native, no Docker required, dual-stack IPv4 + IPv6.

## AI Integration (MCP)

Lerd ships a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server. Connect it to Claude Code, Cursor, JetBrains Junie, Codex CLI, Gemini CLI, GitHub Copilot, Google Antigravity, Windsurf, OpenCode, or any MCP-compatible AI assistant and manage your dev environment without leaving the chat.

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

Twelve grouped tools, each driven by an `action`: `site`, `service`, `db`, `env`, `runtime`, `worker`, `exec`, `framework`, `diag`, `logs`, `workspace`, and `worktree`. Scaffold projects, run migrations, manage services, toggle workers, tail and search logs, enable Xdebug, manage databases and PHP extensions, park directories, switch runtimes between PHP-FPM and FrankenPHP, and more, all from your AI assistant.

📖 [MCP documentation](https://lerd.sh/features/mcp)

## Why Lerd?

|                    | Lerd | DDEV | Lando | Laravel Herd | Laragon |
|--------------------|------|------|-------|--------------|---------|
| Podman-native      | ✅   | 🟡   | ❌    | ❌           | ❌      |
| Rootless           | ✅   | ❌   | ❌    | ✅           | ❌      |
| Web UI             | ✅   | ❌   | ❌    | ✅           | ❌      |
| Terminal dashboard | ✅   | ❌   | ❌    | ❌           | ❌      |
| Linux              | ✅   | ✅   | ✅    | ❌           | ❌      |
| macOS              | ✅   | ✅   | ✅    | ✅           | ❌      |
| Windows            | 🧪   | ✅   | ✅    | ✅           | ✅      |
| MCP server         | ✅   | ❌   | ❌    | ✅           | ❌      |

🟡 DDEV runs on Docker by default and can also use Podman as an alternative runtime; Lerd is built exclusively for rootless Podman.

🧪 Lerd's Windows support runs inside WSL2 and is currently **beta**, see the [Windows (WSL2) guide](https://lerd.sh/getting-started/wsl2). Laragon runs natively on Windows and has no Linux or macOS build, see [Laragon for Linux](https://lerd.sh/getting-started/laragon-linux) if that is what brought you here.

## Install

### Linux

```bash
curl -fsSL https://lerd.sh/install.sh | bash
```

Update later with:

```bash
lerd update
```

Betas are published as prereleases, which the plain installer skips, so `bash -s -- --beta` is how you ask for one, on a first install or on an update.

<details>
<summary>Install via apt instead (Ubuntu/Debian)</summary>

The PPA publishes for every Ubuntu release in standard support and for the current development release. On one of those:

```bash
sudo add-apt-repository ppa:lerd/lerd
sudo apt update
sudo apt install lerd
```

On any other release the PPA has no packages, and `add-apt-repository` leaves behind a source entry that fails every later `apt update`. Remove it with `sudo add-apt-repository --remove ppa:lerd/lerd` and use the script installer above.

The package finishes setup with no prompt: its maintainer script applies the root-level steps and runs the per-user install, so `.test` DNS and HTTPS come up on their own. Update with `sudo apt upgrade`; a packaged lerd lives under `/usr`, so `lerd update` defers to your package manager instead of fighting it.

</details>

<details>
<summary>Install via dnf instead (Fedora)</summary>

The COPR builds for every Fedora release in standard support and for rawhide:

```bash
sudo dnf copr enable georged/lerd
sudo dnf install lerd
```

The package finishes setup with no prompt, exactly like the apt one: `.test` DNS and HTTPS come up on their own. Update with `sudo dnf upgrade`; a packaged lerd lives under `/usr`, so `lerd update` defers to your package manager instead of fighting it.

</details>

<details>
<summary>Install via Homebrew instead</summary>

```bash
brew install lerd-env/lerd/lerd
lerd install
```

Podman comes from your distro rather than as a brew dependency, and Homebrew on Linux needs its usual prerequisites (notably a C compiler). Update with `brew upgrade lerd`.

</details>

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
brew install lerd-env/lerd/lerd
lerd install
```

Recent Homebrew versions block third-party taps until trusted, so you may need to run `brew trust lerd-env/lerd` first. Update later with `brew upgrade lerd && lerd install`.

</details>

### NixOS

NixOS has its own flake, since its declarative model doesn't fit the one-line installer's imperative DNS and self-install steps.

```bash
nix run github:lerd-env/lerd-nixos -- --help
```

The [`lerd-nixos`](https://github.com/lerd-env/lerd-nixos) flake packages the binary and ships the `configuration.nix` blocks the stack needs. See the [NixOS guide](https://lerd.sh/getting-started/nixos) for the full runbook.

> [!NOTE]
> See the [installation docs](https://lerd.sh/getting-started/installation) for details.

### Desktop app (optional)

Prefer a dedicated window to a browser tab? [Lerd Desktop](https://github.com/lerd-env/lerd-desktop) wraps the dashboard in a native app with native desktop notifications for captured mail, worker failures, and finished operations, no browser needed. Linux, shipped as a Flatpak:

```bash
flatpak install --user https://lerd.sh/lerd.flatpakref
```

Once it's installed, `lerd dashboard` opens the app instead of the browser.

## Quick Start

```bash
cd my-laravel-project
lerd link
# → https://my-laravel-project.test
lerd open           # the site in your browser
lerd code           # the project in your editor
```

Starting from nothing, `lerd new` asks which framework and which major from the store, scaffolds it, and links the result, so you land on a served site rather than on three commands to type.

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

Install community framework definitions from [lerd-env/frameworks](https://github.com/lerd-env/frameworks):

```bash
lerd framework search                   # list all available
lerd framework install symfony          # auto-detects version from composer.lock
lerd framework install drupal@11        # explicit version
lerd framework list --check             # compare local vs store
```

Frameworks auto-detect when you `lerd link` a project, and its workers, env values, nginx proxy and setup commands are configured from there.

Services work the same way. Lerd ships the default stack, and every add-on lives in [lerd-env/services](https://github.com/lerd-env/services), so a new service reaches you without updating lerd:

```bash
lerd service search                     # browse the store
lerd service preset pgadmin             # install a store-only preset
```

## Documentation

📖 **[lerd.sh](https://lerd.sh)**

- [Requirements](https://lerd.sh/getting-started/requirements)
- [Installation](https://lerd.sh/getting-started/installation)
- [Quick Start](https://lerd.sh/getting-started/quick-start)
- [Start, Stop & Autostart](https://lerd.sh/usage/lifecycle)
- [Frameworks](https://lerd.sh/usage/frameworks)
- [Services](https://lerd.sh/usage/services)
- [Command Reference](https://lerd.sh/reference/commands)

## Sponsors

Lerd is free and open source, kept going by the people and companies who sponsor it. [Become a sponsor](https://github.com/sponsors/geodro).

### Companies

<a href="https://www.motional.io"><img src="https://avatars.githubusercontent.com/u/34476080?s=120&v=4" width="60" alt="Motional"></a>

### Backers

[Edward van Tonder](https://github.com/EA-Wardie) · [rolaca11](https://github.com/rolaca11) · [rais](https://github.com/raisilhamn) · [Anca Emcken](https://github.com/ancaemcken)

## Built on

Lerd stands on a set of excellent open-source projects it bundles or fetches to power the experience:

- [phpantom_lsp](https://github.com/PHPantom-dev/phpantom_lsp) - the PHP language server behind tinker autocomplete, diagnostics and semantic highlighting
- [Monaco](https://github.com/microsoft/monaco-editor) - the editor engine for every in-browser editing surface
- [php-spx](https://github.com/NoiseByNorthwest/php-spx) - the profiler behind the SPX flame graphs
- [mkcert](https://github.com/FiloSottile/mkcert) - the local CA that backs `.test` HTTPS
- [mise](https://github.com/jdx/mise) - the per-project Node version manager on new installs
- [fnm](https://github.com/Schniz/fnm) - the Node version manager installs set up before mise
- [static-php-cli](https://github.com/crazywhalecc/static-php-cli) - the prebuilt static PHP binaries lerd falls back to when a project's own bundled runtime is missing an extension a command needs
- [Composer](https://getcomposer.org) - fetched on the host for dependency operations
- [Starship](https://starship.rs) - the prompt in the container shell drop-in
- [Simple Icons](https://simpleicons.org) - the service marks in the dashboard, CC0

## License

lerd is MIT licensed, see [LICENSE](LICENSE).

The binary also carries code from the Go modules it links and the npm packages the dashboard is built from. Their copyright notices and license terms are reproduced in [THIRD-PARTY-LICENSES.md](THIRD-PARTY-LICENSES.md), which ships in every release archive and is embedded in the binary itself, so `lerd licenses` prints it on any machine. The file is generated from the real dependency graph by `make licenses`, and CI fails when it drifts.

The tools lerd downloads onto your machine at runtime, listed above, are not redistributed by lerd and keep their own licenses where they are installed.
