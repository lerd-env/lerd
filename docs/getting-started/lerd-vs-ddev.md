---
title: Lerd vs DDEV
description: 'Lerd and DDEV compared on isolation, production parity, reproducibility, OS support and licence: a shared rootless Podman stack against per-project Docker containers.'
---

# Lerd vs DDEV

[DDEV](https://ddev.com/) and Lerd are both open-source local PHP environments built on containers. The difference is the shape: DDEV starts a set of containers for each project behind a shared router, while Lerd runs one shared nginx, PHP-FPM per PHP version and one instance of each service that every project drops into. That single choice drives most of the rows below.

## At a glance

|  | Lerd | DDEV |
|---|---|---|
| OS support | Linux (systemd), macOS, Windows through WSL2 | Linux, macOS, Windows |
| Licence | MIT | Apache 2.0 |
| Container runtime | Rootless Podman, no daemon | Docker, or Podman, OrbStack, Colima |
| Architecture | Shared nginx and PHP-FPM across all projects | Per-project web and database containers behind a shared router |
| Isolation | Projects share the PHP-FPM container of their PHP version and one server per database engine, each with its own database | Each project has its own containers and database server |
| Production parity | nginx with PHP-FPM on Alpine images; database versions chosen per service; a site can bring its own `Containerfile.lerd` | Per-project choice of web server, PHP and database version, close to a production image |
| Reproducibility | `.lerd.yaml` in the repo records PHP, Node, services, workers and custom containers; optional, Lerd works without it | `.ddev/config.yaml` in the repo, required |
| PHP versions | 7.4 to 8.6 | 5.6 to 8.4 and newer, per project |
| Domains | `<name>.test`, automatic | `<name>.ddev.site` or custom, automatic |
| HTTPS | `lerd secure`, trusted local CA | Built in through mkcert |
| Non-PHP projects | `Containerfile.lerd` for Node, Python, Go and others | `nodejs` and `generic` project types |
| Interfaces | CLI, web dashboard, terminal dashboard, tray, PhpStorm plugin, MCP server | CLI, add-ons for web UIs |
| Git worktrees | Each worktree at its own branch domain, sharing or [copying the database](/features/git-worktrees#per-worktree-database) | Not built in |
| Cost | Free | Free |

## When DDEV fits better

- Two projects need different major versions of the same database server running at once, fully apart.
- Your team is on Windows without WSL2, or already standardised on Docker.
- You want the environment fully described in the repo and identical for every developer, including the web server choice.
- You rely on DDEV's hosting integrations or its add-on ecosystem.

## When Lerd fits better

- You work across many projects and want them all up at once on one stack, with one MySQL and one Redis.
- You link repositories you cannot add files to, client or legacy code, since `lerd link` needs nothing committed.
- You want rootless Podman with no daemon, or a web dashboard, terminal dashboard and MCP server for AI assistants out of the box.

## Moving a DDEV project to Lerd

Export the database with `ddev export-db --gzip=false --file=myapp.sql`, stop the project with `ddev stop`, then run `lerd link`, `lerd env` and `lerd db:import myapp.sql` in the project folder. The `.ddev` directory can stay, Lerd ignores it. See the [quick start](/getting-started/quick-start) for the full first run.

[All comparisons](/getting-started/comparison) · [Lerd vs Lando](/getting-started/lerd-vs-lando) · [Laravel Sail alternative](/getting-started/sail)
