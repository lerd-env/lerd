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
| `lerd framework add <name>` | Install a published framework from the store, or author a custom one with flags |
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
╭─────────────┬─────────────┬────────┬────────────────╮
│ Name        │ Label       │ Latest │ Versions       │
├─────────────┼─────────────┼────────┼────────────────┤
│ laravel     │ Laravel     │ 13     │ 13, 12, 11, 10 │
│ symfony     │ Symfony     │ 8      │ 8, 7           │
│ wordpress   │ WordPress   │ 7      │ 7, 6, 5        │
│ drupal      │ Drupal      │ 11     │ 11, 10         │
│ cakephp     │ CakePHP     │ 5      │ 5, 4           │
│ codeigniter │ CodeIgniter │ 4      │ 4              │
│ statamic    │ Statamic    │ 6      │ 6, 5           │
│ magento     │ Magento     │ 2      │ 2              │
│ tempest     │ Tempest     │ 3      │ 3              │
╰─────────────┴─────────────┴────────┴────────────────╯
```

### Getting definitions from the store

Definitions arrive automatically. Linking a project detects its framework and version and fetches the matching definition from the store, and the cached catalogue refreshes on its own in the background, so there is no install step to run. Because the catalogue is cached locally, detection resolves the right framework and version even offline and for frameworks you have not linked before.

To install a published definition on demand, `lerd framework add <name>` fetches it from the store, the natural next step after `lerd framework search`. It resolves the version from the project in the current directory when there is one and otherwise takes the latest, and `lerd framework add symfony@7` pins a version. A name the store does not publish falls back to authoring a definition by hand (see below).

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
lerd new                                # ask for the name, the framework and the version
lerd new myapp                          # ask which framework to use
lerd new myapp --framework=symfony      # scaffold Symfony, no questions
lerd new myapp --framework=laravel --framework-version=11   # scaffold an older major
lerd new /path/to/myapp                 # create at an absolute path
lerd new myapp -- --no-interaction      # pass extra flags to the scaffold command
```

On a terminal it asks which framework to scaffold rather than assuming one,
offering the catalogue [`lerd framework list`](#available-frameworks) shows, which
your install seeds from the store. When the framework you pick has more than one
major published it asks which, defaulting to the current release and pulling that
major's definition before scaffolding. Called with no name at all it asks for
that too. Naming a framework with `--framework` skips the question, and a run
with no terminal skips all of them and scaffolds the default, so scripts and CI
never start blocking on a prompt. `--framework-version` answers the version
question the same way, which is how the dashboard's site wizard scaffolds a
chosen major; it needs a `--framework` to apply to.

The major you pick reaches composer as well as lerd. Each definition's create
command names its own major, `composer create-project laravel/laravel:^11.0`, so
the release that lands on disk is the one you asked for rather than whatever is
newest, and the PHP range, workers and env wiring the definition brings match the
code beside them.

Not every major has a skeleton to start from. A framework can outlive the project
template it shipped with, and the definition for such a major carries no create
command at all. The version question leaves those majors out, and a run that
pinned none scaffolds from the newest one that still has a skeleton, so the
project on disk is a release that really exists. Asking for that major by name
with `--framework-version` says so rather than quietly building an older one.

The command then carries the project the rest of the way: it links the new
directory, which routes a project with no `.lerd.yaml` through the
[init wizard](/usage/sites) for PHP version, HTTPS and services, and offers setup
at the end. What it cannot do is move your own shell, so it closes by telling you
to `cd` into the project. A run with no terminal stops after scaffolding and
prints the `lerd link && lerd setup` hint as before.

`--framework` works before or after the name. The definition comes from the
store, so a new project starts from the currently published create command rather
than whichever snapshot of it your lerd binary was built with, and a framework
the store publishes but you have not installed yet is fetched on demand, so you
can scaffold a project type you have never built before without an install step.
A definition already on disk from the last day is taken as current and used
without a round trip, and when the store cannot be reached lerd falls back to
what you have installed, then to its built-in definition; only a name nothing
knows is refused. Flags belong to lerd wherever they
appear on the line, so anything meant for the scaffold command itself goes after
`--`. An absolute target outside your home directory is fine: lerd creates the
parent directory and mounts it into the PHP container before scaffolding.
Temporary system directories (`/tmp`, `/var/tmp`, `/run`) are never mounted, so
scaffolding into one is refused unless you [park](/usage/sites) its parent first.

Every framework's create command starts with composer, and it is lerd's own
composer that runs it, inside the project's PHP container. You do not need
composer, or any PHP, installed on the host.

The scaffold runs under a PHP version the framework supports, not whatever the
machine defaults to. The framework definition declares a PHP range, so a
framework whose current release tops out below your default PHP is scaffolded on
a version inside that range instead, and composer resolves against a PHP the
framework actually supports rather than rejecting every candidate.

After creation, move into the project:
```bash
cd myapp
```

A run with no terminal did not link or set the project up, so finish it by hand:
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
| `native` | NativePHP | `php artisan native:serve` | `nativephp/electron` | runs on the host, see [NativePHP](#nativephp) |
| `jump` | NativePHP Jump | `php artisan native:jump` | `nativephp/mobile` | runs on the host, see [NativePHP](#nativephp) |

### NativePHP

[NativePHP](https://nativephp.com) turns a Laravel app into a native application, and it is two products with two toolchains. `nativephp/electron` builds a desktop app; `nativephp/mobile` builds iOS and Android apps. Both register the same `native:*` artisan commands, so a project picks one. lerd links and serves either like any other Laravel site, at its own `.test` domain, and adds workers, commands and health checks gated on whichever package is installed, so a plain Laravel project sees none of it.

The full end-to-end is in the [NativePHP walkthrough](../getting-started/nativephp.md); what follows is what the definition actually declares.

**Where the artisan process runs.** Everything native shells out to tools that exist only on the host: Electron needs a display, `native:run` needs Gradle and adb or xcodebuild and simctl, and the PHP-FPM container has none of them. lerd's commands and `host: true` workers already run on the host, but `php` there is lerd's shim, which routes straight back into the container. So the artisan process itself runs through a real host PHP, the per-platform binary NativePHP ships in `nativephp/php-bin`. The desktop package already depends on it; the mobile `native:install` adds it as a dev dependency and unpacks the build matching the site's PHP version, which is the version `composer.lock` was resolved against.

**Desktop** gets a `native` worker running `native:serve`, and `native:install` and `native:build` commands. Starting and stopping the worker opens and closes the Electron window, the same as any other worker.

**Mobile** gets a `jump` worker running `native:jump`, the development server that puts a build on an address a phone can reach, and `native:install`, `native:run` and `native:open` as commands. Those three finish rather than staying up, and `native:run` and `native:open` produce terminal output because they ask which platform and which device.

**Doctor checks.** Each product gets one for the runtime lerd can install, carrying `native:install` as its fix, since a fresh clone always arrives without it. Mobile adds two more for the toolchain lerd cannot install: one when neither Xcode nor the Android SDK is present, and one when the Android SDK is there but the JDK Gradle would pick is older than 17. Both report and offer no button, because installing either is a multi-gigabyte vendor download.

**The site keeps its vhost.** On desktop the app runs its own PHP server on its own port and the `.test` domain is a convenience. On mobile it is more: `native:run` takes a `--start-url` and `native:jump` exists precisely to serve the app to a device over the network, which is the problem [LAN sharing](lan-sharing.md) already solves.

`native:install` and `native:build` appear twice in the definition, once per product, with mutually exclusive `composer` gates, so `lerd run native:install` means the right thing whichever package you have and only one is ever resolved.

Plugins, code signing and store uploads are not covered.

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
