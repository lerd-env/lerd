# Start, Stop & Autostart

Day-to-day lifecycle commands for the entire lerd stack: DNS, nginx, PHP-FPM containers, services, workers, the Web UI, the watcher, and the system tray.

::: tip You don't need to run `lerd start` after installing
`lerd install` already starts everything for you on first run: it boots `lerd-dns`, `lerd-nginx`, the `lerd-watcher`, and the system tray. Services like MySQL or Redis are started on demand the first time something needs them (`lerd service start`, `lerd init`, or `lerd env`). Reach for `lerd start` only after a `lerd stop`, a reboot without autostart enabled, or after you've manually killed containers.
:::

---

## Commands at a glance

| Command | Stops | Starts |
|---|---|---|
| `lerd start` | nothing | DNS, nginx, watcher, tray, all PHP-FPM containers in use, services that were running before stop, queue / schedule / reverb / messenger workers, stripe listeners, Web UI |
| `lerd stop` | All containers and workers above **except** `lerd-dns`. Leaves the watcher, Web UI, and the DNS forwarder alone. | nothing |
| `lerd quit` | Everything `lerd stop` does, **plus** the DNS forwarder, Web UI, watcher, and tray. macOS: also stops the Podman Machine VM. | nothing |

`lerd stop` is the everyday "give my laptop back its CPU" command. `lerd quit` is a full shutdown: use it before a reinstall, a system reboot without autostart, or when you really want lerd out of the way.

---

## `lerd start`

```bash
lerd start
```

Walks the install in dependency order:

1. Pre-flight: checks for **port conflicts** on 53, 80, and 443; refuses to start if another process is bound.
2. Rebuilds or pulls any missing container images (e.g. after a `podman rmi` or a podman cleanup).
3. Boots core: `lerd-dns`, `lerd-nginx`, `lerd-watcher`.
4. Boots every PHP-FPM container that has at least one site referencing its version. Unused PHP versions stay stopped.
5. Boots all installed services that are **not** marked as manually paused (see [Manually stopped services](services.md#manually-stopped-services) for the pause-state contract).
6. Restores per-site workers (`lerd-queue-*`, `lerd-schedule-*`, `lerd-reverb-*`, `lerd-messenger-*`, custom workers) and stripe listeners (`lerd-stripe-*`) from the `workers` list saved in each site's `.lerd.yaml`.
7. Starts the Web UI (`lerd-ui`) and the system tray.

A live spinner shows the per-unit progress. If a single SSL vhost references a missing certificate file, lerd switches that site back to HTTP automatically and continues; one broken cert no longer blocks the whole nginx start.

::: info After a reinstall
If you ran `lerd uninstall` and then reinstalled, worker units and service quadlets are recreated by `lerd start` from each site's `.lerd.yaml`. Sites with a committed `.lerd.yaml` come back fully wired up. Sites without one need their workers restarted manually.
:::

::: info After the binary moves
The `lerd-ui`, `lerd-watcher` and `lerd-tray` services and the shims on your `PATH` (`php`, `composer`, `laravel`, the client tools) all record where the lerd binary is. A package manager that installs each version into its own directory, Homebrew above all, retires that path on the next upgrade. On Linux, `brew upgrade lerd` repoints them as part of the upgrade and restarts the daemons, which matters on a machine that upgrades its packages unattended. Otherwise `lerd start` rewrites the services to point at the binary that is running and repoints any shim whose path has gone, and the shims themselves fall back to whatever `lerd` is on your `PATH`.
:::

::: info Deleted project directories are auto-cleaned
`lerd-watcher` removes sites from `sites.yaml` whenever their project directory disappears on disk. Two paths do this:

- **Instant**: fsnotify on every parked directory (configured via `lerd park`). When a direct subdirectory gets deleted, the corresponding site is unlinked within milliseconds.
- **Periodic**: every 30 seconds the watcher sweeps the full site registry (parked and non-parked) and removes any site whose path no longer exists. The UI refreshes via the sites eventbus so the dashboard reflects the removal without a manual page reload.

Either way the site gets the same teardown `lerd unlink` performs: its workers are stopped, its shares closed, its vhost and certificates dropped, any per-site container removed, and its recorded request timings forgotten. Its worktrees go with it, so the branch subdomains stop being served and their workers, LAN shares and tunnels are released too. Isolated worktree databases are kept, as they are when you remove a worktree by hand, so nothing you have not backed up is deleted behind your back.

Both paths skip `Ignored: true` sites, those are explicitly parked by the user (e.g. via `lerd unpark` leaving a tombstone) and must not be reaped.
:::

---

## `lerd stop`

```bash
lerd stop
```

Stops everything `lerd start` started **except** the Web UI, watcher, tray, and the `lerd-dns` forwarder; those keep running so the dashboard stays reachable to bring lerd back up.

A few important details:

- **The DNS forwarder stays up.** `lerd-dns` is treated as install-level plumbing: the system resolver keeps pointing `.test` at it until `lerd uninstall`, so stopping it would leave the resolver aimed at a dead port and make `.test` lookups stall. It is only torn down by `lerd quit` or `lerd uninstall`.
- **Manually paused services are remembered.** If you stopped Mailpit earlier with `lerd service stop mailpit`, then `lerd stop` + `lerd start` will not bring Mailpit back. The pause flag survives the cycle.
- **Pinned services start anyway.** A `lerd service pin <name>` overrides auto-stop logic; pinned services are always started by `lerd start` regardless of which sites are active.
- **Worker state is preserved.** Workers running before `lerd stop` are restarted by the next `lerd start`; workers you manually stopped stay stopped.

---

## `lerd quit`

```bash
lerd quit
```

The full off-switch:

1. Runs everything `lerd stop` does.
2. Stops `lerd-ui` (Web UI).
3. Stops `lerd-watcher`.
4. Kills the system tray process.
5. Stops the `lerd-dns` forwarder. Unlike `lerd stop`, quit is a full teardown, so it takes DNS down too. The watcher is stopped first (step 3) because it is the only thing that would restart `lerd-dns`.
6. **macOS only:** stops the Podman Machine VM.

After `lerd quit` there are no lerd processes left running. On macOS the Podman Machine VM is also shut down, so `lerd start` will bring it back up on the next run. This is the right command before a reinstall or before pulling a major update. Before a reboot it is optional on macOS, where the watcher now runs the same teardown for you.

The system tray's **Quit Lerd** menu item calls `lerd quit`.

---

## Shutdown on logout or restart (macOS)

On macOS you no longer have to remember `lerd quit` before rebooting. When you log out, restart, or shut down, launchd sends the watcher a `SIGTERM` and the watcher tears lerd down before it exits:

1. Stops containers, services, and workers.
2. **Stops the Podman Machine VM.**
3. Stops `lerd-ui`, `lerd-tray`, and `lerd-dns`.
4. Exits, which lets launchd finish terminating the session.

This matters most for databases. Killing the VM with a database mid-write leaves the data files dirty, and the container spends minutes replaying its write-ahead log on the next start; TimescaleDB and Postgres are the usual victims. Stopping the VM properly avoids that recovery pass entirely.

The VM goes down at step 2 rather than last precisely because it is the step whose loss costs something. The host processes above it are ones launchd is terminating anyway, so if the exit grace ever runs out they are the right thing to lose. That grace is 180 seconds for the watcher; every other lerd job keeps launchd's default, which `launchctl print` reports as 5 seconds.

The watcher needs that much because the containers stop before the VM does, and a database is entitled to the `stop_timeout` its service declares, 60 seconds for MySQL and Postgres, to finish writing. A grace sized for the containers alone would already be spent by the time the VM stop began, and launchd would kill the watcher partway through the one step this whole sequence exists to protect.

The watcher never stops its own unit here. Asking launchd to bootout the job the watcher is running inside would block until that process exits, which it cannot do from inside the call, so the teardown would hang until the grace expired and never reach the VM at all.

### Restarting the watcher does not tear anything down

A signal on its own does not mean the machine is going away. `lerd install`, `lerd update` and `lerd quit` all stop the watcher too, and launchd and systemd deliver those as exactly the same `SIGTERM` a logout does. Tearing the environment down there would stop every container and the Podman Machine VM in the middle of an install.

So lerd marks its own stops. Every path that stops or restarts the watcher writes a short-lived marker first, and the watcher reads it as "this came from lerd" and exits without running the teardown. You will see this in `~/Library/Logs/lerd/lerd-watcher.log`:

```
lerd watcher: received terminated from lerd itself, exiting without teardown
lerd watcher: received terminated, stopping lerd
```

The first is lerd restarting its own watcher. The second is a real logout.

The marker expires after a minute, so a watcher that was killed before it could read one can never leave a later logout suppressed. Booting the watcher out by hand (`launchctl bootout`) is not marked and does run the full teardown, which is the same thing `lerd quit` would have done.

Only `SIGTERM` counts as a logout. Neither launchd nor systemd signals a shutdown any other way, so running `lerd watch` in a terminal and pressing Ctrl-C exits the watcher and leaves everything else running.

Because this runs the full teardown, lerd is left in the stopped state after a reboot, exactly as if you had run `lerd stop`. That is deliberate: `lerd-ui`'s health watcher may still be alive while the machine is powering off, and without the marker it would read every unit being stopped as a crash and fire heal attempts and notifications against the teardown. If [autostart](#autostart-on-login) is enabled the marker is cleared automatically when `lerd-ui` comes back up; otherwise run `lerd start`.

::: tip Linux
None of this runs on Linux, and nothing is missing there. Podman runs natively, so there is no VM whose loss would cost you a crash recovery, and systemd already stops the container units on shutdown honouring the `stop_timeout` each one declares. The watcher simply exits when it is signalled, which also keeps `systemctl --user stop lerd-watcher` meaning what it says.
:::

---

## `lerd machine reset` (macOS)

```bash
lerd machine reset        # asks for confirmation first
lerd machine reset --yes  # skip the prompt
```

Recreates the Podman Machine VM. Reach for it only when `lerd start` reports a container-storage error such as `getting graph driver info ... overlay: invalid argument`, which happens after the macOS host is shut down ungracefully while the VM is still running and leaves the VM's container storage corrupt. See [Troubleshooting → Podman Machine overlay-storage error](../troubleshooting.md).

The command stops the VM, removes it (`podman machine rm -f`), and re-initialises it. **Your data is preserved:** lerd bind-mounts every database and site directory to the host, not into the VM, so only the VM's container storage and images are discarded. Images are rebuilt automatically on the next `lerd start`.

::: tip lerd start already tries to self-heal
On macOS, `lerd start` detects this exact error and attempts an automatic recovery first (remount the VM's storage, rebuild the stale containers, retry once). `lerd machine reset` is the manual fallback for when that recovery isn't enough. This command is macOS-only; Linux runs podman natively with no VM.
:::

---

## Autostart on login

Lerd can boot itself every time you log in. Autostart is a single switch over every lerd-owned systemd user unit on the machine:

- the dashboard (`lerd-ui.service`), project watcher (`lerd-watcher.service`) and system tray (`lerd-tray.service`)
- every container quadlet (`lerd-mysql`, `lerd-nginx`, `lerd-redis`, `lerd-postgres`, `lerd-dns`, `lerd-php*-fpm`, `lerd-mailpit`, `lerd-meilisearch`, `lerd-minio`, `lerd-rustfs`)
- every per-site worker, queue, schedule, horizon, reverb, and stripe-listen unit

```bash
lerd autostart enable      # boot lerd on every login
lerd autostart disable     # stop booting on login
```

`lerd autostart enable` runs `systemctl --user enable` on the full set; `lerd autostart disable` runs the matching `disable`. The dashboard's enabled state is the canonical "is autostart on" indicator surfaced by the UI and tray.

The switch is sticky. A worker you enable later, while autostart is off, stays disarmed as well, so it cannot come back on the next boot ahead of the databases and caches it needs and crash-restart against them.

The same toggle also appears in the **System Tray** menu under **Autostart**; see [System Tray](../features/system-tray.md).

The tray unit (`lerd-tray.service`) is wired to `graphical-session.target` and so requires a desktop environment that reaches that target on login: GNOME, KDE Plasma, and any compositor launched through `uwsm` (Omarchy's Hyprland setup included). Bare Hyprland / Sway / i3 launched without `uwsm` won't autostart the tray; see [System Tray, Autostart](../features/system-tray.md#autostart) for the workaround. Every other lerd unit uses `default.target` and is unaffected.

---

## From the Web UI

The dashboard at `http://127.0.0.1:7073` has **Start** and **Stop** buttons in the header:

- **Start** appears only when one or more core services (DNS, nginx, PHP-FPM) are not running. Clicking it calls `lerd start` via the API.
- **Stop** is always visible while lerd is running. Clicking it calls `lerd stop`.
- The tray's **Quit Lerd** menu item calls `lerd quit` (full shutdown including the UI).

These map one-to-one to the CLI commands above, no special UI-only behaviour.

---

## Status & verification

```bash
lerd status
```

Shows a live snapshot: DNS reachability, nginx, PHP-FPM containers, watcher, host tools, services, certificate expiry, and LAN exposure. Run it after every `lerd start` to confirm everything is healthy. See [Troubleshooting](../troubleshooting.md) if anything is reported as down.

The `[Tools]` section lists the host binaries lerd manages (Composer, fnm, mkcert) with their installed versions, and flags any that differ from the versions lerd currently pins. The same information appears in the web UI under System > Tools, where the pending version on a tool's card is a button that updates that one tool. Apply pending updates from the terminal with:

```bash
lerd tools:update
```

The pins are read from a manifest that is cached for a day, so a newly published pin can take that long to show up on its own. "Check for updates" on the Tools page re-reads it immediately, and a tool that falls behind also raises an `update_available` notification.

Tools that are already at their pinned version are left untouched, and tools that are deliberately absent (fnm on an nvm-managed setup) are skipped. A tool whose version shows as unknown, for example Composer after a `composer self-update`, is re-downloaded at the pinned version.

Each tool is an independent download, so one that cannot be updated does not stop the others. The run carries on and closes with a count of what failed.

### Where the pins come from

The pinned versions live in `internal/tools/tools.yaml`. A copy is embedded in the binary as the offline fallback, and the published one is fetched before a download so a bad pin can be fixed without a release.

Because that file reaches every install without going through a release, a published pin is only honoured when its URL points at a host lerd downloads tools from: `getcomposer.org`, `github.com`, and GitHub's asset hosts. Anything else falls back to the embedded pin. Set `LERD_TOOLS_HOSTS` to a comma-separated list to allow additional hosts, and `LERD_TOOLS_URL` to fetch the manifest from somewhere other than GitHub.

A pin may also carry a `digests` map alongside `assets`, giving the sha256 of each platform's asset:

```yaml
tools:
  mkcert:
    version: v1.4.4
    url: https://github.com/FiloSottile/mkcert/releases/download/{version}/{asset}
    assets:
      linux/amd64: mkcert-{version}-linux-amd64
    digests:
      linux/amd64: 6d31c65b03972c6dc4a14ab429f2928300518b26503f58723e532d1b0a3bbb52
```

Where a digest is given the download is checked against it and rejected on a mismatch. The field is optional, so a manifest without it installs exactly as before, and binaries that predate the field ignore it. Either way a download is written to a temporary file and moved into place only once it is complete, so a failed or rejected download never replaces a working binary.

---

## Cheat sheet

| Situation | Command |
|---|---|
| Just installed lerd | Nothing, `lerd install` already started everything |
| Coming back to your laptop after `lerd stop` | `lerd start` |
| Reboot, autostart disabled | `lerd start` |
| Reboot, autostart enabled | Nothing, happens automatically |
| Free up CPU / RAM during a heavy build | `lerd stop` |
| Full shutdown before a reinstall | `lerd quit` |
| `lerd start` fails with an overlay / graph-driver storage error (macOS) | `lerd machine reset` |
| Verify everything's healthy | `lerd status` |
| Update Composer / fnm / mkcert to their pinned versions | `lerd tools:update` |
| Uninstall a service entirely (data preserved) | `lerd service remove <name>` |
| Uninstall and wipe data (snapshots the databases first) | `lerd service remove <name> --purge` |
| Reinstall a service in place | `lerd service reinstall <name>` |
| Reinstall with fresh data + reprovision linked sites (snapshots the databases first) | `lerd service reinstall <name> --reset-data` |
