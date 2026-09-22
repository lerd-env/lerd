# PhpStorm plugin

[Lerd for PhpStorm](https://plugins.jetbrains.com/plugin/34420-lerd) puts the site you are
editing into the IDE you are editing it in: its logs, its PHP version, its workers, its
services, its queries and everything it dumped, without a browser tab.

It is a separate, optional install from the JetBrains Marketplace, with its own repository
at [lerd-env/lerd-phpstorm](https://github.com/lerd-env/lerd-phpstorm). lerd does not need
it and it needs nothing from lerd beyond the dashboard already running.

---

## What it shows

The project you have open resolves to exactly one site. Paths are compared the way lerd
compares them, so a symlinked home or a case-folding volume still matches, and opening a
git worktree resolves to its parent site plus the branch.

A status bar widget carries the answer: the site's domain and PHP version, greyed out when
lerd is stopped, hidden entirely for a project lerd does not know. Clicking it gives the
handful of actions worth reaching for mid-edit: open in browser, restart, HTTPS on or off,
pause, switch PHP version, open the dashboard.

A tool window at the bottom carries the rest:

**Site** is the overview. Domains, framework, branch, the PHP and Node version pickers,
a toggle per worker, a row per service with its admin UI, and the site doctor's findings.

**Logs** covers the two shapes logs come in. A framework log is collapsed into one row per
distinct error, newest first, with how many times it happened and the full record beside
it, because a log with the same failure in it two hundred times is one problem and not two
hundred lines. Everything else, PHP-FPM or FrankenPHP, every worker, every service the
site uses and nginx, is a live tail. Both panes are IDE consoles, which is the point: a
path in a stack trace is a link, so an exception in the queue worker is one click from the
line that threw it.

A service row carries **Open in Database**, which opens it in the IDE's Database tool
window and lands on this site's own database rather than on the server node, filling in
the password lerd already knows so nothing stops to ask for it. It reuses the connection
lerd wrote into `.idea/dataSources.xml` rather than adding a second one beside it. Redis, Postgres and
MongoDB get the same treatment even though lerd writes no entry for them. **Shell** is
next to it for the times a client is what you want, running the binaries lerd ships:
`mysql`, `psql`, `redis-cli`, `mongosh`.

**Container Shell** on the toolbar opens an IDE terminal inside the site's own container,
which is where its PHP actually lives.

**N+1 & Slow** is the verdict on the site's queries: repeats inside a single request and
anything over the slow threshold, read from real captured traffic, worst first, each one
linked to the application line it came from and openable as a runnable `.sql` scratch
against the site's database.

**Dumps** is everything the site captured, split into the same lenses the dashboard uses,
each with its count and only drawn when it has something in it: dumps, queries, jobs,
views, mail, cache, events, HTTP, logs, exceptions and messages. A search box narrows the
selected lens, and events captured inside a test run stay one toggle away.

**Tools > Lerd > Run Lerd Command** lists the commands the site's framework definition
declares, streams the chosen one into its own console tab, and links its output the same
way.

---

## What it reads

Everything goes to `127.0.0.1:7073`, the same API the dashboard uses: `/api/sites` and
`/api/status` for the state, `/api/ws` for live updates, the SSE log endpoints for tails,
`/api/app-logs/*` for framework logs, `/api/php-versions` and `/api/node-versions` for the
pickers, `/api/services`, `/api/sites/{domain}/doctor` and `/api/sites/{domain}/commands`.

Because it is a loopback client it needs no credentials, the same way the dashboard in
your browser does not. Nothing is sent anywhere else. When lerd is stopped the plugin
falls back to reading `~/.local/share/lerd/sites.yaml`, so it can still tell you that this
project is a lerd site and that lerd is not running.

---

## What it can change

Only what the dashboard can, through the same endpoints: the site actions
(`restart`, `secure`, `unsecure`, `pause`, `php`, `node`, the worker start and stop
actions), starting and stopping a service, and running one of the site's declared
commands. A command a framework definition marks as destructive asks first.

It registers the site's domain as a PHP server so Xdebug resolves breakpoints into this
project instead of asking, points the project's PHP CLI interpreter at lerd's `php` shim, which is the only php on
the host that runs the site's real version with its real extensions, and keeps PhpStorm's
PHP language level on that version, so the IDE's own PHP widget cannot disagree with
lerd's. That sync runs one way on purpose:
lerd owns what actually runs the site, and a language level changed in the IDE is an
inspection setting, not a reason to rebuild a container.

It does not touch `.idea/dataSources.xml`; lerd already
[maintains the database connection](../usage/database.md) there itself.

---

## Requirements

PhpStorm 2025.2 or newer, and lerd running on the same machine. If you serve the dashboard
on another port, change it under **Settings > Tools > lerd**.
