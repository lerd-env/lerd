# Drupal walkthrough

End-to-end: from `lerd install` to a Drupal 11 site running on `https://mysite.test` with MySQL, Redis, Mailpit and a cron worker.

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
lerd new mysite --framework=drupal
# runs: composer create-project drupal/recommended-project ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project drupal/recommended-project mysite
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
 → detecting framework… ✓ drupal
 → provisioning PHP-FPM runtime… ✓ php 8.5 · node 22 · nginx vhost written
 ✓ linked in 1.3s

  Site        http://mysite.test
  PHP         8.5 · FPM
  Node        22
  Framework   drupal
```

Drupal is detected from `drupal/core-recommended` or `drupal/core` in `composer.json`, and the document root is set to `web/`, so nothing outside it is ever served.

::: info Already parked?
If `~/Lerd` was registered with `lerd park ~/Lerd` earlier, every subdirectory under it is auto-linked and you can skip `lerd link`.
:::

---

## 3. Configure PHP, database, services

```bash
lerd init
```

The wizard asks for the PHP version, Node version, HTTPS, a database and any services. Drupal runs on PHP 8.3 to 8.5 and has no front-end build of its own, so leaving the Node field blank is fine. Pick MySQL or PostgreSQL, and add `redis` and `mailpit` if you want a cache backend and a mail catcher.

The answers land in `.lerd.yaml`:

```yaml
domains:
  - mysite
php_version: "8.5"
framework: drupal
framework_version: "11"
secured: true
services:
  - mailpit
  - mysql
  - redis
```

Commit that file; on another machine `lerd link` reads it and restores the same setup without re-running the wizard. `lerd init` also links the site, issues the TLS certificate, creates the project database and writes the connection values, so the site is already on `https://mysite.test` when the wizard exits.

---

## 4. Add Drush

Everything lerd knows how to do with a Drupal site goes through Drush: the setup steps, the cron worker, and the quick actions in the dashboard. `drupal/recommended-project` does not ship it, so install it before going further:

```bash
lerd composer require drush/drush
```

Until Drush is present, `lerd setup` offers only `composer install` and `lerd mcp:inject`, the cron worker does not appear, and `lerd run` reports no commands at all. That is deliberate, every one of them is gated on the package being installed, but it does mean a fresh project looks emptier than it is.

---

## 5. Install the site

```bash
lerd run site:install --yes
```

```
 [notice] Performed install task: install_base_system
 [notice] Performed install task: install_profile_modules
 [notice] Performed install task: install_finished
 [success] Installation complete. (Admin)
```

That installs against the database `lerd init` created, with an admin user of `admin` / `lerdadmin1`. It asks for confirmation in the dashboard and the TUI because it creates a schema; `--yes` is what skips that prompt on the CLI.

::: warning Drupal does not read `.env`
`lerd init` writes `DB_DRIVER`, `DB_HOST`, `DB_NAME`, `DB_USER` and `DB_PASSWORD` into `.env`, which is where lerd records what it provisioned and where the dashboard reads it from. Drupal itself has no dotenv wiring in `drupal/recommended-project`, so it never reads that file. That is why the install command passes `--db-url` explicitly: without it, Drush has nothing to connect to on a fresh project and fails with `Call to a member function getInstallTasks() on null`. Once it has run, the real credentials live in `web/sites/default/settings.php`, which is what Drupal loads from then on.

The `.env` values still matter if you wire `settings.php` up to read them yourself, which is the usual pattern for a project that keeps credentials out of the repo.
:::

The command points at lerd's MySQL. On a PostgreSQL project, run the install by hand with the connection your project uses:

```bash
lerd console site:install --db-url=pgsql://postgres:lerd@lerd-postgres:5432/mysite --yes
```

---

## 6. Run the remaining setup steps

```bash
lerd setup
```

With Drush installed the full step list appears, pre-selecting what a normal run needs:

```
? Setup steps
  [ ] composer install
  [ ] lerd mcp:inject
  [•] Run database updates
  [•] Rebuild cache
  [ ] Import config
  [ ] cron:start
```

`Run database updates` and `Rebuild cache` are the two that matter after a pull. `Import config` runs `drush config:import` for a project that tracks its configuration in the repo, and `cron:start` launches the cron worker as a systemd user service.

::: info One-shot
`lerd setup --all` skips the prompt and runs every step, not only the pre-selected ones. Installing the site is deliberately a command rather than a setup step, so a blanket run can never drop and recreate the schema of a store you already have.
:::

---

## 7. Verify

```bash
lerd status
```

The site answers on `https://mysite.test` with a trusted certificate, and the cron worker shows as running once started:

```bash
lerd worker start cron
```

```
 → starting Cron… ✓
   logs: journalctl --user -u lerd-cron-mysite -f
```

Live logs are in the [Web UI](../features/web-ui.md) at `http://127.0.0.1:7073` under the **App Logs** tab.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `web/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL, Redis and Mailpit |
| `lerd env` (via init) | Wrote `DB_*`, `REDIS_*` and `SMTP_*` into `.env` |
| `drush site:install` | Installed Drupal and wrote the connection into `web/sites/default/settings.php` |
| `lerd worker start cron` | Launched `lerd-cron-mysite`, which runs `drush cron` under systemd |

---

## Drush quick commands

The Drupal definition ships six one-click actions, available from `lerd run`, on the site's card in the dashboard, in the TUI, and to an AI assistant over MCP:

| Command | Runs | What it does |
|---|---|---|
| `site:install` | `drush site:install …` | Install a fresh site (confirm-gated) |
| `cr` | `drush cr` | Rebuild all Drupal caches |
| `uli` | `drush uli --uri=…` | Generate a one-time admin login URL |
| `updb` | `drush updb -y` | Apply pending `hook_update_N` updates |
| `cex` | `drush cex -y` | Export configuration to the sync directory |
| `cim` | `drush cim -y` | Import configuration from the sync directory |

`uli` returns a URL on the site's own domain, so the dashboard shows it as a link you can open straight into an authenticated session. `cim` asks for confirmation first, since it overwrites live configuration. All six stay hidden until `drush/drush` is installed.

Any other Drush command works through the console:

```bash
lerd console cache:rebuild
lerd console status
```

The [Tinker tab](../features/tinker.md) is wired to `drush php:eval`, so you can evaluate code against a booted Drupal from the dashboard.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): Meilisearch for search, RustFS for S3, or a database you run on the host
- [HTTPS](../features/https.md): wildcard certs for git worktrees
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
