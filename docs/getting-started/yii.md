---
title: "Yii 2 local development with Lerd"
description: "Run a Yii 2 app locally with Lerd on Linux or macOS: create the basic template, serve it on a .test domain over HTTPS and connect it to MySQL."
---

# Yii 2 local development with Lerd

End-to-end: from `lerd install` to a [Yii 2](https://www.yiiframework.com/) basic app running on `https://yiiapp.test` against MySQL.

::: info Prerequisites
You've already run `lerd install` once on this machine. If not, see [Installation](installation.md).
:::

::: tip Drive it from your AI assistant
Run `lerd mcp:enable-global` once and your AI assistant (Claude Code, Cursor, Junie, Codex, Gemini, Copilot, Antigravity, Windsurf) can call every command below through the grouped MCP tools: `site` `action: "link"`, `env` `action: "setup"`, `framework` `action: "setup"`, `db` `action: "create"`, `worker`, etc. See [AI Integration](../features/mcp.md).
:::

---

## 1. Create the project

::: code-group

```bash [lerd new]
cd ~/Lerd
lerd new yiiapp --framework=yii --framework-version=2
```

```bash [existing repo]
cd ~/Lerd
git clone git@github.com:you/yiiapp.git
cd yiiapp && lerd composer install
```

:::

`lerd new` runs `composer create-project yiisoft/yii2-app-basic`, the scaffold the Yii definition declares. On a terminal it carries on into link and setup; from a script it stops after the scaffold and prints the next step.

---

## 2. Register the site

```bash
cd yiiapp
lerd link
```

```
 → detecting framework… ✓ yii
 → Using PHP 8.4 (yii supports 7.4–8.4)
 → provisioning PHP-FPM runtime… ✓ php 8.4 · node 22 · nginx vhost written
 ✓ linked in 935ms

  Site        http://yiiapp.test
  PHP         8.4 · FPM
  Framework   yii
```

Yii 2 is told apart from Yii 3 by `yiisoft/yii2` in `composer.lock`, since both ship the `yii` console script. The document root is `web/`, and the definition supports PHP 7.4 to 8.4.

---

## 3. Configure the database

```bash
lerd init
```

Yii keeps its connection in `config/db.php` as a PHP array rather than in a `.env`, and lerd writes it there. Picking MySQL sets the `dsn` to `mysql:host=lerd-mysql;dbname=yiiapp` with the `root` / `lerd` credentials, PostgreSQL sets a `pgsql:` dsn against `lerd-postgres`, and every other key in the file is left as it was. The original is kept as `config/db.php.before_lerd` the first time.

```bash
lerd secure
lerd env
```

`lerd env` reads the dsn prefix already in `config/db.php` to tell which service the project uses, starts it, and creates the `yiiapp` database, along with `yiiapp_testing` unless the project's PHPUnit config runs its tests on another database. No application URL is written, because Yii keeps none here and `yii\db\Connection` refuses a key it does not know.

---

## 4. Bootstrap the project

```bash
lerd setup
```

Setup offers `php yii migrate --interactive=0`, ticked by default, and `php yii fixture/load "*"` once the project has a `tests/unit/fixtures` folder.

---

## 5. Verify

```bash
curl -i https://yiiapp.test
lerd site:doctor
```

The doctor checks the PHP version against the definition's range, the vhost, the worker units and that the site answers.

---

## Workers

```bash
lerd worker start queue
```

The queue worker runs `php yii queue/listen --verbose` and appears once the project requires `yiisoft/yii2-queue`. The basic app does not, so a fresh project shows no worker until you add it.

---

## Quick commands

| Command | Runs | What it does |
|---|---|---|
| `cache:flush` | `php yii cache/flush-all` | Clear all application caches |
| `migrate` | `php yii migrate --interactive=0` | Apply outstanding migrations |

---

## Tinker

The Tinker tab builds the console application the way the `yii` script does, so `Yii::$app->db->dsn` or a model query runs against the real configuration. See [Tinker](../features/tinker.md).

---

## Yii 3

Yii 3 is the definition `lerd new --framework=yii` picks by default. It ships no framework package, so an app with the `yii` console script and no `yiisoft/yii2` is read as Yii 3, whatever version its runner packages carry.

```bash
cd ~/Lerd
lerd new yii3app --framework=yii
```

That runs `composer create-project yiisoft/app`, serves `public/` and creates `.env` from `.env.example` on `lerd env`. The template keeps no database, so `lerd env` wires none. `lerd console` maps to `php yii`, and the Tinker tab builds the DI container first, with `$container` in scope:

```php
$container->get(\Psr\Log\LoggerInterface::class)
```

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Queue Workers](../usage/queue-workers.md): tuning, retries, and worker logs
- [Database](../usage/database.md): `lerd db:import`, `lerd db:shell`, snapshots
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
