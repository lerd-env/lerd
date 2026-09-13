# Lumen walkthrough

End-to-end: from `lerd install` to a [Lumen](https://lumen.laravel.com/) API running on `https://api.test` with a queue worker and a scheduler.

::: info Prerequisites
You've already run `lerd install` once on this machine. If not, see [Installation](installation.md).
:::

::: tip Drive it from your AI assistant
Run `lerd mcp:enable-global` once and your AI assistant (Claude Code, Cursor, Junie, Codex, Gemini, Copilot, Antigravity, Windsurf) can call every command below through the grouped MCP tools: `site` `action: "link"`, `env` `action: "setup"`, `framework` `action: "setup"`, `db` `action: "create"`, `worker`, etc. See [AI Integration](../features/mcp.md).
:::

---

## 1. Create the project

Lumen ships no scaffold through `lerd new`, because Laravel has recommended Laravel itself for new API projects since Lumen went into maintenance. Existing Lumen projects are fully supported, and a new one is one composer command:

::: code-group

```bash [composer]
cd ~/Lerd
composer create-project laravel/lumen api
```

```bash [existing repo]
cd ~/Lerd
git clone git@github.com:you/api.git
cd api && lerd composer install
```

:::

---

## 2. Register the site

```bash
cd api
lerd link
```

```
 → detecting framework… ✓ lumen
 → provisioning PHP-FPM runtime… ✓ php 8.4 · nginx vhost written
 ✓ linked in 480ms

  Site        http://api.test
  PHP         8.4 · FPM
  Framework   lumen
  DB          sqlite
```

Lumen is detected from `laravel/lumen-framework` rather than from `artisan`, which it shares with Laravel, so it gets its own definition rather than Laravel's. Definitions ship for majors 6 through 11; 11 supports PHP 8.2 to 8.4.

The differences from the Laravel definition are the ones that matter in practice. There is no `npm`, because Lumen has no front end to build. There is no `storage:link` step, because there is no `public/storage` to serve. `.env` is not created from an example by Lumen itself, so lerd writes the keys your services need into it directly.

---

## 3. Configure PHP, database, services

```bash
lerd init
```

The wizard writes `.lerd.yaml`, offers HTTPS, and picks the database. Choosing MySQL writes `DB_CONNECTION`, `DB_HOST=lerd-mysql`, `DB_DATABASE`, `DB_USERNAME` and `DB_PASSWORD`, with `REDIS_*` and `MAIL_*` following the services you add.

```bash
lerd secure
lerd db:create
lerd env
```

Lumen needs `APP_KEY` set and ships no `key:generate` command to produce one. lerd generates the key itself during env setup, so the site comes up with encryption working rather than erroring on the first encrypted cookie.

::: tip Reading `.env` at all
Lumen only loads `.env` when the bootstrap file calls `Dotenv`. A project whose `bootstrap/app.php` has that line commented out will ignore everything lerd writes, which looks like the wiring failing when it is the app declining to read it.
:::

---

## 4. Bootstrap the project

```bash
lerd setup
```

Setup offers `php artisan migrate`, and `php artisan db:seed` when you want it.

---

## 5. Verify

```bash
curl -i https://api.test
lerd site:doctor
```

The doctor runs `app_debug` (debug left on in production) and `migrations` (migrations still pending), both reading the same `.env` and database lerd wired.

---

## Workers

```bash
lerd queue:start
lerd schedule:start
```

The queue worker is `php artisan queue:work --queue=default --tries=3 --timeout=60` and takes its queue, tries and timeout as flags: `lerd queue:start --queue=emails --tries=5`. What you pass is committed under `worker_options` in `.lerd.yaml`, so the next start runs the same thing without retyping it. Set `QUEUE_CONNECTION=redis` and lerd will refuse to start the worker while `lerd-redis` is down, rather than leaving it to crash-loop on a DNS error.

The scheduler runs `schedule:work`, which stays resident and fires each due task itself.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `api.test` with nginx + dnsmasq, document root `public/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `api` database, started MySQL |
| `lerd env` (via init) | Generated `APP_KEY` and wrote `DB_*`, `REDIS_*` and `MAIL_*` into `.env` |
| `lerd setup` | Ran the migrations |
| `lerd queue:start` / `schedule:start` | Launched `lerd-queue-api` and `lerd-schedule-api` |

---

## Quick commands

| Command | Runs | What it does |
|---|---|---|
| `cache:clear` | `php artisan cache:clear` | Drop the application cache |
| `migrate` | `php artisan migrate --force` | Apply outstanding migrations |
| `migrate:fresh` | `php artisan migrate:fresh --seed --force` | Drop every table, migrate again, reseed |

`migrate:fresh` asks before it runs. The [Tinker tab](../features/tinker.md) is wired to `artisan tinker` and appears once `laravel/tinker` is installed, which a Lumen project has to add itself.

---

## Moving to Laravel

Lumen is in maintenance upstream, and a project that outgrows it becomes a Laravel one. lerd does not stand in the way: change the framework in `.lerd.yaml`, or delete the key and let detection answer from `composer.json`, then `lerd link` again to re-provision the site on the Laravel definition.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Queue Workers](../usage/queue-workers.md): tuning, retries, and worker logs
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
