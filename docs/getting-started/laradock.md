---
title: Laradock alternative
description: 'Laradock puts a submodule in your repo and a workspace container between you and artisan. Lerd removes both: composer and artisan run from your own shell, images are pulled rather than built from source, and one shared nginx and service layer serves every project on its own .test domain.'
head:
  - - script
    - type: application/ld+json
    - |
      {
        "@context": "https://schema.org",
        "@type": "FAQPage",
        "mainEntity": [
          {
            "@type": "Question",
            "name": "What is the best Laradock alternative?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Lerd, if what you want to keep from Laradock is having every service on tap and what you want to lose is the workspace container and the submodule in the repo. Lerd pulls prebuilt images instead of building them from source, runs composer and artisan from your own shell rather than through docker compose exec, and lets the project directory live wherever you keep it instead of beside a laradock checkout. DDEV and Lando are the alternatives if you would rather stay on a per-project Docker stack."
            }
          },
          {
            "@type": "Question",
            "name": "Why is Laradock slow?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Laradock builds large images from source on first run, and each project you spin up brings its own copy of PHP, nginx and every service it needs. Running several projects at once means several full stacks in memory. Lerd shares one nginx, one PHP-FPM per version and one instance of each service across every site, so five running projects cost around 200 MB rather than a full stack each."
            }
          },
          {
            "@type": "Question",
            "name": "How do I migrate a Laradock project to Lerd?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Dump the database out of the Laradock MySQL or PostgreSQL container with docker compose exec, move the project directory out from under the Laradock parent folder, then run lerd link inside it, lerd env to rewrite the .env connection values, and lerd db:import to restore the dump. The laradock/ submodule and its docker-compose.yml can then be deleted."
            }
          },
          {
            "@type": "Question",
            "name": "Does Lerd need Docker?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "No. Lerd runs nginx, PHP-FPM and services as rootless Podman containers under your own user account, managed by systemd user units. There is no daemon, no Docker Desktop licence and no root. Docker can stay installed alongside it."
            }
          },
          {
            "@type": "Question",
            "name": "Can I still pick service versions without Laradock?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Yes, per service rather than per project. Lerd runs versioned service presets, so you choose which MySQL, PostgreSQL, Redis, MongoDB or Meilisearch version runs, and every site shares it. If two projects genuinely need different major versions of the same database at the same time, a per-project Docker stack still fits that case better."
            }
          }
        ]
      }
---

# Laradock alternative

**Laradock puts two things between you and your project: a `laradock/` checkout inside the repo, and a `workspace` container you have to shell into before `artisan` will run.** Lerd removes both. Tooling runs from your own shell, in your own directory, against your own files.

```bash
curl -fsSL https://lerd.sh/install.sh | bash
cd ~/code/myapp
lerd link
lerd composer install
lerd artisan migrate
```

No `docker compose exec workspace bash` first, no Compose file in the repo, no port to remember. The project is live at `https://myapp.test` with a trusted certificate.

[Laradock](https://laradock.io) solved a real problem, and solved it early: every service a PHP project might want, behind one `docker-compose.yml`. What dates it is the shape it leaves behind. A submodule and an `.env` to keep in sync, a parent folder the project has to sit inside because `APP_CODE_PATH_HOST` points at it, images that compile from source on first run, and one full stack per project once four repos are open at the same time.

Lerd keeps the part that was worth having, every service one command away, and drops the rest. Images are pulled, not built. One nginx, one PHP-FPM per version and one instance of each service are shared across every site, as rootless Podman containers under your own user, so the memory cost stops scaling with how many projects you have open.

## Every Laradock habit, and the Lerd equivalent

| What you did in Laradock | The same thing in Lerd |
|---|---|
| `docker compose up -d nginx mysql redis` before you can work | `lerd start` once, then everything is up for every project |
| `docker compose exec workspace bash` to reach PHP tooling | `lerd php`, `lerd composer`, `lerd artisan`, run from your own shell in the project directory |
| Edit `laradock/.env`, set `PHP_VERSION`, rebuild the image | `lerd isolate 8.4`, or let it read `composer.json`, no rebuild |
| A `laradock/nginx/sites/*.conf` per project, plus `/etc/hosts` entries | An nginx vhost generated on `lerd link`, and the `.test` hostname registered for you |
| Self-signed cert wired in by hand, or plain HTTP | `lerd secure`, a real mkcert certificate trusted by your system and browsers |
| Uncomment a service in `docker-compose.yml` and rebuild | `lerd service start mongodb`, shared across every site |
| `APP_CODE_PATH_HOST` pointing at a sibling project directory | Projects live wherever you keep them, there is no parent folder to respect |
| `docker compose logs -f php-fpm` | A [log viewer](/features/logs) in the dashboard, or `lerd logs` |
| Queue and scheduler as extra Compose services | `lerd worker start queue` / `schedule`, as systemd user services with [self-healing](/usage/worker-heal) |
| `laradock/` submodule committed to the repo | [`.lerd.yaml`](/configuration#per-project-config-lerd-yaml), optional, a handful of lines |
| Docker Desktop, or Docker Engine plus a daemon running as root | Rootless Podman, no daemon, no root |

## Lerd vs Laradock

|  | Lerd | Laradock |
|---|---|---|
| Platforms | Linux (systemd), macOS, Windows via WSL2 (beta) | Linux, macOS, Windows |
| License | Open source (MIT) | Open source (MIT) |
| Container runtime | Rootless Podman, no daemon | Docker, with a root daemon or Docker Desktop |
| Architecture | One shared nginx, PHP-FPM and service layer across every site | A per-project Compose stack, one container per service |
| RAM with 5 projects running | ~200 MB | Several GB, five full stacks |
| First run | Pulls prebuilt images, minutes | Builds images from source, often much longer |
| PHP versions | 7.4, 8.0 to 8.5, per project, no rebuild | Set in `laradock/.env`, rebuild the image to change |
| `.test` domains | Automatic, through lerd's built-in DNS server | Manual `/etc/hosts` entries plus a site conf per project |
| HTTPS | `lerd secure`, trusted mkcert certificate | Bring your own certificate and wire it into the nginx conf |
| Running tooling | `lerd artisan`, `lerd composer`, `lerd node` from your own shell | `docker compose exec workspace` first |
| Files in the repo | None required, `.lerd.yaml` optional | `laradock/` submodule, `docker-compose.yml`, its own `.env` |
| Works on a client repo you cannot modify | Yes, just `lerd link` | Only if you can add Laradock to it |
| Per-project service versions | No, services are shared and versioned globally | Yes, each project pins its own |
| Dashboard | [Web UI](/features/web-ui) at `127.0.0.1:7073`, [system tray](/features/system-tray), [terminal dashboard](/features/tui) | CLI, plus whatever Docker Desktop shows |
| AI / MCP | Built-in [MCP server](/features/mcp) for Claude Code, Cursor, Junie and Windsurf | Not built in |

**Choose Laradock when:** two projects genuinely need different major versions of the same database at the same time, your team already standardised on it, or you want the environment fully described inside the repo.

**Choose Lerd when:** you work across several projects at once and do not want a stack per repo, you are tired of rebuilding images to change a PHP version, you want `.test` URLs and HTTPS without wiring them, or you want to work on repos you cannot add files to.

## Moving a Laradock project to Lerd

There is no automated importer for Laradock the way there is for [Laravel Sail](/usage/import-sail), because Laradock stacks vary too much to migrate blind. The manual path is five commands.

**1. Dump the database while Laradock is still up.** From the `laradock/` directory:

```bash
docker compose up -d mysql
docker compose exec mysql mysqldump -u root -proot myapp > ~/myapp.sql
```

For PostgreSQL:

```bash
docker compose exec postgres pg_dump -U default myapp > ~/myapp.sql
```

Use whatever credentials your `laradock/.env` actually sets, they are commonly `root` / `root` for MySQL and `default` / `secret` for PostgreSQL.

**2. Move the project out of the Laradock layout.** Laradock expects the project to sit beside the `laradock/` checkout, pointed at by `APP_CODE_PATH_HOST`. Lerd has no such requirement, so the project directory can go anywhere, for example `~/code/myapp`.

**3. Install Lerd and start it.**

```bash
curl -fsSL https://lerd.sh/install.sh | bash
lerd start
```

**4. Link the project.**

```bash
cd ~/code/myapp
lerd link
```

Lerd detects the framework, picks the PHP version from `composer.json`, writes the nginx vhost, registers `myapp.test` and provisions the certificate. Use `lerd init` instead if you want to choose the PHP version, HTTPS and services through a wizard and save the answers to [`.lerd.yaml`](/configuration#per-project-config-lerd-yaml).

**5. Start the services and restore the dump.**

```bash
lerd service start mysql
lerd env
lerd db:import ~/myapp.sql
```

`lerd env` rewrites the `DB_*`, `REDIS_*` and `MAIL_*` entries in `.env` to match the services Lerd is running, so the connection values line up without you editing them. The original file is saved as `.env.before_lerd` the first time, and `lerd env:restore` puts it back if you want to return to Laradock.

**6. Delete the Laradock leftovers.** The `laradock/` submodule, the Compose file and its `.env` no longer do anything. Laradock's persistent data lives outside the repo in `~/.laradock/data`, so remove that too once you have confirmed the import.

Full detail lives in the [quick start](/getting-started/quick-start) and the [site management](/usage/sites) guide.

## What is genuinely different

These are the places where the mental model changes, worth knowing before you commit to the move:

- **Services are shared, not per project.** One MySQL, one Redis, one Mailpit across every site, which is where the memory saving comes from. It is also the one thing Laradock does that Lerd does not: uncommenting a second database service and pinning a different MySQL major version for one project.
- **No workspace container.** PHP tooling runs through `lerd php`, `lerd composer` and `lerd artisan`, which execute in the project's PHP container but from your own shell, in your own directory, with your own files. There is nothing to `exec` into first.
- **The environment is not in the repo.** Instead of a committed Compose file, you commit an optional [`.lerd.yaml`](/configuration#per-project-config-lerd-yaml) describing the PHP version, Node version, services and workers. A teammate with Lerd installed gets the same environment from it; a teammate without Lerd is unaffected, since nothing else in the project changed.
- **Nginx, not Apache.** If a project leaned on Laradock's Apache container and `.htaccess` rules, translate them into an [nginx override](/usage/nginx-overrides).
- **One `sudo` at install time.** Only to point the system resolver at the `.test` domains. Everything after that runs as your own user.

## Frequently asked questions

**Why is Laradock so slow to start?**
It builds its images from source on first run, and each project brings a full stack. Lerd pulls prebuilt images once and shares one nginx, one PHP-FPM per version and one copy of each service across every site.

**Can I keep Docker installed?**
Yes. Lerd uses rootless Podman with no daemon, so the two do not conflict. Keep Laradock running on the projects you have not moved yet.

**Do I need to change my project's code?**
No. Lerd reads projects where they are. The only file it touches is `.env`, and only when you run `lerd env`, which backs up the original first.

**What about Xdebug and profiling?**
Both are built in. See the [profiler](/features/profiler), the [dump viewer](/features/dumps) and the [query viewer](/features/queries).

**Is Lerd free for commercial work?**
Yes. MIT licensed, with no paid tier and no licence popup.

## Next steps

- [Requirements](/getting-started/requirements) and [installation](/getting-started/installation)
- [Quick start](/getting-started/quick-start), a project served in two commands
- [Full comparison](/getting-started/comparison) against Laravel Herd, Sail, DDEV and Lando
- [Importing from Laravel Sail](/usage/import-sail), if you also have Sail projects
