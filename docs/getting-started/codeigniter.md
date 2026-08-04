# CodeIgniter walkthrough

End-to-end: from `lerd install` to a CodeIgniter 4 app running on `https://mysite.test` with MySQL, Redis and Mailpit.

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
lerd new mysite --framework=codeigniter
# runs: composer create-project codeigniter4/appstarter ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project codeigniter4/appstarter mysite
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
 → detecting framework… ✓ codeigniter
 → Using PHP 8.4 (codeigniter supports 8.2–8.4)
 → provisioning PHP-FPM runtime… ✓ php 8.4 · node 22 · nginx vhost written
 ✓ linked in 571ms

  Site        http://mysite.test
  PHP         8.4 · FPM
  Node        22
  Framework   codeigniter
```

CodeIgniter is detected from the `spark` console script or the `codeigniter4/framework` package, and the document root is set to `public/`. The definition declares PHP 8.2 to 8.4, so lerd picks 8.4 rather than the newest PHP installed and tells you why.

---

## 3. Configure PHP, database, services

```bash
lerd init
```

The wizard writes `.lerd.yaml`, links the site, issues the certificate, creates the database and writes the connection values:

```yaml
domains:
  - mysite
php_version: "8.4"
framework: codeigniter
framework_version: "4"
secured: true
services:
  - mailpit
  - mysql
  - redis
```

CodeIgniter's `.env` uses dotted configuration keys rather than flat names, and lerd writes them in that form, seeding the file from the `env` template that `appstarter` ships:

```dotenv
CI_ENVIRONMENT=development
app.baseURL=https://mysite.test
database.default.hostname=lerd-mysql
database.default.database=mysite
database.default.username=root
database.default.password=lerd
database.default.DBDriver=MySQLi
database.default.port=3306
encryption.key = hex2bin:67403d00c510367abee38de013b6073103e5b4b2dfa060e9dabc48a5580bb9fc
cache.handler=redis
cache.redis.host=lerd-redis
cache.redis.port=6379
email.protocol=smtp
email.SMTPHost=lerd-mailpit
email.SMTPPort=1025
```

`CI_ENVIRONMENT=development` turns on the debug toolbar and full error pages, and `encryption.key` is generated on the first run so sessions and encrypted cookies work without another command. Choosing PostgreSQL in the wizard writes `DBDriver=Postgre` and the matching host and port instead.

---

## 4. Bootstrap the project

```bash
lerd setup
```

```
? Setup steps
  [ ] composer install
  [ ] lerd mcp:inject
  [•] Run migrations
  [•] Clear cache
```

```
 → Run migrations… ✓
 → Clear cache… ✓
 → lerd open… ✓
 ✓ setup complete in 243ms
```

The two CodeIgniter steps are `php spark migrate` and `php spark cache:clear`.

::: info One-shot
`lerd setup --all` skips the prompt and runs every pre-selected step. Useful in scripts or after a fresh clone.
:::

---

## 5. Verify

```bash
lerd status
```

The site answers on `https://mysite.test` with a trusted certificate. Run any Spark command through the project's container with `lerd console`:

```bash
lerd console migrate --all
```

```
CodeIgniter v4.7.4 Command Line Tool - Server Time: 2026-08-04 11:35:48 UTC+00:00

Running all new migrations...
Migrations complete.
```

Application logs are picked up from `writable/logs/*.log` and shown in the [Web UI](../features/web-ui.md) at `http://127.0.0.1:7073` under the **App Logs** tab.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `public/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL, Redis and Mailpit |
| `lerd env` (via init) | Wrote the `database.*`, `cache.*` and `email.*` keys plus `app.baseURL` and `encryption.key` into `.env` |
| `lerd setup` | Ran migrations and cleared the cache |

---

## Queue worker

CodeIgniter's queue lives in a separate package, so the worker appears once it is installed:

```bash
lerd composer require codeigniter4/queue
lerd worker start queue
```

It runs `php spark queue:work default` under systemd with `restart: always`. `lerd queue:start` takes `--queue` and `--tries` to run a different queue or change the retry count; CodeIgniter has no per-job timeout flag, so `--timeout` does not apply here. See [Queue Workers](../usage/queue-workers.md).

---

## Quick commands

The CodeIgniter definition ships six one-click actions, available on the site's card in the dashboard, in the TUI, and to an AI assistant over MCP:

| Command | Runs | What it does |
|---|---|---|
| `cache:clear` | `php spark cache:clear` | Clear every configured cache pool |
| `migrate` | `php spark migrate` | Apply pending migrations |
| `migrate:rollback` | `php spark migrate:rollback` | Roll back the last migration batch |
| `queue:retry` | `php spark queue:retry all` | Retry every failed queue job |
| `queue:failed` | `php spark queue:failed` | List failed queue jobs |
| `queue:flush` | `php spark queue:flush` | Remove all failed queue jobs |

The three queue actions only appear when `codeigniter4/queue` is installed. `migrate:rollback` and `queue:flush` ask for confirmation first, since both throw work away.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): Meilisearch for search, RustFS for S3, or a database you run on the host
- [HTTPS](../features/https.md): wildcard certs for git worktrees
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
