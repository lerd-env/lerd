---
title: "php.ini overrides"
description: "Change php.ini settings such as memory_limit or upload_max_filesize in Lerd, for one PHP version or every version at once, from the CLI, the dashboard or MCP."
---

# php.ini overrides

To change a php.ini setting such as `memory_limit`, `upload_max_filesize` or `max_execution_time`, run `lerd php:ini` and edit the file it opens. Saving restarts PHP-FPM, so the change applies straight away. Use `lerd php:ini shared` for a setting every PHP version should get.

Related: [PHP versions per project](/usage/php-versions), [PHP extensions](/usage/php-extensions), [Xdebug](/usage/xdebug).

## Commands

| Command | Description |
|---|---|
| `lerd php:ini [version]` | Open the user php.ini for a PHP version in `$EDITOR` |

## Per-version php.ini

Each PHP version has a user-editable ini file at `~/.local/share/lerd/php/<version>/98-user.ini`, mounted read-only into the FPM container as `conf.d/98-lerd-user.ini`. Edit it with:

```bash
lerd php:ini          # detected/default version
lerd php:ini 8.3      # explicit version
```

This opens the file in `$EDITOR` (falls back to `nano`/`vim`). Saving restarts the affected FPM containers automatically, so the change applies straight away.

The file is created automatically with commented-out examples when lerd first sets up the PHP version.

## Shared settings across versions

A setting placed in a per-version file only applies to that version, so a site that changes PHP version silently loses it. For a setting you want everywhere, edit the shared file instead:

```bash
lerd php:ini shared   # applies to every PHP version
```

The shared file lives at `~/.local/share/lerd/php/shared/95-shared.ini` and is mounted into every PHP container (FPM, custom-image FPM, and FrankenPHP) as `conf.d/95-lerd-shared.ini`, below the per-version `98-lerd-user.ini`. Because `conf.d` loads alphabetically and the last file wins, layering happens for free:

- A key set only in the shared file applies to all versions.
- A key set in both files takes the per-version value on that version, and the shared value everywhere else.

Nothing is merged and nothing is migrated: your existing per-version files keep working and keep winning. A key becomes shared only when you put it in the shared file. An unknown or removed directive on a given version is ignored (a startup notice, not a fatal), so a version-specific setting never breaks the others.

In the web UI, open **System → PHP**, pick a version, and use the **Editing** dropdown at the top of its php.ini tab to switch to the shared file. Over MCP it is reachable through `runtime` with `ini_read` / `ini_write` / `ini_reset` (pass `shared: true`).
