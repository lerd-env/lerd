---
description: What lerd gives you beyond serving sites, the web dashboard, TUI, MCP server, automatic HTTPS, .test DNS, profiler, tinker and more.
---

# Features

Beyond serving PHP sites, lerd ships a set of tools for working with them. Everything here is part of the single binary, there is nothing extra to install.

## Interfaces

- [Web UI](/features/web-ui) is the browser dashboard at `lerd.localhost`, for sites, services, logs, databases and workers.
- [TUI](/features/tui) is the terminal dashboard, informative with reversible quick actions.
- [Framework Commands](/usage/framework-commands) covers the per-framework admin actions and custom commands.
- [System tray](/features/system-tray) puts start, stop and site shortcuts in your desktop tray.
- [MCP server](/features/mcp) exposes lerd to AI assistants so they can inspect and drive your environment.

## Serving sites

- [HTTPS](/features/https) issues locally trusted certificates automatically.
- [DNS](/features/dns) resolves `.test` domains without editing `/etc/hosts`.
- [Project setup](/features/project-setup) is how lerd detects a framework and configures it.
- [Env setup](/features/env-setup) wires service credentials into your site's `.env`.
- [Git worktrees](/features/git-worktrees) serves branches side by side on their own domains.
- [FrankenPHP](/features/frankenphp) is the alternative runtime to PHP-FPM.

## Inspecting and debugging

- [Logs](/features/logs) tails application, Nginx and container logs in one place.
- [Queries](/features/queries) shows database queries per request.
- [Request timing](/features/request-timing) charts response times and slow routes from your site's live traffic.
- [Profiler](/features/profiler) captures request timings and hands off to SPX.
- [Tinker](/features/tinker) is an in-browser REPL against your application.
- [Dumps](/features/dumps) collects `dump()` output from your code.
- [Notifications](/features/notifications) surfaces failures on your desktop.

For installation see [Getting Started](/getting-started/requirements), and for day to day workflows see [Usage](/usage/sites).
