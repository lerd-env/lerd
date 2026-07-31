---
title: Laravel Herd for Linux
description: 'Laravel Herd has no Linux build. Lerd is the closest Laravel Herd alternative for Linux: automatic .test domains, one-click HTTPS, per-site PHP versions, bundled services and queue workers, free and open source.'
head:
  - - meta
    - name: keywords
      content: laravel herd linux, herd for linux, herd linux, laravel herd alternative linux, does laravel herd work on linux, laravel herd ubuntu, herd linux equivalent, local php development linux
  - - script
    - type: application/ld+json
    - |
      {
        "@context": "https://schema.org",
        "@type": "FAQPage",
        "mainEntity": [
          {
            "@type": "Question",
            "name": "Is there a Laravel Herd for Linux?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "No. Laravel Herd ships for macOS and Windows only. There is no Linux build, no .deb or .rpm package, and no announced plan for one. To get the Herd workflow on Linux you need a different tool. Lerd is the closest equivalent: automatic .test domains, one-click HTTPS, per-site PHP versions, shared databases and queue workers, on Linux and macOS."
            }
          },
          {
            "@type": "Question",
            "name": "What is the best Laravel Herd alternative for Linux?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Lerd covers the same day-to-day surface as Herd: pretty .test URLs with no hosts file edits, a virtual host generated per site, one-command TLS through mkcert, per-site PHP version selection, parked directories, and shared MySQL, PostgreSQL, Redis, Meilisearch, S3-compatible storage and Mailpit. It is MIT licensed with no paid tier, and it adds queue and scheduler workers, FrankenPHP and Octane worker mode, and a built-in MCP server. DDEV and Lando are alternatives if you prefer a per-project Docker stack."
            }
          },
          {
            "@type": "Question",
            "name": "Can I install Laravel Herd on Ubuntu?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "No. Herd is distributed as a macOS application and a Windows application, both built around native platform binaries and a native desktop app, so there is nothing to install on Ubuntu or any other distribution. Installing a native Linux tool is the practical route."
            }
          },
          {
            "@type": "Question",
            "name": "Does Lerd need Docker or sudo?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "No. Lerd runs Nginx, PHP-FPM and services as rootless Podman containers under your own user account. There is no Docker daemon and no sudo, except once during installation to point the system resolver at the .test domains."
            }
          },
          {
            "@type": "Question",
            "name": "How do I move my Herd sites to Linux?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Copy the project folders out of ~/Herd to any directory on Linux, export your databases with mysqldump or pg_dump, then run lerd park on the parent directory or lerd link inside each project, and lerd db:import to restore the dump. Lerd generates the nginx vhost, the .test hostname and the TLS certificate for you, so nothing inside the project has to change. A committed herd.yml keeps working in Herd, and its PHP version and site name map onto .lerd.yaml."
            }
          }
        ]
      }
---

# Laravel Herd for Linux

**Laravel Herd does not run on Linux.** It ships as a macOS application and a Windows application, both built around native platform binaries and a native desktop app. There is no Linux build, no `.deb`, no `.rpm`, no AppImage, and no announced plan for one, which is why "Laravel Herd for Linux" is a question that keeps coming up.

If you have moved to Linux, or your team is on Herd and you are not, **Lerd is the closest equivalent**. Same idea: one tool that gives every project a `.test` URL, HTTPS, a PHP version and the services it needs, with nothing to configure per project. MIT licensed, free for commercial use, on Linux and macOS.

```bash
curl -fsSL https://lerd.sh/install.sh | bash
cd ~/code/myapp
lerd link
```

Your project is live at `https://myapp.test`. That is the whole setup.

## Every Herd feature, and the Lerd equivalent

| What you used in Herd | The same thing in Lerd |
|---|---|
| `herd park ~/Herd`, every folder inside becomes a site | `lerd park ~/code`, same model, every project inside is served |
| `herd link`, serve the current directory | `lerd link`, plus framework detection and PHP version picked from `composer.json` |
| Automatic `.test` domains through the native resolver | Automatic `.test` domains through a dnsmasq container wired into your system resolver, no `/etc/hosts` edits |
| The Secure Site toggle | `lerd secure`, a real mkcert certificate trusted by your system and browsers |
| `herd isolate php@8.4`, PHP version per site | `lerd isolate 8.4`, or auto-detected, PHP 7.4 and 8.0 to 8.5 |
| `herd php`, `herd composer`, always the site's version | `lerd php`, `lerd composer`, `lerd php:shell`, on the version that site is registered on |
| Services, MySQL, PostgreSQL, Redis, Mailpit | `lerd service start mysql`, plus PostgreSQL, Redis, Meilisearch, MongoDB, RustFS (S3) and Mailpit, shared across every site |
| The database inspector, log viewer and dumps window in Herd Pro | The same surfaces in the [web dashboard](/features/web-ui), free: databases, [live logs](/features/logs), the [dump viewer](/features/dumps) and the [query viewer](/features/queries) |
| The Xdebug toggle | `lerd xdebug:on`, a tray toggle and a dashboard switch |
| `herd share`, expose a site publicly | `lerd share`, over ngrok, cloudflared, Expose, serveo or localhost.run, or on your LAN with a QR code |
| `herd.yml` committed to the repo | [`.lerd.yaml`](/configuration#per-project-config-lerdyaml) committed to the repo, covering PHP, Node, framework, services, workers and custom containers |
| The Herd desktop app | A [web dashboard](/features/web-ui) at `127.0.0.1:7073`, a [system tray](/features/system-tray) and a [terminal dashboard](/features/tui) |
| Octane, run manually alongside Herd | `lerd runtime frankenphp --worker`, FrankenPHP and Octane worker mode per site |
| Queue workers, run with supervisor or by hand | `lerd worker start queue` and `schedule`, as systemd user services, with self-heal |

## Lerd vs Laravel Herd

|  | Lerd | Laravel Herd |
|---|---|---|
| Platforms | Linux (systemd), macOS, Windows via WSL2 (beta) | macOS, Windows |
| License | Open source (MIT), free for commercial use | Proprietary, freemium with a Herd Pro subscription |
| Cost | Free, no paid tier | Free tier plus paid Pro subscription |
| Stack | Nginx, PHP-FPM and services as rootless Podman containers | Native binaries on macOS and Windows |
| PHP versions | 7.4, 8.0 to 8.5, shared FPM containers | 7.4 through 8.5, native |
| `.test` domains | Automatic, through a dnsmasq container | Automatic, through the native dnsmasq resolver |
| HTTPS | `lerd secure` and mkcert, trusted system-wide | Built-in Secure Site toggle |
| FrankenPHP / Octane | Built in, `lerd runtime frankenphp [--worker]` per site, free | Not built in, Octane runs manually alongside Herd |
| Xdebug | `lerd xdebug:on`, tray toggle | Per-site toggle in Herd Pro |
| Services | Built in and free, with add-ons fetched from a store that updates without a new release | Some in the free tier, most advanced services and UIs behind Herd Pro |
| Queue and scheduler workers | `lerd worker start queue` / `schedule`, systemd user services | Not built in, run Horizon or supervisor yourself |
| Per-project config | `.lerd.yaml`, covering PHP, Node, framework, services, workers and custom containers | `herd.yml`, covering name, PHP, TLS and aliases on the free tier, services need Pro |
| Non-PHP projects | First-class through `Containerfile.lerd` or a [host-proxy site](/usage/host-proxy) | Not directly supported, Herd focuses on PHP |
| Dashboard | Web UI, system tray, terminal dashboard, installable PWA | Native desktop app |
| AI / MCP | Built-in [MCP server](/features/mcp) | Built-in MCP server |

**Choose Herd when:** you work on macOS or Windows, you want native binaries with no container layer, you are already in the Laravel Forge and Envoyer ecosystem, or you are happy paying for Pro.

**Choose Lerd when:** you work on Linux, where Herd has no build at all, you prefer open source with no paid tier, you want the environment itself committed to the repo, or you need workers, non-PHP projects and profiling in the same tool.

## Moving from Herd to Lerd

Nothing inside your projects has to change. Lerd reads them where they are, and the command names line up closely enough that muscle memory mostly carries over.

**1. Copy the projects across.** Everything in `~/Herd` can go anywhere on Linux, for example `~/code`. There is no document root to respect.

**2. Export your databases** before you leave the machine Herd is on:

```bash
mysqldump -u root myapp > myapp.sql
```

**3. Install Lerd and start it.**

```bash
curl -fsSL https://lerd.sh/install.sh | bash
lerd start
```

**4. Park or link.** If you kept everything under one directory, park it once and every project inside is served:

```bash
lerd park ~/code
```

Or link a single project:

```bash
cd ~/code/myapp
lerd link
```

Lerd detects the framework, picks the PHP version from `composer.json`, writes the nginx vhost, registers `myapp.test` and provisions the certificate. Use `lerd init` instead if you want to choose the PHP version, HTTPS and services through a wizard and save the answers to `.lerd.yaml`.

**5. Restore the database.**

```bash
lerd service start mysql
lerd db:import myapp.sql
```

**6. Point the app at the shared services.** `lerd env` rewrites the `.env` database, cache and mail entries to match the services lerd is running, so `DB_HOST`, `REDIS_HOST` and `MAIL_HOST` line up without you editing them by hand.

**7. Keep `herd.yml` if the team still uses it.** Lerd ignores it and reads `.lerd.yaml`, so both can sit in the repo. The PHP version and site name carry over directly.

Full detail lives in the [quick start](/getting-started/quick-start) and the [site management](/usage/sites) guide.

## What is genuinely different

Worth knowing before you switch, these are the places where the mental model changes:

- **Containers, not native binaries.** PHP and the services run in rootless Podman containers. On Linux the overhead is negligible, on macOS Podman Machine costs you a sliver of the native performance Herd gets. What you gain is isolation and a stack that is identical on both.
- **Services are shared, not per project.** One MySQL, one Redis, one Mailpit across every site, which is why five running projects cost around 200 MB of RAM rather than five full stacks.
- **Nothing is gated.** The database browser, log viewer, dumps window and Xdebug toggles that sit behind Herd Pro are part of the dashboard.
- **One `sudo` at install time.** Only to point the system resolver at the `.test` domains. Everything after that runs as your own user.

## Staying on macOS

Herd is a good tool on macOS and this page is not an argument against it. Lerd also runs on macOS if you want one environment across a Linux desktop and a Mac laptop, with the same `.lerd.yaml` producing the same stack on both. See [installation](/getting-started/installation).

## Frequently asked questions

**Is there a Laravel Herd for Linux?**
No. Herd ships for macOS and Windows only, and no Linux build has been announced.

**Can I install Herd on Ubuntu?**
No. Both builds are native desktop applications wrapped around platform binaries, so there is no package for any distribution.

**What is the best Laravel Herd alternative for Linux?**
Lerd, if you want the Herd model of a shared zero-config stack that any project can be dropped into. [DDEV or Lando](/getting-started/comparison) if you would rather have a per-project Docker stack.

**Does Lerd need Docker?**
No. It uses rootless Podman, which has no daemon, and it never asks for Docker Desktop.

**Is Lerd free for commercial work?**
Yes. MIT licensed, with no paid tier and no Pro subscription.

**Does my `herd.yml` still work?**
It keeps working in Herd. Lerd reads [`.lerd.yaml`](/configuration#per-project-config-lerdyaml), which covers the same ground and adds services, workers and Node, so both files can live in the repo.

## Next steps

- [Requirements](/getting-started/requirements) and [installation](/getting-started/installation)
- [Quick start](/getting-started/quick-start), a project served in two commands
- [Full comparison](/getting-started/comparison) against Herd, Sail, DDEV, Lando and Laragon
- [Laragon for Linux](/getting-started/laragon-linux) if you are coming from Windows instead
