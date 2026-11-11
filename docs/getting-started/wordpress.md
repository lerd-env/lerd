# WordPress walkthrough

End-to-end: from `lerd install` to a WordPress site running on `https://myblog.test` with MySQL.

::: info Prerequisites
You've already run `lerd install` once on this machine. If not, see [Installation](installation.md).
:::

::: tip Drive it from your AI assistant
Run `lerd mcp:enable-global` once and your AI assistant (Claude Code, Cursor, Junie, Codex, Gemini, Copilot, Antigravity, Windsurf) can call every command below through the grouped MCP tools: `site` `action: "link"`, `env` `action: "setup"`, `framework` `action: "setup"`, `db` `action: "create"`, `site` `action: "tls_enable"`, etc. See [AI Integration](../features/mcp.md).
:::

---

## 1. Download WordPress

::: code-group

```bash [wp-cli]
cd ~/Lerd
wp core download --path=myblog
```

```bash [curl + tar]
cd ~/Lerd
mkdir myblog && cd myblog
curl -O https://wordpress.org/latest.tar.gz
tar -xzf latest.tar.gz --strip-components=1
rm latest.tar.gz
```

:::

Then create the config file from the sample that ships with it:

```bash
cd ~/Lerd/myblog
cp wp-config-sample.php wp-config.php
```

Do this before linking. `wp-config.php` is where lerd writes WordPress's configuration, and it only writes into a file that already exists; a project without one gets a `.env` that WordPress never reads.

---

## 2. Register the site

```bash
lerd link
```

`lerd link` detects WordPress (via `wp-login.php` or `wp-config.php`), assigns `http://myblog.test`, and serves from the project root. Detection reads the version out of `wp-includes/version.php` and pulls the matching definition, `wordpress@7` for a current download, from the [framework store](../usage/frameworks.md#framework-store). There is nothing to register by hand.

---

## 3. Configure PHP and start MySQL

```bash
lerd init
```

```
? PHP version: 8.3
? Node version (clear to follow the lerd default instead of pinning): 22
? Enable HTTPS? Yes
? Services: [mysql]
Saved .lerd.yaml
```

Workers are not shown; the WordPress definition declares none.

Picking MySQL is all the database setup there is. `lerd init` starts the container, creates `myblog` and `myblog_testing` inside it, and rewrites the connection constants in `wp-config.php`:

```php
define( 'DB_NAME',     'myblog' );
define( 'DB_USER',     'root' );
define( 'DB_PASSWORD', 'lerd' );
define( 'DB_HOST',     'lerd-mysql' );
```

WordPress keeps its configuration as PHP constants rather than in a `.env`, so lerd reads and writes `wp-config.php` directly. That is also what tells lerd the site owns a database, which is what makes `lerd db:import`, `lerd db:shell` and the dashboard's database panel work against it.

::: info Database credentials
| Setting | Value |
|---|---|
| Host | `lerd-mysql` |
| Port | `3306` |
| User | `root` |
| Password | `lerd` |
| Database | `myblog` |

These come from the lerd built-in MySQL service. See [Services](../usage/services.md#service-credentials).
:::

---

## 4. Generate the salts

`wp-config-sample.php` ships its authentication keys as placeholders, and because the file now exists WordPress goes straight to the installer without ever offering to write real ones. Replace that block yourself:

```bash
lerd wp config shuffle-salts
```

`lerd wp` needs nothing installed. When the project has no `vendor/bin/wp` of its own, lerd downloads the wp-cli release it pins, checks it against its sha256, and runs it on the site's PHP. After that first run plain `wp` works in the site too, and wp-cli shows on the dashboard's Tools page, where new releases arrive as an update. A project that requires `wp-cli/wp-cli-bundle` through composer gets its own copy instead.

---

## 5. Finish the HTTPS wiring

Answering yes to HTTPS in the wizard already issued a trusted local cert via mkcert, switched the vhost, and set `WP_HOME` in `wp-config.php` to `https://myblog.test`. On a site linked without it, `lerd secure myblog` does the same thing later.

WordPress keeps its canonical URL in a second constant that lerd leaves alone, so set that one yourself:

```php
// wp-config.php
define( 'WP_SITEURL', 'https://myblog.test' );
```

(Or update the same value in **Settings > General** from the WordPress admin.)

---

## 6. Open it

```bash
lerd open
```

Walk through the five-minute install (admin user, site title, password). When you're done, `https://myblog.test/wp-admin` is your dashboard.

---

## 7. Verify

```bash
lerd status
```

`myblog` should be listed as `active` and `mysql` as `running`. Live nginx and PHP-FPM logs are in the [Web UI](../features/web-ui.md) at `http://127.0.0.1:7073`, and anything WordPress writes to `wp-content/debug.log` shows up under **App Logs**.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Detected WordPress, fetched `wordpress@7` from the store, assigned `myblog.test`, set document root to project root |
| `lerd init` | Wrote `.lerd.yaml` with PHP 8.3 and the MySQL service, created `myblog` and `myblog_testing` inside lerd-mysql |
| `lerd env` (via init) | Wrote `DB_NAME`, `DB_USER`, `DB_PASSWORD` and `DB_HOST` into `wp-config.php` |
| `lerd secure` (via init) | Issued mkcert TLS, switched the vhost to HTTPS, set `WP_HOME` |

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): add log paths or custom workers (e.g. `wp cron event run`) with a user overlay
- [Database](../usage/database.md): `lerd db:import` to load a production dump, `lerd db:shell` for quick queries
- [Services](../usage/services.md): add a Mailpit service to capture outgoing mail in dev
- [HTTPS](../features/https.md): wildcard certs for multi-site or git worktrees
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, Junie, etc.
