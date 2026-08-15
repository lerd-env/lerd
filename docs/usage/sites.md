# Site Management

This page covers getting a project registered and served: the init wizard, linking and parking, and how unattended registration differs from a link you type. The topics that used to share this page now have their own: [Domains](domains.md), [Workspaces](workspaces.md), [Pausing Sites](pausing.md), [Sharing Sites](sharing.md), and [Request Timing](../features/request-timing.md).

## Commands

| Command | Description |
|---|---|
| `lerd init` | Interactive wizard: choose PHP version, HTTPS, and services, then save `.lerd.yaml` and apply |
| `lerd init --fresh` | Re-run the wizard with existing `.lerd.yaml` values as defaults |
| `lerd park [dir]` | Register every PHP project inside `dir` as a site, and keep doing so as new ones appear (defaults to cwd) |
| `lerd unpark [dir]` | Remove a parked directory and unlink all its sites |
| `lerd link [domain]` | Register the current directory as a site (domain name without TLD, defaults to directory name). On a fresh project in an interactive terminal it runs the `lerd init` wizard first |
| `lerd unlink` | Unlink the current directory site (removes all domains) |
| `lerd sites` | Table view of all registered sites |
| `lerd open [name]` | Open the site in the default browser |
| `lerd code [name]` | Open the site's directory in the configured editor |
| `lerd secure [name]` | Issue a mkcert TLS cert and enable HTTPS, updates `APP_URL` in `.env` |
| `lerd unsecure [name]` | Remove TLS and switch back to HTTP, updates `APP_URL` in `.env` |
| `lerd env` | Configure `.env` for the current project with lerd service connection settings |

`lerd code` opens the project the way `lerd open` opens the browser: no argument uses the site rooted at the current directory, a name comes from the registry, and inside a git worktree it opens that checkout rather than the parent it inherits its registration from. It runs the `editor` command from `~/.config/lerd/config.yaml` when you have set one, with `{file}` standing for the directory (a `{line}` in the template is dropped, a directory has no line to jump to). With nothing configured it uses the first of VS Code, Cursor, VSCodium, Windsurf, Sublime, Zed, PhpStorm or IntelliJ IDEA it finds on PATH, and if none of them are there it says so instead of handing the directory to your file manager.

The `lerd domain`, `lerd share`, `lerd pause` and `lerd workspace` commands are documented on the [Domains](domains.md), [Sharing Sites](sharing.md), [Pausing Sites](pausing.md) and [Workspaces](workspaces.md) pages.

---

## Project initialisation

`lerd init` runs an interactive wizard, writes the answers to `.lerd.yaml` in the project root, and then applies the configuration: linking the site, enabling HTTPS if requested, picking a database, and starting any required services.

`lerd link` and `lerd init` overlap on purpose. When you run `lerd link` on a project that has no `.lerd.yaml` yet and you're in an interactive terminal, link routes straight into the init wizard, so you don't have to know to reach for `init` first. If the project already has a `.lerd.yaml`, link just applies it. In a non-interactive shell (a script, CI, or any piped invocation) link does a fast auto-detected registration with no wizard, so automation never blocks on a prompt. Passing an explicit domain (`lerd link myapp`) also skips the wizard and links directly.

Every way of linking a project resolves the same plan: the CLI, the dashboard's **+** button, `lerd park`, the parked-directory watcher, and the MCP `site link` action. They differ only in what they are allowed to do, and the difference is deliberate. A link you type can prompt, write `.php-version` and `.node-version`, install services, issue a certificate, and supervise a dev command the project declares. An unattended link (park and the watcher) reads the same committed configuration but never prompts, never writes into the project, and never runs anything the repository authored.

```bash
cd ~/Projects/my-app
lerd init
```

```
? PHP version: 8.5
? Node version (clear to follow the lerd default instead of pinning): 22
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
- **Node version**: from `.lerd.yaml` if already saved, otherwise from `.nvmrc`, `.node-version`, `package.json` engines, or the global default; clear the field to save no version and keep following those
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

## Parking a directory of projects

`lerd park ~/Code` registers every PHP project directly inside a directory, and
records the directory so the watcher keeps up with it: a project you clone into
it later is registered on its own, and one you delete is unlinked.

```bash
lerd park ~/Code
```

```
 parking /home/me/Code
 → linking 128 projects… ████████████░░░░░░░░ 74/128 · shop ⠙
 ✓ linking 128 projects 126 linked, 2 skipped
 → publishing… ✓ 126 site(s) serving
```

Each project writes only its own vhost and PHP unit; the reloads that publish
them run once for the whole batch. That matters at scale, because those steps
rewrite every quadlet and every container hosts entry, so doing them per project
made a large directory take minutes rather than seconds.

A parked link reads the project's committed `.lerd.yaml`, its domains, public
directory and PHP version, but it runs unattended, so it stops short of
anything that needs a decision or runs code the repository wrote. It never
prompts, never writes `.php-version` or `.node-version` into your project, never
installs services, and never issues a certificate.

Some projects are reported as skipped rather than registered:

- A directory that is not a PHP project at all.
- A git worktree of a project that is already a site; worktrees inherit the
  parent's registration and are served at `branch.domain.test`.
- A project that declares its own runtime, a custom container, a host-proxy dev
  server, FrankenPHP, or a custom FPM image. Each needs an image built or a
  command run, which an unattended sweep should not do on its own. Run `lerd
  link` in the project to set it up, after which the watcher leaves it alone.

Use `lerd unpark <dir>` to stop watching a directory and unlink its sites.

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

`lerd site:doctor` validates that listed workers are valid for the detected framework.

`lerd status` includes a Workers section showing all active, restarting, or failed workers across sites. In the web UI, failing workers show a pulsing red toggle and their log tab appears with a "!" indicator.

---

## Name collision handling

When a directory is parked or linked and another site is already registered with the same name:

- **Same path**: treated as a re-link of the same site. The existing registration is updated and the TLS state is preserved.
- **Different path**: the new site is registered with a numeric suffix (`myapp-2`, `myapp-3`, etc.) so both sites can coexist.

Paths are compared after resolving symlinks, and the resolved path is what gets stored. On atomic images (Fedora Silverblue, Bazzite, and other ostree systems) `/home` is a symlink to `/var/home`, so linking a project through either spelling maps to the one site instead of registering it twice.

---

## Linking from the web UI

You can link a new site directly from the dashboard by clicking the **+** button in the sites panel header. A directory browser modal lets you navigate to the project folder and click **Link This Directory**. After linking, the site's `.env` is auto-configured and the UI switches to the new site's settings.

Clicking **Link This Directory** is the consent a terminal link would ask for, so
a project that declares a host-proxy dev command has that command started for
you. The command is printed in the modal's output, so what lerd runs on your host
is on screen either way.

The environment step can fail on its own, a project with no framework, or a
framework that declares no env file, has nothing to configure. The site is still
linked, and the modal says what went wrong instead of closing on a clean
success.

---

## Unlink behaviour

When you unlink a site that lives inside a parked directory, the vhost is removed but the registry entry is kept and marked as *ignored*; the watcher will not re-register it on its next scan. Running `lerd link` in that directory clears the ignored flag and restores the site.

Either way, unlinking also drops the site's per-site [request-timing](../features/request-timing.md) and idle state: its rows in the durable request store, its entries in the persisted request-timing and idle-activity snapshots, and the running watcher's in-memory copy, so an unlinked site leaves no stale traffic history behind. A site's git worktrees are covered too.

---

## Git worktrees

Lerd automatically creates a subdomain for each `git worktree` checkout. See [Git Worktrees](../features/git-worktrees.md) for details.
