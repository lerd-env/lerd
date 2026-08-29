# TYPO3 walkthrough

End-to-end: from `lerd install` to a TYPO3 14 site running on `https://mysite.test` with MySQL, Mailpit and a scheduler worker.

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
lerd new mysite --framework=typo3
# runs: composer create-project typo3/cms-base-distribution ./mysite
```

```bash [composer]
cd ~/Lerd
composer create-project typo3/cms-base-distribution mysite
```

```bash [existing repo]
cd ~/Lerd
git clone git@github.com:you/mysite.git
```

:::

`typo3/cms-base-distribution` always resolves to the newest TYPO3 your PHP supports. To pin an older major, ask composer for it directly:

```bash
lerd composer create-project typo3/cms-base-distribution:^13.4 mysite
```

---

## 2. Register the site

```bash
cd mysite
lerd link
```

```
 → detecting framework… ✓ typo3
 → provisioning PHP-FPM runtime… ✓ php 8.5 · node 22 · nginx vhost written
 ✓ linked in 501ms

  Site        http://mysite.test
  PHP         8.5 · FPM
  Node        22
  Framework   typo3
```

TYPO3 is detected from `typo3/cms-core` in `composer.json`, or from a `public/typo3` directory, and the document root is set to `public/`, so nothing outside it is ever served. The major comes from the `typo3/cms-core` constraint, which is what decides the PHP version lerd provisions:

| TYPO3 | PHP | Config file |
|---|---|---|
| 14 | 8.2 – 8.5 | `config/system/settings.php` |
| 13 | 8.2 – 8.4 | `config/system/settings.php` |
| 12 | 8.1 – 8.3 | `config/system/settings.php` |
| 11 | 7.4 – 8.2 | `public/typo3conf/LocalConfiguration.php` |
| 10 | 7.4 | `public/typo3conf/LocalConfiguration.php` |

If the range needs a PHP you don't have, `lerd link` builds that image for you before it finishes.

::: info Already parked?
If `~/Lerd` was registered with `lerd park ~/Lerd` earlier, every subdirectory under it is auto-linked and you can skip `lerd link`.
:::

---

## 3. Configure PHP, database, services

```bash
lerd init
```

The wizard asks for the PHP version, Node version, HTTPS, a database and any services. TYPO3 has no front-end build of its own, so leaving the Node field blank is fine. Pick MySQL or PostgreSQL, and add `mailpit` to catch outgoing mail.

The answers land in `.lerd.yaml`:

```yaml
domains:
  - mysite
php_version: "8.5"
framework: typo3
framework_version: "14"
secured: true
services:
  - mailpit
  - mysql
```

Commit that file; on another machine `lerd link` reads it and restores the same setup without re-running the wizard. `lerd init` also links the site, issues the TLS certificate and creates the project database, so the database the installer needs is already waiting.

---

## 4. Install the site

```bash
lerd run setup
```

```
✓ Congratulations - TYPO3 Setup is done.
```

That installs against the database `lerd init` created, with an admin user of `admin` / `Lerd.Admin.1234`, and creates a root page and a site configuration pointing at your `.test` domain. It asks for confirmation first, since it writes a schema.

The command behind it differs by major, because TYPO3 changed its installer:

- **12, 13 and 14** use TYPO3's own `vendor/bin/typo3 setup`.
- **10 and 11** use `vendor/bin/typo3cms install:setup`, from `helhum/typo3-console`, which those two distributions ship.

Either way `lerd run setup` is the command you type.

::: warning TYPO3 writes its own config file
`lerd env` never creates `config/system/settings.php` (or `LocalConfiguration.php` on 10 and 11). TYPO3's installer writes it, and to TYPO3 an empty one is not a blank slate but a claim that the site is already configured: while the file is absent the site serves the TYPO3 installer, and an empty one makes it fail outright. So until the installer has written it, `lerd env` leaves the file alone and writes the project's `.env` instead, which is also where the database it creates for you is recorded.

Once the installer has written it, `lerd env` wires your services into it as usual, `DB.Connections.Default.*` for the database and `MAIL.*` for Mailpit, and re-running `lerd env` after adding a service is safe.
:::

On 12 and up the installer refuses a database that already has tables, and there is no flag to override it. If you are reinstalling, drop and recreate the database first:

```bash
lerd db:shell
# DROP DATABASE mysite; CREATE DATABASE mysite;
```

---

## 5. Run the remaining setup steps

```bash
lerd setup
```

```
? Setup steps
  [ ] composer install
  [ ] lerd mcp:inject
  [•] Set up extensions
  [ ] Run upgrade wizards
  [•] Flush caches
```

`Set up extensions` applies the database schema and static data for every installed extension, which is what you run after adding one. `Run upgrade wizards` is for a major upgrade, so it stays unselected.

::: info One-shot
`lerd setup --all` skips the prompt and runs every step, not only the pre-selected ones. Installing the site is deliberately a command rather than a setup step, so a blanket run can never recreate the schema of a site you already have.
:::

---

## 6. Verify

```bash
lerd status
```

The front end answers on `https://mysite.test` and the backend on `https://mysite.test/typo3/`, both with a trusted certificate.

The scheduler worker appears once `typo3/cms-scheduler` is installed:

```bash
lerd composer require typo3/cms-scheduler
lerd worker start scheduler
```

It runs `typo3 scheduler:run` every minute. Live logs are in the [Web UI](../features/web-ui.md) at `http://127.0.0.1:7073` under the **App Logs** tab.

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `mysite.test` with nginx + dnsmasq, document root `public/` |
| `lerd init` | Wrote `.lerd.yaml`, issued the TLS certificate, created the `mysite` database, started MySQL and Mailpit |
| `lerd run setup` | Installed TYPO3 and wrote the connection into `config/system/settings.php` |
| `lerd env` | Wired `DB.Connections.Default.*` and `MAIL.*` into that file |
| `lerd worker start scheduler` | Launched the scheduler, which runs `typo3 scheduler:run` every minute |

---

## Quick commands

The TYPO3 definition ships six one-click actions, available from `lerd run`, on the site's card in the dashboard, in the TUI, and to an AI assistant over MCP:

| Command | Runs | What it does |
|---|---|---|
| `setup` | `typo3 setup` / `typo3cms install:setup` | Install a fresh site (confirm-gated) |
| `flush` | `typo3 cache:flush` | Flush all TYPO3 caches |
| `warmup` | `typo3 cache:warmup` | Warm every cache back up |
| `extension-setup` | `typo3 extension:setup` | Apply schema and static data for all extensions |
| `upgrade` | `typo3 upgrade:run` | Run pending upgrade wizards (confirm-gated) |
| `scheduler` | `typo3 scheduler:run` | Execute scheduler tasks that are due |

`scheduler` stays hidden until `typo3/cms-scheduler` is installed.

Any other TYPO3 command works through the console:

```bash
lerd console cache:flush
lerd console site:list
```

---

## Older majors

TYPO3 10, 11 and 12 are supported as definitions, and lerd will provision the PHP they need, down to 7.4. They are aimed at a project you already have rather than a new one: composer currently blocks `create-project` for all three, because every published release in those majors carries a security advisory. `lerd link` on an existing checkout works normally.

The config file also moves. On 10 and 11 it is `public/typo3conf/LocalConfiguration.php`, and from 12 it is `config/system/settings.php`. The definitions handle that for you; it only matters if you are looking for the file by hand.

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [Services](../usage/services.md): Meilisearch for search, RustFS for S3, or a database you run on the host
- [HTTPS](../features/https.md): wildcard certs for git worktrees
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
