---
title: "PHP"
description: "How Lerd runs PHP on Linux and macOS: per-project PHP versions, extensions, php.ini overrides, Xdebug and Composer, plus the php and composer shims and PHP shells."
---

# PHP

Lerd runs PHP for you, in containers or natively on macOS, so you never install PHP on the host. The `php` and `composer` commands on your PATH route to the right version for the project you are in.

- [PHP versions per project](/usage/php-versions): switch the global version, pin one per project, and keep 7.4 to 8.5 side by side.
- [PHP extensions](/usage/php-extensions): what ships in the image, adding any other extension or system package, locales and ODBC.
- [php.ini overrides](/usage/php-ini): `memory_limit`, upload sizes and other settings, per version or for all of them.
- [Xdebug](/usage/xdebug): step debugging from PhpStorm or VS Code, for web requests, workers and CLI scripts.
- [Composer](/usage/composer): the pinned `composer.phar`, private repositories over SSH, and global packages.

The rest of this page covers the shims, PHP shells, custom images and passing environment from outside.

<script setup>
// Sections of this page moved to their own pages; old #anchors follow them there.
const MOVED = {"xdebug": "xdebug", "php-ini-settings": "php-ini", "version-resolution": "php-versions", "framework-php-ranges": "php-versions", "legacy-projects": "php-versions", "fpm-lifecycle": "php-versions", "pre-built-images": "php-versions", "when-the-base-image-is-refreshed": "php-versions", "legacy-php-versions": "php-versions", "prerelease-php-versions": "php-versions", "turning-it-on": "php-extensions", "turn-off-break-at-first-line": "xdebug", "picking-a-mode": "xdebug", "on-demand-debugging-workers-and-cli": "xdebug", "per-version-php-ini": "php-ini", "shared-settings-across-versions": "php-ini", "custom-extensions": "php-extensions", "what-each-version-actually-loaded": "php-extensions", "locales-and-internationalisation": "php-extensions", "composer-json-detection": "php-extensions", "odbc-databases": "php-extensions", "registering-a-driver": "php-extensions", "where-a-driver-may-live": "php-extensions", "drivers-that-need-more-than-gcompat": "php-extensions", "which-composer-runs": "composer", "which-php-version-it-uses": "composer", "private-repositories-over-ssh": "composer", "running-a-package-you-have-not-installed": "composer", "tools-in-vendor-bin": "composer", "composer-install-on-a-new-project": "composer", "extension-requirements": "composer"}
if (typeof window !== 'undefined') {
  const page = MOVED[decodeURIComponent(location.hash.slice(1))]
  if (page) location.replace(`/usage/${page}${location.hash}`)
}
</script>

## Commands

| Command | Description |
|---|---|
| `lerd php:bun install [version]` | Install a musl bun inside the PHP-FPM container, into a persistent volume |
| `lerd php:bun remove` | Remove the in-container bun and clear its shared persistent volume |
| `lerd php:bun update [version]` | Update the container's bun in place (`bun upgrade`) |
| `lerd php:bun version [version]` | Show the bun version installed in the container |
| `lerd php:ports add <host:container...> [--php version]` | Publish a host port on the version's shell container; a bare number publishes the same port straight through, and a busy host port shifts to the next free one |
| `lerd php:ports remove <host...> [--php version]` | Unpublish a host port from the version's shell container |
| `lerd php:ports list [--php version]` | List the extra host ports published for a PHP version |
| `lerd pest:browser install [version]` | Set up in-container Pest browser testing (musl chromium, Xvfb, Playwright shim); see [browser testing](browser-testing#pest-browser-testing-playwright) |
| `lerd pest:browser remove [version]` | Remove chromium and Xvfb from the FPM image and disable Pest browser testing |
| `lerd pest:browser doctor [version]` | Diagnose the Pest browser testing setup for a PHP version |

---

## Usage

`lerd install` places shims for `php` and `composer` in `~/.local/share/lerd/bin/`, which is added to your `PATH`. You use them exactly as you normally would, lerd routes them through the correct PHP-FPM container version automatically:

```bash
php artisan migrate
composer install
```

Because the `php` shim runs inside the PHP-FPM container, `php artisan`, `lerd artisan`, and the MCP `exec` tool's `artisan` action are all equivalent; they all execute inside the same container with the same PHP version and extensions. Use whichever form you prefer.

Prefer typing `lerd php` explicitly and keeping `php` pointed at a host install? Run `lerd path:disable`: it removes lerd's shims dir from your shell PATH and keeps installs and updates from re-adding it, while every `lerd …` command works unchanged (child processes lerd spawns still resolve the shims internally). `lerd path:enable` reverses it. One thing to know either way: the shimmed `php` runs inside the container, so a PHP script that `exec()`s host tools sees the container's PATH, not your shell's — with the shim disabled, a host `php` behaves like any other host process.

The PATH line goes into your shell rc once, at install, so a PHP that lands below it later takes `php` back and console commands start failing on the service hostnames in your `.env` while the browser stays fine. `lerd doctor` checks for it: the Configuration section reports `php on PATH` and names the binary in front when it is not lerd's.

### Shortcuts and `vendor/bin` fallback

For common workflows there are a few built-in shortcuts:

- `lerd a [args...]`: short alias for `lerd artisan` (also `lerd console`)
- `lerd test [args...]`: runs `lerd artisan test`

In addition, any composer-installed binary in the project's `vendor/bin` directory is callable directly as `lerd <name>`. For example, with the usual Laravel dev tooling installed:

```bash
lerd pest
lerd pint
lerd phpstan analyse
lerd rector process
```

These run inside the project's PHP-FPM container with the project's working directory mounted, so configuration files (`pest.xml`, `pint.json`, `phpstan.neon`, etc.) are picked up automatically. Real lerd commands always take precedence; if you have a `vendor/bin/composer`, `lerd composer` still resolves to the built-in command.

### IDE quality tools

An IDE runs PHPStan, Psalm, PHP CS Fixer, PHP_CodeSniffer and Rector against a copy of the buffer you are editing rather than against the file in the project tree, and it writes that copy into its own temp directory, usually somewhere under `/tmp`. Nothing under an ephemeral system tree is mounted into the PHP container, so a tool pointed at one of those copies would report a file that does not exist.

The `php` shim handles it. Any absolute path passed as an argument to the tool that the container cannot reach is copied into a scratch directory it can, the argument is rewritten to the copy, and the tool's output is rewritten back so every diagnostic still names the path your IDE passed in and lands on the right line of the right file. Tools that fix in place rather than only report, `php-cs-fixer fix` and `phpcbf`, get their changes copied back over the original before the command returns. Nothing needs configuring: point the IDE's PHP interpreter at `~/.local/share/lerd/bin/php` and the quality tools work as they would against a host PHP.

The copy is what makes it work, so there is a ceiling on it. A path larger than 64 MB is refused with a message rather than duplicated, because a data file that size is not IDE scratch and wants a real mount, either by parking the directory it lives in or by listing it under `mounts:` in `~/.config/lerd/config.yaml`.

### Terminal colours

Commands that run in a container get the colour capability of the terminal you started them from. `lerd php`, `lerd composer`, `lerd console`, `lerd shell` and the shell you open from the TUI forward `COLORTERM`, so Symfony Console, laravel/prompts, Pest, the Filament CLI and starship pick the same colour mode inside the container as the same tools do against a host PHP. Without it, podman hands the container a sixteen-colour terminal and anything styled with hex or 256 colours is flattened to the nearest basic colour.

`NO_COLOR` and `FORCE_COLOR` travel with it, so a preference set in your shell holds on both sides of the boundary. `TERM` is deliberately not forwarded: the image only carries terminfo for the xterm family, and a terminal that names itself `foot` or `wezterm` would end up with no colour at all. A terminal that advertises more than sixteen colours gets `TERM=xterm-256color` instead of its own value, which keeps the 256-colour path reachable too. `lerd tinker` is the exception and still runs uncoloured, since its output is captured and rendered by the UI rather than written to your terminal.

### When the command is cut short

`podman exec` leaves a command running in the container when whatever started it goes away, so a browser test stopped with Ctrl+C or by a timeout would otherwise keep its headless Chromium and its open pages for good, and those pages keep reconnecting to the site, which counts as activity and stops [idle-suspend](idle-suspend.md) from ever putting it to sleep. lerd stamps every `lerd php`, `lerd console`, `vendor/bin` and tinker run, and the MCP `artisan` tool, with a `LERD_EXEC` variable its children inherit. When lerd is interrupted, the dashboard gives up on a run, or an MCP session closes mid-call, everything carrying that run's stamp in the container is stopped with it. Whatever is still carrying it later, a child the command put in the background, one that Ctrl+C in a terminal did not reach, or the whole run when lerd was killed outright, is stopped by `lerd-watcher` within a minute.

---

## External environment providers

Secrets managers and environment tools run a command with an ephemeral environment, `ghostable env run --env local -- lerd php artisan migrate`, `op run -- lerd test`, `doppler run -- lerd artisan queue:work`. The variables reach the `lerd` process, but podman forwards no host environment into a container, so without a contract they stop at the boundary.

lerd forwards them on request, by name. The provider (or your shell) sets `LERD_PASSTHROUGH_ENV` to the names it injected, comma or space separated, glob patterns allowed:

```bash
LERD_PASSTHROUGH_ENV="APP_KEY,DB_PASSWORD,STRIPE_*" \
  doppler run -- lerd php artisan migrate
```

lerd expands the patterns against its own environment and hands podman a bare `--env NAME` for each match, so podman reads the value out of lerd's environment. No secret is written to `.env`, to `.lerd.yaml`, or into the argument list where `ps` would show it.

The semantics are the ones systemd's `PassEnvironment=`, ssh's `SendEnv` and sudo's `env_keep` already use. Names that match nothing are skipped silently. `PATH`, `HOME`, `COMPOSER_HOME` and anything starting with `LD_` or `LERD_` are never forwarded: lerd sets those for the container itself.

When your provider exports into the shell rather than wrapping a command (direnv, sops, a shell function), there is no wrapper to carry the variable. Declare the names in the project's `.lerd.yaml` instead:

```yaml
env_passthrough:
  - STRIPE_*
  - DB_PASSWORD
```

Both sources are merged. Forwarding applies to the commands you invoke yourself, `lerd php`, framework console commands, composer, tests, tinker, `lerd shell`, and the MCP exec tools. Web requests served by PHP-FPM and long-running workers are not covered; their environment belongs in the site's `.env`.

Passthrough hands each value to `podman exec`, and podman keeps the exec's process spec, values included, in its container storage under `~/.local/share/containers` for as long as the command runs. For a secrets manager whose point is that values never touch disk, use `env_provider` instead.

### Keeping secrets off disk: env_provider

Web requests and workers run in long-lived processes, so there is no wrapper to inject into. Declare a command that prints the secrets as dotenv lines, and lerd keeps them in memory for PHP to read:

```yaml
env_provider: infisical export --env=dev --path=/my-app --format=dotenv --silent
```

Any command that prints `KEY=value` lines to stdout works. Values may be quoted, and a quoted value may span several lines, so a PEM key comes through whole; an unquoted value ends at the line and drops a trailing ` # comment`. Some common providers:

```yaml
# Infisical
env_provider: infisical export --env=dev --path=/my-app --format=dotenv --silent

# Doppler
env_provider: doppler secrets download --no-file --format env --project my-app --config dev

# 1Password, filling op:// references in a committed template
env_provider: op inject -i .env.secrets.tpl

# Bitwarden Secrets Manager
env_provider: bws secret list <project-id> --output env

# sops, decrypting a committed encrypted dotenv file
env_provider: sops decrypt --output-type dotenv secrets.enc.env

# HashiCorp Vault, KV v2
env_provider: vault kv get -format=json secret/my-app | jq -r '.data.data | to_entries[] | "\(.key)=\(.value)"'

# AWS Secrets Manager, a secret holding a JSON object
env_provider: aws secretsmanager get-secret-value --secret-id my-app/dev --query SecretString --output text | jq -r 'to_entries[] | "\(.key)=\(.value)"'
```

The command runs through `sh -c`, so pipes work, which is also how to keep the provider to the secrets alone when it would otherwise export everything, connection settings included:

```yaml
env_provider: >-
  infisical export --env=dev --path=/my-app --format=dotenv --silent
  | grep -E '^(STRIPE_SECRET|MAILGUN_SECRET|AWS_SECRET_ACCESS_KEY)='
```

On `lerd env`, `lerd start` and, on Linux, the watcher's start on boot, lerd runs the command on the host from the project directory and writes its output to `$XDG_RUNTIME_DIR/lerd/env/<site>.env`. That directory is tmpfs, owner-only, and mounted read-only into the FPM containers at `/run/lerd/env`. The first run asks for approval, like any project-supplied host command, and an unapproved provider is skipped with a warning when there is no terminal to ask on. `lerd env --yes` approves it without the prompt and remembers the approval, for scripts and editors that run commands without a terminal.

lerd's always-on PHP prepend reads the file for the site named by `LERD_SITE`, which nginx sets for web requests and lerd sets for workers and the CLI commands it runs, and populates `getenv()`, `$_ENV` and `$_SERVER` before the framework boots. A variable the process already has wins, and frameworks that read the real environment before `.env`, Laravel and Symfony among them, see the provided values over the file's. Keep connection settings such as `DB_HOST` in `.env`: lerd writes them there, and a provider exporting your production or shared-dev hosts would otherwise override them.

On macOS the FPM containers run inside the Podman Machine VM, so the file lives in the VM's tmpfs at `/run/lerd/env` instead. lerd writes it over `podman machine ssh`, passing the values on stdin, so they go from the provider straight into the VM's memory without touching the Mac's disk or showing up in a process list. A machine restart empties it until the next `lerd start`. The [native runtime](../features/native-runtime.md) has no VM and no tmpfs on the host, so `env_provider` is not supported there yet and `lerd env` says so.

Re-run `lerd env` to pick up changed secrets; the next request sees them, no restart needed. A reboot empties the file. On Linux, systemd brings the sites back without `lerd start`, so the watcher runs every active site's provider when it starts; a provider that fails then, for example because its CLI keeps its login in a keyring that is still locked, is logged to the watcher's journal, and `lerd env` fills the file once the provider can run. A config cache puts the values back on disk (`php artisan config:cache` writes every resolved value into `bootstrap/cache/config.php`), so don't cache config locally.

Values never cross between sites. The file is picked by the per-request `LERD_SITE`, and lerd writes the site's directory and its worktrees into it; a script outside them loads nothing, so a shell `cd`'d into another project or a stale `LERD_SITE` cannot pick up the wrong site's secrets. Within a reused FPM worker, `$_ENV` and `$_SERVER` are rebuilt per request and PHP restores `putenv()` changes at request end. A worktree created after the last `lerd env` gets its secrets on the next one. `php -r` never runs the prepend, so use a script file there.

This guards against values bleeding through by accident, not against code that goes looking: every FPM container mounts the whole directory, so PHP on one site can still read another site's file directly. That is the same trust boundary as the shared `$HOME` mount every site already has.

---

## Debug bridge

Calls to `dump()` and `dd()` can be captured into the lerd dashboard, TUI, and MCP tools instead of (or alongside) the response. Enable with:

```bash
lerd dump on        # touch the sentinel; next request captures
lerd dump tail      # follow the live feed
lerd dump off       # remove the sentinel; subsequent requests are no-ops
```

Toggling never restarts FPM or its workers. The bridge auto-prepend file and its conf.d ini are always mounted into every FPM container; the on/off state lives in a runtime sentinel the bridge stats on each request. By default the bridge captures only and the HTTP response stays clean. Set `dumps.passthrough: true` in `config.yaml` to also keep the original `sf-dump` output in the response. See the [Dump viewer feature page](../features/dumps.md) for the wire format, the surfaces (per-site tab, System sidebar, antenna toggle), and tuning knobs.

---

## Custom image (Containerfile)

When `php:ext` and per-version ini tweaks are not enough and a single site needs its own bespoke image (an extra system toolchain, a patched binary, arbitrary build steps), you can give that PHP site its own `Containerfile.lerd`. lerd builds a per-site image and serves the site by fastcgi from a dedicated FPM container, instead of the shared `lerd-php<ver>-fpm` one. It is the same `container:` key used for [custom containers](../getting-started/containers.md), with one difference: **no port**. A `container:` block with a port is a reverse-proxied app; a `container:` block with no port on a PHP project is served by fastcgi from your image.

Your `Containerfile.lerd` must build `FROM` the lerd base image for the site's PHP version, so it keeps php-fpm, the bundled extensions, and the pool config. That `:local` tag is lerd-managed and rebuilt on updates, so the `FROM` stays valid:

```dockerfile
FROM lerd-php84-fpm:local
RUN apk add --no-cache htop vim
```

```yaml
# .lerd.yaml
domains:
  - myapp
container:
  containerfile: Containerfile.lerd
```

Then `lerd link`. lerd builds `lerd-custom-myapp:local`, runs a dedicated FPM container `lerd-cfpm-myapp`, and points nginx fastcgi at it. The per-site container reuses every lerd mount, so xdebug, dumps, the debug bridge, the profiler, and `lerd shell` all work exactly as on a normal PHP site, and `lerd php`, `artisan`, `composer`, `tinker`, queue/horizon workers, and the commands an AI assistant runs through the MCP server all run inside it. Toggling xdebug for that PHP version restarts the per-site container too.

The PHP version is fixed by the `FROM` line, not by `.php-version` or the dashboard, so the version selector is shown read-only for these sites. To change the version, edit the `FROM` and relink.

```bash
lerd rebuild        # rebuild the per-site image after editing Containerfile.lerd
lerd restart        # restart the container without rebuilding
```

::: info PHP projects only
A no-port `container:` is for PHP projects served by fastcgi. For a non-PHP app (Node, Python, Go) give the `container:` block a `port` so nginx reverse-proxies to it; see the [containers walkthrough](../getting-started/containers.md).
:::

::: warning One container per site
Each custom-image PHP site runs its own FPM container rather than sharing the per-version one, so it uses more memory in the Podman VM. Reach for it only when a site genuinely needs its own image; for adding an extension or a package to every site on a version, `php:ext` stays lighter.
:::

---

## PHP shell

`lerd shell` opens an interactive shell inside the PHP-FPM container for the current project. With a version it opens one in that version's shared container instead, from anywhere, which is what the dashboard's **Terminal** button on a PHP version runs:

```bash
lerd shell
lerd shell 8.4
```

The PHP version is resolved the same way as every other lerd command (`.php-version`, `composer.json`, global default). The shell's working directory is set to the project root. A version shell has no project behind it, so it opens on the container's own directory, and it starts a stopped container but never offers to build a missing one.

If the container is not running, lerd prints the platform-appropriate command (`launchctl kickstart` on macOS, `systemctl --user start` on Linux) to bring it back up rather than silently failing.

If the site is paused, any services referenced in `.env` (MySQL, Redis, etc.) are started automatically before the shell opens; the site itself stays paused.

### Shell environment

The lerd PHP-FPM image ships zsh with a self-contained config (starship prompt, persistent history, sensible defaults). When you run `lerd shell` or open a shell from the TUI, lerd execs zsh inside the container; for non-PHP service containers (Redis, MySQL, etc.) the fallback chain is `zsh > bash > sh` depending on what the upstream image provides.

The in-container shell is deliberately isolated from your host shell config. Every developer's `~/.zshrc` or `~/.config/fish` is different, and sourcing distro-specific paths or host-only binaries inside the alpine container cascades into noisy errors (missing oh-my-zsh, missing pacman, missing fastfetch, etc.). Rather than play whack-a-mole, lerd ships a clean, predictable shell environment that's identical across every machine and contributor.

What you get inside the container:

- **starship** as the default prompt, branch, dir, git status, all the usual.
- **eza**, **bat**, **fzf**, **zoxide** on `$PATH` for nicer file listing, paging, fuzzy-find, and `cd` history.
- **The site's Node** first on `$PATH`, the same version lerd runs for it on the host, fetched as a musl build on the first `lerd shell` (see [Node inside the PHP-FPM container](node.md#node-inside-the-php-fpm-container)).
- Shell history persisted under `~/.local/share/lerd/shell-state/php-<version>/zsh/history`, so commands survive container rebuilds.
- `HostName=` set to your host's hostname so the prompt reads `root@your-machine` instead of the auto-generated container id.

If you want extra packages in the image (additional CLI tools, language toolchains, etc.), use `lerd php:ext` for PHP extensions, or fork the Containerfile at `internal/podman/quadlets/lerd-php-fpm.Containerfile`.

For other tools and runtime libraries, `lerd php:pkg add <packages>` installs Alpine packages into the FPM image's runtime stage and rebuilds, for example `lerd php:pkg add htop vim`. The packages are saved in `~/.config/lerd/config.yaml` (under `php.packages`) and re-applied on every rebuild, so they survive `php:rebuild` and base image updates, exactly like custom extensions. Like extensions, one declared set applies to every PHP version. They are layered onto the shared image rather than baked into the published base, so they only affect your local build. A non-existent package name fails the rebuild and the change is reverted.

For [bun](https://bun.sh) specifically, run `lerd php:bun install` to drop a musl bun into the container's persistent `/root/.bun` volume (so a bare `bun` resolves inside the container without rebuilding the image). See [bun](node#bun) for the full host and container story.

### Reachable ports

By default a TCP port you open by hand inside `lerd shell` is not reachable at `localhost:PORT` on the host, because the PHP-FPM container has no published host ports of its own. Framework servers like [Reverb](queue-workers) or an in-container Vite worker are reachable, but only because a worker exposes them through the nginx proxy on the site's `.test` domain (`https://your-site.test/app`), not as a raw localhost port; Xdebug likewise connects outbound to your IDE rather than listening on a published port. When you instead want to run a process directly in the container (a Vite dev server, a websocket, an ad-hoc HTTP or debug listener) and hit it straight from a host browser or tool, publish the port on the version's shell container:

```bash
lerd php:ports add 5173        # localhost:5173 -> container 5173 (same port through)
lerd php:ports add 8080:80     # localhost:8080 -> container 80
lerd php:ports list
lerd php:ports remove 5173
```

The same list is available in the dashboard under **System > PHP > (version) > Ports**. If the host port is already taken (by a lerd service, another PHP version's list, or any other listener) lerd shifts it to the next free one and tells you where it landed, so an add never fails on a collision. Ports bind loopback by default and follow `lerd lan:expose` like every other lerd port. Changing the list restarts that version's FPM container, so PHP bounces for every site on that version.

This is a per-version pool, not per site. There is one shared FPM container per PHP version serving every site on it, so a published port maps to whichever single process binds it inside, and two sites wanting the same in-container port on the same version collide. It is a power-user escape hatch: for anything you can reach for a blessed path instead, prefer [host-proxy](host-proxy) (run the dev server on the host) or a worker with a proxy, which both scope cleanly to a single site.

---
