# Framework definitions

Framework definitions are YAML files that tell Lerd how to detect a PHP framework, where its document root is, which env file it uses, and which workers and log paths it has. This page is the full schema reference.

## Definition sources and priority

Lerd resolves framework definitions from multiple sources. Higher priority wins:

| Priority | Source | Location | Purpose |
|----------|--------|----------|---------|
| 1 | User overlay | `~/.config/lerd/frameworks/<name>.yaml` | Manual overrides (merged on top) |
| 2 | Project embedded | `.lerd.yaml` `framework_def` | Portability for user-defined frameworks |
| 3 | Store-installed | `~/.local/share/lerd/frameworks/<name>@<version>.yaml` | Community definitions (auto-fetched) |
| 4 | Built-in | Compiled into lerd binary | Laravel fallback only |

Workers from the user overlay and project `.lerd.yaml` are merged on top of store or built-in definitions. See [Framework workers](framework-workers.md) for the worker lifecycle and how custom workers are added and managed.

::: warning Untrusted projects
A `.lerd.yaml` ships inside a project, so its embedded `framework_def` is treated as untrusted, and lerd strips its host-execution surfaces when restoring it into the store: `command`-type doctor checks, `host: true` workers, the whole `commands:` list, the `nginx:` block, and `requires:` are dropped, because each would otherwise run on your host, rewrite your nginx config, or start containers straight from a cloned repo. Those run only for frameworks that come from the store, a built-in, or your user overlay (`~/.config/lerd/frameworks/`); a definition already installed there is never overwritten by a project's embedded copy. In-container workers, env, symlink, and combo checks are inert and still work from a project definition.

A project's own host extensions still work, just with consent: a `host: true` entry in top-level `custom_workers`, and any top-level `commands:` you run via `lerd run` or the dashboard, prompt once showing the exact command before they run on your host, and the approval is remembered per site. Set `host_commands.skip_confirmation: true` (or `host_commands.disabled: true` to refuse them outright) in the global config to change that.
:::

## Version resolution

When loading a framework definition for a project, the version is resolved in order:

1. `composer.lock`: the actual installed version (source of truth)
2. `.lerd.yaml` `framework_version`: pinned version (fallback when no `composer.lock`)
3. Latest available in store

When `composer.lock` shows a different version than `.lerd.yaml`, the pinned version is auto-updated.

## Environment setup

The `env` section in a framework definition controls how `lerd env` works:

```yaml
env:
  file: .env                        # primary env file
  example_file: .env.example        # copied to file if missing
  format: dotenv                    # dotenv | php-const | php-array
  fallback_file: wp-config.php      # used when file doesn't exist
  fallback_format: php-const        # format for fallback_file
  url_key: APP_URL                  # env key holding the app URL

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

### Env file formats

| Format | Shape | Key syntax |
|---|---|---|
| `dotenv` | `KEY=value` lines | `DB_HOST` |
| `php-const` | `define('KEY', 'value')` calls, as in WordPress's `wp-config.php` | `DB_HOST` |
| `php-array` | a PHP file that `return`s a nested array, as in Magento's `app/etc/env.php` | dotted path, `db.connection.default.host` |

The `php-array` reader flattens the returned array to dotted keys, and the writer sets a dotted path, creating the intermediate arrays when they are missing. Scalar types are preserved, so an int stays an int and a bool stays a bool. The file is reparsed and reprinted rather than patched line by line, which is what Magento's own `DeploymentConfig\Writer` does, so comments in it are not preserved by lerd or by Magento.

## YAML schema

```yaml
# Required
name: symfony                     # slug [a-z0-9-], must match filename stem
label: Symfony                    # display name
public_dir: public                # document root relative to project

# Version (required for store definitions)
version: "8"                      # framework major version this definition targets

# PHP version range (optional, used during lerd link/init to clamp PHP version)
php:
  min: "8.2"                      # minimum supported PHP version
  max: "8.5"                      # maximum supported PHP version

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
  vars:                           # unconditional env defaults, always applied (optional)
    - "CI_ENVIRONMENT=development" # e.g. force CodeIgniter into dev mode for local work
  key_generation:                 # application key generation (optional)
    env_key: APP_KEY
    command: key:generate
    fallback_prefix: "base64:"

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
      composer: symfony/messenger
    conflicts_with:               # workers to stop before starting (optional)
      - other-worker
    proxy:                        # nginx proxy config (optional)
      path: /ws
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

# Application log files shown in the UI "App Logs" tab
logs:
  - path: "var/log/*.log"             # glob relative to project root
    format: raw                       # monolog | raw (plain text, default)

# Extra nginx config spliced into the site's server block (optional)
nginx:
  snippet: |
    location /static/ {
      try_files $uri $uri/ /static.php?$args;
    }
```

## Site placeholders

The `{{site}}`, `{{site_testing}}`, `{{bucket}}`, `{{domain}}`, `{{scheme}}`, and `{{<service>_version}}` placeholders listed above are expanded in three places: the `env.services` vars, every `setup:` command, and every `commands:` entry. They resolve against the registered site the command runs for. A git worktree is not a registered site, so a command run against one resolves `{{site}}` but leaves `{{domain}}` and `{{scheme}}` alone.

This is what lets a framework whose bootstrap needs to know where the site lives declare that step as data. Magento 2.4 removed its web installer, so a fresh store is installed with `bin/magento setup:install --base-url=… --db-name=…`; the definition can now express exactly that. A step that creates schema should carry `default: false` so it is opt-in rather than running on every `lerd setup`.

A placeholder whose value is empty, or one lerd does not recognise, is left in the command verbatim rather than being replaced with an empty string, so a half-resolved context can never quietly produce `--base-url=://`.

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

A snippet that passes requests to PHP should assign `{{fpm}}` to a variable first, `set $myfpm "{{fpm}}";` then `fastcgi_pass $myfpm:9000;`, exactly as the generated vhost does. nginx resolves a literal upstream name once when the config loads and caches it for the life of the process, so a container that comes back on a new address is never picked up.

The snippet must have balanced braces, since an unbalanced one would close the enclosing `server` block and start declaring its own. Balance alone is not enough, because a `}` followed by a `server {` still balances, so the values substituted into the placeholders are rejected too if they contain `{`, `}`, `;`, `#`, or a newline. A snippet failing either check is dropped and the site renders without it, rather than risking an nginx config that fails to load for every site.

Snippets are only honoured from the framework store and from user-defined definitions: an embedded `framework_def` in a project's `.lerd.yaml` is untrusted input, so its `nginx` block is stripped, the same way its host workers and command-type doctor checks are.

This is distinct from the per-site [nginx override](nginx-overrides.md) in `custom.d/`, which you author yourself and which is included at the *end* of the server block. Use the framework snippet for what every site of that framework needs; use the override for what one site needs.

## Framework detection

Framework detection only runs during `lerd link`, `lerd init`, `lerd env`, `lerd setup`, and `lerd park`. All other commands read the saved framework from the site registry.

Detection order:

1. **Laravel** (built-in): checks for `artisan` file or `laravel/framework` in `composer.json`
2. **Local definitions**: iterates user-defined and store-installed YAML files, applying detection rules
3. **Framework store** (interactive): checks the store index and prompts to install, or fetches silently when `.lerd.yaml` specifies the framework name

The first match wins. Detection rules are OR-based, any single matching rule is enough.

## Document root detection

If no framework matches and no `--public-dir` is specified, lerd tries these candidate directories in order, accepting the first that contains an `index.php`:

`public` → `web` → `webroot` → `pub` → `www` → `htdocs` → `.` (project root)

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
