---
title: Lerd vs Lando
description: 'Lerd and Lando compared on isolation, production parity, reproducibility, OS support and licence: a shared rootless Podman stack against per-project Docker recipes.'
---

# Lerd vs Lando

[Lando](https://lando.dev/) builds a container stack for each project from a `.lando.yml` file and a recipe for the platform, behind a shared proxy. Lerd runs one shared nginx, PHP-FPM per PHP version and one instance of each service for every project, with no file required in the repo. Both are open source.

## At a glance

|  | Lerd | Lando |
|---|---|---|
| OS support | Linux (systemd), macOS, Windows through WSL2 | Linux, macOS, Windows |
| Licence | MIT | GPL-3.0 |
| Container runtime | Rootless Podman, no daemon | Docker |
| Architecture | Shared nginx and PHP-FPM across all projects | Per-project containers behind a shared proxy |
| Isolation | Projects share the PHP-FPM container of their PHP version and one server per database engine, each with its own database | Each project has its own services |
| Production parity | nginx with PHP-FPM on Alpine images; database versions chosen per service; a site can bring its own `Containerfile.lerd` | Recipes mirror hosting platforms such as Pantheon, Platform.sh and Lagoon |
| Reproducibility | `.lerd.yaml` in the repo records PHP, Node, services, workers and custom containers; optional | `.lando.yml` in the repo, required |
| PHP versions | 7.4 to 8.6 | Per project, set in the recipe or `.lando.yml` |
| Domains | `<name>.test`, automatic | `*.lndo.site`, automatic |
| HTTPS | `lerd secure`, trusted local CA | Built in, per project |
| Non-PHP projects | `Containerfile.lerd` for Node, Python, Go and others | Services for Node, Python, Ruby, Go, .NET |
| Interfaces | CLI, web dashboard, terminal dashboard, tray, PhpStorm plugin, MCP server | CLI |
| Git worktrees | Each worktree at its own branch domain, sharing or [copying the database](/features/git-worktrees#per-worktree-database) | Not built in |
| Cost | Free | Free |

## When Lando fits better

- You deploy to Pantheon, Platform.sh or Lagoon and want their recipes and `lando pull` / `lando push`.
- Each project needs its own isolated services defined in the repo.
- Your team is on Windows without WSL2, or already standardised on Docker.

## When Lerd fits better

- You keep many projects running at once and want them on one shared stack.
- You serve repositories you cannot add a Landofile to.
- You want rootless Podman with no daemon, a web and terminal dashboard, and an MCP server for AI assistants.

## Moving a Lando project to Lerd

Export the database with `lando db-export myapp.sql`, stop the app with `lando stop`, then run `lerd link`, `lerd env` and `lerd db:import myapp.sql` in the project folder. `.lando.yml` can stay where it is. See the [quick start](/getting-started/quick-start) for the full first run.

[All comparisons](/getting-started/comparison) · [Lerd vs DDEV](/getting-started/lerd-vs-ddev) · [Laradock alternative](/getting-started/laradock)
