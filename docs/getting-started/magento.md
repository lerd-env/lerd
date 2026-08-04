# Magento walkthrough

End-to-end: from `lerd install` to a Magento 2 store running on `https://mysite.test` with MySQL, OpenSearch, a cron worker and queue consumers.

::: info Prerequisites
You've already run `lerd install` once on this machine. If not, see [Installation](installation.md).
:::

::: tip Drive it from your AI assistant
Run `lerd mcp:enable-global` once and your AI assistant (Claude Code, Cursor, Junie, Codex, Gemini, Copilot, Antigravity, Windsurf) can call every command below through the grouped MCP tools: `framework` `action: "project_new"`, `site` `action: "link"`, `env` `action: "setup"`, `framework` `action: "setup"`, `db` `action: "create"`, `worker`, etc. See [AI Integration](../features/mcp.md).
:::

---

## What is different about Magento

Worth knowing before you start, because each one changes a step below:

- **A search engine is not optional.** Magento 2.4 removed the MySQL catalog search engine, so a store cannot boot without one. The definition declares OpenSearch as a requirement and lerd installs and starts it for you.
- **There is no web installer.** It was removed in 2.4, so a fresh store is installed from the CLI.
- **Configuration lives in `app/etc/env.php`**, a PHP file returning a nested array, not in a `.env`. lerd writes into it by dotted path.
- **The document root is `pub/`**, and `/setup`, `/static` and `/media` need nginx rules of their own. lerd's vhost ships them.
- **The CLI needs far more memory than PHP's default.** Magento's console cannot even bootstrap in 128M, so the definition raises the CLI limit to 2G. Adobe asks for the same.

---

## 1. Create the project

::: code-group

```bash [lerd new]
cd ~/Lerd
lerd new mysite --framework=magento
# runs: composer create-project --repository-url=https://mirror.mage-os.org/ \
#         magento/project-community-edition ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project --repository-url=https://mirror.mage-os.org/ \
  magento/project-community-edition mysite
```

```bash [existing repo]
cd ~/Lerd
git clone git@github.com:you/mysite.git
```

:::

The Mage-OS mirror is used rather than `repo.magento.com`, so you do not need Adobe Commerce keys to get a store on disk. Point the repository URL at `repo.magento.com` and use your own keys if you need the Adobe distribution specifically.

Expect this to take a few minutes and about 750 MB.

---

## 2. Register the site

```bash
cd mysite
lerd link
```

```
 → detecting framework… ✓ magento
 → provisioning PHP-FPM runtime… ✓ php 8.5 · node 22 · nginx vhost written
 ✓ linked in 459ms

  Site        http://mysite.test
  PHP         8.5 · FPM
  Node        22
  Framework   magento
```

Magento is detected from `bin/magento` or the `magento/product-community-edition` package, and the document root is set to `pub/`. The vhost lerd writes carries the extra rules Magento needs: `/setup` served from outside `pub/`, `/static/` falling back to `pub/static.php` for assets generated on the fly in developer mode, and `/media/` falling back to `pub/get.php`. Without those an uninstalled store redirects to `/setup/` forever and developer-mode assets never resolve.

---

## 3. Configure PHP, database, services

```bash
lerd init
```

Pick MySQL when the wizard asks. You do not have to pick OpenSearch: the definition requires it, so lerd installs the preset, starts it, and records it in `.lerd.yaml` on its own.

```yaml
domains:
  - mysite
php_version: "8.5"
framework: magento
framework_version: "2"
secured: true
services:
  - mysql
  - opensearch:
      preset: opensearch
```

The connection values go into `app/etc/env.php` by dotted path:

```php
<?php
return [
    'system' => [
        'default' => [
            'catalog' => [
                'search' => [
                    'engine' => 'opensearch',
                    'opensearch_index_prefix' => 'mysite',
                    'opensearch_server_hostname' => 'lerd-opensearch',
                    'opensearch_server_port' => '9200',
                ],
            ],
        ],
    ],
    'db' => [
        'connection' => [
            'default' => [
                'active' => '1',
                'dbname' => 'mysite',
                'host' => 'lerd-mysql',
                'password' => 'lerd',
                'username' => 'root',
            ],
        ],
    ],
];
```

The port is `9200` because that is the container-internal port on the `lerd` network. If lerd published OpenSearch on a shifted host port because 9200 was taken, that affects host tools only, not the store.

::: info Where the base URL lives
Magento keeps its base URL in the database, in `core_config_data`, so there is no URL key for lerd to write here. It is set once during the install, from the `--base-url` the next step passes.
:::

---

## 4. Install the store

```bash
lerd run setup:install --yes
```

Before the store exists, that is the only command `lerd run` offers; everything else is gated on `app/etc/config.php`, which `setup:install` writes. It runs the full CLI install against lerd's MySQL and OpenSearch, with an admin user of `admin` / `lerdadmin1`:

```
Module 'PayPal_BraintreeReward':
[Progress: 1476 / 1480]
Disabling Maintenance Mode:
[Progress: 1479 / 1480]
Indexing...
14 indexer(s) are indexed.
[Progress: 1480 / 1480]
[SUCCESS]: Magento installation complete.
[SUCCESS]: Magento Admin URI: /admin_h8va586
```

Note the admin URI. Magento randomises it at install time, so yours will differ, and it is the only way into the admin panel.

Allow ten minutes or so. The command is confirm-gated in the dashboard and the TUI because it creates a schema and an admin user; `--yes` is what skips that prompt on the CLI.

---

## 5. Run the remaining setup steps

```bash
lerd setup
```

Now that the store is installed the rest of the list appears:

```
? Setup steps
  [ ] composer install
  [ ] lerd mcp:inject
  [•] Apply schema and data updates
  [•] Flush the cache
  [ ] Switch to developer mode
  [ ] Compile dependency injection
  [ ] Reindex everything
  [ ] cron:start
  [ ] consumers:start
```

`setup:upgrade` and `cache:flush` are pre-selected because they are what a normal run after a pull needs. Switching to developer mode is worth doing on a local store: templates and static assets are then served without a deploy step, at the cost of speed.

::: info One-shot
`lerd setup --all` skips the prompt and runs every pre-selected step. Note that this deliberately does not include `setup:install`, which is a quick command rather than a setup step precisely so a blanket run can never reinstall a store.
:::

---

## 6. Verify

```bash
lerd site:doctor
```

```
  ✓ Required Services
  ✓ Env File
  ✓ Installed
  ✓ Db Status
  ✓ Deploy Mode
  ✓ Composer Dependencies
  ✓ Composer Audit
  ✓ PHP Version
```

Three of those come from the Magento definition. **Installed** looks for `app/etc/config.php` and offers `setup:install` as its fix. **Db Status** runs `setup:db:status` and fails when the module code has moved ahead of the database schema, offering `setup:upgrade`. **Deploy Mode** warns when the store is in production mode, where template changes need a static content deploy before they show up.

The storefront answers on `https://mysite.test` and the admin on `https://mysite.test/admin_h8va586` with the URI from the install. A first request to an uncached store takes several seconds; that is Magento, not lerd, and the site doctor's response-time check will say so.

Logs are read from `var/log/*.log` and parsed as Monolog for the **App Logs** tab in the [Web UI](../features/web-ui.md).

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `pub/`, plus the `/setup`, `/static/` and `/media/` rules |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL, installed and started OpenSearch |
| `lerd env` (via init) | Wrote the `db.connection.default.*` and `system.default.catalog.search.*` paths into `app/etc/env.php` |
| `lerd run setup:install` | Installed the store, created the schema and the admin user, set the base URL |
| `lerd setup` | Applied schema and data updates, flushed the cache |

---

## Workers

```bash
lerd worker start cron
lerd worker start consumers
```

The two behave differently on purpose:

```
lerd-cron-mysite.timer            active   waiting   Lerd Cron timer (mysite)
lerd-cron-mysite.service          inactive dead      Lerd Cron (mysite)
lerd-consumers-mysite.service     active   running   Lerd Queue Consumers (mysite)
```

Cron is a scheduled worker: a systemd timer fires `bin/magento cron:run` every minute and the service goes back to `inactive dead` in between. That is the healthy state. The consumers worker is an ordinary long-running process running `queue:consumers:start async.operations.all`, restarted automatically if it dies, and it only appears when `magento/module-asynchronous-operations` is present.

Both run with the 2G CLI memory limit from the definition. Without it neither can bootstrap, and they crash-loop instead.

::: warning Cron does not run on macOS yet
lerd refuses scheduled workers on macOS with `scheduled workers aren't supported on macOS yet`, because the launchd equivalent of the timer has not been wired through. The consumers worker is unaffected and runs normally. Everything else on this page behaves the same on both platforms; on macOS, run `lerd console cron:run` by hand when you need the queue drained.
:::

---

## Quick commands

Once the store is installed, the definition's actions are available on the site's card in the dashboard, in the TUI, over MCP, and from `lerd run`:

| Command | Runs | What it does |
|---|---|---|
| `setup:install` | `bin/magento setup:install …` | Install a fresh store (confirm-gated) |
| `cache:flush` | `bin/magento cache:flush` | Purge every cache storage, full page cache included |
| `cache:clean` | `bin/magento cache:clean` | Invalidate cached entries without emptying storage |
| `setup:upgrade` | `bin/magento setup:upgrade` | Apply pending schema and data updates |
| `setup:di:compile` | `bin/magento setup:di:compile` | Regenerate DI and interceptor classes |
| `indexer:reindex` | `bin/magento indexer:reindex` | Rebuild every index |
| `deploy:mode:developer` | `bin/magento deploy:mode:set developer` | Put the store in developer mode |
| `maintenance:enable` | `bin/magento maintenance:enable` | Take the storefront offline (confirm-gated) |
| `maintenance:disable` | `bin/magento maintenance:disable` | Bring it back online |

Any other Magento command works through the console:

```bash
lerd console cache:status
lerd console indexer:status
```

---

## Git worktrees

Magento is the one framework where a worktree cannot share the parent's database, and the definition marks database isolation as **required** rather than optional. A worktree is served on its own subdomain, so its `base_url` differs, and `app/etc/env.php` overrides the base URL held in the database. That changes the config hash Magento stores, and it refuses to serve until the hash is re-imported.

So each worktree gets its own database, seeded from the parent, and `app:config:import` runs against it. Running that import against the parent's database would rewrite the parent's hash to match the worktree's URL and break the parent instead, which is exactly why isolation is not left to choice here. See [Git worktrees](../features/git-worktrees.md) for the flow itself.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): OpenSearch Dashboards, RustFS for S3, or a database you run on the host
- [Git worktrees](../features/git-worktrees.md): per-branch stores with isolated databases
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
