# Terminal Dashboard (TUI)

`lerd tui` opens a full-screen dashboard in your terminal. It shows your sites, services, databases and runtimes, updates live, and drives the same reversible operations the web UI exposes without leaving the terminal.

```bash
lerd tui
```

This is the terminal-native counterpart to the [Web UI](/features/web-ui) and the [System Tray](/features/system-tray). Use it when you prefer to keep everything in a tmux or terminal pane, or when you're on a remote machine over SSH.

## Layout

The screen is a **sidebar** on the left and a **main area** beside it, with a single hint line at the bottom of the main area.

- The **sidebar** is the navigation: Dashboard, Databases, PHP & Node and Settings at the top, then your sites grouped by workspace, then your services, and at the foot lerd's own processes (dns, nginx and the watcher) with their state.
- The **main area** shows whatever the sidebar has selected: the dashboard, a site, a service, the databases, the runtimes, settings, or one of lerd's processes.
- The **hint line** shows site health counts on the left and the keys for whatever has focus on the right, dropping the least useful keys when space runs out.

Regions are separated by shade rather than box borders: the content sits a shade darker than the sidebar, and cards and selections rise from there. Every colour comes from your terminal's own palette and the shades are derived from its background, so the TUI follows your terminal theme, including Omarchy's, with no configuration.

The layout adapts to the terminal. A wide terminal gets the full sidebar with framework and version tags, a medium one a slimmer sidebar without them, and below about 96 columns the sidebar folds away and `\` brings it back over the content. A short terminal drops optional spacing, and a section that does not fit whole is left out rather than cut in half.

Dots follow the same convention everywhere: green `●` running, grey `○` stopped, amber `◐` paused, red `✖` failing. A worker the idle engine has put to sleep reads `suspended` (or asleep) with an amber `◔`, so a worker stopped for idleness isn't mistaken for one that crashed; it wakes on the next request.

Mouse support is on: clicking a sidebar row opens it, clicking a tab, worktree tab or dashboard card selects it, and the wheel scrolls whatever it's over. Hold `Shift` (or your terminal's selection modifier) to select text the usual way while the TUI runs.

## Command palette

`ctrl+p` opens a fuzzy palette over the dimmed screen, from anywhere. Everything the TUI can do is in it; the keys described below are shortcuts on top.

- **Fixes first**: restart a crashed worker, heal every crashed worker, or start lerd when a core process is down.
- **Actions**: every action on every site (including each worktree's controls), service, database and PHP or Node version. An action selects its target before it runs, so nothing has to be opened first, and the selected item's actions are listed before the rest.
- **Places**: every page (Dashboard, Databases, PHP & Node, Settings, System, the debug window, dns, nginx, the watcher, help) and every site, worktree and service.
- **Settings**: every Settings toggle, named for what it will do (`Turn on Streaming mode`).
- **Run a lerd command…**: hands over to the `:` prompt for anything else.

Type to filter; each word matches on its own and in any order, so `drupal https` and `https drupal` find the same entry, and typing a name puts the place before the things done to it. `↑` `↓` select, `enter` runs, `esc` closes. Destructive actions are never offered, as with the keys.

## Sidebar

The sites section groups sites by workspace, in the order the workspaces are configured, with sites in no workspace after them. A workspace folds with `enter`, and a crashed worker inside it rolls up to the workspace row as a count, so a failure stays visible when the workspace is folded. A group's secondary sites sit under their main with a `↳`.

With the sidebar focused, `↑` `↓` move, `enter` opens the row and hands focus to the main area, `/` filters the section the selection is in, and `tab` moves to the main area. `tab` or `esc` from the main area comes back.

## Dashboard

The dashboard leads with **Needs attention**: a stopped DNS, nginx or watcher, and every crashed worker, one card each. `tab` moves onto the cards, `enter` opens the site behind one, and `r` applies its fix, restarting that worker on its own unit or bringing a stopped core process back with `lerd start`. With nothing wrong it says so in one line.

Under it, **Resources** fills the first column and **System** sits in the second with **Recent** below it, both columns ending on the same row. System pairs its facts into two columns when the panel is wide enough. On a narrow pane the three stack, System first:

- **Resources**: a CPU sparkline over the last few minutes, memory against the host total, and the three largest containers. Memory excludes reclaimable page cache and CPU is a share of the whole machine, the same figures the web dashboard shows, polled every 3 seconds.
- **System**: DNS, nginx and the watcher, workers running, asleep and crashed, autostart, LAN, the lerd version with any available update, and the platform.

**Recent** lists site link, pause, resume, start and stop, service add, remove, start and stop, worker fail and heal, and DNS transitions, derived live from successive snapshots since the TUI opened.

## Sites

A selected site opens with a fixed header over its tabs. The header carries the breadcrumb, the site's URL and app name, its framework, PHP, Node and runtime, its path, and its HTTPS, LAN and paused flags. It never scrolls, so the site and its tabs stay in view however long the content gets.

### Worktrees

A site with git worktrees shows a row of branch tabs under its header, its own checkout first. `b` or a click moves between them, and a worktree whose own worker crashed carries the failure mark on its tab. Picking a worktree scopes the whole view to it: the breadcrumb names the branch, the header shows the worktree's domain, path and versions, the Overview shows only that worktree's controls (its workers, isolated database, LAN share, PHP and Node), the Env tab reads the worktree's `.env`, and the request-timing panel shows its traffic. `W` opens the command palette on `worktree add` in the site's directory, so lerd's own setup prompts appear as they do from the CLI.

### Tabs

| Key | Tab | Contents |
| --- | --- | --- |
| `1` | Overview | Domains, toggles, services used, workers, suggested services and the [request-timing panel](#request-timing) |
| `2` | Logs | A live tail of any of the site's log sources (see [Log sources](#log-sources)). `l` jumps here from anywhere on a site |
| `3` | Env | The site's (or worktree's) `.env`, read-only, up to 256 KB |
| `4` | Debug | This site's slice of the [debug window](#debug-window) |
| `5` | Doctor | The framework-agnostic health checks the web dashboard runs, plus the framework's own. The run is on demand and read-only: it names the suggested fix rather than running it |

### Overview

The Overview lays sections out as a grid: **Domains** beside **Toggles**, **Services used** beside **Workers**, full width below the breakpoint. `↑` `↓` walk the controls, `←` `→` cross between paired columns, and `space` or `enter` toggles the focused row:

- **Domains**: each domain with its role, `e` to edit, `x` to remove (confirmed), and `+ add domain`.
- **PHP / Node**: a picker of installed versions (`lerd isolate` / `lerd isolate:node`); a FrankenPHP site only lists versions FrankenPHP publishes an image for, and a host bun appears as a Node option.
- **HTTPS** (`lerd secure` / `unsecure`), **LAN share** (`lerd lan share` / `unshare`), and **Auto snapshots** (cycles following the global policy, always and never).
- **Keep awake**: pins the site so idle suspend leaves it alone (`lerd idle pin` / `unpin`).
- **Runtime**: switches between php-fpm and FrankenPHP (`lerd runtime`); the switch pulls and starts the image, so it can take a moment.
- **Reload horizon** (sites with Horizon) and **Stripe listener** (sites with a Stripe secret).
- **Workers**: each with its state; `space` starts or stops it.

**Suggested services** lists the services the site's packages ask for and why, read-only, since adding one rewrites `.lerd.yaml` and `.env`.

Other site keys: `s` / `x` start and stop, `r` restart, `p` pause, `t` a shell in the site's container, `O` the browser, `E` your editor, `F` the folder.

## Request timing

The bottom of the Overview carries the same request-timing view the web dashboard shows, read straight from the durable request store the watcher fills from the nginx access feed, so it works whether or not `lerd-ui` is up. It shows the median and p95, the request count and cold starts (kept out of every figure), the status mix, a response-time distribution, the slowest routes by recent p95, and the latest requests.

When the [SPX profiler](/features/profiler) has caught one of the slow routes, the route's hottest function and its share of the time appear on a line under it, read from its freshest capture.

`[` / `]` cycle the window through `15m · 1h · 24h · 7d`. The worktree tabs set the branch, since a worktree records its traffic under its own key. Static assets, nginx-served files and WebSocket upgrades are filtered out.

## Services

A selected service opens with a header (name, version, state, the published port with the default it moved from and any extra mappings, its dashboard URL, and whether it is pinned or custom) over an **Overview** and a **Logs** tab (`1` / `2`, or `l` for logs). The tail only runs while the Logs tab is open.

The Overview lists what the service depends on, the sites using it, its env vars, its client tools (`space` toggles the focused tool's host shim), its tuning overrides (edit with `lerd service config`), and the entities it holds. A matching admin dashboard preset that is not installed yet is suggested.

Keys: `s` / `x` / `r` start, stop and restart, `P` pins or unpins it (a pinned service keeps running when no site uses it), `A` opens the palette on `service preset` to add a preset through lerd's own picker, `u` / `b` update and roll back, `O` opens its dashboard, `t` a shell.

## Databases

The Databases page lists every installed engine with the databases it holds, beside the selected database's detail. A database's `<name>_testing` twin folds into its row, as the web UI folds it into the same card; one whose app database is gone keeps a row of its own. Rows carry the name and size; the detail shows the engine, the owning site (and branch, for a worktree's isolated database), whether that site is on the automatic snapshot schedule, the folded testing database, and every snapshot with when the schedule will drop it.

| Key | Action |
| --- | --- |
| `n` | Take a snapshot |
| `K` | Keep an automatic snapshot for good, or put it back under retention |
| `e` | Export to `<name>.sql` in the owning site's folder (or your home folder) |
| `c` | Create a database on the engine (the palette opens on `db:create`) |
| `a` | Put the owning site on or off the automatic snapshot schedule |
| `R` | Re-list |

Restore, drop and import overwrite or destroy data, so they stay in the CLI.

## PHP & Node

Lists every installed PHP and Node version beside the selected one's detail: for PHP whether its FPM runs, whether it is the default, its Xdebug state and mode and any custom extensions, and for both the sites that use it. `d` makes it the default, `x` toggles Xdebug, `R` opens the palette on `php:rebuild`, and `i` on `use` or `node:install` to install another version.

## Settings

Settings is in the sidebar (and on `S`). `↑` `↓` walk the toggles and `space` flips one: LAN expose, managed service LAN access, autostart, automatic database snapshots, idle suspend (with its timeout), streaming mode, the tray applet, notifications, lerd's DNS and the SPX profiler. Each runs the matching `lerd` verb.

## Core processes

dns, nginx and the watcher sit at the foot of the sidebar with a state word each. Opening one shows what it does, whether it is up, and its live log: the dns and nginx containers' output, or the watcher's journal. `s` brings lerd back up with `lerd start`.

## Debug window

`D` (or the Debug tab of a site) shows the same capture the web dashboard does. `[` / `]` switch lens across `Dumps · Queries · Jobs · Views · Mail · Cache · Events · HTTP · Logs · Exceptions · Messages`; queries group by request with N+1 and slow-query flags, and every other lens groups by request too. `/` searches the lens, `1` / `2` toggle the FPM and CLI context chips, `enter` expands a row, `w` shows or hides worker events, `c` clears the buffer and `T` toggles the bridge. The buffer is the TUI's own, fed by the same stream.

`Y` opens the **System** window: DNS, nginx, the watcher, notifications, the debug bridge, the profiler, PHP and Node, worker mode on macOS, and lerd itself.

## Log sources

Wherever logs are showing, `[` and `]` cycle through every source for what's selected:

- **FPM / custom container**: `podman logs -f` of the site's FPM or custom container, or the service's container.
- **Workers**: the user journal of `lerd-queue-<site>` and the other worker units.
- **App logs**: any file matching the framework's declared log globs, tailed so rotated logs keep following.

`{` / `}` scroll back through the buffer and return to the live tail, and `f` finds within it (matches highlighted, other lines dim). Error and warning lines are always coloured.

## Overlays and toasts

Help (`?`), the `:` command prompt, the version pickers and confirmations open on a raised surface over the dimmed screen, like the palette, and own every key while open; `esc` closes them. Action results land as toasts in the bottom-right corner, drawn over the content so they never move it; up to three stack, they fade after 30 seconds, `d` dismisses the newest, and identical ones coalesce. While an action runs, the status line shows a spinner.

## Keybindings

| Key | Action |
| --- | --- |
| `ctrl+p` | Command palette: everything |
| `↑` `↓` / `j` `k` | Move in the sidebar or the main area |
| `enter` | Open the sidebar row, or act on the focused row |
| `tab` / `esc` | Between the sidebar and the main area |
| `\` | Show the sidebar on a narrow terminal |
| `/` | Filter the sidebar section · `o` cycle its sort order |
| `1`-`5` · `b` | Site tabs · worktree tabs |
| `s` `x` `r` `p` | Start, stop, restart, pause |
| `t` `O` `E` `F` | Shell, browser, editor, folder |
| `W` | New worktree |
| `H` | Heal every crashed worker |
| `:` | Run any `lerd` command |
| `?` | Help |
| `R` | Refresh |
| `q` / `ctrl+c` | Quit |

The keys specific to a page (services, databases, PHP & Node, settings) are listed in its section above, in the hint line while it has focus, and in `?`.

## Live updates

The TUI draws state from the same sources `lerd-ui` uses, in-process. It subscribes to the shared event bus, so a change it makes shows up immediately, and re-reads every 2 seconds, so a change made from another terminal surfaces within a couple of seconds. Sites and services are built from the same `siteinfo` and podman state the web UI uses, so the two surfaces can't disagree.

## Troubleshooting

- **Terminal too small**: under 60 columns by 12 rows the dashboard asks you to resize, and redraws on the next frame.
- **Non-interactive shells**: `lerd tui` exits with an error when stdout isn't a TTY. Run it inside a real terminal.
- **Worker log says nothing**: check the worker is running (its state in the site Overview). Journal logs only exist once the unit has run.
