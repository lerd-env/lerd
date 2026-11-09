---
title: "PHP versions per project"
description: "Run a different PHP version for every project with Lerd on Linux and macOS: set the global version, pin one per project with .php-version, and keep PHP 7.4 to 8.5 side by side."
---

# PHP versions per project

Lerd keeps any number of PHP versions installed side by side and picks one for each project. `lerd use 8.4` sets the global default, and `lerd isolate 8.3` run inside a project pins that project to 8.3 by writing `.php-version`. With nothing pinned, the version comes from the `require.php` constraint in `composer.json`. None of this touches a system PHP, so switching works the same on Ubuntu, Fedora, Arch and macOS.

Related: [PHP extensions](/usage/php-extensions), [php.ini overrides](/usage/php-ini), [Xdebug](/usage/xdebug), [Composer](/usage/composer).

## Commands

| Command | Description |
|---|---|
| `lerd use <version>` | Set the global PHP version and build the FPM image if needed |
| `lerd isolate <version>` | Pin PHP version for cwd: writes `.php-version` and updates `.lerd.yaml` if it exists, then re-links. Builds the version's image first when the machine does not have it yet, so the site is never pointed at a runtime that cannot start. Running workers move to the new version with it. Refuses a version the framework, the project's `composer.json`, or its installed dependencies rule out; `--force` pins it anyway |
| `lerd php:list` | List all installed PHP-FPM versions |
| `lerd php:rebuild [version] [--local]` | Force-rebuild PHP-FPM images, or add a version this machine does not have yet; `--local` builds from source instead of pulling a base |
| `lerd php:update [version]` | Update PHP to the newest published patch. On the native runtime this downloads the new build and restarts the pools; on the container runtime it rebuilds the images from the newest base |
| `lerd fetch [version...] [--local]` | Pull pre-built PHP FPM base images from ghcr.io; `--local` builds from source instead |

If no version is given, the version is resolved from the current directory (`.php-version` or `composer.json`, falling back to the global default).

Versions are written as `major.minor`, but common spellings are accepted everywhere a version is typed: `php8.4`, `84` and `8.4.7` all normalize to `8.4`. Anything that does not resolve to a supported version is rejected up front, so a typo can never end up as the stored default and break image names.

Inside a linked site, the commands that run PHP in a container (`lerd php`, `lerd composer`, `lerd console`, `lerd shell`) use the version the site is registered on, which is the version its FPM container serves. That matters when a framework clamps the version at link time: a Laravel 13 project pinning `.php-version` to 8.1 is linked on 8.5, because Laravel 13 supports 8.3 to 8.5, and composer then runs on 8.5 too rather than resolving 8.1 from the file and quietly using a different PHP than the site itself.

A git worktree resolves ahead of the site it belongs to. A worktree inherits its parent site's version until you pin one with `lerd isolate` from inside the checkout, and from then on the whole toolchain follows that pin: the worktree's own vhost, `lerd php`, `lerd composer`, and everything else that runs PHP in a container. This holds wherever the checkout lives, including inside the parent site's own directory, so a worktree on 8.3 under a site on 8.5 runs composer on 8.3 rather than picking up the parent's version.

When a command needs a version that is not installed and you decline the install, lerd offers to switch to one you already have and pins the choice. Inside a worktree that pin is written on the checkout itself, so the switch travels with the branch and the parent site keeps the version it was on.

So that the project agrees with what actually runs, `lerd link` pins the resolved version into `.php-version`, the same file `lerd isolate` and the dashboard's PHP dropdown write. A version outside the framework's range is clamped rather than accepted, so the file, the site registry and the container can never drift apart, and when the clamp moves a version `php_version` asked for, the link says so rather than reporting the version it landed on as the choice. Sites with no lerd-managed PHP version (host-proxy, and custom containers whose version comes from their Containerfile) are left untouched.

`lerd isolate` answers differently, because there a human named the version: a request the framework range or the project's own `composer.json` rules out is refused and nothing is written, so a file the project commits is never edited to agree with a version its owner did not choose. The refusal names what the version had to satisfy and the closest installed version that does, and `--force` pins it regardless.

What the installed dependencies require counts too, and it can be stricter than the project's own manifest. Laravel 13 declares `"php": "^8.3"` and then resolves Symfony 8 components that each require `>=8.4.1`, so the app cannot boot on 8.3 even though its manifest allows it. Composer works this out when it installs and records it in `vendor/composer/platform_check.php`; lerd reads that floor alongside the other two, which is why a fresh Laravel 13 site refuses `lerd isolate 8.3` rather than accepting it and answering 500 on the first request. A project with nothing installed yet has no such file, and composer omits the check when no package constrains PHP, so in both cases only the framework range and the manifest apply.

A framework definition describes the framework, `composer.json` describes the application that has to boot, and both apply. Where the two cannot both be met, the project wins: an app served by a definition whose cap sits below what its own dependencies require would otherwise be linked onto a version that fails at the first request.

## Version resolution

When serving a request, Lerd picks the PHP version for a project in this order:

1. `.lerd.yaml` in the project root: `php_version` field (explicit lerd override)
2. `.php-version` file in the project root (plain text, e.g. `8.2`)
3. `composer.json`: `require.php` constraint, resolved to the best installed version (e.g. `^8.4` with PHP 8.4 and 8.5 installed resolves to `8.5`)
4. Global default in `~/.config/lerd/config.yaml`

Once a project is linked, that order is what lerd detects with, not what it runs. Linking clamps the detected version to the range the framework supports and stores the result in the site registry, and everything scoped to the site afterwards, the vhost, `lerd console`, `lerd php`, tinker, the workers and `lerd logs`, uses the stored version. So a project pinned to a version its framework does not support is served by the clamped one, and the pin only takes effect once it falls inside the range. A worktree inherits its parent site's version unless its own `.lerd.yaml` overrides it.

When `.php-version` changes on disk, the lerd watcher automatically updates the site registry and regenerates the nginx vhost, no manual reload needed.

To pin a project permanently:

```bash
cd ~/Lerd/my-app
lerd isolate 8.5
```

This writes `.php-version: 8.5` (so CLI `php`, asdf, and other tools see the right version) and, when `.lerd.yaml` already exists in the project, also updates its `php_version` field to keep lerd's priority-1 override in sync. The site is re-linked automatically so nginx picks up the new version immediately. If 8.5 is outside what the framework or the project supports, the pin is refused with both files left as they were, and `lerd isolate 8.5 --force` applies it anyway.

The UI PHP version selector and the MCP `site` tool's `php` action follow the same rules; they always write both files when applicable.

The composer constraint is matched against all installed PHP versions using full semver rules (`^`, `~`, `>=`, `<`, `||`, `*`). The highest installed version that satisfies the constraint wins. If no installed version matches, the literal minimum from the constraint is used (and the FPM will be built on first use).

::: tip Overriding a `composer.json` constraint
If `composer.json` requires `^8.3` but you need to run the project on a specific version, `lerd isolate 8.5` is the right tool. It writes `.php-version` which takes priority over the composer constraint. Running `lerd use 8.5` alone won't help; that only sets the global fallback, which loses to the composer constraint.
:::

To change the global default (applies to all projects that don't have a per-project pin):

```bash
lerd use 8.5
```

## Framework PHP ranges

Each framework version declares the PHP range it supports (for example Laravel 11 supports PHP 8.2 to 8.4). Lerd clamps the resolved PHP version into that range so a site never runs on a version its framework can't boot, and the PHP version picker in the dashboard and the TUI shows out-of-range versions as disabled rather than hiding them, so the constraint is visible.

The range comes from the framework definition that matches your project. Lerd detects the framework major version from `composer.json` (for Laravel, the `laravel/framework` constraint) and loads that version's definition.

### Legacy projects

When a project's framework version predates every definition lerd ships, lerd serves it with the **lowest** available definition instead of the latest. A Laravel 6 project, for instance, is served by the Laravel 10 definition rather than Laravel 13.

In that case the version is a best-effort guess: the definition targets a newer framework than your project, so its PHP range is **not** enforced. PHP clamping is relaxed and every installed version stays selectable, which lets a legacy Laravel 6 app keep running on PHP 7.4 even though the Laravel 10 definition asks for 8.1 and up. Pin the version you want with `lerd isolate 7.4`.

Newer-than-shipped versions fall back to the latest definition as before, with its range enforced normally.

## FPM lifecycle

Lerd automatically manages which PHP-FPM containers are running based on which versions are actually needed by your sites.

**`lerd start`**: only starts FPM containers for versions referenced by at least one site (active or paused). Unused versions are left stopped.

**Auto-stop**: when you unlink a site, lerd checks every installed PHP version. If no remaining active (non-ignored, non-paused) site uses a version, its FPM container is stopped. The version itself stays installed; the container is just not running.

**Paused sites count**: a site that is paused still counts as using its PHP version, so that version's FPM container is not stopped. When the site is resumed, FPM is guaranteed to be running.

**Auto-start**: FPM is started automatically when you link a site (`lerd link`, `lerd park`, `lerd isolate`) or change the global default (`lerd use`). When unpausing a site, lerd also ensures the required FPM container is running before restoring the nginx vhost.

**Build on first use**: when a link lands on a PHP version this machine has never built (an older framework clamps to a version below the ones you have, say), `lerd link` builds that version's image before starting it, so the site serves rather than answering 502. The build streams its progress as a link step. If the build cannot run (an unattended `lerd park` sweep withholds builds) or fails, the site is still registered and lerd names the one command that finishes the job, `lerd php:rebuild <version>`.

**Adding a version by hand**: `lerd php:rebuild <version>` names a version this machine has never installed and installs it, registering the unit before it builds so the version shows up in `lerd php:list` either way. That is the command every surface points at when one is missing, so a pin written ahead of the install (`lerd isolate 8.4` in a directory that is not a site yet) has a way to be satisfied without waiting for a link.

**A rebuild that fails** says so: `lerd php:rebuild` (and `lerd php:update` on the container runtime) prints each failed version, still restarts the ones that built, and exits non-zero naming what was not rebuilt. The image is not marked current, so the next `lerd update` or `lerd install` builds it again. A version whose unit kept failing while its image was missing is cleared before the restart, so it comes up as soon as the build lands.

**Manual control**: unused PHP versions (no active sites) can be started and stopped manually from the dashboard (System > PHP > Start / Stop). From the CLI:

```bash
systemctl --user start  lerd-php84-fpm
systemctl --user stop   lerd-php84-fpm
```

**`lerd status`**: stopped FPM containers for unused versions are reported as a warning, not an error.

## Pre-built images

lerd ships pre-built PHP-FPM base images on ghcr.io for all supported versions (7.4 and 8.0–8.6), covering both `amd64` and `arm64`. When you run `lerd fetch` or `lerd php:rebuild`, lerd pulls the matching base image and layers just your mkcert CA certificate on top, bringing first-time build time from ~5 minutes down to ~30 seconds.

The base image tag is derived from the embedded Containerfile, so lerd always pulls the exact image that matches the version of lerd you have installed. If the pull fails (no internet, image not yet published) lerd falls back to a full local build transparently.

The images are public, so no ghcr.io login is required. lerd pulls them anonymously even if you are already logged into ghcr.io, to avoid authentication errors from expired or unrelated credentials.

`lerd start` checks all required images before starting containers. If any are missing (e.g. after `podman image rm`), it rebuilds or pulls them automatically using the same parallel spinner UI, so containers always start against a valid image.

To build entirely from source instead:

```bash
lerd fetch --local
lerd fetch --local 8.5
lerd php:rebuild --local
```

The image's PECL extensions (`redis`, `imagick`, `igbinary`, `mongodb`, `pcov`, `xdebug`) are installed with [PIE](https://github.com/php/pie), the replacement for the deprecated PECL, which builds each one from its GitHub source. pecl.php.net is only the fallback when PIE cannot install one, and the only source on PHP 7.4 and 8.0, which PIE does not run on.

`lerd fetch` prepares the image; it does not install the version. A version whose image is ready but that has no PHP-FPM service behind it yet is still missing as far as `lerd php:list` and `lerd new` are concerned, and the fetch says so:

```
 ✓ all requested PHP images ready
 → PHP 7.4 has an image but no runtime yet — run 'lerd php:rebuild 7.4' to install it
```

Linking a project that needs the version installs it on demand, so this only comes up when you fetch ahead of time.

### When the base image is refreshed

The base image tag is a hash of the recipe, so an upstream `php:X.Y-fpm-alpine` refresh, including a security patch Alpine has already shipped, republishes the same tag with new content. Nothing about your machine changes when that happens, so lerd records the digest of the base each image was built from and compares it against what the registry serves now, a manifest lookup with no pull. When they differ, the version is flagged as having an update available.

You see it in four places. The dashboard's System health card carries a **PHP · N updates** pill that opens **Update all**, and marks the version with an up arrow. **System → PHP** marks the version card with an up arrow and offers "Rebuild on the new base" as its first action, which streams the rebuild the same way an install does. While any version has an update the page header also has **Update all**: it lists each outdated version with a switch, all on to begin with, rebuilds the ones still switched on together, the way `lerd php:rebuild` does, with each one's latest build line on its row, restarts them in turn, and reports the failures by version without stopping the rest. The same menu has **Check for updates**, which bypasses the cached digest and asks the registry right now. And `lerd doctor` reports the version as a warning with `lerd php:rebuild <version>` as the fix, so `lerd doctor --fix` picks it up too. If push notifications are on, a version that becomes stale while the dashboard is open announces itself like a service update does.

A rebuild ends by listing the Alpine packages it changed, `brotli-libs 1.2.0-r1 → 1.2.1-r0` for instance, so you can see the fixes it brought in. The base image workflow writes the same changelog to its run summary whenever it republishes a tag.

The check is cached for six hours and never runs on the dashboard's critical path, so an offline machine stays quiet rather than reporting a false update. A version whose image was built locally (`--local`) has no recorded base and is never flagged: there is no published image behind it to compare against.

A security fix Alpine ships before upstream refreshes its image triggers the same republish. A daily scan checks the published bases for high or critical vulnerabilities with a fix out, and the rebuild runs `apk upgrade`, so the patched packages land under the same tag and the version shows as having an update. PHP 7.4 and 8.0 are left out, since their Alpine branches no longer get fixes.

On the publishing side, an upstream refresh rebuilds the hash tag main computes plus the ones the last two stable releases resolve to, each from its own git checkout, with `apk upgrade` added where that release's recipe lacks it so Alpine's security fixes reach it too. Staying a release or two behind still gets you the patched base without updating lerd first.

## Legacy PHP versions

PHP 7.4 and 8.0 are available as a frozen legacy tier for old projects (Laravel 6–8 on 7.4, Laravel 8–9 on 8.0). They build from the same Alpine-based recipe as the current versions, including ICU full locale data, but with a few caveats:

- They are end-of-life upstream and get no security patches. Use them only for local work on legacy apps.
- Xdebug is pinned to the last release supporting that PHP line (3.1.6 for 7.4, 3.3.2 for 8.0).
- The `mongodb` extension is unavailable (it requires PHP 8.1+), and so is `random` (a PHP core extension only from 8.2); everything else in the standard bundle is present.
- The base image is Alpine 3.16, so the bundled Node.js is 16.x.

Use them like any other version:

```bash
lerd use 7.4
lerd isolate 8.0
lerd fetch 7.4 8.0
```

## PHP 8.6

PHP 8.6 is a released version like the others: `lerd fetch` builds it and it is offered everywhere a version is picked. Two parts of its ecosystem have not caught up yet:

- FrankenPHP publishes no image for it, so an 8.6 site is served by FPM. Switching a site to the FrankenPHP runtime on 8.6 is refused rather than quietly run on a different PHP, and a site already on FrankenPHP falls back to FPM when it moves to 8.6.
- PHP 8.6 removed PEAR, so `pecl` is gone from the upstream image and lerd installs PECL extensions from their release tarballs instead. `redis`, `imagick` and `mongodb` build; `igbinary`, `pcov` and `xdebug` do not compile against 8.6 yet, so they are absent from the image and lerd does not advertise them on that version. They come back as their upstreams catch up. The profiler needs `php-spx`, which is in the same position, so it is unavailable on 8.6.
