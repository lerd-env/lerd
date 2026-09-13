# Winter CMS walkthrough

End-to-end: from `lerd install` to a Winter CMS site running on `https://mysite.test` with its backend, a queue worker and a scheduler.

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
lerd new mysite --framework=winter
# runs: composer create-project wintercms/winter:^1.0 ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project wintercms/winter mysite
```

```bash [existing repo]
cd ~/Lerd
git clone git@github.com:you/mysite.git
```

:::

Winter's own `create-project` finishes by running `winter:install`, `winter:env` and `winter:mirror public`, so the project arrives with a `.env` and a `public/` folder already built.

---

## 2. Register the site

```bash
cd mysite
lerd link
```

```
 → detecting framework… ✓ winter
 → Using PHP 8.3 (winter supports 8.1–8.4)
 → provisioning PHP-FPM runtime… ✓ php 8.3 · node 22 · nginx vhost written
 ✓ linked in 1.2s

  Site        http://mysite.test
  PHP         8.3 · FPM
  Node        22
  Framework   winter
  DB          sqlite
```

Winter ships `artisan` and requires `laravel/framework`, so without a definition of its own it would be served as Laravel. It is detected from the `winter/wn-system-module` package instead, which also sidesteps a lock that pins `laravel/framework` to a dev branch with no major to read. The definition supports PHP 8.1 to 8.4.

---

## 3. The document root is `public/`

Winter can serve from the project root, where `index.php` sits beside `artisan`, or from the `public/` folder that `winter:mirror` builds. lerd serves `public/`, because a document root that is also the project root answers to `.env`, `composer.json` and `vendor/` over HTTP:

```bash
curl -o /dev/null -w '%{http_code}\n' http://mysite.test/.env
# 404 on public/, 200 with the project root as the document root
```

A project that predates the mirror has no `public/` and will not serve. The site doctor names it:

```
⚠ Public Mirror
    public/ is missing, so nothing is served: Winter mirrors it with
    `winter:mirror public`, and serving the project root instead would
    put .env and vendor/ on the web.
```

```bash
lerd artisan winter:mirror public --relative
```

---

## 4. Configure PHP, database, services

```bash
lerd init
```

The wizard writes `.lerd.yaml`, offers HTTPS, and picks the database. Winter's `.env` uses the same `DB_*` keys Laravel does, so choosing MySQL writes `DB_CONNECTION`, `DB_HOST=lerd-mysql`, `DB_DATABASE`, `DB_USERNAME` and `DB_PASSWORD` for you, alongside `REDIS_*` and `MAIL_*` when you add those services. SQLite lives at `storage/database.sqlite` rather than Laravel's `database/`.

```bash
lerd secure          # HTTPS on mysite.test
lerd db:create       # create the database
lerd env             # (re)write the service keys into .env
```

---

## 5. Bootstrap the project

```bash
lerd setup
```

Setup runs `php artisan winter:up`, which is how Winter migrates: `migrate` belongs to Laravel and is not the command here. It applies the module and plugin migrations together, and it is safe to run again.

---

## 6. Verify

```bash
curl -I https://mysite.test
lerd site:doctor
```

The doctor runs the three checks the definition declares: `app_debug` (debug left on in production), `public_mirror` (the folder above), and `csrf` (`ENABLE_CSRF` turned off, which lets the backend accept cross-site form posts).

The backend is at `https://mysite.test/backend` with the account `winter:install` created.

---

## Front-end assets

Winter builds assets **per package**: a theme or a plugin carries its own `vite.config.mjs` and `package.json`, while `node_modules` is hoisted to the project root as a workspace. Which packages a site watches belongs to the project, so the framework definition ships no asset worker and you declare one per package instead.

```bash
lerd artisan vite:create theme-mytheme   # writes the package's vite.config.mjs
lerd artisan vite:install theme-mytheme  # installs its dependencies
```

A theme is named `theme-<code>`, a plugin `vendor.plugin`. Point the package's own config at the values lerd generates, so the dev server advertises the site's origin rather than localhost:

```js
// themes/mytheme/vite.config.mjs
import lerd from '../../node_modules/.lerd/dev-server.mjs';

export default defineConfig({
    server: { ...lerd.server },
    // ...
});
```

Then declare the worker in `.lerd.yaml`:

```yaml
custom_workers:
  vite:
    label: Vite
    icon: vite
    command: php artisan vite:watch theme-mytheme
    restart: on-failure
    host: true
    per_worktree: true
    replaces_build: true
    dev_server:
      tool: vite
    proxy:
      paths:
        - /themes/mytheme/assets/dist
      port: pinned
      port_env_key: VITE_PORT
      default_port: 5173
```

`artisan` re-enters the PHP container through lerd's `php` shim, so the dev server listens there rather than on the host: the proxy points at the container and the pinned port is forwarded through the shim, which is why both ends agree on it. Start it with `lerd vite:start`. The first run asks once before running a project-supplied command on your host.

Add the tag to your layout and the assets are served from the site's own origin, with HMR:

```twig
{{ vite(['assets/src/css/theme-mytheme.css', 'assets/src/js/theme-mytheme.js'], 'theme-mytheme') }}
```

A site still on Laravel Mix uses the same shape with `mix:watch` and no `dev_server` block.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `public/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL |
| `lerd env` (via init) | Wrote `APP_URL`, `DB_*`, `REDIS_*` and `MAIL_*` into `.env` |
| `lerd setup` | Ran `winter:up` to apply the module and plugin migrations |
| `lerd worker start queue/schedule` | Launched `lerd-queue-mysite` and `lerd-schedule-mysite` |

---

## Quick commands

The Winter definition ships four one-click actions, available on the site's card in the dashboard, in the TUI, and to an AI assistant over MCP:

| Command | Runs | What it does |
|---|---|---|
| `winter:up` | `php artisan winter:up` | Apply outstanding module and plugin migrations |
| `cache:clear` | `php artisan cache:clear` | Drop the application and CMS caches |
| Purge thumbnails | `php artisan winter:util purge thumbs --force` | Delete generated thumbnails so they rebuild on demand |
| `plugin:refresh` | `php artisan plugin:refresh` | Roll a plugin's tables back and migrate them again |

`plugin:refresh` asks before it runs: it drops that plugin's tables.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Queue Workers](../usage/queue-workers.md): tuning, retries, and worker logs
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): Redis, Meilisearch, RustFS for S3
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
