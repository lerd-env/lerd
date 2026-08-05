# Pre-release VM test plan

Internal maintainer checklist, not part of the published docs. Run it before
tagging a release. It exercises the whole app end to end on real guests, not
just the unit suite: install, upgrade, rollback, uninstall, a real framework on
a real site, services, workers, worktrees, and every user-facing surface.

CI and `/lerd-preflight` cover the code. This plan covers the parts only a real
machine can tell you about: the host installer, sudo bootstrap, systemd units,
DNS resolvers, podman networking, and the browser.

---

## The 200 rule

**A phase is not done until a real site answers a real HTTP request with 200.**

CLI output that says "started", a green dashboard pill, and a passing unit test
are all evidence of intent, not of a working stack. After every phase below, run
the check against the site you created in phase 2:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -k https://demo.test        # expect 200
curl -s -o /dev/null -w '%{http_code}\n' http://demo.test            # expect 200 or 301 to https
```

On a `.localhost` guest (external-DNS mode, see phase 3) the same check is:

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://demo.localhost       # expect 200
```

Anything other than 200 (or the documented 301) fails the phase, even if every
command before it printed success. Record the code you actually saw in the
sign-off table, not the code you expected.

---

## Guest matrix

Guests live under **system** libvirt (`qemu:///system`). George is in the
`libvirt` group, so `virsh -c qemu:///system start|console|shutdown` works
without sudo. **Run at most two guests at a time** (the host has 32 GB).

| Guest | Distro | Lane | Why it's in the matrix |
|---|---|---|---|
| `ubuntu26.04` | Ubuntu 26.04 | A — full pass | Primary reference platform, systemd-resolved |
| `ubuntu26.04` (snapshot) | Ubuntu 26.04 | B — upgrade/rollback | Needs an N-1 install to upgrade *from* |
| `omarchy` | Arch, Hyprland | C — DNS variants | NetworkManager + `dns=dnsmasq`, resolved masked |
| `fedora43-2-clone` | Fedora 44 | D — packaging | Homebrew-on-Linux path, `/usr`-owned binary |
| `ubuntu-24` | Ubuntu 24.04 | D — packaging | apt / PPA path, older systemd + podman |
| `silverblue` | Fedora Silverblue | E — immutable | rpm-ostree host, no writable `/usr` |
| `nixos` | NixOS | E — immutable | Non-FHS, no `~/.local/bin` on PATH by default |

Lane A is mandatory every release. Lanes B and C are mandatory for any release
that touches the installer, DNS, or systemd units. Lanes D and E are mandatory
for a minor or major bump, optional for a patch that touches neither packaging
nor the host layer.

Guest notes worth remembering: `omarchy` is LUKS-encrypted and needs George at
the console to unlock, so ask before starting it, and never `pacman -Sy` there.
`ubuntu26.04` ships configured with `dns.tld: localhost`, so flip it back with
`lerd dns:enable` before running lane A, or run lane A's `.test` phases after
phase 3's round trip. `virsh shutdown` is ignored on `ubuntu26.04`: `sync` over
SSH then `virsh destroy`.

---

## Phase 0 — baseline snapshots

Before touching anything, take two snapshots per guest so the destructive phases
are repeatable without a reinstall.

```bash
virsh -c qemu:///system snapshot-create-as ubuntu26.04 clean-no-lerd
```

Then install the **previous stable** release, create a site, and snapshot again
as `n-minus-1-with-site`. Lane B restores that one; every other lane restores
`clean-no-lerd`.

- [ ] `clean-no-lerd` snapshot exists on each guest in the matrix
- [ ] `n-minus-1-with-site` snapshot exists on the lane B guest

---

## Phase 1 — fresh install

Restore `clean-no-lerd` first.

- [ ] Ports 80, 443 and 5300 are free (`ss -ltnp | grep -E ':80|:443|:5300'`)
- [ ] `wget -qO- https://lerd.sh/install.sh | bash` completes without a traceback
- [ ] sudo is asked for **once**, up front, and the prompt names what it needs
- [ ] The one-off `sudo lerd bootstrap --system` step is visible in the output
- [ ] A new shell has `lerd` on PATH and `lerd --version` prints the RC version
- [ ] `lerd status` shows DNS, nginx, watcher healthy, no update notice
- [ ] `lerd doctor` is clean, or every finding is a known pre-existing one
- [ ] `lerd dns:check` walks the full chain and every layer is green

Repeat once with the local-build path on any guest, since the release binary and
the local one exercise different install branches:

```bash
make build && bash install.sh --local ./build/lerd
```

- [ ] `--local` install completes and `lerd status` is healthy

---

## Phase 2 — a real framework on a real site

This is the phase the 200 rule anchors on. Use a genuine framework scaffold, not
an `index.php` with `echo`.

```bash
cd ~/Projects
lerd new demo                # Laravel by default
cd demo
lerd setup --all --skip-open
```

- [ ] `lerd new` scaffolds through the framework's own create command
- [ ] `lerd setup --all` runs composer install, npm install, `lerd env`, and the
      framework's own setup steps (migrations, storage link) without prompting
- [ ] `.lerd.yaml` and `.env` exist, `.env.before_lerd` was written
- [ ] `lerd sites` lists `demo` with the right PHP version and doc root
- [ ] `lerd which` resolves PHP version, Node version, doc root, nginx config
- [ ] **`curl -k -s -o /dev/null -w '%{http_code}' https://demo.test` → 200**
- [ ] The framework's welcome page renders in a browser with a valid padlock
- [ ] `lerd site:doctor` is clean

### The `.test` HTTP / HTTPS toggle

The most-used toggle in the app, and the one with the most moving parts: nginx
vhost, mkcert cert, `.env` `APP_URL`, and the framework's own URL generation.
Test it in both directions, twice, and check both schemes after each flip. A
site is only correct when the *other* scheme behaves correctly too.

Start secured (`lerd secure demo` if it isn't):

- [ ] **`curl -k https://demo.test` → 200**
- [ ] **`curl http://demo.test` → 301 redirecting to `https://demo.test`**
- [ ] `curl https://demo.test` **without** `-k` also returns 200, proving the
      mkcert CA is trusted by the system store, not just tolerated
- [ ] `APP_URL` in `.env` is `https://demo.test`
- [ ] The browser shows a valid padlock, no mixed-content warnings in the console
- [ ] Generated asset and route URLs in the page source are `https://`

Flip to plain HTTP:

- [ ] `lerd unsecure demo`
- [ ] **`curl http://demo.test` → 200**
- [ ] `https://demo.test` no longer serves a redirect loop or a stale cert: it
      refuses cleanly, and the browser shows no half-secured state
- [ ] `APP_URL` in `.env` is `http://demo.test`
- [ ] Page source now emits `http://` asset and route URLs
- [ ] `lerd sites` and the dashboard both show the site as HTTP

Flip back:

- [ ] `lerd secure demo` reissues the cert
- [ ] **`curl -k https://demo.test` → 200** and **`curl http://demo.test` → 301**
- [ ] `APP_URL` is back to `https://`, nothing else in `.env` was rewritten
- [ ] `lerd secure --renew demo` reissues on demand, expiry resets, **https → 200**

Then the same toggle from the other surfaces, since they take different code
paths than the CLI:

- [ ] Dashboard per-site HTTPS toggle: off → **http 200**, on → **https 200**
- [ ] TUI inline toggle does the same
- [ ] Toggle three times in a row with no `lerd restart` in between and confirm
      nginx never ends up serving a stale vhost, **200 after each flip**
- [ ] A second site secured while the first is unsecured: both serve correctly
      at the same time, **200 each on their own scheme**
- [ ] `lerd stop && lerd start` with one secured and one unsecured site: both
      come back on the right scheme, **200 each**

---

## Phase 3 — the `.localhost` path and the DNS round trip

Both TLD modes must serve. Phase 2 covered `.test` on both schemes; this phase
covers external-DNS mode and, more importantly, the transition between the two,
which is where the interesting failures live.

Starting from lerd-managed DNS (`.test`):

- [ ] `lerd dns:disable` tears down `lerd-dns` and moves sites to `*.localhost`
- [ ] `lerd sites` now shows `demo.localhost`
- [ ] **`curl -s -o /dev/null -w '%{http_code}' http://demo.localhost` → 200**
- [ ] `lerd secure` refuses with a clear message about needing managed DNS
- [ ] `lerd init` skips the HTTPS question
- [ ] `lerd dns:check` prints "DNS managed externally", does not probe
- [ ] The dashboard DNS panel shows a `disabled` pill, System tab drops the row,
      the tray shows a muted DNS dot
- [ ] The per-site HTTPS toggle is a muted lock with an explanation

Back the other way:

- [ ] `lerd dns:enable` brings `lerd-dns` up and moves sites to `*.test`
- [ ] Sites that were HTTPS before the disable come back **as HTTPS**, cert
      reissued, `.env` synced to `https://`
- [ ] Sites deliberately left on plain HTTP stay HTTP
- [ ] **`curl -k ... https://demo.test` → 200**

On a guest with a custom TLD configured, confirm the toggle preserves it rather
than flipping it to the canonical default.

---

## Phase 4 — PHP versions

- [ ] `lerd php:list` shows the installed versions
- [ ] `lerd use 8.5` builds/pulls and switches the global version
- [ ] `lerd isolate 8.3` in the site writes `.php-version`, updates `.lerd.yaml`,
      re-links, and `lerd which` reports 8.3
- [ ] **https → 200 on the isolated version**
- [ ] `lerd fetch` pulls a prebuilt base for a version not yet installed
- [ ] `lerd php:rebuild` completes and sites still serve
- [ ] `lerd xdebug on 8.3 --mode debug`, `lerd xdebug status` reflects it,
      **https → 200 with Xdebug loaded** (`lerd php -m | grep xdebug`)
- [ ] `lerd xdebug off 8.3`, **https → 200**
- [ ] `lerd php:ext add redis` rebuilds and the extension is loaded
- [ ] `lerd php:ext remove redis` rebuilds cleanly
- [ ] `lerd php:ini shared` opens in `$EDITOR` and an edit survives a rebuild
- [ ] Legacy tier: `lerd isolate 7.4` on a throwaway site pulls the frozen image
      and serves **200**, then put the site back on 8.4

---

## Phase 5 — services

Add, use, and remove at least one database and one non-database service.

- [ ] `lerd service preset` lists presets, `lerd service search redis` filters
- [ ] `lerd service start mysql` auto-installs on first use and comes up
- [ ] `lerd env` wires `DB_*` into the site `.env` from the preset
- [ ] `lerd db:create demo` creates `demo` and `demo_testing`
- [ ] Migrations run against the service (`lerd artisan migrate`)
- [ ] **https → 200 on a route that hits the database**
- [ ] `lerd service start redis`, env wiring lands, cache/queue driver works
- [ ] `lerd service start mailpit`, its web UI answers on its published port
- [ ] `lerd service list` shows status, version, and the Update column
- [ ] `lerd service port mysql 3307` moves the published port, `.env` follows,
      **https → 200**
- [ ] `lerd service expose mysql 33061:3306` publishes an extra port
- [ ] `lerd service pin redis` / `unpin` persists across a restart
- [ ] `lerd service update mysql` applies the in-strategy update, **https → 200**
- [ ] `lerd service rollback mysql` swaps back, **https → 200**
- [ ] `lerd service migrate mysql <target>` does the dump + restore and the old
      data dir and dump are under `~/.local/share/lerd/backups`
- [ ] `lerd service reinstall redis` comes back at the same version
- [ ] `lerd service remove mailpit` stops and removes it cleanly
- [ ] `lerd service remove mysql --purge` renames the data dir aside as
      `mysql.pre-remove-<ts>` and leaves it recoverable
- [ ] After removing mysql the site degrades honestly: `lerd site:doctor` names
      the missing database rather than the app 500ing silently
- [ ] Reinstall mysql, restore, **https → 200 again**

Database operations:

- [ ] `lerd db:export -o dump.sql` then `lerd db:import dump.sql` round trips
- [ ] `lerd db:snapshot before-change`, change data, `lerd db:restore
      before-change` puts it back, `lerd db:snapshots` lists it,
      `lerd db:snapshot:rm` removes it
- [ ] `lerd db:shell` opens an interactive shell
- [ ] `lerd db:move --from mysql --to mariadb --site demo` moves the schema and
      repoints `.env`, **https → 200**

---

## Phase 6 — workers

- [ ] `lerd queue:start`, dispatch a job, it runs; `lerd queue:stop` stops it
- [ ] `lerd schedule:start` / `stop`
- [ ] `lerd worker list` shows the framework's workers from the store YAML
- [ ] `lerd worker start <name>` / `stop <name>` for a non-queue worker
- [ ] Horizon: `lerd horizon:start`, its dashboard route answers **200**,
      `lerd horizon:reload on` toggles watch mode, `lerd horizon:stop`
- [ ] Reverb: `lerd reverb:start`, a WebSocket client connects, `lerd reverb:stop`
- [ ] Kill a worker process by hand and confirm self-heal restarts it
- [ ] `lerd idle on`, `lerd idle timeout 1m`, wait, confirm workers suspend, then
      hit the site and confirm they resume, **https → 200**
- [ ] `lerd idle pin demo` keeps it awake; `lerd idle status` reports both states
- [ ] `lerd idle off` resumes everything

---

## Phase 7 — git worktrees

Both the wrapper and bare git, since the watcher pipeline has to handle both.

```bash
cd ~/Projects/demo
lerd worktree add -b feat-x
```

- [ ] The wrapper prompts for DB isolation and the frontend build
- [ ] The checkout lands at `~/Projects/demo-feat-x`
- [ ] Dependencies install, env is seeded, a vhost appears
- [ ] `lerd sites` lists the worktree site
- [ ] **`curl -k https://feat-x.demo.test` (or the assigned domain) → 200**
- [ ] With isolated DB: the schema `demo_feat_x` exists, the worktree `.env`
      points at it, `db_isolated: true` is in its `.lerd.yaml`
- [ ] With shared DB: the worktree uses the parent's schema
- [ ] Per-worktree asset worker starts as its own unit and Vite picks a free port
- [ ] `lerd worktree wait ../demo-feat-x --timeout 10m` returns only once the
      pipeline has actually settled
- [ ] Now the bare-git path: `git worktree add ../demo-feat-y -b feat-y`, the
      watcher runs the same pipeline unprompted, **200 on the new site**
- [ ] Restart the daemon (`lerd stop && lerd start`) and confirm per-worktree
      units recover, **200 on both worktrees**
- [ ] `lerd worktree remove feat-y` stops the units before git tears the tree
      down, and no unit restart-loops afterwards (`journalctl --user -u 'lerd-*'`)
- [ ] Remove the isolated one with the drop-database option **off**, re-add the
      branch, and confirm the preserved schema is offered for reuse
- [ ] Remove it again with drop-database **on**, schema is gone
- [ ] **Parent site still → 200 after all worktree churn**

---

## Phase 8 — site lifecycle and sharing

- [ ] `lerd pause demo` swaps in the landing page and stops workers; the landing
      page itself answers **200**
- [ ] `lerd unpause demo` restores the vhost and restarts workers, **200**
- [ ] `lerd restart demo`, **200**
- [ ] `lerd link` with a custom `--domain foo.test`, **200 on foo.test**
- [ ] `lerd unlink` stops serving it (connection refused or 404, not a stale 200)
- [ ] `lerd park ~/Projects` picks up existing projects and a newly created one
- [ ] `lerd unpark ~/Projects` unlinks them
- [ ] Groups: `lerd group add demo admin` serves the secondary at
      `admin.demo.test` → **200**; `lerd group db share` then `separate` both
      work; `lerd group list`; `lerd group remove` restores a standalone domain
- [ ] Workspaces: `add`, `assign`, `move`, `rename`, `list`, `rm` and the
      dashboard grouping reflects each one
- [ ] `lerd lan:share` prints a URL and QR; from a second machine or the host,
      **`curl http://<lan-ip>:<port>` → 200** with assets loading (URL rewriting)
- [ ] `lerd lan:unshare` releases the port
- [ ] `lerd lan:expose` / `lan:status` / `lan:services on|off` / `lan:unexpose`
- [ ] `lerd remote-control full-access on|off|status` gates host actions
- [ ] `lerd share` with one tunnel tool: the public URL answers **200**, then
      stop it. Cover a signup-free one (`--serveo` or `--pinggy`) at minimum
- [ ] Custom container path: a non-PHP project with `Containerfile.lerd` plus
      `container: {port: N}` links, `lerd rebuild` works, **200**

---

## Phase 9 — a second framework

Framework-agnosticism is a design law, so one framework proves nothing. Scaffold
a second one from a different family:

```bash
lerd new shop --framework=symfony     # or wordpress / statamic / craft
cd shop && lerd setup --all --skip-open
```

- [ ] Detection picks the right framework definition
- [ ] `lerd console` maps to that framework's console binary
- [ ] Its env wiring, workers, and doctor checks come from the store YAML
- [ ] **https → 200 on the second site**
- [ ] `lerd framework list` shows both, `lerd framework prune` leaves both alone
- [ ] Both sites serve simultaneously, **200 on each**

---

## Phase 10 — surfaces

Dashboard (drive it in a browser, not with curl):

- [ ] `lerd dashboard` opens `http://127.0.0.1:7073`
- [ ] Sites, services, workers, worktrees all render with live status
- [ ] Creating a site, adding a worktree, and toggling HTTPS from the UI all work
      and the modal streams progress
- [ ] No empty cards or placeholder widgets anywhere
- [ ] The System tab's LAN and remote-access toggles match the CLI state
- [ ] After every UI action, **the affected site still → 200**

TUI:

- [ ] `lerd tui` renders sites, services, workers with live status
- [ ] Detail pane, inline domain and version editing, filter, sort all work
- [ ] Shell drop-in and log tail work
- [ ] Destructive commands are **absent** (scope guard)

Tray:

- [ ] `lerd tray` appears in the system tray, menu actions work
- [ ] `lerd tray icon high-contrast` changes the running icon

Other surfaces:

- [ ] `lerd logs -f` for the site, `nginx`, a service, and a PHP version
- [ ] `lerd dump on`, a `dump()` in a request shows in dashboard, TUI and
      `lerd dump tail`; `dump clear`; `dump off` restores containers
- [ ] `lerd profile on`, load a page, `lerd profile open` shows a flame graph;
      `lerd profile run` on a CLI command; `profile clear`; `profile off`
- [ ] `lerd notify on|target|status|off`, a notification actually arrives
- [ ] `lerd shell` drops into the container with the lerd zsh + starship, and no
      host shell config is bind-mounted
- [ ] `lerd mcp:inject`, an assistant can call the MCP tools; `lerd mcp:eject`
- [ ] `lerd mcp:enable-global` / `mcp:disable-global`
- [ ] `lerd man` browses docs in the terminal
- [ ] `lerd completion bash|zsh|fish` produces working completion
- [ ] `lerd open demo` opens the browser
- [ ] Node: `node:install`, `node:use`, `isolate:node`, `lerd npm run build`
- [ ] `lerd js:runtime bun`, `lerd php:bun install`, a bun build runs, **200**
- [ ] Runtime: `lerd runtime frankenphp` → **200**, `--worker` → **200**,
      `lerd octane:reload on`, then `lerd runtime fpm` → **200**

---

## Phase 11 — diagnostics and housekeeping

- [ ] `lerd doctor` clean; `lerd doctor --json` well-formed with fix tiers
- [ ] Break something on purpose (stop `lerd-nginx`), confirm doctor names it,
      `lerd doctor --fix --dry-run` previews, `--fix --yes` repairs it,
      **https → 200 afterwards**
- [ ] `lerd site:doctor --json` on both sites
- [ ] `lerd check` validates `.lerd.yaml`, and rejects a deliberately broken one
- [ ] `lerd cleanup --dry-run` then `lerd cleanup --yes` reclaims only what it
      listed, and no in-use image, database or volume is touched
- [ ] `lerd cleanup auto status|off|on`
- [ ] `lerd bug-report -o report.txt` anonymizes names by default and
      `--show-real-names` keeps them
- [ ] `lerd tools:update` brings Composer/fnm/mkcert to the current pins
- [ ] `lerd env:check`, `lerd env:override`, `lerd env:restore` round trip
- [ ] `lerd auth ssh` loads a key and `lerd composer` reaches a private repo
- [ ] `lerd stop` then `lerd start`: everything comes back, **200 on both sites**
- [ ] Reboot the guest: with autostart enabled everything comes back on login,
      **200 on both sites without any manual command**
- [ ] `lerd quit` stops everything including `lerd-dns`, UI, watcher and tray

---

## Phase 12 — upgrade and rollback (lane B)

Restore `n-minus-1-with-site`. This lane's whole point is that an existing
install with real sites survives the jump.

- [ ] `lerd --version` reports N-1 and the site answers **200 before upgrading**
- [ ] `lerd status` shows the update notice
- [ ] `lerd whatsnew` lists the changes between N-1 and the RC
- [ ] `lerd update` upgrades after confirmation, without needing a reinstall
- [ ] Config, sites, services and databases all survive untouched
- [ ] **https → 200 immediately after the upgrade, before any manual repair**
- [ ] Any migration the release needs runs automatically or is clearly announced
- [ ] `lerd doctor` clean post-upgrade
- [ ] `lerd update --rollback` reverts to N-1, **https → 200**
- [ ] `lerd update` again returns to the RC, **https → 200**
- [ ] `lerd update --beta` on a guest tracking pre-releases picks the RC
- [ ] On a packaged guest (apt/dnf/brew), `lerd update` **defers** to the package
      manager with the right command instead of self-replacing

Repeat the upgrade leg on the packaging lanes:

- [ ] apt: `sudo apt upgrade` on `ubuntu-24`, **200 after**
- [ ] dnf: `sudo dnf upgrade` on a COPR guest, **200 after**
- [ ] brew: `brew upgrade lerd` on `fedora43-2-clone`, **200 after**

---

## Phase 13 — uninstall

Run last on each guest, because it is destructive.

- [ ] `lerd uninstall` prompts, then stops every container and unit
- [ ] The sudoers rule and the mkcert CA are removed from the system
- [ ] `~/.local/bin/lerd` is gone on a script install; on a packaged install the
      binary **stays** and the matching `apt remove` / `dnf remove` /
      `brew uninstall` command is printed
- [ ] No `lerd-*` units, containers, or networks remain (`podman ps -a`,
      `systemctl --user list-units 'lerd-*'`)
- [ ] `.test` no longer resolves, and the system resolver is back to its
      pre-lerd state (nothing broken, general DNS still works)
- [ ] Project directories, `.env` files and databases on disk are untouched
- [ ] `lerd uninstall --force` skips prompts on a second guest
- [ ] Reinstall on top of the uninstalled machine and **https → 200** again on a
      re-linked existing project, with its data intact

---

## Sign-off

One row per lane per guest. A lane passes only when its final 200 check passed
after the last destructive step in it.

| Lane | Guest | Version tested | Final HTTP code | Result | Notes |
|---|---|---|---|---|---|
| A — full pass | ubuntu26.04 | | | | |
| B — upgrade/rollback | ubuntu26.04 | | | | |
| C — DNS variants | omarchy | | | | |
| D — apt | ubuntu-24 | | | | |
| D — dnf | fedora COPR | | | | |
| D — brew | fedora43-2-clone | | | | |
| E — Silverblue | silverblue | | | | |
| E — NixOS | nixos | | | | |

Anything that failed gets an issue before the tag goes out. A release ships only
when lane A is green and every mandatory lane for that release type is green.

Shut the guests down afterwards (`sync` over SSH, then `virsh destroy` on the
ones that ignore ACPI) and restore `clean-no-lerd` so the next release starts
from the same baseline.
