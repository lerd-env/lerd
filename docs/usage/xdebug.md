---
title: "Xdebug with PhpStorm and VS Code"
description: "Turn Xdebug on in Lerd with one command and debug PHP from PhpStorm or VS Code on Linux and macOS: port 9003, on-demand triggers, workers and CLI scripts."
---

# Xdebug with PhpStorm and VS Code

`lerd xdebug on` turns Xdebug on for a PHP version and restarts its PHP-FPM container. Set PhpStorm or VS Code to listen on port 9003 and your breakpoints are hit, from web requests, queue workers and CLI scripts alike. `lerd xdebug off` turns it back off, since Xdebug slows every request it is loaded into.

Related: [PHP versions per project](/usage/php-versions), [php.ini overrides](/usage/php-ini), [PhpStorm plugin](/features/phpstorm-plugin).

## Commands

| Command | Description |
|---|---|
| `lerd xdebug on [version] [--mode MODE] [--on-demand]` | Enable Xdebug for a PHP version with the given mode (default `debug`) and restart the FPM container. `--on-demand` sets `start_with_request=trigger` so nothing auto-connects |
| `lerd xdebug off [version]` | Disable Xdebug and restart the FPM container |
| `lerd xdebug status` | Show Xdebug enabled/disabled state and active mode for all installed PHP versions |
| `lerd xdebug pause [site] [--list] [--pid PID]` | Break the IDE debugger into a running worker/CLI process via Xdebug's control socket. `--list` shows candidate processes |

## Turning it on

Toggling Xdebug restarts the version's FPM container, and the command waits for the pool to start accepting again before it reports success. That wait is the point: a restarted container comes back on a new address on the lerd network, and returning as soon as systemd reported the job done meant the next request could still reach the old one and sit there until it timed out. Every command that restarts a pool (`lerd php:ext`, `lerd php:rebuild`, `lerd php:ini`, a runtime switch) waits the same way, so when one of them returns, the site is serving.

::: details Xdebug configuration values
Xdebug is configured with:

- `xdebug.mode=<mode>` (defaults to `debug`, configurable per PHP version)
- `xdebug.start_with_request=yes` (or `trigger` with `--on-demand`)
- `xdebug.client_host=host.containers.internal` (reaches your host IDE from the container)
- `xdebug.client_port=9003`

Set your IDE to listen on port `9003`. In VS Code, the default PHP Debug configuration works without changes. In PhpStorm, set **Settings > PHP > Debug > Debug port** to `9003`. The [Lerd PhpStorm plugin](/features/phpstorm-plugin) also shows the site's PHP version, logs and dumps inside the IDE.

Port `9003` is reserved: no lerd service is ever published on it, because a container that held it would answer Xdebug's connect-back itself and your IDE would simply never see a session. An install that already shifted a service onto it before this was reserved keeps it, so `lerd doctor` names the service and the command that moves it:

```bash
lerd service port rustfs 9004 --container 9001
```

`host.containers.internal` is resolved via a real reachability probe: when lerd writes the shared hosts file it tries each candidate IP (netavark's `host.containers.internal` entry, the address on lerd's `lerd0` link, the host's primary LAN IP, slirp4netns's `10.0.2.2`) by opening a TCP connection to lerd-ui on port 7073 from inside lerd-nginx, and writes the first one that succeeds. If none succeed, `lerd doctor` reports the failure so you get a real diagnosis instead of Xdebug silently timing out with `Time-out connecting to debugging client`.
:::

## Turn off "break at first line"

lerd mounts a small `auto_prepend_file` into every PHP version it builds, the bridge that carries `dump()` and `dd()` output to the dashboard. Being a prepend, it is the first file PHP runs on every request, before a single line of your application.

An IDE set to stop on the first line of every script therefore stops there, in a file that exists only inside the container:

```
Cannot find a local copy of the file on server /usr/local/etc/lerd/dump-bridge.php
```

Nothing is broken, and your own breakpoints are still fine. In **PhpStorm** three settings can cause it, and the two that matter here are not the obvious one:

- **Settings > PHP > Debug > External connections > Force break at first line when no path mapping specified** — the bridge has none, so this one fires and is what draws the *Click to set up path mappings* link
- **Settings > PHP > Debug > External connections > Force break at first line when a script is outside the project** — the bridge is outside it, so this one fires too
- **Settings > PHP > Debug > Break at first line in PHP scripts** — the general one, off by default

All three are worth turning off for a lerd project: the first two default to **on**, so unchecking only the third leaves the session still stopping in the bridge. **VS Code** does not do this by default, but `"stopOnEntry": true` in a launch configuration behaves the same way.

The same is worth knowing about `start_with_request=yes`, which is the default: with the debugger listening, *every* request connects, so the first one lands on the bridge before you have asked to debug anything in particular. [On-demand debugging](#on-demand-debugging-workers-and-cli) is the way to keep the debugger quiet until you trigger a session.

## Picking a mode

Xdebug supports several modes: `debug` (step debugging, the default), `coverage` (code coverage collection), `develop`, `profile`, `trace`, `gcstats`, and `off`. Pick one with `--mode`:

```bash
lerd xdebug on --mode coverage        # code coverage for phpunit / pest
lerd xdebug on --mode debug,coverage  # both at once
lerd xdebug on 8.4 --mode trace       # explicit version
```

When combined with PCOV this matters in one direction: if your test runner's `phpunit.xml` prefers PCOV it still wins for coverage, but once you enable Xdebug in `coverage` mode your runner can fall back to Xdebug when PCOV isn't available or is disabled (`pcov.enabled = 0` in `lerd php:ini`). Running Xdebug in `coverage` mode carries the usual runtime cost, so only switch while you actually need coverage.

Re-run `lerd xdebug on --mode <new>` at any time to swap modes without going through `off` first.

## On-demand debugging (workers and CLI)

By default `start_with_request=yes`, so with the debugger listening every request and every running worker tries to connect at once. To debug a single process on demand instead, enable on-demand mode and attach with `pause`:

```bash
lerd xdebug on --on-demand        # start_with_request=trigger — nothing auto-connects
lerd xdebug pause --list          # list running PHP processes that expose a control socket
lerd xdebug pause --pid 1234      # break the IDE into that process
```

`pause` uses Xdebug's [control socket](https://xdebug.org/docs/xdebugctl) (Xdebug >= 3.3, baked into lerd's FPM images) via the `xdebugctl` tool. It is the practical way to debug a **queue/Horizon worker, a scheduled task, or a CLI script**: processes where you can't set a trigger cookie. Run it from a project directory (or pass a site name); lerd resolves the site's container, scopes the candidate list to that site's own processes, and tells the running process to connect to your IDE on port `9003`. The worker must have been started *after* Xdebug was enabled, and your IDE must be listening. Because `xdebugctl` ships only in the shared FPM image, `pause` is PHP-FPM only; FrankenPHP and custom-container sites run their own image without it (the regular `lerd xdebug on` toggle still works on them).

For ordinary web requests under `--on-demand`, use the [Xdebug Helper](https://xdebug.org/docs/step_debug#browser-extensions) browser extension (or append `?XDEBUG_TRIGGER=1`) to trigger a session per page.
