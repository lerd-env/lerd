# Framework definitions

Framework definitions are YAML files that tell Lerd how to detect a PHP framework, where its document root is, which env file it uses, and which workers and log paths it has. This page is the full schema reference.

## Definition sources and priority

Lerd resolves framework definitions from multiple sources. Higher priority wins:

| Priority | Source | Location | Purpose |
|----------|--------|----------|---------|
| 1 | User overlay | `~/.config/lerd/frameworks/<name>.yaml` | Manual overrides (merged on top) |
| 2 | Project embedded | `.lerd.yaml` `framework_def` | Portability for user-defined frameworks |
| 3 | Store package layer | `~/.local/share/lerd/packages/<vendor>-<name>.yaml` | A composer package's own workers, commands and checks (merged on top, see below) |
| 4 | Store-installed | `~/.local/share/lerd/frameworks/<name>@<version>.yaml` | Community definitions (auto-fetched) |
| 5 | Built-in | Compiled into lerd binary | Laravel fallback only |

Workers from the user overlay and project `.lerd.yaml` are merged on top of store or built-in definitions. See [Framework workers](framework-workers.md) for the worker lifecycle and how custom workers are added and managed.

`lerd install` seeds the store: it pulls the index early, so detection sees the whole published catalogue rather than only the frameworks compiled into the binary, then fetches every definition the index lists. A fresh machine therefore ends up with the same definitions an established one has, and resolves any of them offline, instead of collecting them one at a time as the projects that need each one turn up. The refresh keeps any definition you already have that the store has since stopped publishing, and the watcher refreshes the index every six hours. An install that cannot reach the store keeps working on the built-ins and seeds itself on the next run.

::: warning Untrusted projects
A `.lerd.yaml` ships inside a project, so its embedded `framework_def` is treated as untrusted, and lerd strips its host-execution surfaces when restoring it into the store: `command`-type doctor checks, `host: true` workers, the whole `commands:` list, the `nginx:` block, `requires:`, and `php.cli_ini` are dropped, because each would otherwise run on your host, rewrite your nginx config, or start containers straight from a cloned repo. Those run only for frameworks that come from the store, a built-in, or your user overlay (`~/.config/lerd/frameworks/`); a definition already installed there is never overwritten by a project's embedded copy. In-container workers, env, symlink, and combo checks are inert and still work from a project definition.

A project's own host extensions still work, just with consent: a `host: true` entry in top-level `custom_workers`, and any top-level `commands:` you run via `lerd run` or the dashboard, prompt once showing the exact command before they run on your host, and the approval is remembered per site. Set `host_commands.skip_confirmation: true` (or `host_commands.disabled: true` to refuse them outright) in the global config to change that.
:::

## Package definitions

Most of what a definition declares is not really the framework's. A Horizon worker belongs to `laravel/horizon`, a fixtures command to `doctrine/doctrine-fixtures-bundle`, and NativePHP's worker, commands and checks to `nativephp/electron` and `nativephp/mobile`. Written into the version files, each one has to be repeated in every major of every framework that can carry the package, and corrected in all of them at once.

A package definition is that declaration written once, in the same store, as `packages/<vendor>-<name>.yaml`, a sibling of the `frameworks/` directory rather than something inside it, since a package is not a version of a framework. The composer name becomes the file name, the slash it carries being the only thing standing between it and a flat directory:

```yaml
package: nativephp/electron
frameworks:                       # optional: which frameworks, and which majors
  - name: laravel
    min: "11"                     # inclusive; max: works the same way, either may be omitted
workers:
  native:
    label: NativePHP
    command: vendor/nativephp/electron/resources/js/resources/php/php artisan native:serve
    host: true
commands:
  - name: native:build
    label: Build desktop app
    command: vendor/nativephp/electron/resources/js/resources/php/php artisan native:build
setup:
  - label: php artisan native:install
    command: php artisan native:install
    default: false
doctor:
  checks:
    - name: nativephp_runtime
      type: command
      command: test -x vendor/nativephp/electron/resources/js/resources/php/php
```

Workers, commands, post-link setup steps and doctor checks are the whole schema, because that is what the duplication was made of. A `removes:` block takes entries away again, which is what a new major of the package needs (see below). Env wiring, detection and services stay with the framework, which is where they belong.

The layer is merged onto the resolved definition when the project has the package, and only for the frameworks the `frameworks:` list names. An empty list means every framework, which is right for a queue driver and wrong for `nativephp/electron`; `min` and `max` bound the framework majors it applies to, and are compared against the project's own major, so a Laravel 14 project still gets a package that declares `min: "11"`. The package wins any name collision: it is where the entry is maintained now, so a copy left behind in a version file is replaced rather than shadowing it, and the replacement keeps the position the definition listed it in. Your user overlay and a project's `.lerd.yaml` are merged after the package layer and still sit above it.

Having the package means composer installed it, not that the project named it. The manifest is read first, and then `composer.lock`, which is the only place a dependency that arrived under a meta-package shows up, and the only place a package that another one `replace`s or `provide`s exists at all: `tempest/framework` stands in for `tempest/database`, so a stock Tempest project has that code installed while its `composer.json` names neither. A `check:` on a worker, command, setup step or doctor check is answered the same way, since what it is really asking is whether the thing it runs is there. Detection is the exception and reads the manifest alone: a library repo testing against Laravel has `laravel/framework` in its lock, and that must not make it a Laravel site.

### When a package major changes what lerd runs

A package that has kept its interface is one file and says nothing about versions. When a major renames a command, moves a binary, or changes what a worker should run, that major gets a file of its own, `<vendor>-<name>@<major>.yaml`, and the index entry lists it:

```json
{"name": "drush/drush", "versions": ["13", "11"], "latest": "13"}
```

A versioned file serves its own major and every later one until the next versioned file, and the unversioned file serves everything below the first of them. So `drush-drush.yaml` keeps answering for Drush 10 and older, `@11` covers 11 and 12, `@13` covers 13 and up. Adding a major is adding one file: nothing that already worked has to be restated, and no project is moved onto a definition written for a version it does not have. A constraint no major can be read out of (`dev-main`, `*`) takes the latest, since a project tracking a branch is on the newest thing published.

Publishing a major for a package that had one file asks every install for a file it has never fetched. Online that is one fetch, cached like any other. Offline, the newest cached file at or below that version answers instead, down to the unversioned one, so a worker a machine has been running for months does not disappear the day the store gains a version it cannot reach.

Each file is the whole answer for the versions it serves, not a patch on the one before it, so a command that survived the major is repeated in the new file. What a new major *removes* has to be said out loud, because the copy the declaration was lifted out of is still sitting in the framework's own version files and silence there means keep it:

```yaml
package: laravel/horizon
version: "6"
removes:
  commands:
    - horizon:snapshot          # by name
  workers:
    - horizon-metrics
  setup:
    - Publish Horizon assets    # a setup step by its label, the only name it has
  doctor:
    - horizon_supervisor
```

Removals run after the merge, so a package can replace an entry and drop another in the same file.

`lerd framework list` prints the layer under the definitions table: every package the store publishes, what each one declares, which file answers for the project you are standing in, and whether that project requires it. Everything a package contributes surfaces as an ordinary worker or command, so this is the only place that says where a declaration came from. It reads the cache and never fetches.

Only the packages listed under `packages` in the store index are ever looked up, so a project's dependency list never turns into a request for a file the store does not have. They are cached in `~/.local/share/lerd/packages/`, under the same file name the store serves, seeded by `lerd install`, refreshed by `lerd framework update` and on the same 24 hour window as a definition, so a package's worker resolves offline exactly like a framework's.

## Version resolution

When loading a framework definition for a project, the version is resolved in order:

1. `composer.lock`: the version composer resolved for the framework's own package (source of truth)
2. `composer.json`: the major read out of the declared constraint, for a project with no lock
3. A `version_file` regex, for a framework that ships no composer package
4. `.lerd.yaml` `framework_version`: the pinned version
5. Latest available in store

The lock leads because a constraint only says what would be installed: `"^11.0 || ^12.0"` carries two majors and is read as the older one, and `dev-main` carries none at all.

When `composer.lock` shows a different version than `.lerd.yaml`, the pinned version is auto-updated.

A project whose own major version has no definition published for it still gets one. Sitting below the published range, it is served the oldest definition and flagged as guessed, so that definition's PHP range is reported rather than enforced and a Laravel 6 project is still allowed PHP 7.4. Sitting above the range, or in a gap inside it, it is served the newest definition, the same one an install that already had definitions on disk would have fallen back to, so a WordPress 7 site is treated as a WordPress site rather than as no framework at all. A version the store's cached index does not list is never requested, since that fetch can only fail; an install that has never reached the store has no index to rule anything out and asks for the project's own version as before.

## Environment setup

The `env` section in a framework definition controls how `lerd env` works:

```yaml
env:
  file: .env                        # primary env file
  example_file: .env.example        # copied to file if missing
  format: dotenv                    # dotenv | php-const | php-array
  fallback_file: wp-config.php      # read when file doesn't exist (never written)
  fallback_format: php-const        # format for fallback_file
  url_key: APP_URL                  # env key holding the app URL (or "none")
  worktree_url_keys:                # keys set to a worktree's own base URL
    - system.default.web.unsecure.base_url
    - system.default.web.secure.base_url

  # Application key generation
  key_generation:
    env_key: APP_KEY                # env var to check/set
    command: key:generate           # artisan command to run if vendor/ exists
    fallback_prefix: "base64:"     # prefix for random key fallback

  # Per-service detection and env variable injection
  services:
    mysql:
      detect:
        - key: DB_CONNECTION
          value_prefix: mysql
      vars:
        - DB_CONNECTION=mysql
        - DB_HOST=lerd-mysql
        - DB_PORT=3306
        - DB_DATABASE={{site}}
        - DB_USERNAME=root
        - DB_PASSWORD=lerd
```

### Which file lerd writes

`file` is the env file lerd writes, and `lerd env` creates it when it is not there yet, from `example_file` when the definition names one and empty otherwise. `fallback_file` is only ever read, so a project that is already configured is detected through the file it actually has.

`app_file` and `app_format` name the file the application itself reads, for a framework whose configuration is not a dotenv file at all: Drupal's database lives in a `$databases` array its installer writes into `settings.php`, and nothing lerd puts anywhere else reaches it. When a definition declares them, that file is what lerd both reads and writes, from the point its installer has created it: before that lerd writes the `file` the definition names, because a framework reads the skeleton lerd would leave in its place as a site already configured and fails instead of serving the installer. A definition naming no `file` at all has nowhere else to write, so its app file is created as normal. `file`/`fallback_file` also describe what an older binary should do with the same definition. That is why they are a separate pair rather than a change to the existing fields: a published definition reaches every install within a day, whatever version it runs, so one that renamed `format` to something an older release cannot parse would break those installs. An unknown field is ignored; an unknown *format* is not, and a binary too old for a format now refuses to write the file rather than treating it as dotenv.

lerd never seeds an `app_file` the application has not written yet. To a framework an empty one is not a blank slate but a claim that it is already configured: TYPO3 serves its installer while `config/system/settings.php` is absent and fails outright once an empty one exists. So `lerd env` writes the `file` the definition names in the meantime, which keeps the database and the service wiring happening on the way through, and `lerd site:doctor` points at the framework's own install rather than at `lerd env` when there is no example to seed from either. Once the installer has written the file, `lerd env` wires the project's services into it as usual. Magento's `app/etc/env.php` sits in the same position as TYPO3's settings file.

That distinction matters for a framework whose own configuration is not the file lerd writes. Drupal keeps its database in a `$databases` array its installer writes into `settings.php`, and its `site:install` command sources a `.env` at the project root to build the `--db-url` it hands drush. So the definition names `.env` as its `file` and `settings.php` as a read-only `fallback_file`: lerd writes the former, Drupal writes the latter, and neither trips over the other.

A definition that names no `file` at all has only its fallback, and then that fallback *is* the configuration and lerd writes it. That is WordPress, whose `wp-config.php` is the whole story.

### Env file formats

| Format | Shape | Key syntax |
|---|---|---|
| `dotenv` | `KEY=value` lines | `DB_HOST` |
| `php-const` | `define('KEY', 'value')` calls, as in WordPress's `wp-config.php` | `DB_HOST` |
| `php-array` | a PHP file that `return`s a nested array, as in Magento's `app/etc/env.php` | dotted path, `db.connection.default.host` |
| `php-vars` | a PHP file of top-level assignments, as in Drupal's `web/sites/default/settings.php` | dotted path rooted at the variable, `databases.default.default.host` |

`php-vars` is for a framework that configures itself through assignments rather than a returned array. `databases.default.default.host` addresses `$databases['default']['default']['host']`, and writing rewrites only the statements whose values change, leaving the rest of the file, which for Drupal is hundreds of lines of guidance the user may have edited, byte for byte. A key no statement covers is appended as one of its own. That is how lerd writes the file Drupal actually reads: its installer puts the `$databases` array in `settings.php` and reads it back on every request, so connection values left anywhere else wire nothing.

The `php-array` reader flattens the returned array to dotted keys, and the writer sets a dotted path, creating the intermediate arrays when they are missing. Scalar types are preserved, so an int stays an int and a bool stays a bool. The file is reparsed and reprinted rather than patched line by line, which is what Magento's own `DeploymentConfig\Writer` does, so comments in it are not preserved by lerd or by Magento. A rewrite that would not change anything is skipped, so a file already holding every value lerd wants keeps its mtime.

### Wiring the doctor checks

A service a project picks in its `.lerd.yaml` is expected to appear in the env file, and the doctor says so when it does not: it asks whether the file references the `lerd-<service>` container, which is a text question every format answers, so a WordPress site's `wp-config.php` and a Magento site's `app/etc/env.php` are held to it exactly as a `.env` is. Only services this section declares are checked, since those are the ones lerd knows how to wire; a `phpmyadmin` picked alongside `mysql` is picked for its own sake and is never expected in a project's config. A drop-in is checked against the block it stands in for, by family or by its preset's `env_role`, so a project on MariaDB is measured against your `mysql` block. A service listed in `.env.lerd_override`'s `LERD_EXTERNAL_SERVICES`, and `sqlite`, which has no container at all, are both left alone.

### Offering SQLite

A framework that can run on a file database declares an `env.sqlite` block, and that declaration is what puts SQLite in the database choice at `lerd init`. It takes the same `detect` and `vars` a service mapping takes: the detect rules say a project is already on a file database, the vars are what lerd writes to point it at one. A framework declaring none is never offered it, since picking it would configure a project for a database its application cannot open. A project lerd recognises no framework for keeps the option: nothing has declared otherwise.

```yaml
env:
  file: .env
  sqlite:
    detect:
      - key: DB_CONNECTION
        value_prefix: sqlite
    vars:
      - DB_CONNECTION=sqlite
      - DB_DATABASE=database/database.sqlite
  services:
    mysql:
      # …
```

It sits beside `services:` rather than among them because SQLite is not a service. Nothing installs it, starts it or draws a card for it, and a binary that read it as a service entry would announce it and then try to start a container that does not exist. That is also why it is a separate field rather than a `services:` key: a published definition reaches every install within a day, whatever version it runs, and an unknown field is ignored while an unknown service is not.

Choosing it records nothing in `.lerd.yaml`, for the same reason: the project's own configuration already says it is on SQLite, which is what lerd reads to answer that question. An entry left by an older lerd is ignored where it is found.

### Drop-in services

A service preset publishes its connection under Laravel's key names (`DB_HOST`, `REDIS_HOST`), because that is what most projects read. Your framework may not: Drupal reads `DB_NAME` and `DB_USER`, Symfony and CakePHP read a `DATABASE_URL`, Magento addresses its config by dotted path. Those keys are the ones you declare under `env.services`, and they are what lerd writes.

So when a project picks a drop-in for a service you map, lerd wires it up through your mapping rather than the preset's keys, swapping in the drop-in's container. A Drupal site on MariaDB gets `DB_HOST=lerd-mariadb-11-8` alongside the `DB_DRIVER` and `DB_NAME` it actually reads, and a Magento site gets `db.connection.default.host: lerd-mariadb-11-8`. A drop-in is protocol-compatible with the service it stands in for, so the container is the only thing that moves; the port, credentials and driver name in your mapping still hold.

A key the preset sets that your mapping leaves unset is written too, as long as your definition names it somewhere: Laravel's `redis` block sets the host but not the cache, session and queue drivers Valkey switches on, and its `detect` rules name all three, so those still land. Your definition is your whole vocabulary, so a key it never names is a key your app cannot read, and lerd will not write it. That is what keeps the preset's Laravel-shaped `DB_*` keys out of a Symfony `.env`, where `DATABASE_URL` is the only key that means anything.

The drop-in is matched to the mapped service by its family, or by the [`env_role`](/usage/custom-services#yaml-schema) the preset declares when the relationship crosses families (MariaDB for MySQL, Valkey for Redis). On a `php-array` framework a picked service you map nothing for is started but left unwired, since a preset's flat keys are meaningless as dotted paths and lerd will not guess where they belong.

Do not pin a database version your framework passes to its ORM. Doctrine picks its platform from `serverVersion` when it is set, and that string cannot be spelled for a drop-in: a MariaDB server given `11.8` is read as MySQL. Left out, Doctrine asks the server and is right for every database and version, which is why Symfony's `DATABASE_URL` carries no `serverVersion`.

## YAML schema

```yaml
# Required
name: symfony                     # slug [a-z0-9-], must match filename stem
label: Symfony                    # display name
color: "#000000"                  # brand colour the dashboard tints the mark with
public_dir: public                # document root relative to project

# Version (required for store definitions)
version: "8"                      # framework major version this definition targets

# PHP version range (optional, used during lerd link/init to clamp PHP version)
php:
  min: "8.2"                      # minimum supported PHP version
  max: "8.5"                      # maximum supported PHP version
  cli_ini:                        # php.ini directives for PHP processes lerd runs (optional)
    memory_limit: 2G              # bounded: workers get these too

# Detection rules, any match is sufficient
detect:
  - file: symfony.lock
  - composer: symfony/framework-bundle

# Env file configuration
env:
  file: .env.local
  example_file: .env
  format: dotenv                  # dotenv | php-const | php-array
  fallback_file: settings.php     # used when file doesn't exist (optional)
  fallback_format: php-const
  url_key: DEFAULT_URI            # env key holding the app URL (default: APP_URL;
                                  # `none` opts out for frameworks that keep the
                                  # base URL elsewhere, e.g. Magento's database)
  worktree_url_keys:              # keys set to a worktree's own base URL, even
    - web.unsecure.base_url       # when url_key is `none` — lets a worktree whose
    - web.secure.base_url         # canonical base URL is database-hosted (Magento)
                                  # override it in env.php and serve on its own
                                  # domain instead of redirecting to the parent
  vars:                           # unconditional env defaults, always applied (optional)
    - "CI_ENVIRONMENT=development" # e.g. force CodeIgniter into dev mode for local work
  key_generation:                 # application key generation (optional)
    env_key: APP_KEY
    command: key:generate
    fallback_prefix: "base64:"

  sqlite:                         # wiring for a file database (optional). Not a
    detect:                       # service: nothing installs, starts or draws it.
      - key: DB_CONNECTION        # Declaring it is what offers SQLite at `lerd init`.
        value_prefix: sqlite
    vars:
      - "DB_CONNECTION=sqlite"
      - "DB_DATABASE=database/database.sqlite"

  # Per-service env detection and variable injection for `lerd env`
  #
  # Template variables available in vars values:
  #   {{site}}              : project database / handle name (e.g. myapp)
  #   {{site_testing}}      : testing database name (e.g. myapp_testing)
  #   {{bucket}}            : S3-safe bucket name (lowercase, hyphens; e.g. my-app)
  #   {{domain}}            : site's primary domain (e.g. myapp.test)
  #   {{scheme}}            : http or https depending on TLS status
  #   {{mysql_version}}     : running MySQL server version
  #   {{postgres_version}}  : running PostgreSQL server version
  #   {{redis_version}}     : running Redis server version
  #   {{meilisearch_version}} : running Meilisearch server version
  services:
    mysql:
      detect:
        - key: DATABASE_URL
          value_prefix: "mysql://"
      vars:
        - "DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/{{site}}"

# Scaffold command for "lerd new"
create: composer create-project symfony/skeleton

# Service presets the framework cannot run without (optional). Link installs and
# starts them and records them in .lerd.yaml, so a teammate cloning the repo gets
# them too. The doctor fails when one is missing and warns when it is stopped.
requires:
  - opensearch

# Dependency installation. `false` means the framework never uses that package
# manager, and `lerd setup` does not offer its steps at all. Magento and Drupal
# set npm: false; WordPress sets both to false.
composer: auto                    # auto | true | false
npm: auto

# Console command (without 'php' prefix)
console: bin/console

# Console commands that cannot run in the container and the binary that runs them
# on the host (optional). `args` is a space-separated glob matched against the
# arguments as typed, so one entry covers `lerd php artisan native:run` and
# `lerd artisan native:run` alike; `binary` is relative to the project root. The
# first match wins, so a package declaration is never shadowed by the framework
# file it merges over, and a binary that is not installed is an error rather than
# a fall back into the container. `install_command` names the `lerd run` command
# that puts that binary on disk, and belongs to the entry rather than to the
# package, since a project can carry two packages installed by differently named
# commands. An entry declaring none says so rather than naming another's.
# `requires_extensions` names the PHP extensions that binary has to carry for the
# command to work. Bundled runtimes are trimmed static builds with dynamic
# loading compiled out, so a missing extension cannot be added to one. lerd asks
# the binary itself, and when it is short an extension the command needs, runs
# that command with lerd's own pinned PHP, downloaded to ~/.local/share/lerd/bin
# the first time a project needs it. The build matches the project's own PHP
# version, since the command runs project code against a composer.lock resolved
# for it, and falls back to the nearest pinned minor when lerd pins none.
# Everything else about the command is unchanged: same host, same working
# directory, same environment.
host_commands:
  - args: artisan native:*
    binary: vendor/nativephp/electron/resources/js/resources/php/php
    install_command: native:install
  - args: artisan native:jump
    binary: vendor/nativephp/php-bin/bin/host/php
    requires_extensions: [posix, pcntl]

# Background workers
workers:
  messenger:
    label: Messenger
    command: php bin/console messenger:consume async --time-limit=3600
    reload_command: ""            # alternate command for auto-reload (restart on
                                  # file changes) during development (optional). When a
                                  # project opts this worker into reload mode, lerd runs
                                  # this command instead of `command`, and on macOS
                                  # appends `--poll` since the container cannot observe
                                  # host filesystem events. Laravel's horizon worker sets
                                  # it to `php artisan horizon:listen`.
    tune_command: ""              # parameterized variant of `command` (optional). Every
                                  # {placeholder} becomes a flag on the worker's generated
                                  # start command, so
                                  # `messenger:consume {transport} --limit={limit}` gives
                                  # `lerd messenger:start --transport --limit`. Each flag's
                                  # default is read back out of `command`, so the values are
                                  # declared once; a placeholder `command` does not spell has
                                  # no default and must be passed.
    restart_command: ""           # graceful in-container restart, e.g.
                                  # `php artisan queue:restart` (optional)
    requires_service: {}           # a lerd service the worker cannot run without (optional):
                                  #   name: redis
                                  #   when_env: QUEUE_CONNECTION=redis
                                  # `when_env` narrows the requirement to sites whose .env
                                  # carries that KEY=VALUE. lerd refuses the start and names
                                  # the service instead of letting the worker crash-loop.
    restart: always               # always | on-failure (default: always)
    schedule: ""                  # systemd OnCalendar expression (optional). When set, the
                                  # worker is run as a Type=oneshot service triggered by a
                                  # sibling .timer instead of a long-running daemon. Use this
                                  # for cron-style commands like Laravel <=10's
                                  # `php artisan schedule:run`, which exits immediately and
                                  # would otherwise restart-loop under restart=always. Any
                                  # systemd OnCalendar value is accepted (e.g. `minutely`,
                                  # `*:0/5`, `Mon..Fri *-*-* 02:00:00`). Linux only; on
                                  # macOS scheduled workers currently log a warning and skip.
    check:                        # only shown when check passes (optional)
      composer: symfony/messenger # matches a package composer installed, not
                                  # only one composer.json names
    conflicts_with:               # workers to stop before starting (optional)
      - other-worker
    proxy:                        # nginx proxy config (optional)
      path: /ws
      paths:                      # several paths on one port (optional)
        - /ws
        - /ws-api
      port_env_key: WS_PORT
      default_port: 8080
    host: false                   # run on the host via fnm instead of in the FPM
                                  # container (optional, default: false). Used for
                                  # HMR-sensitive Node tools (Vite, Tailwind watcher).
    per_worktree: false           # run independently per git worktree under
                                  # lerd-<wname>-<site>-<wt> (optional, default:
                                  # false). Required for worktree auto-start.
    replaces_build: false         # while running, provides the asset manifest;
                                  # `lerd worktree add` skips the build prompt for
                                  # opted-in workers (optional, default: false).

# One-off setup commands
setup:
  - label: "Run migrations"
    command: "php bin/console doctrine:migrations:migrate --no-interaction"
    default: true
    check:
      composer: doctrine/doctrine-migrations-bundle  # skipped if package not installed
  - label: "Install the app"                         # placeholders work here too
    command: "bin/install --url={{scheme}}://{{domain}}/ --db={{site}}"
    default: false
    check:
      missing_file: config/installed.php             # only while the app is not installed yet

# Application log files shown in the UI "App Logs" tab
logs:
  - path: "var/log/*.log"             # glob relative to project root
    format: raw                       # monolog | raw (plain text, default)

# Custom commands, shown in the dashboard and runnable with `lerd run` (optional)
commands:
  - name: cache:clear                 # stable id, unique; the `lerd run` argument
    label: Clear cache                # display name
    command: bin/console cache:clear  # shell, run through `sh -c`
    description: Clear the Symfony cache for the current environment
    output: silent                    # silent | text | url | terminal (default: text)
    confirm: false                    # gate behind a confirmation (optional, default: false)
    icon: broom                       # name from the known icon set (optional)
    cwd: .                            # working dir relative to project root (optional, default: .)
    check:                            # hide the command unless the rule matches (optional)
      composer: symfony/framework-bundle

# Site doctor checks, run after the universal baseline every framework gets (optional)
doctor:
  migrate_command: doctrine:migrations:migrate   # the command that applies the schema
  checks:
    - name: storage_link              # stable id
      type: symlink                   # env_key_set | env_combo | symlink | command
      label: Storage Link             # display label
      link: public/storage            # the path that must be a symlink
      target: storage/app/public      # skipped unless this dir exists
      requires_dir: public            # skipped unless this dir exists too (optional)
      fix: storage:link               # names one of the framework's own commands
      detail: The public/storage link is missing.   # overrides the generated message (optional)
      severity: warn                  # warn | fail (optional, per-type default)
      check:                          # drop the check unless the rule matches (optional)
        composer: nativephp/electron

# Where the Debug window's engine-level capture should observe this framework
# (optional). Jobs are the only kind so far: one entry per method that runs a
# queued job, reported as processing then processed or failed, timed.
devtools:
  jobs:
    - implements: Drupal\Core\Queue\QueueWorkerInterface   # or class: / extends:
      method: processItem
      name: this                    # this | arg:N, optionally .method:getHook / .prop:queue

# Extra nginx config spliced into the site's server block (optional)
nginx:
  snippet: |
    location /static/ {
      try_files $uri $uri/ /static.php?$args;
    }

# What a new worktree needs once its env file is seeded (optional)
worktree:
  db_isolation: required              # required | (unset, which prompts as usual)
  db_source: main                     # what an isolated database starts from: empty | main
  commands:                           # console commands run once env and database are ready
    - app:config:import
```

An app that keeps deployment state in its database cannot share the parent's. Magento hashes its file config and stores the hash in the database, so seeding a worktree's own base URL into `env.php` makes the store refuse to serve until `app:config:import` re-syncs it, and running that import against a shared database would rewrite the hash out from under the parent site. `db_isolation: required` therefore skips the prompt and isolates, `db_source: main` clones the parent's data (an empty schema is useless to a store that cannot bootstrap itself), and `commands` run afterwards, in the worktree, through the framework's own `console` binary.

## Queued job seams

A queue is the one part of a framework the Debug window cannot reach through a shared library: Laravel dispatches its own events, Symfony has Messenger, and everyone else runs jobs through a class only that framework knows. Rather than teach the extension each name, a definition declares the method that runs a job and lerd renders every framework's declarations into one file the extension reads at startup.

Exactly one of `class`, `implements` or `extends` selects what the seam applies to. `implements` is usually the right one: a framework's queue almost always defines an interface every job implements, so a single line covers whatever the project queues. The extension resolves the interface without autoloading, which is safe because the class being executed is loaded already and so is everything it inherits from.

`name` says where the job's label comes from, and defaults to `this`. The vocabulary is deliberately small: `this` or `arg:N` names the subject, and an optional `.method:getHook` or `.prop:queue` reads one value off it. A subject with no accessor yields its class, which is what a queued job is normally called; WordPress is the exception, where every job is an `ActionScheduler_Action` and the useful name is the hook it runs, so its seam reads `name: this.method:get_hook`.

Nothing about a seam is compiled in, so adding one is a store change that reaches every install within a day. A container already running picks up a new seam when it next restarts.

## The framework's own mark

A framework had a label and nothing else to identify it, so it showed up as a
text badge on the site header, in the sites widget, in a site tile's subtitle, as
the heading a sites dashboard groups under, and as the hint in the command
palette. A definition in the store can now bring its own logo: a
`frameworks/<name>.svg` beside the versioned files, fetched and cached with the
definition, so a framework published tomorrow arrives with its mark and no lerd
release.

The mark is per family, not per version. Laravel 11 and Laravel 12 are the same
logo, so one file sits next to `<name>/<version>.yaml` rather than inside each of
them, and every version resolves to it. The colour is the opposite: it is
declared in the YAML, which only exists per version, so each version file repeats
the same `color:`.

It is **monochrome**: a silhouette of filled paths with no colours of its own,
rendered through `currentColor` and tinted by `color:`. Because that markup is
remote and ends up inlined in the page, lerd cuts it down on the way in to the
same drawing subset a service mark gets, dropping script, `foreignObject`, event
handlers, external references and any `fill`, `stroke` or `style` of its own. See
[service presets](service-presets.md) for the exact subset; the rule is shared.

`color:` must be a plain hex literal. Anything else, a colour function or a CSS
variable, is dropped rather than passed through, and a colour too dark or too
light for the card it lands on is nudged toward it until it separates, so
Symfony's black still reads on the dark card. A framework that declares a colour
but ships no mark still gets the tint; one with neither renders as its label
alone, which is what every framework did before.

This is not the `icon:` a framework command declares. That names a glyph from the
built-in set for a button in the dashboard and is a different thing entirely.

## Site placeholders

The <code v-pre>{{site}}</code>, <code v-pre>{{site_testing}}</code>, <code v-pre>{{bucket}}</code>, <code v-pre>{{domain}}</code>, <code v-pre>{{scheme}}</code>, and <code v-pre>{{&lt;service&gt;_version}}</code> placeholders listed above are expanded in three places: the `env.services` vars, every `setup:` command, and every `commands:` entry. They resolve against the registered site the command runs for. A git worktree is not a registered site, so a command run against one resolves <code v-pre>{{site}}</code> but leaves <code v-pre>{{domain}}</code> and <code v-pre>{{scheme}}</code> alone.

This is what lets a framework whose bootstrap needs to know where the site lives declare that step as data. Magento 2.4 removed its web installer, so a fresh store is installed with `bin/magento setup:install --base-url=… --db-name=…`; the definition can now express exactly that. A step that creates schema should carry `default: false` so it is opt-in rather than running on every `lerd setup`, and it should gate itself on `check: missing_file:` naming the file the install writes, so it is offered on a project that has never been bootstrapped and nowhere else. `default: false` alone is not enough for that: `lerd setup --all` runs every step it is offered regardless of the default, and rerunning an installer over a working app is how its data goes away.

A placeholder whose value is empty, or one lerd does not recognise, is left in the command verbatim rather than being replaced with an empty string, so a half-resolved context can never quietly produce `--base-url=://`.

## Custom commands

The `commands:` list is the framework's own verbs: the things you would otherwise type into a console by hand. Each entry shows up on the site's dashboard, in the command palette, and as an argument to `lerd run`, and can be named as the `fix:` of a doctor check.

`name` and `command` are the only required keys. The name is a stable identifier, unique within the definition, and is what `lerd run <name>` and a doctor `fix:` both refer to, so treat it as API and don't rename it casually. The command is a shell string handed to `sh -c`, with the [site placeholders](#site-placeholders) expanded first. It runs in the site's PHP-FPM container, from the project root unless `cwd` moves it; `cwd` is a path relative to that root, and `.` and an empty value both mean the root itself. When a command is run against a git worktree, the root is the worktree's own checkout.

`output` decides where the command's output goes, and the four values are genuinely different surfaces:

| Value | What happens |
|---|---|
| `text` | Streams stdout and stderr into the run modal as they arrive, and the modal stays open afterwards showing the exit code and duration. This is what you get when `output` is omitted. |
| `silent` | Runs without opening the modal at all, and shows a toast when it finishes. A non-zero exit is the exception: the modal opens after all, carrying the captured output, because that output is the only thing that explains the failure. Use it for commands whose output nobody reads, like a cache clear. |
| `url` | Streams like `text`, and additionally lifts the first `http://` or `https://` URL out of the output into a copy-and-open panel on the finished modal. This exists for one-time login links, like Drupal's `drush uli`. |
| `terminal` | Spawns the user's terminal emulator running the command, instead of streaming it anywhere. Nothing is captured, so there is no output pane, no exit code, and no run history. Use it for commands that are interactive or long-lived enough that a modal is the wrong container. It is rejected over MCP, which has no terminal to open. |

`confirm: true` puts the command behind a confirmation showing the exact command line before anything runs, and the dashboard, `lerd run` (unless you pass `--yes`) and MCP (unless the caller forces it) all honour it. This is what lets a genuinely destructive command ship as a command rather than as a setup step: Laravel's `migrate:fresh` drops every table, and Magento ships `setup:install` this way.

`check` takes the same rule shape as a worker's or a setup step's, so `composer: <package>`, `file: <path>`, or `missing_file: <path>` for the opposite reading, and a command whose check fails is dropped from the resolved set rather than merely hidden, which means it also disappears from `lerd run` and from any doctor `fix:` pointing at it. Use it for commands that only make sense when an optional package is installed.

`icon` is drawn from a fixed vocabulary, and a name outside it renders a generic fallback rather than failing. The set is:

`broom`, `database`, `refresh`, `link`, `check`, `list`, `key`, `edit`, `arrow-down`, `arrow-up`, `play`, `terminal`

`lerd site:doctor` validates a definition's commands, and it is the fastest way to catch a typo: an unknown `output` is an error, and an unknown `icon` is a warning.

## Declining a warning

A definition can turn off a warning that says nothing useful about sites built on it:

```yaml
notifications:
  nplusone: false                 # this framework's own layers issue the repeats
```

The repeated-query warning is the case that needs it. On a content management system the entity, config and cache layers issue the repeats during an ordinary request, so the warning names a loop inside the framework that nobody using it can change, and browsing an admin fires one after another. Where the queries come from the code a developer writes, which is most application frameworks, it stays on and is worth having, so a definition saying nothing keeps it.

## Doctor checks

The `doctor:` section adds framework-specific health checks to the ones every site gets for free (a valid `.lerd.yaml`, env file present, every picked service wired into it, dependencies installed and locked, audit clean, PHP version in range, nginx vhost current). They run on `lerd site:doctor` and in the dashboard's doctor panel. Keeping them declarative is what stops the doctor from growing a Go branch per framework.

The section also takes a `migrate_command`, naming whichever of the framework's own `commands:` applies the schema. The universal database checks offer it as their fix, so an empty or missing database is reported with the button that fills it. A server database that does not exist at all is the exception: migrations have nowhere to run until the schema is there, so that finding carries a button that creates it and the migrate button returns on the re-check. Every framework spells it differently (Laravel `migrate`, Symfony `doctrine:migrations:migrate`, Drupal `updb`), so nothing but the definition can say; a framework that declares none, or names a command it does not have, gets a finding with no fix rather than a button that maps to nothing.

Each check carries a `name` (a stable id), a `type` that selects the evaluator, an optional `label` for display, an optional `detail` that overrides the generated message, an optional `severity`, an optional `fix`, and an optional `check`.

`check` takes the same rule shape as a worker's or a command's, so `composer: <package>`, `file: <path>`, or `missing_file: <path>`. A check whose rule fails is dropped rather than run, which is what a check about an optional package wants: NativePHP's desktop runtime has nothing to say on a Laravel project that never installed it, and a permanently green row is clutter.

A top-level `cache_command` names the console subcommand that clears the framework's compiled caches, `cr` for Drupal, `cache:clear` for Symfony, `optimize:clear` for Laravel. lerd runs it through `console` after rewriting a project's database connection: a framework caches the container definitions built from that configuration, and one built against the old database survives the swap and answers every request with an error about something it can no longer find. It runs only when a database key's value actually changed, and only when the project's dependencies are installed.

`fix` names one of the framework's own `commands:` entries, by `name`. That indirection is the whole design: the doctor never grows its own mutation endpoints, it just points at a command the framework already exposes, and the UI renders a Fix button that runs it. A `fix` naming a command that does not exist, or one whose `check` rule failed, simply renders no button, and nothing validates the reference, so check your spelling. Five universal keys are also accepted, for the fixes that are not framework-specific: `composer_install`, `composer_update`, `npm_install`, `npm_audit_fix` and `vhost_regenerate`. The first four run in the site's container like any command; `vhost_regenerate` rewrites the site's vhost on the host and reloads nginx, and is the fix the vhost check carries.

The Fix button runs the command through the same gate as everywhere else, so a fix pointing at a `confirm: true` command still asks first, and the doctor re-checks only once the command has actually run.

There are four check types, each with its own fields.

`env_key_set` fails when a single env key is empty. It takes `env_key`, the key to read.

```yaml
- name: mailer_dsn
  type: env_key_set
  label: Mailer
  env_key: MAILER_DSN
  detail: MAILER_DSN is empty, so no mail will be sent.
```

`env_combo` catches a combination of env values that is individually legal but collectively a footgun, the classic being debug mode left on in production. It takes `when` and `warn_if`, both maps of key to expected value, and only triggers when every pair in both maps matches. Values are compared truthily, so a `warn_if` of `true` matches `1`, `on` and `yes` as well.

```yaml
- name: app_debug
  type: env_combo
  label: Debug Mode
  when: { APP_ENV: production }
  warn_if: { APP_DEBUG: true }
  detail: APP_DEBUG is on in production, which leaks stack traces.
```

`symlink` checks that a path is a symlink, for the likes of Laravel's `public/storage`. It takes `link`, the path that should be one, and `target`, the directory it should point into. The check skips itself entirely when `target` does not exist, since the link is meaningless then, and `requires_dir` adds a second directory that must exist for the check to apply at all.

`command` runs a console command inside the site's container and judges the result. It takes `command`, and `fail_if_output_contains`, a plain substring that marks the finding as triggered when it appears in the output. `timeout` caps the run in seconds, defaulting to 25. `unknown_on_error: true` is the important one: when the command cannot run at all, because the app is wedged or the database is unreachable, the check reports "unknown" instead of failing, so a down app does not turn the whole panel red with checks that never actually ran.

`fail_if_error_contains` is the escape hatch from that. Some commands exit non-zero for the very condition the check exists to catch: `php artisan migrate:status` fails with "Migration table not found" on a database that has never been migrated, which is the normal state of a freshly linked project and exactly what `fix: migrate` resolves. A substring named here is matched against the output of a non-zero run and reports the finding with its fix, ahead of `unknown_on_error`. Anything else still degrades to "unknown", so that bucket keeps meaning genuine connectivity problems.

```yaml
- name: migrations
  type: command
  label: Migrations
  command: php artisan migrate:status
  fail_if_output_contains: "Pending"
  fail_if_error_contains: "Migration table not found"
  unknown_on_error: true
  timeout: 30
  fix: migrate
```

`severity` overrides the status a triggered check reports, and takes `warn` or `fail`. The default differs by type, which is not something you would guess: a `command` check defaults to `fail`, and the other three default to `warn`. So a pending-migrations check is a failure unless you say otherwise, while a missing symlink is a warning. An unrecognised severity is ignored rather than rejected, falling back to the type default.

An unknown `type` is skipped rather than treated as an error, so a definition using a check type a newer lerd added still loads on an older binary; the new check just does not run.

## PHP ini for the CLI

A project can already raise php.ini settings for its **web** requests by shipping a `.user.ini` in the document root, which PHP-FPM reads per directory. Magento does exactly that, with `memory_limit = 756M`.

The **CLI SAPI never reads `.user.ini`**, not even from inside the document root. So a framework whose commands need more than PHP's 128M default has nowhere to say so, and Magento's `setup:upgrade` and `deploy:mode:set` die with an allocation failure deep inside `symfony/cache` that never mentions memory.

`php.cli_ini` fills that gap. Each directive is passed as `-d name=value` to every PHP process lerd starts for that project: the `php` shim (and therefore `lerd artisan`, `lerd run`, a `vendor/bin` binary, and a `command`-type doctor check), and the `setup:` steps and the workers, which exec in the container directly. Directives are sorted, so the argv is stable, and they are prepended, so a `-d` you type yourself lands later and wins.

Workers get the directives too. They exec their command straight from a systemd unit, and Magento cannot even bootstrap at PHP's 128M default, so a cron or consumer worker without them simply crash-loops. That makes the value a definition author's responsibility: give it a bounded `memory_limit` rather than `-1`, because a worker runs until you stop it. A worker whose command is not a `php` invocation, a host-side `npm run dev`, is left alone.

Set only what the CLI needs. Copying a framework's web values across is a trap: CLI `max_execution_time` defaults to `0`, meaning unlimited, so applying a web value of `600` would cap a long install at ten minutes.

`PHP_VALUE`-style directives can set `auto_prepend_file`, which makes every PHP process execute a file from the repo, so `cli_ini` is honoured only from the trusted store and from a user overlay. An embedded `framework_def` in a project's `.lerd.yaml` has it stripped.

## Required services

Most frameworks run against whatever services the project happens to reference. A few cannot start at all without one. Magento 2.4 removed the MySQL catalog search engine, so a store without OpenSearch or Elasticsearch fails partway through `setup:install` with a stack trace that never mentions the search engine.

A definition lists those in `requires:`, naming service presets. On `lerd link` each one is resolved from the service store, installed if it is not already, started, and appended to the project's `.lerd.yaml` so the requirement travels with the repo. Re-linking does not duplicate an entry, and a name the service store does not know is reported and skipped rather than written into the project's committed config.

The site doctor reports the same thing after the fact: a required service that is not installed is a failure, since the app cannot boot, and one that is installed but stopped is a warning, since starting it is a single command.

A required service pulls an image and runs a container, so, like host workers and `nginx.snippet`, `requires:` is honoured only from the trusted store and from a user overlay. An embedded `framework_def` in a project's `.lerd.yaml` has it stripped.

## Framework nginx config

Most frameworks route every request through a single front controller, which lerd's generic `location /` already handles. A few need paths that the generic rules would otherwise swallow: Magento keeps `setup/` outside the document root and generates `/static/` and `/media/` on demand through `pub/static.php` and `pub/get.php`.

The optional `nginx.snippet` is raw nginx config, spliced into the site's server block **before** lerd's `location /` and `location ~ \.php$`. Placement matters, because nginx picks the first matching regex location in declaration order, so a framework block always gets first refusal on the paths it claims.

Three placeholders are expanded before the config is written:

| Placeholder | Expands to |
|---|---|
| `{{root}}` | the project root |
| `{{public}}` | the document root (project root joined with `public_dir`) |
| `{{fpm}}` | the site's PHP-FPM container name |

The two path placeholders expand to nginx variables, `${lerd_root}` and `${lerd_public}`, which lerd declares at the top of the server block with the real paths. A project can live under a path with a space in it, and nginx splits a directive on whitespace: a literal path would turn `root` into three arguments and nginx would reject the whole config, taking every other site on the machine down with it. Quoting the value would fix a standalone `root {{root}};` but not a path used mid-token, as in `alias {{public}}/static/;`, since nginx will not glue a quoted token to a bare one. A variable is resolved after tokenizing, so it works in both positions. Write the placeholders exactly where you would write the path and lerd handles the rest.

A snippet that passes requests to PHP should assign `{{fpm}}` to a variable first, `set $myfpm "{{fpm}}";` then `fastcgi_pass $myfpm:9000;`, exactly as the generated vhost does. nginx resolves a literal upstream name once when the config loads and caches it for the life of the process, so a container that comes back on a new address is never picked up.

A git worktree of the site gets the same block, expanded against its own checkout: `{{root}}` and `{{public}}` point at the worktree's directory, not the parent's, so a branch serves its own `setup/`, `/static/` and `/media/` paths.

The snippet must have balanced braces, since an unbalanced one would close the enclosing `server` block and start declaring its own. Balance alone is not enough, because a `}` followed by a `server {` still balances, so the values substituted into the placeholders are rejected too if they contain `{`, `}`, `;`, `#`, or a newline. A snippet failing either check is dropped and the site renders without it, rather than risking an nginx config that fails to load for every site.

Snippets are only honoured from the framework store and from user-defined definitions: an embedded `framework_def` in a project's `.lerd.yaml` is untrusted input, so its `nginx` block is stripped, the same way its host workers and command-type doctor checks are.

This is distinct from the per-site [nginx override](nginx-overrides.md) in `custom.d/`, which you author yourself and which is included at the *end* of the server block. Use the framework snippet for what every site of that framework needs; use the override for what one site needs.

## Framework detection

Framework detection only runs during `lerd link`, `lerd init`, `lerd env`, `lerd setup`, and `lerd park`. All other commands read the saved framework from the site registry, and fall back to detecting one for a site whose registry entry holds no framework, so a site registered before its definition existed picks it up on the next read rather than needing a relink. A project that names a framework no definition can be found for keeps that name: it is registered and labelled as what it says it is, without the public dir or PHP range a definition would have supplied.

Detection order:

1. **Laravel** (built-in): checks for `artisan` file or `laravel/framework` in `composer.json`
2. **Local definitions**: iterates user-defined and store-installed YAML files, applying detection rules
3. **Framework store** (interactive): checks the store index and prompts to install, or fetches silently when `.lerd.yaml` specifies the framework name

The first match wins. Detection rules are OR-based, any single matching rule is enough.

## Document root detection

If no framework matches and no `--public-dir` is specified, lerd tries these candidate directories in order, accepting the first that contains an `index.php`:

`public` → `web` → `webroot` → `pub` → `www` → `htdocs` → `.` (project root)

Serving a site resolves the root again from three places, in this order: the `public_dir` the project commits in its `.lerd.yaml`, the root recorded for the site when it was linked, and the framework definition's `public_dir`. The recorded one gives way to the definition's in one case, when it holds no `index.php` and the definition's does. A root lerd guessed is only as good as the moment it was guessed in, and a project linked before `composer install` has an empty document root to walk, so the guess lands on the project root and would otherwise pin the site there long after the real root appeared.

## Log viewer

Frameworks can define application log file locations so they appear in the UI's **App Logs** tab. The tab only appears when matching log files actually exist on disk; for example, WordPress defines `wp-content/debug.log` but the tab stays hidden until `WP_DEBUG_LOG` is enabled. Custom frameworks can add their own:

```yaml
logs:
  - path: "var/log/*.log"
    format: raw
```

The `path` is a glob relative to the project root. The `format` controls parsing:

| Format | Description |
|---|---|
| `monolog` | Monolog format: `[date] channel.LEVEL: message {context}` with stacktrace grouping |
| `raw` | Plain text, each line shown as a separate entry (default) |

The App Logs tab is the first tab in the site detail view. When the UI opens it automatically selects the site with the most recent log activity, so you immediately see logs from the project you last visited in your browser.

Features:

- **File selector**: switch between available log files (e.g. `laravel.log`, `worker.log`), sorted by modification time with the newest file pre-selected
- **Latest / All toggle**: "Latest" shows the last 100 entries (default), "All" reads the entire file
- **Search**: filter entries by message, level, date, or stacktrace content
- **Expandable entries**: click any entry to expand and see the full detail and stacktrace
- **Auto-refresh**: polls every 5 seconds while the tab is active, keeping the expanded entry open
- **Color-coded levels**: entries are color-coded by severity (red for ERROR/CRITICAL/EMERGENCY/ALERT, yellow for WARNING, blue for INFO/NOTICE, grey for DEBUG)

To customise Laravel's log paths (e.g. add a custom channel log):

```yaml
# ~/.config/lerd/frameworks/laravel.yaml
name: laravel
logs:
  - path: "storage/logs/*.log"
    format: monolog
  - path: "storage/logs/custom/*.log"
    format: monolog
```

---

See also: [Frameworks](frameworks.md) for the store and commands; [Framework workers](framework-workers.md) for worker lifecycle.
