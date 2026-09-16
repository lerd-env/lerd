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
curl -s -o /dev/null -w '%{http_code}\n' http://demo.test            # expect 200 or 302 to https
```

On a `.localhost` guest (external-DNS mode, see phase 3) the same check is:

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://demo.localhost       # expect 200
```

Anything other than 200 (or the documented 302) fails the phase, even if every
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
| `silverblue` | Fedora Silverblue | E — immutable | rpm-ostree host, no writable `/usr` |
| `bazzite` | Bazzite (Kinoite) | E — immutable | The one guest kept in `.localhost` mode |

Lane A is mandatory every release. Lanes B and C are mandatory for any release
that touches the installer, DNS, or systemd units. Lanes D and E are mandatory
for a minor or major bump, optional for a patch that touches neither packaging
nor the host layer.

Guest notes worth remembering: never `pacman -Sy` on `omarchy`, its package set
is old enough that a partial upgrade breaks it, and it is the only guest with a
real graphical session, so the tray and a spawned terminal only appear there.
`bazzite` is deliberately left on `.localhost`, so put it back with
`lerd dns:disable` if a run converts it, and it is the guest lane E's
external-DNS phases belong on. `virsh shutdown` is ignored on `ubuntu26.04`:
`sync` over SSH then `virsh destroy`. The `ubuntu25.10` domain is not Ubuntu, it
boots Zorin to a login screen with no sshd, so nothing in this plan can drive
it.

---

## What 1.35.0 adds to this plan

The checks a release brings live in the phase they belong to rather than in a
section of their own, so a phase is always the whole story for its subject. This
index is only a reminder of where 1.35.0's went, and of what to delete from the
phases once the next release makes it ordinary.

| Change | Phase |
|---|---|
| PHP on the host: the native runtime and every surface that reads it | 14 |
| `.lerd.local.yaml` overriding the committed project config | 2 |
| A fetched version with no runtime behind it saying so | 4 |
| A service on its own `.test` domain | 5, 11 |
| Scheduled database snapshots and `db:snapshot:keep` | 5 |
| A worker declaring its dev server and owning its port | 6 |
| `worktree_include` | 7 |
| A re-link keeping what only the registry knew | 8 |
| Detection reading every major's rules, and the env file a framework has | 9 |
| `lerd wp` and `lerd drush` running their wrapper | 9 |
| A package routing a globally installed CLI onto the host PHP | 9, 14 |
| Eleven dashboard themes kept in the config, and the share line | 10 |
| A worker's logs one click from its toggle | 10 |
| A shim on PATH not reapplying the whole environment | 10 |
| Doctor catching a host php in front of the shim | 11 |
| Disk reclaimed and reported honestly | 11, 14 |
| Rolling backups of the site registry and `lerd sites:restore` | 11 |
| A catalogue refresh in two seconds instead of fourteen | 11 |
| Beta updates following the beta line, `lerd update:beta` | 12 |
| An update no longer doing its own install pass twice | 12 |

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
- [ ] Re-running the installer on this machine does **not** ask the DNS question
      again and does not change the mode it settled on

Nothing may download without saying so first, and there has to be a way to say
no. The estimates are read off the registry manifest, so working them out must
itself download nothing.

- [ ] `lerd start --dry-run` reports what a start would pull or rebuild, and
      exits without starting anything
- [ ] Every command that can pull names the image and roughly its size before
      the first byte moves: `start`, `install`, `fetch`, `php:rebuild`, the
      service install and update paths, the FrankenPHP switch
- [ ] `--no-pull` and `LERD_OFFLINE=1` skip pulls and rebuilds unless the image
      is missing outright, and an already-working stack still starts
- [ ] Pull an image with the network down: the failure names the image, and
      nothing is left half-installed

A host with IPv6 turned off used to exit every container, so it gets its own
pass on one guest:

- [ ] With IPv6 disabled on the host, `lerd install --no-ipv6` (or
      `LERD_DISABLE_IPV6=1`) brings the stack up and **https → 200**

Then the ways in that are not a terminal:

- [ ] Opening the dashboard on a stopped lerd offers a Start button, and the
      start streams back unit by unit rather than leaving a dead page
- [ ] The Linux application entry cold-starts lerd and reports its progress
- [ ] macOS: the Lerd app's splash reads the same stream

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
- [ ] `.env` exists, and the backup is named after the file the framework
      actually reads and sits beside it (`.env.before_lerd` at the root for a
      plain dotenv project). `.lerd.yaml` is written on demand, by the commands
      that have something to record in it, so a freshly scaffolded project not
      having one yet is correct
- [ ] `lerd sites` lists `demo` with the right PHP version and doc root
- [ ] `lerd which` resolves PHP version, Node version, doc root, nginx config
- [ ] **`curl -k -s -o /dev/null -w '%{http_code}' https://demo.test` → 200**
- [ ] The framework's welcome page renders in a browser with a valid padlock
- [ ] `lerd site:doctor` is clean

A `.lerd.yaml` is committed, so the answers that belong to this machine alone
live beside it and have to win without ever being written back to the repo:

- [ ] A key set in `.lerd.local.yaml` overrides the same key in `.lerd.yaml`,
      and a save from any command keeps the local key out of the committed file
- [ ] A PHP or Node version pinned only in the local file is the one the link
      applies and the one `lerd which` reports
- [ ] Changing a key the local file owns from the CLI says so rather than
      appearing to take
- [ ] `git status` in the project is clean after all of it

### The `.test` HTTP / HTTPS toggle

The most-used toggle in the app, and the one with the most moving parts: nginx
vhost, mkcert cert, `.env` `APP_URL`, and the framework's own URL generation.
Test it in both directions, twice, and check both schemes after each flip. A
site is only correct when the *other* scheme behaves correctly too.

Start secured (`lerd secure demo` if it isn't):

- [ ] **`curl -k https://demo.test` → 200**
- [ ] **`curl http://demo.test` → 302 redirecting to `https://demo.test`**
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
- [ ] **`curl -k https://demo.test` → 200** and **`curl http://demo.test` → 302**
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
- [ ] Prerelease tier: a bare `lerd fetch` leaves 8.6 alone, `lerd use 8.6`
      marks it as a prerelease wherever a version is picked, **https → 200**
      on it, and FrankenPHP does not offer it
- [ ] On 8.6 the image ships redis, imagick and mongodb (built without pecl,
      which 8.6 removed) and advertises no igbinary, pcov or xdebug
- [ ] `lerd shell 8.5` drops into that version's container from anywhere, and
      the dashboard's shell button opens the same one
- [ ] `lerd php:ports` and `lerd php:pkg` report the version's ports and packages
- [ ] `lerd php:rebuild` discloses the base image and its size before pulling
- [ ] `lerd fetch` names the versions whose image is built but that nothing
      serves from yet, and points at `lerd php:rebuild`, so `lerd php:list` and
      `lerd new` no longer contradict the command before them

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

A service can be reached on a domain rather than on a port:

- [ ] A preset that declares one is served at its own `.test` domain, answers a
      browser preflight there, and `lerd env` writes that domain into the site
- [ ] The app and the browser use the same address, and `lerd site:doctor` calls
      the site wired whether its PHP runs in a container or on the host

Database operations:

- [ ] `lerd db:export -o dump.sql` then `lerd db:import dump.sql` round trips
- [ ] `lerd db:snapshot before-change`, change data, `lerd db:restore
      before-change` puts it back, `lerd db:snapshots` lists it,
      `lerd db:snapshot:rm` removes it
- [ ] `lerd db:snapshot:auto` schedules snapshots globally and per site,
      retention drops the oldest past the window, and `lerd db:snapshot:keep`
      exempts one from it for good
- [ ] `lerd db:shell` opens an interactive shell
- [ ] `lerd db:move --from mysql --to mariadb --site demo` moves the schema and
      repoints `.env`, **https → 200**
- [ ] `lerd db:extension` lists what the engine can create, and adds one
- [ ] `lerd minio:migrate` moves an existing MinIO volume onto RustFS

Every service path that can pull says what it will fetch first:

- [ ] The install, update, migrate, rollback and reinstall paths each name the
      image and roughly its size before downloading
- [ ] The dashboard turns the same estimate into a confirmation naming the image
      and the download, and an image already in the local store needs no click
- [ ] The MCP server answers with the image and its size instead of starting a
      download on behalf of someone who never typed the command

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

The named start commands are generated from the framework definition now, so
what they accept has to come from the definition rather than from a fixed set:

- [ ] Each generated command's flags match its worker's `tune_command`
      placeholders, and each default is the value in the plain command
- [ ] A worker with a reload variant gets its reload toggle the same way
- [ ] A project cloned but not linked yet still gets its worker commands,
      resolved from the framework named in `.lerd.yaml`, and they answer with
      the link hint rather than an unknown command
- [ ] A worker declaring `requires_service` refuses to start without it, and its
      unit orders after that service rather than racing it at boot

A worker can also declare the dev server it starts and hold the port it needs:

- [ ] A worker declaring a `dev_server` starts it, the page it serves answers,
      and the host reaches it through the worker's proxy
- [ ] Its `dev_server_port` is held while the worker runs, carried into the
      container, and released when it stops
- [ ] Two sites running such a worker at once do not collide on a port

A project's answers to those flags are committed rather than retyped:

- [ ] Passing a flag to a start command writes it under `worker_options` in
      `.lerd.yaml` instead of tuning a single run
- [ ] A value equal to the framework's default is not stored, so a later store
      change to that default still lands
- [ ] A value carrying whitespace is refused
- [ ] The dashboard's gear beside a worker offers one field per declared option,
      prefilled from the project and showing the definition's default, and
      saving restarts a running worker
- [ ] macOS: `lerd workers mode` shows and sets how workers are launched. On
      Linux the command is config only and reports nothing about what is running

The store's package layer sits under the definitions:

- [ ] `lerd framework list` prints every package the store publishes, what each
      declares, which file answers for the project you are in, and whether that
      project requires it, reading the cache without fetching
- [ ] A package declaration wins a name collision with a version file, and a
      user overlay and the project's `.lerd.yaml` still sit above both
- [ ] With the network down, a version never fetched falls back to the newest
      cached file below it rather than losing the worker

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
- [ ] `lerd db:isolate --source main` clones the parent's schema into
      `<parent_db>_<branch>`, repoints the worktree's env key and writes
      `db_isolated: true`, **200**
- [ ] A bare `lerd db:isolate` takes its documented default and starts the
      schema **empty**, so a framework that keeps sessions or cache in the
      database answers 500 until something populates it. That is the contract,
      not a fault: the check is that the empty schema exists and `db:share`
      recovers, not that the site keeps serving
- [ ] `lerd db:isolate --source <branch>` clones from another isolated worktree
- [ ] `lerd db:share` drops the isolated schema and puts the worktree back on
      the parent's, **200**
- [ ] Paths listed under `worktree_include` are copied into a fresh worktree,
      and one the worktree already carries is left as git checked it out
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
- [ ] Workspaces: `lerd workspace add`, `assign`, `move`, `rename`, `list`, `rm`
      and the dashboard grouping reflects each one
- [ ] `lerd lan:share` prints a URL and QR; from a second machine or the host,
      **`curl http://<lan-ip>:<port>` → 200** with assets loading (URL rewriting)
- [ ] `lerd lan:unshare` releases the port
- [ ] `lerd lan:expose` / `lan:status` / `lan:services on|off` / `lan:unexpose`
- [ ] `lerd remote-setup` pairs a device, and `lerd remote-control full-access
      on|off|status` gates host actions
- [ ] `lerd domain add` and `remove` on a site that already carries the TLD:
      the name is not doubled, and each domain serves **200**
- [ ] A domain declared in the project's `.lerd.yaml` is registered once, with
      the TLD applied once
- [ ] `lerd share:tool`, `share:domain` and `share:token` record the tunnel
      settings, and `lerd share` then uses them without asking again
- [ ] `lerd share` with one tunnel tool: the public URL answers **200**, then
      stop it. Cover a signup-free one (`--serveo` or `--pinggy`) at minimum
- [ ] `lerd stripe:config` stores the keys and `lerd stripe:listen` **starts**
      the listener (it once stopped one instead), then stops it
- [ ] `lerd nginx` opens the site's override, a location-scope block survives a
      `lerd restart`, and `lerd nginx reset` puts it back, **200** after each
- [ ] A re-link re-detects the framework and keeps everything only the registry
      knew: approved host commands, the pinned dev-server and worker ports, the
      LAN and public share ports, the group, the idle bookkeeping, the per
      machine `APP_URL` override and the paused state
- [ ] `lerd import` pulls a project in from another local environment
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
- [ ] A framework shipping one definition per major is matched on the right
      major's own rules: CodeIgniter 3 and 4 both link, each on the PHP range
      its definition declares, rather than one file claiming the name
- [ ] On a framework whose configuration is not a dotenv file (CakePHP,
      WordPress, Magento), the backup sits beside the file lerd edits and
      `lerd env:restore` puts it back over that file
- [ ] `lerd console` maps to that framework's console binary
- [ ] Its env wiring, workers, and doctor checks come from the store YAML
- [ ] **https → 200 on the second site.** A bare skeleton with no routes of its
      own answers 404 on `/` by design (Symfony serves its welcome page with
      that status), so either scaffold one route first or read the check as
      "the framework answered", not "200"
- [ ] `lerd framework list` shows both, `lerd framework prune` leaves both alone
- [ ] Both sites serve simultaneously, **200 on each**
- [ ] A package the project requires contributes its workers, commands, setup
      steps and doctor checks to whichever framework carries it, and a package
      scoped to one framework stays out of the other
- [ ] `lerd sail` maps a Sail-shaped project onto lerd's own containers
- [ ] `lerd wp` on WordPress and `lerd drush` on Drupal run the command rather
      than printing their own wrapper's source and exiting clean
- [ ] A package that installs a binary globally puts it where the host can run
      it and reports which binaries it moved

NativePHP is supported end to end, and both halves need a look:

- [ ] A NativePHP desktop project links, `lerd setup --all` completes, and its
      workers and commands come from the package layer rather than the framework
- [ ] The mobile half builds and its commands are offered on Laravel only
- [ ] `lerd site:doctor` runs the package's own checks and names what is missing

---

## Phase 10 — surfaces

Dashboard (drive it in a browser, not with curl):

- [ ] `lerd dashboard` opens `http://127.0.0.1:7073`
- [ ] Sites, services, workers, worktrees all render with live status
- [ ] Creating a site, adding a worktree, and toggling HTTPS from the UI all work
      and the modal streams progress
- [ ] No empty cards or placeholder widgets anywhere
- [ ] The System tab's LAN and remote-access toggles match the CLI state
- [ ] A command pinned to a site's control row runs from there, survives a
      reload, and unpins again
- [ ] The shell button opens the terminal the desktop is configured to use, not
      whatever happens to be on PATH, on both platforms
- [ ] Selecting a service that is not a database engine requests neither its
      databases nor its snapshots, and logs no 404
- [ ] The service Env tab matches the current card style and its copy button
      behaves like every other one
- [ ] At phone width the back bar names the site rather than the rest of the
      route
- [ ] After every UI action, **the affected site still → 200**

Themes:

- [ ] Each of the twelve themes applies, and its accent reads correctly as link
      text on both the light and the dark tone
- [ ] The choice lives in the config, not in browser storage: it survives a hard
      reload, follows you to another browser, and reaches a second open
      dashboard the moment it changes
- [ ] The installed app's own chrome takes the theme too
- [ ] The Theme card's share line opens a prefilled post on X, Bluesky or
      Reddit, about the theme and carrying no list of local sites

TUI:

- [ ] `lerd tui` renders sites, services, workers with live status
- [ ] Detail pane, inline domain and version editing, filter, sort all work
- [ ] Shell drop-in and log tail work
- [ ] The databases pane lists the databases and opens one
- [ ] A service's client tools, tuning and entities are reachable, matching what
      the web UI offers
- [ ] Services with a web dashboard are marked, and opening one works
- [ ] Destructive commands are **absent** (scope guard)

Tray:

- [ ] `lerd tray` appears in the system tray, menu actions work
- [ ] `lerd tray icon high-contrast` changes the running icon
- [ ] The tray can be turned off, and everything else keeps working with no
      tray unit running and nothing reporting it as broken

Other surfaces:

- [ ] `lerd logs -f` for the site, `nginx`, a service, and a PHP version
- [ ] Every worker toggle's second segment opens the Logs tab with that worker
      already selected, the address reads `sites/<domain>/logs/<source>` and
      survives a reload, and a site with no Logs tab keeps the plain toggle
- [ ] The app logs tab refreshes on its own while it is the selected source,
      keeps the reader's scroll position, and stops polling for a site the idle
      engine has suspended
- [ ] The tinker editor highlights PHP the way the editor does everywhere else
- [ ] `lerd dump on`, a `dump()` in a request shows in dashboard, TUI and
      `lerd dump tail`; `dump clear`; `dump off` restores containers
- [ ] A worker whose dump bridge never loaded costs the dump, not the request:
      the site answers **200** instead of a 500 from an invalid callback
- [ ] `lerd profile on`, load a page, `lerd profile open` shows a flame graph;
      `lerd profile run` on a CLI command; `profile clear`; `profile off`
- [ ] `lerd notify on|target|status|off`, a notification actually arrives
- [ ] `lerd shell` drops into the container with the lerd zsh + starship, and no
      host shell config is bind-mounted
- [ ] `lerd mcp:inject`, an assistant can call the MCP tools; `lerd mcp:eject`
- [ ] `lerd mcp:enable-global` / `mcp:disable-global`
- [ ] The MCP config lands for OpenCode as well as the other assistants
- [ ] Over MCP: a worker's tunable values come off its framework definition, an
      action that would download names the image and its size instead of
      fetching, and `project_new` scaffolds the major it was asked for
- [ ] `lerd man` browses docs in the terminal
- [ ] `lerd completion bash|zsh|fish` produces working completion
- [ ] `lerd path:disable` takes lerd's shims off PATH and `path:enable` puts
      them back, with `lerd shims` reporting the same state either way
- [ ] After an upgrade, a shim on PATH (`php`, `composer`, `node`, `npm`,
      `npx`, and the ones behind `mysql` and `psql`) runs the command it was
      given instead of reapplying the whole environment first, and the next
      command typed by hand does the reapply
- [ ] A build from a checkout reports its version with a single `v` in the
      banner and in the dashboard footer
- [ ] `lerd open demo` opens the browser
- [ ] Node: `node:install`, `node:use`, `isolate:node`, `lerd npm run build`
- [ ] `lerd node:manage` installs the shims and a default, `node:manager` shows
      and switches the manager, `node:unmanage` and `node:uninstall` undo it
- [ ] `lerd npx` and `lerd cpx` both run through the project's own versions
- [ ] `lerd pest:browser` runs headed, and the headless Playwright binary is
      shimmed so a plain run works too
- [ ] `lerd code` and `lerd test` reach the project's editor and test runner
- [ ] `lerd js:runtime bun`, `lerd php:bun install`, a bun build runs, **200**
- [ ] Runtime: `lerd runtime frankenphp` → **200**, `--worker` → **200**,
      `lerd octane:reload on`, then `lerd runtime fpm` → **200**

---

## Phase 11 — diagnostics and housekeeping

- [ ] `lerd doctor` clean; `lerd doctor --json` well-formed with fix tiers
- [ ] Break something on purpose (stop `lerd-nginx`) and confirm doctor names
      it and hints `lerd start`. It offers no auto fix for this one on purpose:
      starting nginx means `lerd start`, which reconfigures the resolver under
      sudo, and the auto tier never does that. Run the hint, **https → 200
      afterwards**
- [ ] `lerd doctor --fix --dry-run` previews the fixes it does offer, and
      `--fix --yes` applies them, still confirming the ones classified heavy
- [ ] `lerd doctor` sweeps **every** linked site, not only the host, and names
      the site each finding belongs to
- [ ] Drop a site's database, then let the doctor create it from the finding
      itself, **https → 200** afterwards
- [ ] The doctor reports whether containers can resolve an internet name, and
      says so honestly with the network down
- [ ] Put a host `php` ahead of lerd's shim in the shell rc: doctor names what
      leads, and stays quiet on a machine that chose its own with
      `lerd path:disable`
- [ ] A site pointed at a service's own domain is not reported as unwired, and
      the offered fix does not overwrite the hostname the browser and the app
      have to agree on
- [ ] A doctor fix clicked in the dashboard finds `lerd` under the daemon's own
      PATH and actually lands, rather than coming back as exit 127
- [ ] `lerd site:doctor --json` on both sites
- [ ] `lerd dns:repair` fixes a deliberately broken but enabled `.test` setup
- [ ] `lerd check` validates `.lerd.yaml`, and rejects a deliberately broken one
- [ ] `lerd cleanup --dry-run` then `lerd cleanup --yes` reclaims only what it
      listed, and no in-use image, database or volume is touched
- [ ] The interactive tier reaps a tagged image nothing holds, including the
      base a custom container's `Containerfile` pulled and then moved off, while
      the unattended tiers stay catalog-only
- [ ] Every installed quadlet's image counts as protected: a cleanup run while a
      site is stopped does not cost that site a rebuild
- [ ] What a run reports freed is measured against the image store on both
      sides and matches roughly what the disk gave back, and the preview says
      at least rather than about
- [ ] The resources widget's disk figure is what lerd's images occupy whether or
      not any of it is reclaimable, and clicking it opens the breakdown heaviest
      first
- [ ] `lerd cleanup auto status|off|on`
- [ ] `lerd bug-report -o report.txt` anonymizes names by default and
      `--show-real-names` keeps them
- [ ] `lerd framework update` finishes in a couple of seconds rather than
      fourteen, and `--check` still reports a diff per definition
- [ ] `sites.bkp` beside the registry holds the last ten versions, a save that
      changes nothing takes no slot, and `lerd sites:restore` lists them and
      puts one back with every site serving **200** afterwards
- [ ] `lerd tools:update` brings Composer/fnm/mkcert to the current pins
- [ ] `lerd env:check`, `lerd env:override`, `lerd env:restore` round trip
- [ ] `lerd auth ssh` loads a key and `lerd composer` reaches a private repo
- [ ] `lerd autostart on|status|off`, and with autostart **off** no worker is
      armed for boot
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
- [ ] Once the update lands it reports what changed, without being asked
- [ ] No step is drawn as failed for a built-in service that was simply not
      running
- [ ] `lerd doctor` clean post-upgrade
- [ ] `lerd update --rollback` reverts to N-1, **https → 200**
- [ ] `lerd update` again returns to the RC, **https → 200**
- [ ] `lerd update --beta` on a guest tracking pre-releases picks the RC
- [ ] An install already running a beta is offered the next beta rather than
      going quiet until the stable release overtakes it, and the stable release
      of that cycle outranks the betas it supersedes
- [ ] `lerd update:beta on` and `off` set `update.beta`, the bare command
      reports where the install sits, and the dashboard's Beta updates card on
      the Lerd page agrees with it
- [ ] A build that is itself a beta follows the beta line whatever the flag says
- [ ] One `lerd update` writes each AI skill file once and bounces each daemon
      once, and with autostart off it leaves a stopped daemon stopped while
      still restarting one that was running
- [ ] On a packaged guest (apt/dnf/brew), `lerd update` **defers** to the package
      manager with the right command instead of self-replacing

Repeat the upgrade leg on the packaging lanes:

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
- [ ] Answering **keep my data** actually keeps it: the data directory still
      holds the databases and volumes it did before, and the run says so only
      when it is true
- [ ] A service that ignores SIGTERM is stopped properly rather than reported
      stopped early, and no unit is left `failed` behind the uninstall
- [ ] `lerd uninstall --force` skips prompts on a second guest
- [ ] Reinstall on top of the uninstalled machine and **https → 200** again on a
      re-linked existing project, with its data intact

---

---

## Phase 14 — PHP on the host (macOS only, by hand)

No guest in the matrix can reach this. The native runtime exists to remove a
boundary only macOS has, and on every other platform lerd reports the container
runtime whatever the config file says, deliberately, so a config arriving
through a synced home cannot point Linux vhosts at a listener that does not
exist. Run this on a Mac against a real site, or record the whole phase as
skipped rather than quietly passing it.

- [ ] `lerd php:runtime` reports the current runtime, and the dashboard's
      System, Runtime card agrees with it
- [ ] Switching to native rewrites each site's env for the addresses the new
      runtime can reach, regenerates the vhosts, drops the framework config
      caches and restarts the workers, and tears the old runtime down only once
      nginx is serving from the new one, **https → 200 across the switch**
- [ ] Sites reach their services over loopback and the published host ports,
      including a service the project never declared, and including a framework
      whose configuration is a PHP array with dotted keys
- [ ] With a site pinned to 8.0 or 7.4, the switch refuses up front and names
      every site standing in the way
- [ ] `lerd php:ext` and `lerd php:pkg` refuse under this runtime, and
      `lerd shell` says there is no container to enter rather than starting one
- [ ] Xdebug, `dump()` and the profiler all still work, **200 with each**
- [ ] An untouched site's pool falls to zero workers
- [ ] `lerd status` lists the host builds and says whether each pool is
      accepting connections, hinting at `lerd start` when one is down, instead
      of failing every version with a rebuild hint that refuses here
- [ ] `lerd doctor` walks the host builds: no build missing for a version that
      exists only as an image, no unbuildable for a prerelease, and a site
      pinning a version with no native build is still caught per site
- [ ] Version Info reports the builds that are serving rather than none
- [ ] Removing a version from the dashboard takes the launchd pool down and
      deletes the build, its extensions and the generated pool config, the
      version really leaves `lerd php:list`, and the confirmation names the
      binaries it deletes and the command that fetches them back
- [ ] The update action downloads the published build and restarts the pool,
      and its modal says update rather than rebuild
- [ ] At phone width the detail panel loads the runtime itself, so no image
      actions are offered there
- [ ] A vendor binary that is a shell script, and a globally installed package
      CLI, both run on the host with the project's own `vendor/bin` ahead of
      lerd's shim dir
- [ ] A proxied worker keeps its port across a restart, and a container worker
      is started with its site name
- [ ] A command the store declares that calls `lerd` works from the dashboard,
      which launchd started with the system PATH and nothing else
- [ ] `lerd machine reclaim` caps the guest journal, vacuums what it grew past,
      trims the filesystem and reports the host disk returned, touching no
      container, image, volume or site data
- [ ] Switching back to the container runtime brings every site home,
      **https → 200**

## Sign-off

One row per lane per guest. A lane passes only when its final 200 check passed
after the last destructive step in it.

| Lane | Guest | Version tested | Final HTTP code | Result | Notes |
|---|---|---|---|---|---|
| A — full pass | ubuntu26.04 | | | | |
| B — upgrade/rollback | ubuntu26.04 | | | | |
| C — DNS variants | omarchy | | | | |
| D — dnf | fedora COPR | | | | |
| D — brew | fedora43-2-clone | | | | |
| E — Silverblue | silverblue | | | | |
| E — Bazzite | bazzite | | | | |
| Native runtime | a Mac (phase 14) | | | | |

## What this matrix cannot reach

Three surfaces have no guest here and are signed off by hand on real hardware,
or knowingly skipped and recorded as skipped rather than quietly passed: the Mac
(`lerd machine`, the macOS app, and all of phase 14, which is the release's
headline and reaches no guest at all), `lerd wsl:setup` on Windows, and any tray
or spawned-terminal behaviour on a desktop other than omarchy's, which is the
only guest with a graphical session.

Anything that failed gets an issue before the tag goes out. A release ships only
when lane A is green and every mandatory lane for that release type is green.

Shut the guests down afterwards (`sync` over SSH, then `virsh destroy` on the
ones that ignore ACPI) and restore `clean-no-lerd` so the next release starts
from the same baseline.
