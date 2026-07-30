# Framework Workers

Every framework can define long-running workers (queue consumers, schedulers, WebSocket servers). This page covers the worker commands, conditional rules, conflicts, proxy wiring, project-specific custom workers, and orphan cleanup.

Each framework can define **workers**: long-running processes managed as systemd user services inside the PHP-FPM container.

| Command | Description |
|---|---|
| `lerd worker start <name>` | Start a named worker for the current project |
| `lerd worker stop <name>` | Stop a named worker |
| `lerd worker list` | List all workers defined for this project's framework |

The shortcut commands `lerd queue:start`, `lerd schedule:start`, `lerd reverb:start`, and `lerd horizon:start` are aliases; they look up the worker from the framework definition and delegate to the generic handler. They work for any framework that defines a worker with that name.

## Worker features

**Conditional workers**: Workers with a `check` rule only appear when the condition passes (e.g. `laravel/horizon` is in `composer.json`):

```yaml
workers:
  horizon:
    command: php artisan horizon
    check:
      composer: laravel/horizon
```

**Conflict resolution**: Workers can declare conflicts. When a conflicting worker starts, the other is stopped automatically and hidden from the UI:

```yaml
workers:
  horizon:
    command: php artisan horizon
    conflicts_with:
      - queue      # stops queue before starting horizon; hides queue toggle in UI
```

**WebSocket/HTTP proxy**: Workers that need an nginx proxy block define a `proxy` config. Lerd auto-assigns a collision-free port and regenerates the nginx vhost:

```yaml
workers:
  reverb:
    command: php artisan reverb:start
    proxy:
      path: /app                    # URL path for the proxy location block
      port_env_key: REVERB_SERVER_PORT  # env key holding the port
      default_port: 8080            # starting port for auto-assignment
```

Port assignment scans all proxy port env keys across all sites to prevent collisions between different workers and frameworks.

The generated nginx location anchors on `path`, so `/app` proxies `/app` and everything under it without also swallowing an unrelated route that merely starts with the same letters (`/appstore`, say). Write `path` as a literal URL path; lerd escapes any regex-special characters (a literal `.` in something like `/socket.io`, for instance) before it reaches nginx's location block.

**Server health probe**: A worker whose process can outlive its server (a Vite dev server that dies under `npm` while the Node process lingers) declares a `health` block, so lerd probes reachability rather than mere process liveness:

```yaml
workers:
  vite:
    command: npm run dev
    host: true
    health:
      url_file: public/hot   # a file the server writes on boot, holding its URL
```

`url_file` names a file the dev server writes when it binds (Vite's `public/hot` holds a URL like `http://[::1]:5173`). While the process is up, lerd reads that file and makes a short TCP dial to its host and port; if nothing is accepting, the worker reports **unreachable** instead of running and [worker-heal](/usage/worker-heal) restarts it. A worker with no `health` block keeps the process-only liveness check.

A missing `url_file` is never itself a failure, only a signal lerd cannot use: the worker keeps the process-only check. Plenty of healthy setups never write one, from a Vite config with a custom `hotFile` to `vite build --watch`, and idle-suspend clears `public/hot` while the unit is briefly still up. The failure the probe exists to catch is a *stale* file whose advertised port refuses a connection, which is what a dev server that died behind a live unit leaves behind. A file older than the unit's last activation is a leftover from a previous run and is not dialled.

**Host workers**: Workers that need to run on the host instead of inside the PHP-FPM container set `host: true`. The command runs via fnm at the project's pinned Node.js version. This is used for tools like Vite that need direct filesystem access for HMR:

```yaml
workers:
  vite:
    label: Vite
    command: npm run dev
    restart: on-failure
    host: true
    check:
      file: vite.config.js
```

The `command` is wrapped in `/bin/sh -c` so shell features (`&&`, `|`, env-var expansion, redirects) work as written. A composite command like `npm run build && npm run preview` runs end-to-end without quoting tricks.

Host workers auto-start in three places:

- when a worktree is created, with per-worktree units (`lerd-vite-<site>-<branch>`, supervised by systemd on Linux and launchd on macOS) so multiple Vite instances can run simultaneously with auto-incremented ports.
- at daemon boot, so worktree units recover after a host reboot or `lerd stop && lerd start` even when fsnotify hasn't fired.
- on `lerd worktree remove`, the matching unit is stopped and its file removed; without this the unit would restart-loop against the deleted `WorkingDirectory`.

Host workers run with lerd's bin dir prepended to `PATH`, so subprocesses spawned by `npm run dev` (for example Inertia's wayfinder Vite plugin shelling out to `php artisan`) reach lerd's `php`, `composer` and `laravel` shims and route into the containerised runtime. Stopping a host worker via the UI or `lerd worker stop` is now sticky: a HEAD-write event (commit, checkout, rebase, branch rename) inside a worktree no longer resurrects it, and on macOS the heal loop respects a missing plist as a user-stop signal instead of recreating it.

On macOS the unit is a launchd plist (`~/Library/LaunchAgents/lerd-<worker>-<site>[-<branch>].plist`) backed by a guard script under `~/.local/share/lerd/run/workers/` that `cd`s into the site/worktree and `fnm exec`s the command. The watcher self-heals the unit independently of the worker exec mode, host workers always need launchd-level supervision because they aren't behind podman's `--restart=always`. Scheduled workers (`schedule != ""`) still aren't supported on macOS; launchd's `StartCalendarInterval` isn't wired through the unit translator yet.

**Dev servers on the site's own domain**: A dev server normally advertises its own address, so a Vite app renders asset URLs pointing at `localhost:5173`. That address means nothing to anyone else, so the page arrives unstyled over a share tunnel, over [LAN sharing](/usage/lan-sharing), or on any host other than the one that started it.

lerd puts a supported dev server behind the site's own domain instead. Everything the tool serves lives under one prefix (`/@lerd-vite/`), which the site's vhost proxies to it, so the assets and the hot-reload websocket both travel on whatever hostname the visitor actually used. Nothing needs rewriting, because the client derives its host, port and protocol from the URL it was loaded from.

This needs no configuration and no framework definition. A host worker qualifies when the project has the tool installed and the worker command starts it directly, following one level of `npm run` indirection. A command that only reaches the tool through a runner such as `concurrently` is left alone, since the flags lerd appends would land on the wrong process.

Nothing in the project is edited. lerd writes a generated config to `node_modules/.lerd/` that imports the project's own config and merges in the base, origin and allowed hosts for `serve` only, then starts the tool against it. That file is rewritten on every start, since a worktree seeds `node_modules` from its parent and would otherwise inherit the parent's domain. A project with no config file for the tool, or one that tracks the generated path in git rather than ignoring it, keeps its dev server exactly as it was.

The port is pinned, because the vhost proxies to it and the tool would otherwise drift to the next free one whenever several sites run. It is kept clear of other sites, of the site's own worktrees, and of whatever else the machine is holding, and a pin something has since taken is re-picked rather than left to fail. Each worktree pins its own port and takes its origin from its own subdomain.

Some plugin middleware registers itself ahead of the tool's own base handling and only answers unprefixed, which would 404 on URLs it advertised itself. nginx retries any 404 under the prefix once with the prefix removed, so those routes work without anything having to name them.

When [idle-suspend](/usage/idle-suspend) is enabled it stops every one of a site's workers once the site has been idle, so workers carry no special configuration for it. A worker marked `per_worktree: true` (Vite is the only one by default) is suspended per worktree, on each worktree's own idle timer.

## Project-specific custom workers

Add workers to `.lerd.yaml` for project-specific needs that don't belong in the framework definition:

```yaml
# .lerd.yaml
framework: symfony
framework_version: "8"
workers:
  - messenger
  - pdf-generator
custom_workers:
  pdf-generator:
    label: PDF Generator
    command: php bin/console app:generate-pdfs --daemon
    restart: always
```

Custom workers with proxy support:

```yaml
custom_workers:
  mercure:
    label: Mercure Hub
    command: php bin/console mercure:run
    restart: always
    proxy:
      path: /.well-known/mercure
      port_env_key: MERCURE_PORT
      default_port: 3000
```

Custom workers are merged with the framework's workers at runtime. They are committed to git so teammates get the same setup.

## Worker logs

```bash
journalctl --user -u lerd-messenger-myapp -f
```

## Managing custom workers

Use `lerd worker add` to add project-specific or global custom workers without manually editing YAML:

```bash
# Add a project-specific worker (saved to .lerd.yaml)
lerd worker add pulse --command "php artisan pulse:work" --label "Pulse" --check-composer laravel/pulse

# Add a worker that conflicts with another (stops it on start, hides it in UI)
lerd worker add custom-queue --command "php artisan queue:work --queue=emails" --conflicts-with queue

# Add a global worker (saved to ~/.config/lerd/frameworks/<name>.yaml)
lerd worker add pulse --command "php artisan pulse:work" --global

# Remove a custom worker (stops it if running)
lerd worker remove pulse
lerd worker remove pulse --global
```

Project workers (`.lerd.yaml`) apply to a single project and are committed to git. Global workers (user overlay) apply to all projects using that framework. Both survive framework store updates.

The resulting `.lerd.yaml` looks like:

```yaml
framework: laravel
custom_workers:
  pulse:
    label: Pulse
    command: php artisan pulse:work
    check:
      composer: laravel/pulse
  custom-queue:
    command: php artisan queue:work --queue=emails
    conflicts_with:
      - queue
```

After adding, start the worker with `lerd worker start pulse`.

When running `lerd init --fresh`, existing custom workers are shown in a multi-select step before the workers step. Deselecting a custom worker removes it from `.lerd.yaml` and excludes it from the workers selection. If the removed worker had `conflicts_with`, those workers become available again.

## Orphaned workers

A worker becomes orphaned when its systemd unit is still running but its definition has been removed from `.lerd.yaml` (e.g. after a `git pull` or manual edit). Orphaned workers are detected and surfaced in several places:

- **`lerd worker list`**: shows orphaned workers with a stop hint
- **`lerd worker stop <name>`**: can stop orphaned workers even without a definition
- **`lerd setup`**: offers orphaned workers as pre-selected stop steps before framework worker starts
- **UI**: the stop button works for orphaned workers directly

## Web UI (worker toggles)

Framework workers appear as toggles in the Sites panel. Workers with a `check` rule only appear when the condition passes. Workers with `conflicts_with` suppress each other (e.g. when Horizon is available, the queue toggle is hidden).

Custom framework workers from `.lerd.yaml` also appear as toggles alongside the framework's standard workers.

---

See also: [Frameworks](frameworks.md) for the framework store and Laravel definition; [Framework definitions](framework-definitions.md) for the YAML schema.
