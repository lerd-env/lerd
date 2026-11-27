---
title: "Composer"
description: "How Lerd runs Composer: one pinned composer.phar on the project's own PHP version, private repositories over SSH, global packages with cpx, and vendor/bin tools."
---

# Composer with Lerd

`composer` works as usual once Lerd is installed: `composer install`, `composer require` and `composer update` run inside the project's PHP container, on the PHP version the site is registered on rather than whatever PHP the host has. Platform checks therefore match what serves the site, and you never install PHP or Composer on the host.

```bash
composer install
lerd composer require laravel/horizon   # the explicit form, same thing
```

## Which composer runs

Composer works the same way, with one thing worth knowing: lerd runs its own `composer.phar`, downloaded to `~/.local/share/lerd/bin/` at install and pinned in the tools manifest, and it runs it with the container's PHP. That is the copy behind `composer` on your PATH, behind `lerd composer`, behind the setup steps and behind the MCP composer tool, so an assistant and your terminal are never on two different versions of it, and a bumped pin reaches every install without an image rebuild. Every one of those routes also runs it with your own composer home, so a global `auth.json` there is read whichever way composer is started. The FPM image ships a composer of its own, frozen at the day the image was built, so lerd's phar is bind-mounted over it and `composer` typed inside `lerd shell` is the same one again. If you already had a composer of your own on the host, it stays exactly where it is and `lerd install` says so; `lerd path:disable` puts it back in front.

## Which PHP version it uses

Inside a linked site, Composer uses the version the site's PHP-FPM container serves. Elsewhere it resolves the version the way every Lerd command does, from `.php-version`, then `composer.json`, then the global default. [PHP versions per project](/usage/php-versions) covers the order and how to pin one.

## Private repositories over SSH

Composer runs in a container, so it cannot reach your host SSH agent directly. Load your keys into Lerd's own agent once and `composer install` can clone private git repositories, passphrase-protected keys included:

```bash
lerd auth ssh             # loads ~/.ssh/id_*
lerd auth ssh --list      # show what is loaded
lerd auth ssh --remove    # forget the keys and stop the agent
```

The agent's socket is shared into the PHP containers through a volume, which is also what makes it work on macOS, where the host agent cannot cross into the Podman machine.

## Running a package you have not installed

[cpx](https://cpx.dev) is to Composer what npx is to npm: it runs a command from any Composer package without adding it to the project. Require it once, and `lerd cpx` runs it inside the project's container:

```bash
lerd composer global require cpx/cpx
lerd cpx phpstan/phpstan analyse
```

The package runs on the PHP version the site is registered on rather than whatever PHP is on the host, which is the whole reason to route it through lerd. Fetched packages are cached under `~/.cpx` on the host, so they survive a container restart and are shared with any cpx you run outside lerd.

Requiring it globally also puts a `cpx` shim on your PATH, so bare `cpx` works too and runs through the same container. `lerd cpx` is the explicit form, and the one that tells you what to install when cpx is missing.

## pun

[pun](https://punphp.com) is a single binary that formats, lints and analyses PHP and installs Composer packages, standing in for Pint, PHP-CS-Fixer, Rector, PHPStan and Composer itself. `lerd pun` runs it at the version lerd pins, fetching it the first time:

```bash
lerd pun check --fix
lerd pun install
```

::: warning pun is in beta
pun is still in beta, so expect rough edges, and check its output before relying on it in place of the tools it replaces. Problems with pun itself belong in [its issue tracker](https://github.com/shipfastlabs/pun/issues).
:::

pun is a static binary, so it runs on the host and needs no container. Composer scripts it runs call `php`, which is lerd's shim, so they still run on the PHP version the site is registered on. lerd also puts a `pun` shim on your PATH, so bare `pun` goes through lerd too. It is managed like the database client shims: when you already have a pun of your own, `lerd install` asks before shadowing it, and `lerd shims remove pun` or `lerd shims add pun` changes your mind later. See [Managing shims](/usage/database#managing-shims). AI assistants run it through the `exec` MCP tool with `action: "pun"`, and pun notices the agent and prints one line per problem.

The MCP integration exposes the same surface through two tools, `vendor_bins` (list available binaries) and `vendor_run` (execute one), so AI assistants can discover and run project tooling without per-project configuration. A project's tests have [a tool of their own](/features/mcp#running-tests).

## Tools in vendor/bin

Any binary Composer installs into `vendor/bin` runs as `lerd <name>`, so `lerd pest`, `lerd pint` and `lerd phpstan analyse` work without a path. See [Shortcuts and vendor/bin fallback](/usage/php#shortcuts-and-vendor-bin-fallback).

## composer install on a new project

`lerd setup` runs `composer install` for a freshly cloned project when `vendor/` is missing, inside the right PHP container. See [Project setup](/features/project-setup).

## Extension requirements

When you link a site, Lerd reads the `ext-*` requirements in `composer.json` and warns about any extension the image does not have, before `composer install` fails its platform check. See [Composer.json detection](/usage/php-extensions#composer-json-detection).

If Composer cannot resolve hosts from inside the container, see [Troubleshooting](/troubleshooting).
