# Frameworks

Lerd uses **framework definitions** to describe how a PHP project type behaves: where the document root is, how to detect it automatically, which env file to use, and which background workers it supports.

Laravel has a built-in definition. Other frameworks (Symfony, WordPress, Drupal, CakePHP, Statamic, Magento, etc.) can be installed from the [community store](https://github.com/lerd-env/frameworks) or defined manually.

---

## Commands

| Command | Description |
|---|---|
| `lerd new <name-or-path>` | Scaffold a new PHP project using a framework's create command |
| `lerd framework list` | List all framework definitions with source and workers |
| `lerd framework list --check` | Compare local definitions against the store |
| `lerd framework search [query]` | Search the community store for available definitions |
| `lerd framework update [name[@version]]` | Refresh definitions from the store (definitions otherwise auto-fetch on link) |
| `lerd framework update --diff` | Preview changes before applying updates |
| `lerd framework add <name>` | Add or update a user-defined framework definition |
| `lerd framework remove <name>[@version]` | Remove a framework definition (prompts if multiple versions) |
| `lerd framework remove <name> --all` | Remove all versions of a framework definition |
| `lerd framework prune` | Remove installed definitions no site uses |

---

## Framework store

Lerd has a community-driven framework store backed by [lerd-env/frameworks](https://github.com/lerd-env/frameworks). The store hosts definitions for popular PHP frameworks, versioned by major release.

### Available frameworks

```bash
lerd framework search
```

```
╭───────────┬───────────┬────────┬────────────────╮
│ Name      │ Label     │ Latest │ Versions       │
├───────────┼───────────┼────────┼────────────────┤
│ laravel   │ Laravel   │ 13     │ 13, 12, 11, 10 │
│ symfony   │ Symfony   │ 8      │ 8, 7           │
│ wordpress │ WordPress │ 6      │ 6, 5           │
│ drupal    │ Drupal    │ 11     │ 11, 10         │
│ cakephp   │ CakePHP   │ 5      │ 5, 4           │
│ statamic  │ Statamic  │ 6      │ 6, 5           │
╰───────────┴───────────┴────────┴────────────────╯
```

### Getting definitions from the store

Definitions arrive automatically. Linking a project detects its framework and version and fetches the matching definition from the store, and the cached catalogue refreshes on its own in the background, so there is no install step to run. Because the catalogue is cached locally, detection resolves the right framework and version even offline and for frameworks you have not linked before.

To refresh manually, `lerd framework update`. With no arguments it refreshes the cached catalogue and re-fetches every installed definition; with a name it fetches that one, installing it if it isn't cached yet:

```bash
lerd framework update                   # refresh catalogue + all installed definitions
lerd framework update symfony           # fetch/update symfony (auto-detects version from composer.lock)
lerd framework update laravel@12        # explicit version
lerd framework update --diff            # preview changes before applying
```

When no version is specified, lerd reads `composer.lock` to detect the installed major version. If the version can't be determined, it falls back to the latest available.

Store definitions are saved to `~/.local/share/lerd/frameworks/<name>@<version>.yaml`, separate from user-defined frameworks.

Point `LERD_STORE_BASE_URL` at an alternate base (comma-separated for several) to fetch framework definitions from a private or local mirror instead of `lerd-env/frameworks`, mirroring `LERD_SERVICES_BASE_URL` for the [service store](service-presets.md).

### Checking for updates

```bash
lerd framework list --check
```

```
Name            Version  Source     Latest     Status
───────────────────────────────────────────────────────
laravel         -        built-in   13         built-in
symfony         8        store      8          up to date
wordpress       6        store      6          up to date
magento         -        user       -          not in store
```

### Updating

```bash
lerd framework update symfony         # update a single framework
lerd framework update symfony@7       # update to a specific version
lerd framework update                 # update all installed frameworks
lerd framework update --diff          # show changes before applying
```

When run without arguments, every cached version of every framework is refreshed individually. A user with `laravel@10/11/12/13` cached gets all four files re-fetched, not just the latest.

### Auto-detection and auto-fetch

When any command needs a framework definition that isn't installed locally, lerd fetches it from the store automatically. The version is resolved from `composer.lock`, so a Laravel 11 project gets `laravel@11.yaml` and a Laravel 12 project gets `laravel@12.yaml`.

Locally installed definitions are refreshed from the store every 24 hours to pick up upstream fixes (e.g. new log sources, corrected PHP ranges).

During `lerd link`, `lerd init`, or `lerd setup`, if no framework is detected at all:

- **Interactive mode**: prompts to install from the store
- **Non-interactive mode**: fetches silently when `.lerd.yaml` specifies a framework name

### Contributing to the store

Submit a pull request to [lerd-env/frameworks](https://github.com/lerd-env/frameworks) with a YAML file under `frameworks/<name>/<version>.yaml` and update `frameworks/index.json`.

---

## Creating new projects

### Laravel installer

Lerd ships with the [Laravel installer](https://laravel.com/docs/installation#creating-a-laravel-application); it's already available in your CLI after `lerd install`:

```bash
laravel new myapp
cd myapp
lerd link
lerd setup
```

The installer walks you through starter kit selection, database setup, and other options interactively.

### lerd new

`lerd new` is a framework-agnostic shortcut that runs the framework's scaffold command:


```bash
lerd new myapp                          # create using Laravel (default)
lerd new myapp --framework=symfony      # create using Symfony's create command
lerd new /path/to/myapp                 # create at an absolute path
lerd new myapp -- --no-interaction      # pass extra flags to the scaffold command
```

`--framework` works before or after the name. Flags belong to lerd wherever they
appear on the line, so anything meant for the scaffold command itself goes after
`--`. An absolute target outside your home directory is fine: lerd creates the
parent directory and mounts it into the PHP container before scaffolding.
Temporary system directories (`/tmp`, `/var/tmp`, `/run`) are never mounted, so
scaffolding into one is refused unless you [park](/usage/sites) its parent first.

After creation:
```bash
cd myapp
lerd link
lerd setup
```

---

## Laravel definition

Laravel has a built-in definition compiled into the binary as a fallback. When a project is linked, lerd auto-fetches the version-specific definition from the store (e.g. `laravel@11`, `laravel@12`), which includes the correct PHP version range and version-specific behaviour (e.g. Laravel 10 uses `schedule:run` instead of `schedule:work`, and doesn't include Reverb).

Default workers:

| Worker | Label | Command | Check | Extra |
|---|---|---|---|---|
| `queue` | Queue Worker | `php artisan queue:work --queue=default --tries=3 --timeout=60` | - | - |
| `schedule` | Task Scheduler | `php artisan schedule:work` | - | - |
| `reverb` | Reverb WebSocket | `php artisan reverb:start` | `laravel/reverb` | proxy at `/app`, auto-assigned port |
| `horizon` | Horizon | `php artisan horizon` | `laravel/horizon` | conflicts with `queue`; auto-reload via `horizon:listen` (see [queue workers](queue-workers.md)) |

### Adding workers to Laravel

User-defined workers are merged on top of the built-in. Use `lerd framework add` to create an overlay:

```yaml
# horizon.yaml
name: laravel
workers:
  pulse:
    label: Pulse
    command: php artisan pulse:work
    restart: always
```

```bash
lerd framework add laravel --from-file horizon.yaml
```

To remove the overlay (built-in workers remain):
```bash
lerd framework remove laravel
```

### Removing framework definitions

```bash
lerd framework remove symfony          # prompts if multiple versions installed
lerd framework remove symfony@7        # remove a specific version
lerd framework remove symfony --all    # remove all versions
```

When multiple versions of a framework are installed, `lerd framework remove` prompts you to choose which version to remove.

If a linked site still uses the framework, `lerd framework remove` lists those sites and asks you to confirm before deleting it. Pass `--force` to skip that confirmation.

### Pruning unused definitions

Installed definitions accumulate over time as you try different frameworks. To clear out the ones no site references:

```bash
lerd framework prune          # lists unused definitions, then asks to confirm
lerd framework prune --force  # removes them without confirming
```

Pruning only touches store-installed and user-defined definitions, never the built-in ones. It is safe to run: lerd re-fetches a definition from the store automatically the moment a site needs one that is no longer present locally, so a pruned framework comes back on its own if you need it again.

When you `lerd unlink` the last site using a framework, lerd offers to remove that framework's definition right then, so you do not have to remember to prune it later. The offer only appears for removable definitions, never the built-in ones.

---

## PHP version clamping

When a framework definition includes `php.min` and `php.max`, `lerd link` and `lerd init` automatically clamp the detected PHP version to the supported range. For example, if you link a Laravel 10 project (max PHP 8.3) but your system defaults to PHP 8.5, lerd will select PHP 8.3 instead:

```
PHP 8.5 is outside Laravel's supported range (8.1-8.3), using PHP 8.3.
```

This prevents accidentally running a project on an unsupported PHP version.

---

## More

- [Framework workers](framework-workers.md): conditional rules, conflicts, proxy wiring, project custom workers, orphaned workers.
- [Framework definitions](framework-definitions.md): YAML schema, env setup, detection rules, doc-root fallback, log viewer.
