# Pausing Sites

Pausing a site frees up resources without removing it from lerd. It is useful when you're switching focus between projects and want to stop workers and silence a site without fully unlinking it.

## Commands

| Command | Description |
|---|---|
| `lerd pause [name]` | Pause a site: stop its workers and replace the vhost with a landing page |
| `lerd unpause [name]` | Resume a paused site: restore its vhost and restart previously running workers |

---

## Pausing and resuming

```bash
lerd pause              # pause the site in the current directory
lerd pause my-project   # pause a named site
```

When a site is paused:

- All running workers for that site are stopped (queue, schedule, reverb, stripe, and any custom workers)
- The nginx vhost is replaced with a minimal landing page that shows a **Resume** button
- Services no longer needed by any other active site are auto-stopped
- The paused state is persisted, so the site stays paused across `lerd start` / `lerd stop` cycles

The landing page's **Resume** button calls the lerd dashboard API directly, so you can unpause from the browser without opening a terminal.

```bash
lerd unpause              # resume the site in the current directory
lerd unpause my-project   # resume a named site
```

When a site is unpaused:

- The original nginx vhost is restored (including HTTPS if the site is secured)
- Any services referenced in the site's `.env` are started
- Workers that were running before the pause are restarted

Paused sites still appear in `lerd sites` output and the web UI. Their status is shown as `paused`.

## Running CLI commands on a paused site

You can run `php artisan`, `composer`, `lerd db:export`, and other exec-based commands on a paused site without unpausing it first. If any services the site needs (MySQL, Redis, etc.) were auto-stopped when the site was paused, lerd starts them automatically before running the command:

```
$ php artisan migrate
[lerd] site "my-project" is paused, starting required services...
  Starting mysql...

   INFO  Nothing to migrate.
```

On subsequent commands the services are already running, so no notice is printed. The site stays paused; the nginx vhost remains as the landing page and workers are not restarted.

Commands that benefit from this auto-start:

| Command | Notes |
|---|---|
| `php artisan <args>` / `lerd artisan <args>` | Any artisan command |
| `php <args>` / `lerd php <args>` | Any PHP script |
| `composer <args>` | Composer via the lerd shim |
| `lerd shell` | Opens an interactive shell in the PHP-FPM container |
| `lerd db:import` | Imports a SQL dump |
| `lerd db:export` | Exports a database |
| `lerd db:shell` | Opens an interactive DB shell |
