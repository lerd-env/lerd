# Tempest walkthrough

End-to-end: from `lerd install` to a Tempest 3 app running on `https://mysite.test` with MySQL, Mailpit and a scheduler.

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
lerd new mysite --framework=tempest
# runs: composer create-project tempest/app ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project tempest/app mysite
```

```bash [existing repo]
cd ~/Lerd
git clone git@github.com:you/mysite.git
```

:::

Tempest requires PHP 8.5, which is the only version the definition declares, so lerd will not offer you an older one.

---

## 2. Register the site

```bash
cd mysite
lerd link
```

```
 → detecting framework… ✓ tempest
 → provisioning PHP-FPM runtime… ✓ php 8.5 · node 22 · nginx vhost written
 ✓ linked in 387ms

  Site        http://mysite.test
  PHP         8.5 · FPM
  Node        22
  Framework   tempest
```

Tempest is detected from the `tempest/framework` package or the `tempest` console script, and the document root is set to `public/`.

---

## 3. Configure PHP, database, services

```bash
lerd init
```

The wizard writes `.lerd.yaml`, links the site, issues the certificate, creates the database and writes the connection values:

```yaml
domains:
  - mysite
php_version: "8.5"
framework: tempest
framework_version: "3"
secured: true
services:
  - mailpit
  - mysql
  - redis
```

Tempest names its environment keys after the thing rather than after a convention borrowed from elsewhere, and lerd writes them in that shape. `SIGNING_KEY` is generated on the first run:

```dotenv
SIGNING_KEY=QMjwajsuAmUqoaJrpXSr9qyCtiA7vt0mTJeALmXShHs=
BASE_URI=https://mysite.test
DATABASE_HOST=lerd-mysql
DATABASE_PORT=3306
DATABASE_DATABASE=mysite
DATABASE_USERNAME=root
DATABASE_PASSWORD=lerd
MAIL_SMTP_SCHEME=smtp
MAIL_SMTP_HOST=lerd-mailpit
MAIL_SMTP_PORT=1025
```

`BASE_URI` is the key Tempest reads for the application URL, so absolute links and generated URLs follow the `.test` domain with nothing else to set.

---

## 4. Bootstrap the project

```bash
lerd setup
```

```
? Setup steps
  [ ] composer install
  [ ] lerd mcp:inject
  [•] Generate discovery cache
  [ ] schedule:start
```

Discovery is how Tempest finds your controllers, commands and everything else it wires up, and the cache is what keeps it from walking the whole project on every request. Generating it is the one step a fresh project always wants.

`Run migrations` joins the list once the project has `tempest/database`, and a Vite worker appears once `node_modules/vite` exists, replacing the one-off build step with a running dev server.

::: info One-shot
`lerd setup --all` skips the prompt and runs every pre-selected step. Useful in scripts or after a fresh clone.
:::

---

## 5. Verify

```bash
lerd console discovery:status
```

```
// DISCOVERY STATUS
Registered locations ........... 36
Loaded discovery classes ....... 36
Cache .......................... ENABLED
Cache strategy ................. PARTIAL
Cache validity ................. OK
```

`lerd console` runs the `tempest` script inside the project's container, so any Tempest command works the same way. The site answers on `https://mysite.test` with a trusted certificate, and logs are read from `.tempest/logs/*.log` and parsed as Monolog for the **App Logs** tab in the [Web UI](../features/web-ui.md).

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `public/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL and Mailpit |
| `lerd env` (via init) | Generated `SIGNING_KEY` and wrote `BASE_URI`, `DATABASE_*` and `MAIL_SMTP_*` into `.env` |
| `lerd setup` | Generated the discovery cache |

---

## Workers

Tempest declares three, each appearing only when the project can actually run it:

| Worker | Runs | Appears when |
|---|---|---|
| Task Scheduler | `php tempest schedule:run` | always |
| Async Command Monitor | `php tempest command:monitor` | `tempest/command-bus` is installed |
| Vite | `npm run dev` | `node_modules/vite` exists |

```bash
lerd worker start schedule
```

The scheduler is not a process that stays up. On Linux lerd installs it as a systemd timer that fires every minute and a oneshot service that does the run, so between ticks the service is `inactive dead` while the timer sits `active waiting`. That is the healthy state, not a stopped worker.

```
lerd-schedule-mysite.timer      active   waiting   Lerd Task Scheduler timer (mysite)
lerd-schedule-mysite.service    inactive dead      Lerd Task Scheduler (mysite)
```

::: warning Scheduled workers on macOS
This is the one part of the walkthrough that does not work on macOS yet. lerd refuses scheduled workers there with `scheduled workers aren't supported on macOS yet`, because the launchd equivalent of the timer has not been wired through. Everything else on this page behaves the same on both platforms, and the Async Command Monitor and Vite workers, which are ordinary long-running processes, run on macOS fine.
:::

---

## Quick commands

The Tempest definition ships three one-click actions, available on the site's card in the dashboard, in the TUI, and to an AI assistant over MCP:

| Command | Runs | What it does |
|---|---|---|
| `discovery:clear` | `php tempest discovery:clear` | Drop the discovery cache |
| `migrate` | `php tempest migrate:up --force` | Apply pending migrations |
| `cache:clear` | `php tempest cache:clear --force` | Clear every cache |

`discovery:clear` is the one to reach for after adding a class that Tempest does not seem to see: the cache is answering from before it existed.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Queue Workers](../usage/queue-workers.md): worker lifecycle, logs, and self-healing
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): Meilisearch for search, RustFS for S3, or a database you run on the host
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
