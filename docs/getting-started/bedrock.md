# Bedrock walkthrough

End-to-end: from `lerd install` to a [Roots Bedrock](https://roots.io/bedrock/) site running on `https://mysite.test`, with WordPress installed as a composer dependency and its config in `.env`.

::: info Prerequisites
You've already run `lerd install` once on this machine. If not, see [Installation](installation.md).
:::

::: tip Drive it from your AI assistant
Run `lerd mcp:enable-global` once and your AI assistant (Claude Code, Cursor, Junie, Codex, Gemini, Copilot, Antigravity, Windsurf) can call every command below through the grouped MCP tools: `framework` `action: "project_new"`, `site` `action: "link"`, `env` `action: "setup"`, `db` `action: "create"`, etc. See [AI Integration](../features/mcp.md).
:::

---

## 1. Create the project

::: code-group

```bash [lerd new]
cd ~/Lerd
lerd new mysite --framework=bedrock
# runs: composer create-project roots/bedrock:^1.0 ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project roots/bedrock mysite
```

```bash [existing repo]
cd ~/Lerd
git clone git@github.com:you/mysite.git
cd mysite && lerd composer install
```

:::

---

## 2. Register the site

```bash
cd mysite
lerd link
```

```
 → detecting framework… ✓ bedrock
 → provisioning PHP-FPM runtime… ✓ php 8.4 · nginx vhost written
 ✓ linked in 512ms

  Site        http://mysite.test
  PHP         8.4 · FPM
  Framework   bedrock
  DB          mysql
```

Bedrock is WordPress rearranged: the core lives in `web/wp` as a composer package, plugins and themes in `web/app`, and configuration in `.env` instead of a hand-edited `wp-config.php`. lerd detects it from `web/wp-config.php`, which plain WordPress does not have, so it is not served as WordPress with the wrong document root.

That root is **`web/`**, not the project directory: `vendor/`, `config/` and `.env` sit above it and are never reachable over HTTP. The definition supports PHP 7.4 to 8.4 and raises the CLI memory limit to 512M, which is what plugin-heavy WP-CLI runs need.

---

## 3. Configure the database and HTTPS

```bash
lerd init
```

Bedrock needs a database from the first request, so this is not optional as it can be for a flat-file CMS. The wizard writes `.lerd.yaml`, offers HTTPS, creates the database and wires `.env`:

```dotenv
DB_NAME=mysite
DB_USER=root
DB_PASSWORD=lerd
DB_HOST=lerd-mysql
WP_HOME=https://mysite.test
```

`WP_HOME` is the key lerd keeps in step with the site's address, so securing the site or renaming it rewrites it rather than leaving WordPress redirecting to the old scheme.

```bash
lerd secure
```

Bedrock's own `.env.example` also wants `AUTH_KEY` and the other salts. Generate them with wp-cli or paste the block from <https://api.wordpress.org/secret-key/1.1/salt/>:

```bash
lerd wp dotenv salt regenerate
```

---

## 4. Install WordPress

```bash
lerd wp core install \
  --url=https://mysite.test \
  --title="My site" \
  --admin_user=admin \
  --admin_password=secret \
  --admin_email=you@example.test
```

`lerd wp` runs the wp-cli your project already has in `vendor/`. WordPress refuses to run as root, which every lerd container is, and the flag that lifts it cannot come from a config file, so the definition passes `--allow-root` for you unless you typed it yourself.

---

## 5. Verify

```bash
curl -I https://mysite.test
curl -I https://mysite.test/wp/wp-admin/
```

`web/app/debug.log` is picked up as the site's log, so `WP_DEBUG_LOG` output shows in the dashboard's Logs tab and in `lerd logs`.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `web/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL |
| `lerd env` (via init) | Wrote `DB_*` and `WP_HOME` into `.env` |
| `lerd wp core install` | Installed WordPress into the database |

---

## Quick commands

| Command | Runs | What it does |
|---|---|---|
| `cache:flush` | `wp cache flush` | Drop the object cache |
| `rewrite:flush` | `wp rewrite flush` | Rebuild permalinks after changing rewrite rules |

Both are on the site's card in the dashboard, in the TUI, and available to an AI assistant over MCP. The [Tinker tab](../features/tinker.md) runs `wp eval`, so you can call WordPress functions against a booted site.

---

## Bedrock or plain WordPress?

Both are first-class here. Use the [WordPress walkthrough](wordpress.md) for a classic tree with `wp-config.php` at the root and WordPress served from the project directory. Bedrock is the one to pick when you want composer to own the core, plugins and themes, `.env` to own the configuration, and a document root that does not expose the rest of the repository.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): Redis for the object cache, Mailpit for outgoing mail
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
