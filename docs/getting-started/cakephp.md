# CakePHP walkthrough

End-to-end: from `lerd install` to a CakePHP 5 app running on `https://mysite.test` with MySQL, Redis and Mailpit.

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
lerd new mysite --framework=cakephp
# runs: composer create-project cakephp/app ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project cakephp/app mysite
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
 → detecting framework… ✓ cakephp
 → Using PHP 8.4 (cakephp supports 8.1–8.4)
 → provisioning PHP-FPM runtime… ✓ php 8.4 · node 22 · nginx vhost written
 ✓ linked in 1.4s

  Site        http://mysite.test
  PHP         8.4 · FPM
  Node        22
  Framework   cakephp
```

CakePHP is detected from `bin/cake` or the `cakephp/cakephp` package, and the document root is set to `webroot/`. The definition declares support for PHP 8.1 to 8.4, so lerd picks 8.4 rather than the newest PHP on the machine and says so on the way past.

---

## 3. Configure PHP, database, services

```bash
lerd init
```

Answer the wizard and it writes `.lerd.yaml`, links the site, issues the certificate, creates the database and writes the connection values:

```yaml
domains:
  - mysite
php_version: "8.4"
framework: cakephp
framework_version: "5"
secured: true
services:
  - mailpit
  - mysql
  - redis
```

Those connection values go into `config/.env`, which lerd seeds from the `config/.env.example` that `cakephp/app` ships:

```dotenv
APP_URL=https://mysite.test
CACHE_URL=redis://lerd-redis:6379
DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/mysite
EMAIL_TRANSPORT_DEFAULT_URL=smtp://lerd-mailpit:1025
```

---

## 4. Turn on `.env` loading

This is the step that catches people out. CakePHP ships `.env` support switched **off**: the loader block in `config/bootstrap.php` is commented out, so nothing reads `config/.env` and `config/app_local.php` falls back to its placeholder `localhost` / `my_app` / `secret` credentials. A console command fails on `PDO->__construct()` and a page that touches the database throws.

Uncomment the block:

```php
// config/bootstrap.php
if (!env('APP_NAME') && file_exists(CONFIG . '.env')) {
    $dotenv = new \josegonzalez\Dotenv\Loader([CONFIG . '.env']);
    $dotenv->parse()
        ->putenv()
        ->toEnv()
        ->toServer();
}
```

`config/app_local.php` already reads `env('DATABASE_URL')` for the default datasource, so once the loader runs, lerd's connection wins over the placeholders with nothing else to change. Verify with:

```bash
lerd php bin/cake.php migrations status
```

```
using migration table cake_migrations

There are no available migrations. Try creating one using the create command.
```

Reaching the migrations table at all means the app is talking to `lerd-mysql`.

::: info `APP_FULL_BASE_URL`
lerd writes the site address as `APP_URL`, which CakePHP does not read. Its own key is `APP_FULL_BASE_URL`, and `config/.env.example` seeds it with `https://example.com`. Set it to `https://mysite.test` if your app generates absolute URLs.
:::

---

## 5. Bootstrap the project

```bash
lerd setup
```

```
? Setup steps
  [ ] composer install
  [ ] lerd mcp:inject
  [•] Run migrations
  [•] Clear cache
  [•] Build schema cache
```

```
 → Run migrations… ✓
 → Clear cache… ✓
 → Build schema cache… ✓
 → lerd open… ✓
 ✓ setup complete in 374ms
```

The three CakePHP steps are `bin/cake migrations migrate`, `bin/cake cache clear_all` and `bin/cake schema_cache build`, which is also the right sequence after pulling a branch that changed the schema.

::: info One-shot
`lerd setup --all` skips the prompt and runs every pre-selected step. Useful in scripts or after a fresh clone.
:::

---

## 6. Verify

```bash
lerd status
```

The site answers on `https://mysite.test` with a trusted certificate. Application logs are picked up from `logs/*.log` and shown in the [Web UI](../features/web-ui.md) at `http://127.0.0.1:7073` under the **App Logs** tab.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `webroot/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL, Redis and Mailpit |
| `lerd env` (via init) | Wrote `DATABASE_URL`, `CACHE_URL` and `EMAIL_TRANSPORT_DEFAULT_URL` into `config/.env` |
| `lerd setup` | Ran migrations, cleared the cache, built the schema cache |

---

## Queue worker

The queue worker only appears once the project has the queue plugin:

```bash
lerd composer require cakephp/queue
lerd worker start queue
```

It runs `bin/cake queue worker` under systemd with `restart: always`, so a crash brings it straight back. See [Queue Workers](../usage/queue-workers.md) for tuning and log access.

---

## Quick commands

The CakePHP definition ships three one-click actions, available on the site's card in the dashboard, in the TUI, and to an AI assistant over MCP:

| Command | Runs | What it does |
|---|---|---|
| `cache:clear` | `bin/cake cache clear_all` | Clear every configured cache pool |
| `migrate` | `bin/cake migrations migrate` | Apply pending migrations |
| `schema:cache:clear` | `bin/cake schema_cache clear` | Drop the cached database schema |

`migrate` only shows up when `cakephp/migrations` is installed.

::: warning Running the console yourself
`bin/cake` is a shell wrapper, not a PHP file, so `lerd console` cannot run it. Use the PHP entry point instead:

```bash
lerd php bin/cake.php migrations migrate
lerd php bin/cake.php cache clear_all
```

The setup steps and the quick actions above are unaffected; they go through a shell and use `bin/cake` directly.
:::

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): Meilisearch for search, RustFS for S3, or a database you run on the host
- [HTTPS](../features/https.md): wildcard certs for git worktrees
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
