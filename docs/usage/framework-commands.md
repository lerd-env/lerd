# Framework commands

Every PHP site in the lerd dashboard exposes a **Commands ▾** dropdown in the site Overview's Runtime & workers row, alongside the version pickers and worker toggles. It surfaces a curated set of one-shot admin actions for the detected framework, plus anything the project adds in its `.lerd.yaml`. The same set is reachable from the command palette (`⌘K` / `/`) and from the terminal via `lerd run <name>`.

The feature exists for the actions you'd otherwise ssh in for: clearing caches, applying migrations, generating an admin login link, exporting the database before a risky change.

## Canonical commands per framework

| Framework | Commands |
|---|---|
| Laravel | `optimize:clear`, `migrate`, `migrate:fresh` |
| WordPress | `cache:flush`, `rewrite:flush`, `db:export` |
| Drupal | `cr`, `uli`, `updb`, `cex`, `cim` |
| Symfony | `cache:clear`, `doctrine:migrations:migrate`, `doctrine:fixtures:load` |
| CakePHP | `cache:clear`, `migrate`, `schema:cache:clear` |
| Statamic | `cache:clear`, `stache:warm`, `search:update` |
| Magento | `setup:install`, `cache:flush`, `cache:clean`, `setup:upgrade`, `setup:di:compile`, `indexer:reindex`, `deploy:mode:developer`, `maintenance:enable`, `maintenance:disable` |

Destructive commands (`migrate:fresh`, `cim`, `doctrine:fixtures:load`) are gated by a confirmation modal before running.

Some commands include a `check:` rule and only surface when the relevant package or file is present (`doctrine:migrations:migrate` requires `doctrine/doctrine-migrations-bundle`, CakePHP `migrate` requires `cakephp/migrations`). A rule can also read the other way round, with `missing_file:`, for a command that belongs only on a project nobody has bootstrapped yet. Magento uses both: every command that needs a deployment config is hidden until the store is installed, so a fresh checkout offers only `setup:install`, and `setup:install` itself is hidden once it exists.

## How a run works

Clicking a command (or pressing Enter on a palette entry, or running `lerd run <name>`) executes the shell command in the project's directory, with stdio routed depending on the command's `output:` value:

- **`text`** (default), streams stdout and stderr into the modal as a scrollable monospace block, and leaves it open on the exit code. Use for commands whose output you'd want to read (test runs, route lists, config diffs).
- **`silent`**: runs without opening the modal and toasts when it's done, so a cache clear doesn't cost you a click. A failure still opens the modal with the captured output, since that's the only thing that explains it.
- **`url`**: captures stdout, scans it for the first `http(s)://...` URL, and surfaces it with Copy and Open buttons. The killer feature for `drush uli` and similar one-time-login generators.
- **`terminal`**: spawns the user's terminal emulator (kitty, foot, alacritty, wezterm, ghostty, ptyxis, konsole, gnome-terminal, xterm; on macOS iTerm or Terminal.app) with the command running inside. Use for interactive commands like `php artisan tinker`, `bin/cake bake`, `wp shell` that need a real TTY. The lerd-ui modal stays closed.

The dashboard modal streams output as it arrives via Server-Sent Events from `POST /api/sites/:domain/commands/:name/run`; the CLI streams straight to your terminal (`lerd run` is stdio-passthrough).

### Where a command runs

The shell itself runs on your host, in the project directory, but the tools it calls don't. `php`, `composer`, `node` and `npm` on your PATH are lerd shims that exec into the site's PHP container, and each installed service ships client shims (`mysql`, `mysqldump`, `psql`, `pg_dump`, `redis-cli`) that exec into that service's container. So a single command can step across containers the way a Lando `tooling` entry does, with the target implied by the tool rather than named per step:

```yaml
commands:
  - name: refresh-data
    label: Rebuild demo data
    command: ./bin/refresh-data     # php artisan …, then mysqldump …, then php artisan …
    output: text
```

What this does not give you is a step inside an arbitrary container: a command that has to run in a custom service's container has no route, since the only containers reachable this way are the site's PHP container and the services that publish client shims.

## Pinned commands

The command a project runs twenty times a day shouldn't cost the same two clicks as the one it runs twice a year, so a command can be pinned: it then draws its own button on the site's control row, next to the PHP and Node pickers and the doctor button, and clicking it runs exactly what the dropdown entry would, confirm gate and terminal spawn included.

Pin and unpin from the pin icon on the right of each entry in the Commands dropdown. Two pinned commands per site is the limit, because the row folds its secondary actions into a menu on a narrow panel and an unbounded set would land on that fold first. Once two are pinned the other pin icons go inert until you free a slot.

A definition can ship a command pinned by default with `pinned: true`, for the one a project's developers all reach for. Laravel does it for NativePHP's `native:run`, which is the loop of mobile development and only surfaces at all once the mobile runtime is installed. Your own pin choice is stored per site in lerd's registry, not in `.lerd.yaml`, so it stays personal: unpinning a default doesn't turn into a diff every teammate carries, and pinning one of your own doesn't either.

Note this is a different pin from `lerd idle pin`, which excludes a site from idle-suspend. One keeps a site's workers awake, the other keeps a command in reach.

## Project commands

Any `.lerd.yaml` can add or override commands via a `commands:` block:

```yaml
# .lerd.yaml
commands:
  - name: deploy
    label: Deploy to staging
    command: ./bin/deploy staging
    description: Push the current branch to the staging environment
    output: text
    icon: arrow-up

  - name: search:replace
    label: Replace URL across the database
    command: wp search-replace https://old.example https://new.example --all-tables
    output: text
    confirm: true
    icon: edit

  - name: migrate:fresh
    disabled: true                 # suppress the Laravel default
```

The merge rules are:

- A project entry with the same `name` as a framework entry **fully replaces** it. Use this to point `test` at Pest, swap the migration command for a custom wrapper, etc.
- `disabled: true` on a name that matches a framework entry **suppresses** the framework default without contributing a replacement.
- A project entry with a new `name` is **appended** after the framework set.
- Framework entries whose `check:` rule fails are dropped before the merge.

Validation runs as part of `lerd site:doctor`. Invalid `output:` values, unknown icons, duplicate names, and missing commands all surface there.

Because a `.lerd.yaml` `commands:` entry comes from the project (an untrusted cloned repo), lerd asks before running one on your host: the first run via `lerd run` or the dashboard shows the exact command and prompts, and the approval is remembered per site so later runs don't re-prompt. `lerd run --yes` bypasses the prompt, and `host_commands.skip_confirmation: true` (or `host_commands.disabled: true` to refuse them) in the global config changes the default. Framework-provided commands (store, built-in, user overlay) run without this prompt.

## Schema

```yaml
commands:
  - name: optimize:clear         # stable id; also the `lerd run` argument and override key
    label: Clear all caches       # UI label
    command: php artisan optimize:clear   # shell, passed to `sh -c`
    description: Clear config, route, view, event, and compiled caches
    output: silent                # silent | text | url | terminal (default: text)
    confirm: false                # ask before running
    pinned: false                 # draw it as a button on the site's control row (max 2 per site)
    icon: broom                   # from the known icon set
    cwd: .                        # optional, relative to project root
    check:                        # optional; hide when this rule fails
      composer: doctrine/doctrine-migrations-bundle
    # `disabled: true` is only meaningful in .lerd.yaml; ignored in framework yamls
```

**Known icons**: `broom`, `database`, `refresh`, `link`, `check`, `list`, `key`, `edit`, `arrow-down`, `arrow-up`, `play`, `terminal`. An unknown icon falls back to a generic glyph; `lerd site:doctor` warns.

**Output values:** invalid values fail `lerd site:doctor`. Defaults to `text`.

**Check rules**: reuse `FrameworkRule`. The two common forms are `composer: <package>` (the package must be in `composer.json`) and `file: <path>` (the file must exist relative to the project root).

## Agents (MCP)

When the lerd MCP server is registered, an AI assistant can:

- `commands_list(site)`: see what's available for a site
- `commands_run(site, name, force?)`: execute one (with `force: true` to bypass `confirm`)
- `command_add(site, name, command, ...)`: write a new entry into `.lerd.yaml`'s `commands:` block. Same `name` as a framework default replaces it. Use `disabled: true` to suppress a framework default
- `command_remove(site, name)`: delete a project entry

Agents should prefer `commands_run` over invoking `php artisan` / `drush` / `wp` directly so per-project overrides are honored, and `command_add` over hand-editing yaml so the entry passes the same validation `lerd site:doctor` runs.

## CLI

```
$ lerd run                            # list available commands for the current project
    optimize:clear   Clear all caches
    migrate          Run migrations
  * migrate:fresh    Drop and re-migrate

  * = asks for confirmation. Use --yes to skip.

$ lerd run optimize:clear             # execute, stream stdout to your terminal
$ lerd run migrate:fresh --yes        # bypass the confirm prompt
```

`lerd run` walks up from the current directory to find the nearest `.lerd.yaml`, so it works from any subdirectory of a site (including inside a git worktree). The exit code propagates from the underlying shell.

Shell completion populates command names: `lerd run <TAB>` lists what's available in the current project.

## Concurrency & security

Two commands cannot run on the same site at the same time; the API returns `409 Conflict` if a second run is attempted while one is in flight. This protects against accidentally running `migrate:fresh` twice from two tabs.

The run endpoint is available to the local dashboard and to authenticated remote dashboard sessions. The same per-site concurrency guard applies to both.