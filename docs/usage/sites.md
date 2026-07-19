# Site Management

## Commands

| Command | Description |
|---|---|
| `lerd init` | Interactive wizard: choose PHP version, HTTPS, and services, then save `.lerd.yaml` and apply |
| `lerd init --fresh` | Re-run the wizard with existing `.lerd.yaml` values as defaults |
| `lerd park [dir]` | Register all Laravel projects inside `dir` (defaults to cwd) |
| `lerd unpark [dir]` | Remove a parked directory and unlink all its sites |
| `lerd link [domain]` | Register the current directory as a site (domain name without TLD, defaults to directory name). On a fresh project in an interactive terminal it runs the `lerd init` wizard first |
| `lerd unlink` | Unlink the current directory site (removes all domains) |
| `lerd domain add <name>` | Add an additional domain to the current site |
| `lerd domain remove <name>` | Remove a domain from the current site |
| `lerd domain list` | List all domains for the current site |
| `lerd sites` | Table view of all registered sites |
| `lerd open [name]` | Open the site in the default browser |
| `lerd share [name]` | Expose the site publicly via ngrok or Expose (auto-detected) |
| `lerd secure [name]` | Issue a mkcert TLS cert and enable HTTPS, updates `APP_URL` in `.env` |
| `lerd unsecure [name]` | Remove TLS and switch back to HTTP, updates `APP_URL` in `.env` |
| `lerd pause [name]` | Pause a site: stop its workers and replace the vhost with a landing page |
| `lerd unpause [name]` | Resume a paused site: restore its vhost and restart previously running workers |
| `lerd env` | Configure `.env` for the current project with lerd service connection settings |
| `lerd workspace add <name>` | Create an empty workspace |
| `lerd workspace rename <old> <new>` | Rename a workspace, keeping its sites |
| `lerd workspace rm <name>` | Delete a workspace; its sites become ungrouped |
| `lerd workspace assign <site> <workspace\|none>` | Move a site into a workspace, or out of one with `none` |
| `lerd workspace move <name> <position>` | Reposition a workspace in the display order (`0` is first) |
| `lerd workspace list` | List the workspaces and their sites |

---

## Project initialisation

`lerd init` runs an interactive wizard, writes the answers to `.lerd.yaml` in the project root, and then applies the configuration: linking the site, enabling HTTPS if requested, picking a database, and starting any required services.

`lerd link` and `lerd init` overlap on purpose. When you run `lerd link` on a project that has no `.lerd.yaml` yet and you're in an interactive terminal, link routes straight into the init wizard, so you don't have to know to reach for `init` first. If the project already has a `.lerd.yaml`, link just applies it. And in a non-interactive shell (a script, CI, `lerd park`, or any piped invocation) link always does a fast, bare auto-detected registration with no wizard, so automation never blocks on a prompt. Passing an explicit domain (`lerd link myapp`) also skips the wizard and links directly.

```bash
cd ~/Projects/my-app
lerd init
```

```
? PHP version: 8.5
? Node version (leave blank to skip):
? Enable HTTPS? No
? Database:
  > SQLite (no service)
    MySQL (lerd-mysql)
    PostgreSQL (lerd-postgres)
? Services:
  ◉ redis
  ◯ meilisearch
  ◯ rustfs
  ◯ mailpit
Saved .lerd.yaml
Linked: my-app -> my-app.test (PHP 8.5, Node 22, Framework: laravel)
```

Wizard defaults are populated intelligently on first run:

- **PHP version**: from the site registry if already linked, otherwise from `.php-version`, `composer.json`, or the global default
- **Enable HTTPS**: pre-checked if the site is already secured
- **Database**: pre-selected from any database already in `.lerd.yaml`, otherwise from `DB_CONNECTION` in `.env` (or `.env.example` for a fresh clone), falling back to SQLite (Laravel's default for new projects)
- **Services**: pre-checked based on what's detected in the project's `.env` file (only non-database services here, since the database is its own step)

The Database step is a single choice rather than a multi-select, so picking MySQL automatically deselects SQLite and vice-versa. After the wizard completes, `lerd env` runs automatically to write your choices to `.env`:

- **MySQL / PostgreSQL**: `DB_CONNECTION` and the related `DB_HOST` / `DB_PORT` / `DB_DATABASE` / `DB_USERNAME` / `DB_PASSWORD` keys are rewritten to point at `lerd-mysql` / `lerd-postgres`, the service is started if it isn't already, and the project database (plus a `_testing` variant) is created.
- **SQLite**: `DB_CONNECTION=sqlite` and `DB_DATABASE=database/database.sqlite` are written to `.env`, and the `database/database.sqlite` file is created if it doesn't exist. No service is started.

The choice is authoritative: if `.env` already had `DB_CONNECTION=mysql` from a previous setup and you switch to SQLite (or vice versa) in the wizard, lerd skips the auto-detection of the old database and applies your new pick instead.

The same prompt also appears when you run `lerd env` directly on a project whose `.env` says SQLite and whose `.lerd.yaml` doesn't yet have a database picked, for example, after cloning a project that wasn't created with `lerd init`. The prompt is skipped automatically when stdin isn't a TTY (e.g. `lerd setup --all` in CI), and for frameworks with explicit env service rules (`fw.env.services` in the YAML, like Symfony, WordPress, etc.) since those don't use Laravel's `DB_CONNECTION` convention.

Persistence is one-way: lerd reads the source of truth from `.lerd.yaml` and writes only to `.env`. `.env.example` is never modified; it's only used as a template when `.env` doesn't exist yet.

The resulting `.lerd.yaml` is intended to be committed to the repository. On a new machine or after a reinstall, running `lerd init` again reads the saved file and restores the full configuration without any prompts.

```bash
# On a fresh machine, no wizard, config applied directly
git clone ...
cd my-app
lerd init
```

Use `--fresh` to re-run the wizard while keeping existing values as defaults:

```bash
lerd init --fresh
```

---

## Non-PHP / custom container sites

For Node.js, Python, Go, or any other non-PHP runtime, lerd builds a dedicated container image per project and has nginx reverse-proxy to it. The workflow differs from PHP sites:

1. Create a `Containerfile.lerd` in the project root that defines the runtime and start command.
2. Run `lerd init`; it detects the non-PHP project (no `composer.json`) and switches to custom container mode, asking for the port, HTTPS, and services. It writes `.lerd.yaml` for you. Alternatively write `.lerd.yaml` manually with a `container: {port: N}` section.
3. Run `lerd link`; it builds the image, starts the container as `lerd-custom-<sitename>`, and generates the nginx vhost.

> **Important:** calling `lerd link` without the container config registers the project as a PHP-FPM site (wrong). If that happened, run `lerd unlink` first, set up the files, then `lerd link` again.

See [Custom Containers](custom-containers.md) for the full configuration reference.

### Static sites

A project that is just a `public_dir` of HTML/CSS/JS with no `composer.json` and no `.php` files is served directly by nginx as a static site. lerd recognises these as non-PHP, so the site detail panel hides every PHP-only surface: the PHP version dropdown, the Xdebug toggle button, the Tinker and Dumps tabs, and the PHP-FPM logs tab. A site counts as PHP only when it has a `composer.json` or a top-level `.php` file, or runs under a custom container or FrankenPHP.

---

## Projects outside the home directory

By default, the PHP-FPM and nginx containers only have access to files under `$HOME`. If your project lives elsewhere (e.g. `/var/www`, `/opt/projects`, `/var/local`), lerd automatically detects this and adds the required volume mount to both containers.

This happens transparently when you:

- **`lerd link`** or **`lerd park`** a directory outside `$HOME`
- Run **`lerd php`**, **`composer`**, **`laravel new`**, or any exec command from an outside path

The containers are restarted once to pick up the new mount. Subsequent commands from the same path run without delay. When you unlink or unpark, stale mounts are cleaned up automatically.

---

## Domain naming

Directories with real TLDs are automatically normalised: dots are replaced with dashes and the TLD is stripped before appending `.test`.

For example: `admin.example.com` becomes `admin-example.test`

---

## Multiple domains

A site can respond to multiple domains. The argument to `lerd link` is the domain name without the `.test` TLD; it is appended automatically from the global config.

```bash
lerd link myapp                # links as myapp.test
```

After linking, you can add more domains:

```bash
lerd domain add api            # adds api.test
lerd domain add admin          # adds admin.test
lerd domain list
#   myapp.test (primary)
#   api.test
#   admin.test
lerd domain remove api         # removes api.test
```

Domains are stored in `.lerd.yaml` as an array (without the TLD) so the file stays portable across machines with different TLD configurations:

```yaml
domains:
  - myapp
  - admin
```

You can also manage domains from the web UI: click the pencil icon next to the domain in the site header to open the domain management modal. Changing the primary domain there also rewrites `APP_URL` in the project's `.env` to match the new primary, unless you have pinned a custom `app_url` (see [Custom `APP_URL`](#custom-app-url) below).

When a site is secured with HTTPS, the certificate is automatically reissued to cover all domains.

Subdomains (e.g. `anything.myapp.test`) are automatically routed to the same site. Git worktree subdomains take priority when they exist.

To route a subdomain to a **different** site instead (for example a separate admin app at `admin.myapp.test`), group the two sites rather than adding an alias. See [Site Groups](site-groups.md).

---

## Domain conflicts

A domain may only be claimed by one site at a time. When `lerd link`, the watcher's auto-registration, or a `.lerd.yaml`-driven re-link tries to register a domain that another site already owns, the conflicting domain is **filtered out** (not the whole site) and a warning is printed:

```
$ lerd link
  [WARN] domain "shared.test" already used by site "owner-app", skipped
Linked: clone-app -> clone-app.test (PHP 8.5, Node 22, Framework: laravel)
```

The site still gets registered with whatever domains survived the filter. If every requested domain is conflicted, lerd falls back to a freshly generated `<dirname>.<tld>` (with a numeric suffix to avoid name collisions).

`.lerd.yaml` is **never modified** when this happens; the original `domains:` list stays on disk so the conflict is visible to the UI and the entry self-heals on the next link if you remove the owning site. The web UI surfaces filtered domains in two places:

- The site detail header's domain pill shows an amber ⚠️ when one or more declared domains are filtered (`+N more` count includes them). Hovering reveals each conflicted entry with the owning site name.
- The Manage Domains modal lists conflicted entries at the top with a warning icon, the domain struck-through, a `used by <site>` pill, and a small trash button. Clicking the trash removes the entry from `.lerd.yaml` only; the registry, vhost, and certs are untouched.

The conflict check is **strict**: a domain is reserved regardless of TLS scheme. Two sites cannot share the same domain even if one runs HTTPS and the other HTTP; DNS and browser caches don't reliably disambiguate by scheme, and the resulting setup is fragile.

---

## Custom `APP_URL`

By default `lerd env` writes `APP_URL=<scheme>://<primary-domain>` to the project's `.env` on every run. If you need to override that (for example to add a path prefix, point at a staging hostname, or pin a specific protocol), set `app_url` in `.lerd.yaml` (committed, shared across machines) or in the per-machine site entry in `~/.local/share/lerd/sites.yaml`. The precedence chain is:

1. `.lerd.yaml` `app_url`: committed to the repo, takes effect on every machine.
2. `sites.yaml` `app_url`: per-machine override, useful when only one developer needs a different URL.
3. The default generator (`<scheme>://<primary-domain>`): used when neither override is set.

```yaml
# .lerd.yaml
domains:
  - myapp
app_url: http://myapp.test/api
```

`lerd env` reads the chain on every invocation, so editing the file and re-running `lerd setup` (or `lerd env` directly) is enough to apply the change. If the `.lerd.yaml` `app_url` happens to point at a domain that got filtered by the conflict check, lerd silently falls through to the next precedence level so you don't end up writing a `DB_HOST` of `lerd-mysql` next to an `APP_URL` that points at someone else's site.

---

## Workers

The `lerd init` wizard includes a workers step that lets you select which workers to auto-start when linking. Available workers depend on the framework and what's installed:

- **queue**: shown when the framework defines a queue worker (replaced by horizon when `laravel/horizon` is installed)
- **horizon**: shown only when `laravel/horizon` is in `composer.json`
- **schedule**: the task scheduler
- **reverb**: shown only when `laravel/reverb` is installed or `BROADCAST_CONNECTION=reverb` is in `.env`
- **custom workers**: any additional workers defined in the framework definition

Selected workers are saved to `.lerd.yaml`:

```yaml
workers:
  - horizon
  - schedule
```

When `lerd link` runs and workers are configured but not yet running, it prompts to run `lerd setup` so you can install dependencies, run migrations, and start workers in the right order. If workers are already running (re-link), they are left as-is.

`lerd setup` pre-selects worker steps based on the `.lerd.yaml` workers list. Workers not in the list still appear in the step selector but are unchecked.

Toggling workers from the CLI (`lerd queue:start`, `lerd schedule:stop`, etc.) or the web UI syncs the running state back to `.lerd.yaml` when the file exists.

`lerd check` validates that listed workers are valid for the detected framework.

`lerd status` includes a Workers section showing all active, restarting, or failed workers across sites. In the web UI, failing workers show a pulsing red toggle and their log tab appears with a "!" indicator.

---

## Request timing

The Overview of a PHP site carries a **Request timing** section that reads the always-on nginx access feed to show how the site is responding as you work, no debug bridge needed. A range picker (15 minutes up to 7 days) drives the whole view: headline figures for the typical and p95 response times, the request count and error rate, a response-time distribution, a throughput chart, the slowest routes, and a table of every route with its p50 and p95. A **Recent requests** tab lists the latest calls with their time, method, path, status, and duration.

Routes are grouped after collapsing id-like path segments, so `/users/123` and `/users/456` aggregate as one `GET /users/:id` entry, and query strings are dropped before anything is recorded. Requests nginx serves without the app are left out: static assets by file extension, anything with a zero request time (a static file nginx answers directly, like `manifest.json` or `robots.txt`), and upgraded connections such as WebSockets, so a page's dozens of asset requests don't drown out its app routes. An upgrade is logged once, when the socket closes, carrying the whole lifetime of the connection as its request time, so a long-lived Reverb or Vite HMR socket would otherwise read as one route taking thousands of seconds. The first request after a site has sat idle past the idle-suspend timeout is treated as a **cold start**: its wake cost is kept out of every timing figure (the site and per-route percentiles and the distribution) while still counting toward the request total, and it's marked in the Recent list, so a wake never makes a route look slow. The last-seen time is seeded from the durable store on startup, so a wake right after a daemon restart is still recognised as cold rather than counted as warm. Requests are written to a small SQLite store in the data directory, so the history survives a restart and any range up to the seven-day retention window can be read back; the watcher also keeps an in-memory window, which is what the doctor and slow-route notifications read. The same table is available to AI assistants over MCP as `diag route_timing`, and `diag optimize_route` pairs each slow route with the N+1 and slow queries captured against it.

The same flagged routes also surface as a `Response Time` warning in the site doctor (`lerd site:doctor`, the dashboard doctor card, and the MCP `diag site_doctor` action), so the nudge reaches you even when you're on another tab. The doctor reads the watcher's snapshot rather than re-measuring, so it stays quiet on a healthy or idle site. If you've enabled notifications, a route crossing the threshold also fires a `slow_route` push. It's edge-triggered: one push when the route goes slow, then it rearms once the route drops back within the typical band, so you're told again if it regresses later (see [Notifications](../features/notifications.md)).

This is a local, single-developer signal meant to catch a route that is dragging, not a production analytics system. Each flagged route carries a **Profile** button that does the whole handoff in one click: it arms the SPX profiler, waits for it to actually be armed, then opens the route in a new tab so that request is captured and switches you to the Profiler where the fresh capture lands on top. Profiling is global and stays off until you ask for it, so the button turns it on for every request until you turn it back off. A non-navigable route (a POST, say) can't be opened for you, so there the button just arms profiling and opens the Profiler for you to reproduce it (see [Profiler](../features/profiler.md)).

A [git worktree](../features/git-worktrees.md) is timed as its own thing. Requests to `feature-x.myapp.test` are recorded against that branch, not against the main checkout, so switching the worktree picker re-scopes the whole panel to the branch you're on and its routes open and profile on the worktree's own subdomain. The worktree's traffic still counts toward the parent when the sites list is ordered by use, since the project is the same project. The doctor's `Response Time` check and the `slow_route` push follow the same rule when they run against a worktree.

When debug capture is on, each route also gains an **Inspect queries** button that jumps to the Debug tab's Queries lens filtered to that route, the one place that renders captured queries. The Debug lenses share a single search within a site's Debug view, so the filter carries over as you switch between Queries, Dumps, and the kind lenses, and the search matches the request path as well as the SQL and file. Captured queries only exist for requests hit while capture was on, so a route you haven't exercised with the debugger shows nothing until you reload it. See [Queries](../features/queries.md) for the capture itself.

---

## Name collision handling

When a directory is parked or linked and another site is already registered with the same name:

- **Same path**: treated as a re-link of the same site. The existing registration is updated and the TLS state is preserved.
- **Different path**: the new site is registered with a numeric suffix (`myapp-2`, `myapp-3`, etc.) so both sites can coexist.

Paths are compared after resolving symlinks, and the resolved path is what gets stored. On atomic images (Fedora Silverblue, Bazzite, and other ostree systems) `/home` is a symlink to `/var/home`, so linking a project through either spelling maps to the one site instead of registering it twice.

---

## Linking from the web UI

You can link a new site directly from the dashboard by clicking the **+** button in the sites panel header. A directory browser modal lets you navigate to the project folder and click **Link This Directory**. After linking, the site's `.env` is auto-configured and the UI switches to the new site's settings.

---

## Unlinked domains

When you visit a `.test` domain that isn't linked to any site over **HTTP**, lerd shows a branded "Site Not Found" page with a link to the dashboard and a retry button. This replaces the browser's generic connection error.

For **HTTPS** the catch-all uses `ssl_reject_handshake on;`, so the browser sees a clean `ERR_SSL_UNRECOGNIZED_NAME_ALERT` connection error rather than a landing page. This is unavoidable: lerd cannot pre-issue a certificate covering arbitrary `*.test` hostnames because browsers (Chrome especially) reject TLD-level wildcard certificates with `ERR_CERT_COMMON_NAME_INVALID`. If you're hitting this on a domain you used to have linked, the fix is browser-side (clear site data / unregister the service worker), not server-side.

---

## Unlink behaviour

When you unlink a site that lives inside a parked directory, the vhost is removed but the registry entry is kept and marked as *ignored*; the watcher will not re-register it on its next scan. Running `lerd link` in that directory clears the ignored flag and restores the site.

Either way, unlinking also drops the site's per-site request-timing and idle state: its rows in the durable request store, its entries in the persisted request-timing and idle-activity snapshots, and the running watcher's in-memory copy, so an unlinked site leaves no stale traffic history behind. A site's git worktrees are covered too.

---

## Pausing sites

Pausing a site frees up resources without removing it from lerd. It is useful when you're switching focus between projects and want to stop workers and silence a site without fully unlinking it.

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

### Running CLI commands on a paused site

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

---

## Workspaces

Once you have more than a handful of sites, one flat list stops being useful. Workspaces let you group sites the way you actually think about them, separating client work from experiments.

A workspace is purely organisational. It never touches nginx, domains, certificates or `.env`, and it never changes how a site is served. It is also not the same thing as a [site group](site-groups.md), which binds a main site's subdomains together and does rewrite vhosts and certificates. A site can belong to a group and a workspace at the same time.

Workspaces are a personal preference rather than project state, so they live in your global config at `~/.config/lerd/config.yaml` and are never written to `.lerd.yaml` or the site registry:

```yaml
workspaces:
  - name: Client Work
    sites: [astrolov, acme]
  - name: Side Projects
    sites: [blog]
```

A site that appears in no workspace is ungrouped. An empty workspace is fine and survives a restart, so you can create one before you have anything to put in it. The order of the list is the order the sections are shown in. Unlinking a site drops it from its workspace, so a different project linked under the same name later starts out ungrouped.

Only a group main is ever written to the list. A [group secondary](site-groups.md) always displays in its main's workspace, so it has no membership of its own and `lerd workspace assign` will point you at the main instead. The name `none` is reserved: it is how you ungroup a site from the command line, and it labels the ungrouped option in the picker.

### In the web UI

The sites sidebar renders one collapsible section per workspace, followed by the ungrouped sites and then the paused ones. Collapse state is remembered per browser.

Drag a site row between sections to move it. Dragging a [site group](site-groups.md) main carries its secondaries with it, since a secondary always shows in its main's workspace. Drag a workspace header to reorder the sections; that moves whole blocks and never changes the order of sites within them. Rename and delete live in the menu on each header, and deleting a workspace only ungroups its sites, it never removes them. The **Add workspace** button sits next to the sort control at the bottom of the list.

Each site's detail header also has a workspace picker, which can create a new workspace and move the site into it in one step.

The Sites Overview groups its tiles by workspace too. Empty workspaces are hidden there, since the sidebar is where you manage them, and each tile still shows its framework as a badge. Until you create your first workspace the overview keeps grouping by framework, the way it always has.

### In the TUI

Press `o` in the sites pane to cycle the sort order until it reads `sort: workspace`. Sites are then listed under a header per workspace, with the ungrouped ones trailing. The TUI shows workspaces but does not edit them; use the web UI or `lerd workspace`.

---

## Git worktrees

Lerd automatically creates a subdomain for each `git worktree` checkout. See [Git Worktrees](../features/git-worktrees.md) for details.

---

## Sharing sites

`lerd share` exposes the current site via a public tunnel. Requires [ngrok](https://ngrok.com/download), [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/), or [Expose](https://expose.dev) to be installed.

| Command | Description |
|---|---|
| `lerd share` | Share the current site (auto-detects ngrok, cloudflared, or Expose) |
| `lerd share <name>` | Share a named site |
| `lerd share --ngrok` | Force ngrok |
| `lerd share --cloudflare` | Force Cloudflare Tunnel (cloudflared) |
| `lerd share --expose` | Force Expose |
| `lerd share --localhost-run` | Force localhost.run (SSH, no signup) |
| `lerd share --serveo` | Force serveo.net (SSH, no signup) |

A local reverse proxy rewrites the `Host` header to the site's domain so nginx routes to the correct vhost. Response `Location` headers and HTML/CSS/JS/JSON body references to the local domain are also rewritten to the public tunnel URL, so redirects and asset links work correctly in the browser.

When the tunnel forwards an `X-Forwarded-Host` header (the public hostname the visitor actually typed), lerd's generated vhosts propagate it into `HTTP_HOST`, `SERVER_NAME`, and the `HTTP_X_FORWARDED_*` family, so PHP apps that build absolute URLs from `$_SERVER` or Laravel's `url()` helper return the public URL instead of the local `.test` one. See [Nginx Overrides](./nginx-overrides.md#forwarded-headers-and-tunneling) for the full mapping, and for how to drop per-site snippets under `~/.local/share/lerd/nginx/custom.d/` without losing them on the next `lerd update`.
