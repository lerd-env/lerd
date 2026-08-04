# Statamic walkthrough

End-to-end: from `lerd install` to a Statamic 6 site running on `https://mysite.test` with its control panel, a queue worker and a scheduler.

::: info Prerequisites
You've already run `lerd install` once on this machine. If not, see [Installation](installation.md).
:::

::: tip Drive it from your AI assistant
Run `lerd mcp:enable-global` once and your AI assistant (Claude Code, Cursor, Junie, Codex, Gemini, Copilot, Antigravity, Windsurf) can call every command below through the grouped MCP tools: `framework` `action: "project_new"`, `site` `action: "link"`, `env` `action: "setup"`, `framework` `action: "setup"`, `db` `action: "create"`, `worker`, etc. See [AI Integration](../features/mcp.md).
:::

---

## 1. Create the project

::: code-group

```bash [lerd new]
cd ~/Lerd
lerd new mysite --framework=statamic
# runs: composer create-project statamic/statamic ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project statamic/statamic mysite
```

```bash [existing repo]
cd ~/Lerd
git clone git@github.com:you/mysite.git
```

:::

---

## 2. Register the site

```bash
cd mysite
lerd link
```

```
 → detecting framework… ✓ statamic
 → provisioning PHP-FPM runtime… ✓ php 8.5 · node 22 · nginx vhost written
 ✓ linked in 428ms

  Site        http://mysite.test
  PHP         8.5 · FPM
  Node        22
  Framework   statamic
  DB          sqlite · cache file
```

Statamic is detected from the `statamic/cms` package rather than from a file, so it is recognised as Statamic and not as the Laravel underneath it. The document root is `public/`, and the definition supports PHP 8.3 to 8.5.

The `DB` line reads `sqlite` because that is what a fresh Statamic ships with. Statamic keeps content as flat files in `content/`, so a database is optional; the next step is where you decide.

---

## 3. Configure PHP, database, services

```bash
lerd init
```

Statamic declares a queue worker and a scheduler, so the wizard has an extra **Workers** step where you pick which of them should start automatically. The answers land in `.lerd.yaml`:

```yaml
domains:
  - mysite
php_version: "8.5"
framework: statamic
framework_version: "6"
secured: true
services:
  - mysql
workers:
  - queue
  - schedule
```

The connection values go into `.env` in the Laravel-shaped keys Statamic inherits, and `APP_KEY` is generated on the first run:

```dotenv
APP_KEY=base64:02rAW0mc+HSxO8ghCu6bGzYSKdeG3v45DKvH6/l6w/Q=
APP_URL=https://mysite.test
DB_CONNECTION=mysql
DB_HOST=lerd-mysql
DB_PORT=3306
DB_DATABASE=mysite
DB_USERNAME=root
DB_PASSWORD=lerd
REDIS_HOST=lerd-redis
MAIL_HOST=lerd-mailpit
```

Sticking with flat-file content is a perfectly good answer here. Pick a database when you are using Statamic's Eloquent driver for entries, users or forms, or when the project has Laravel code of its own behind the CMS.

---

## 4. Bootstrap the project

```bash
lerd setup
```

```
? Setup steps
  [ ] composer install
  [•] npm install/ci
  [ ] lerd mcp:inject
  [•] npm run build
  [•] php artisan storage:link
  [•] php please install
  [•] Refresh stache
  [•] queue:start
  [•] schedule:start
```

```
 → npm install/ci… ✓
 → npm run build… ✓
 → php artisan storage:link… ✓
 → php please install… ✓
 → Refresh stache… ✓
 → queue:start… ✓
 → schedule:start… ✓
 → lerd open… ✓
 ✓ setup complete in 47.3s
```

`php please install` is Statamic's own installer, and `stache:refresh` rebuilds the Stache, the index Statamic keeps over the flat-file content. Both belong in a post-pull routine along with the front-end build.

::: info One-shot
`lerd setup --all` skips the prompt and runs every pre-selected step. Useful in scripts or after a fresh clone.
:::

---

## 5. Create a control panel user

```bash
lerd php please make:user
```

The control panel lives at `https://mysite.test/cp` and redirects to its login until the first user exists.

---

## 6. Verify

```bash
lerd site:doctor
```

```
  ✓ Env File
  ✓ App Key
  ✓ Env Drift
  ✓ App Debug
  ✓ Storage Link
  ✓ Composer Dependencies
  ✓ Composer Audit
  ✓ Node Dependencies
  ✓ Node Audit
  ✓ PHP Version

  all checks pass
```

**App Debug** and **Storage Link** are the two checks the Statamic definition adds: the first warns when `APP_DEBUG` is on while `APP_ENV=production`, which would leak stack traces and config; the second catches a missing `public/storage` symlink, which silently makes files on the public disk unreachable.

Both workers run as systemd user services:

```
lerd-queue-mysite.service       active running Lerd Queue Worker (mysite)
lerd-schedule-mysite.service    active running Lerd Task Scheduler (mysite)
```

Application logs come from `storage/logs/*.log` and are parsed as Monolog, so the [Web UI](../features/web-ui.md) shows them level by level under the **App Logs** tab.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `public/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL |
| `lerd env` (via init) | Generated `APP_KEY` and wrote `APP_URL`, `DB_*`, `REDIS_*` and `MAIL_*` into `.env` |
| `lerd setup` | Installed and built front-end assets, linked storage, ran the Statamic installer, refreshed the Stache |
| `lerd worker start queue/schedule` (via setup) | Launched `lerd-queue-mysite` and `lerd-schedule-mysite` |

---

## Quick commands

The Statamic definition ships three one-click actions, available on the site's card in the dashboard, in the TUI, and to an AI assistant over MCP:

| Command | Runs | What it does |
|---|---|---|
| `cache:clear` | `php please cache:clear` | Clear the Statamic and Laravel caches |
| `stache:warm` | `php please stache:warm` | Pre-build the Stache index |
| `search:update` | `php please search:update --all` | Reindex every configured search index |

`stache:warm` is the one to reach for after a large content import, so the first request does not pay for the rebuild.

The [Tinker tab](../features/tinker.md) is wired to `artisan tinker`, so you can query entries and collections against a booted app from the dashboard.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Queue Workers](../usage/queue-workers.md): tuning, retries, and worker logs
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): Meilisearch for search, RustFS for S3, or a database you run on the host
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
