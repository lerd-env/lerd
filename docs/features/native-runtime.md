# Native runtime

On macOS your project lives on the host and is mounted into the Podman VM, so a containerised PHP crosses that boundary for every file it reads. The native runtime removes the boundary: PHP-FPM, the CLI, composer and the workers all run directly on the host, and so does nginx, while the services stay in containers.

Measured on an M-series Mac against a Laravel 11 app with Horizon, Filament and Cashier, both runtimes with the framework caches warm:

|  | Container | Native |
| --- | --- | --- |
| Warm request | 61 ms | 55 ms |
| 100 requests, 10 concurrent | 62 rps | 238 rps |

Single requests are close because a warmed framework touches few files. The gap opens under concurrency, which is where the mount contention shows. The underlying difference, same PHP process, 2000 files: `stat` costs 108 ms in a container against 4 ms on the host, and `include` 320 ms against 31 ms.

::: tip macOS only
The native runtime is macOS only. Container mode remains the default; switching back is a single command.
:::

## Switching

```bash
lerd php:runtime native      # move PHP onto the host
lerd php:runtime container   # back to the containers
lerd php:runtime             # show the current runtime
```

Or from the dashboard: **System → PHP runtime**. Either way, a move to native offers to remove the PHP-FPM images once the switch has succeeded, which reclaims the disk they hold; they are built again if you ever switch back, every missing version at once. In a terminal it is a question that defaults to no. With no terminal to ask in, the command prints a hint instead, and `lerd php:runtime native --remove-images` removes them without asking, on an install that is already native too. The offer is only made in that direction, since on the container runtime those images are what serves every site.

`lerd cleanup` keeps the image of every installed PHP version, so this offer is the way to get that disk back. Most of it sits in the base images they were built on, so removing them also sweeps those bases straight away. With automatic cleanup turned off (`lerd cleanup auto off`) that sweep is skipped, and the bases keep their layers until you run `lerd cleanup`. On macOS the space stays inside the Podman Machine disk until `lerd machine reclaim` returns it, and the command says so after removing them.

The setting is install-wide, not per site. The FPM container is shared by every site on a PHP version, so sites cannot be moved one at a time.

Switching rewrites each site's `.env` so its services point at the loopback addresses and published ports a host process can reach, regenerates the vhosts, drops the framework config caches so they re-read those files, and restarts the workers. Going native also stops the shared FPM containers, which have nothing left to serve. Going back reverses all of it. The sites about to be rewritten are named first, and a site whose database is [not one of lerd's](env-setup.md#runs-lerd-starts-itself) keeps that connection.

Each PHP version gets one native listener, the same way the FPM container is shared. It is supervised by launchd as `lerd-native-php<version>` and writes to `~/Library/Logs/lerd/`.

When a listener's definition changes, on an upgrade or a reinstall, lerd waits for launchd to let go of the old job before loading the new one, since loading it while the old one is still exiting leaves the version with no listener and its sites on 502. `lerd uninstall` stops every listener before it removes their launchd files, so none is left running with nothing to manage it.

The pools run on demand: a version nothing has asked for in a minute drops to zero worker processes instead of holding several resident, which matters on a laptop where sites sit untouched for hours. The first request after that pays a fork, not a warm-up, because the master process owns the opcache shared memory and keeps it across idle periods.

nginx keeps its connections to a native pool open and reuses them, up to eight idle per nginx worker, rather than opening one for every request. Each connection crosses the Podman machine's network to reach the host, and under load a connection opened per request was now and then dropped there and answered with a 504 after five seconds. A kept connection holds a PHP worker while it stays open, so a pool allows up to 128 of them; on demand it still forks only for a connection that arrives, and nginx lets an idle one go after ten seconds.

FrankenPHP sites, custom containers and host-proxy sites are unaffected either way: they never used the shared FPM container. That goes for their commands too. In a FrankenPHP, custom-FPM or PHP-carrying custom container site, `php`, `composer`, `artisan` and the other shims run in the site's own container, where its `.env` hosts resolve, and lerd mounts its composer, composer's home and the project's worktrees checked out beside it into FrankenPHP containers for that. A worktree added after the container started is mounted the first time a command runs in it, which restarts the container once, and a removed one is dropped from the container's definition so its next start doesn't fail on a missing folder. A host-proxy site, a custom container without PHP, and any folder that is not a site keep running PHP on your machine.

Moving one site between the two moves where its PHP runs, whether that is `lerd runtime frankenphp` or `lerd runtime fpm`, the MCP `site_runtime` tool, or lerd dropping a FrankenPHP site back to FPM for a PHP version FrankenPHP has no image for. Each of those reruns `lerd env` and the site's `.env` points at the addresses the new side can reach. A FrankenPHP site in worker mode restarts its container afterwards, since Octane reads `.env` once when it boots.

## nginx on the host

Moving PHP to the host takes nginx with it, so a request no longer crosses the Podman machine twice, in to nginx and back out to PHP-FPM. `lerd php:runtime native` installs Homebrew's nginx when it is missing and runs it under launchd as `lerd-nginx`, on lerd's own config, listening on the loopback ports the port forward relays 80 and 443 to, with the Mac's own paths for certificates and your overrides. Switching back brings the nginx container back the same way. Both directions say which way nginx moved.

nginx stays in its container when Homebrew is not installed, when a site runs in a container lerd cannot publish for it (a custom container or a custom FPM), when lerd's data folder has a space in its path, or when `config.yaml` sets `nginx.runtime: container`. Where nginx runs changes only on `lerd php:runtime` and `lerd install`: while it runs on the host, linking a custom container or custom FPM site is refused with a hint to run `lerd php:runtime container` first. When native PHP would take nginx to the host and something keeps it in its container, `lerd php:runtime`, `lerd install`, `lerd status` and `lerd doctor` name the reason. The port forward the host's nginx needs asks for your password, so run `lerd install` in a terminal for it. A LAN exposure toggle rewrites the host's nginx configs and reloads it, so a port other than 80 and 443 follows the new bind at once.

A FrankenPHP site keeps its own container and is served by the host's nginx too. lerd publishes the container on a loopback port, claimed once per site from 18000 and kept in the site registry, and the site's vhost proxies to `127.0.0.1` on that port while nginx runs on the host, by container name while nginx runs in its container. Moving nginx to the host, and resuming a paused FrankenPHP site there, restarts the container when it does not publish its port yet. A hand-edited `_default.conf` is kept as it is, written in the host's form while nginx runs there and restored to the original when nginx moves back. Service domains proxy to the loopback port each service publishes. Your per-site overrides in `custom.d` are loaded as they are and never rewritten, so one that names a container, such as `proxy_pass http://lerd-redis:6379`, or `host.containers.internal`, needs `127.0.0.1` and the published port on the host instead.

While nginx runs on the host, Homebrew's nginx is listed under **System → Tools** with the version Homebrew would upgrade it to, and its update runs `brew upgrade nginx` and restarts lerd's nginx. Podman is listed there too, for its version only.

## Installing and updating

The builds are downloaded, not compiled here. Switching to the native runtime fetches whatever versions your sites run, verifying each against a published sha256, and `lerd use <version>` fetches one on demand:

```bash
lerd use 8.4                 # fetch the native build for 8.4 and make it the default
lerd php:update              # move every installed version to the newest patch
lerd php:update 8.4          # just this one
lerd php:list                # what is installed for the active runtime
```

Installing a version from the dashboard (**System → PHP → Install**) does the same: it fetches the native build and starts its listener, rather than building an image the native runtime would never use. Its list offers only the versions with a native build published for your Mac, so a new PHP release shows up there once its build is. `lerd start` fetches the default version and any version a site runs that is missing, the way the container runtime builds the default's image. If a build is republished under the same version, a download that fails its checksum against the cached pins re-reads them once before giving up.

Builds are published per PHP patch, so `8.4.24` and `8.4.25` are separate downloads and lerd pins one of them per minor. `lerd php:update` re-reads the pins past their cache, which matters because the passive check only looks once a day and the point of running it by hand is to find out now.

`php:update` follows the runtime, so it is one command either way: on the container runtime it rebuilds the FPM images from the newest base, which is what `lerd php:rebuild` does.

New patches arrive without a lerd release. The pins live in a manifest lerd reads at runtime, so a PHP released this week is offered to every install within a day of being built.

A patch is sometimes rebuilt without PHP itself changing, which is what happens when the engine-level extension beside it is fixed. The version is then the same on both sides and cannot say whether you hold that build, so each pin also carries the date it was published and lerd records it next to the binary. An update is offered when either the patch or that date moves, which means a rebuild reaches you the same way a new patch does.

The binaries land in `~/.local/share/lerd/bin` as `php-native-<version>`, with their loadable extensions in `~/.local/share/lerd/native-php/<version>/modules`. Extensions are kept per version because they are named for the extension rather than the build, and a module only loads into the PHP it was compiled against. An update replaces the binary and its extensions together and then restarts the pool, since a running pool holds both open and keeps serving what it started with.

**System → PHP** follows the runtime too. A version card here offers **Update build** and **Remove**, and nothing that names an image: there is no base to rebuild, no ports to map and no container to open a shell in. Update is `php:update` for that one version, downloading the published build and restarting its pool, and Remove takes the host build down and deletes its binaries, extensions and generated pool config, which is what it takes for the version to actually leave the list.

## What stays the same

`php:ini` still edits the same files: the native runtime reads the shared, per-version, mail and xdebug fragments through `PHP_INI_SCAN_DIR`, so a setting applies whichever runtime you are on. Workers get the same files, so a queued job sees exactly what `lerd php` does. Xdebug works, shipped as a loadable extension alongside the binary. `dump()` and `dd()` capture works too, with the bridge reading its assets from their host copies. nginx keeps serving and terminating TLS, so HTTPS, domains, worktrees and site groups are untouched. PHP trusts lerd's certificates the way the containers do: curl and the openssl stream wrapper read a bundle of the macOS root store plus lerd's CA, so a request from one site, or one of its workers, to another `.test` site verifies without turning TLS checks off.

## What is captured

`dump()` and `dd()` work: the debug bridge is plain PHP and reads its assets from their host copies. Xdebug and pcov ship as loadable extensions beside the binary, so `xdebug:on` and coverage runs behave as they always have, and SPX is compiled in, so the profiler works too.

The Debug window's query lens captures too. Those queries come from `lerd_devtools`, an engine-level extension the PHP image compiles in; the published builds ship it as a loadable object beside the binary, built against that exact PHP, and the runtime loads it automatically. A build that does not carry one leaves the lens empty, and the site doctor says so rather than letting it look broken.

## Known gaps

The extension set is fixed at build time, so `lerd php:ext add` has nothing to add to. The build carries the same set the image does, including intl, imagick, mongodb, redis, soap, xsl and SPX, so this bites only a project needing something outside it. The site doctor reports the drift before you hit it at runtime, and container mode remains the escape hatch.

Extensions that cannot be compiled into a static binary, Xdebug and pcov among them, ship as loadable `.so` files beside it instead. That is a packaging detail rather than a limitation: both load and behave normally.

`lerd php:ext` and `lerd php:pkg` refuse here, and `lerd shell` says there is no container to enter rather than starting one. `lerd doctor` follows the runtime as well: it reports the builds installed on the host instead of the FPM images, since there are none to inspect and the rebuild it would otherwise name refuses here.

A version with no published native build cannot be installed at all, and the native runtime needs PHP 8.1 or newer. Switching names the sites standing in the way rather than failing one at a time.
