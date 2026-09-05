# Native PHP runtime (beta)

On macOS your project lives on the host and is mounted into the Podman VM, so a containerised PHP crosses that boundary for every file it reads. The native runtime removes the boundary: PHP-FPM, the CLI, composer and the workers all run directly on the host, while nginx and the services stay in containers.

Measured on an M-series Mac against a Laravel 11 app with Horizon, Filament and Cashier, both runtimes with the framework caches warm:

|  | Container | Native |
| --- | --- | --- |
| Warm request | 61 ms | 55 ms |
| 100 requests, 10 concurrent | 62 rps | 238 rps |

Single requests are close because a warmed framework touches few files. The gap opens under concurrency, which is where the mount contention shows. The underlying difference, same PHP process, 2000 files: `stat` costs 108 ms in a container against 4 ms on the host, and `include` 320 ms against 31 ms.

::: warning Beta
The native runtime is new and macOS only. Container mode is unchanged and remains the default; switching back is a single command.
:::

## Switching

```bash
lerd php:runtime native      # move PHP onto the host
lerd php:runtime container   # back to the containers
lerd php:runtime             # show the current runtime
```

Or from the dashboard: **System → PHP runtime**.

The setting is install-wide, not per site. The FPM container is shared by every site on a PHP version, so sites cannot be moved one at a time.

Switching rewrites each site's `.env` so its services point at the loopback addresses and published ports a host process can reach, regenerates the vhosts, drops the framework config caches so they re-read those files, and restarts the workers. Going native also stops the shared FPM containers, which have nothing left to serve. Going back reverses all of it.

Each PHP version gets one native listener, the same way the FPM container is shared. It is supervised by launchd as `lerd-native-php<version>` and writes to `~/Library/Logs/lerd/`.

The pools run on demand: a version nothing has asked for in a minute drops to zero worker processes instead of holding several resident, which matters on a laptop where sites sit untouched for hours. The first request after that pays a fork, not a warm-up, because the master process owns the opcache shared memory and keeps it across idle periods.

FrankenPHP sites, custom containers and host-proxy sites are unaffected either way: they never used the shared FPM container.

## Installing and updating

The builds are downloaded, not compiled here. Switching to the native runtime fetches whatever versions your sites run, verifying each against a published sha256, and `lerd use <version>` fetches one on demand:

```bash
lerd use 8.4                 # fetch the native build for 8.4 and make it the default
lerd php:update              # move every installed version to the newest patch
lerd php:update 8.4          # just this one
lerd php:list                # what is installed for the active runtime
```

Builds are published per PHP patch, so `8.4.24` and `8.4.25` are separate downloads and lerd pins one of them per minor. `lerd php:update` re-reads the pins past their cache, which matters because the passive check only looks once a day and the point of running it by hand is to find out now.

`php:update` follows the runtime, so it is one command either way: on the container runtime it rebuilds the FPM images from the newest base, which is what `lerd php:rebuild` does.

New patches arrive without a lerd release. The pins live in a manifest lerd reads at runtime, so a PHP released this week is offered to every install within a day of being built.

The binaries land in `~/.local/share/lerd/bin` as `php-native-<version>`, with their loadable extensions in `~/.local/share/lerd/native-php/<version>/modules`. Extensions are kept per version because they are named for the extension rather than the build, and a module only loads into the PHP it was compiled against. An update replaces the binary and its extensions together and then restarts the pool, since a running pool holds both open and keeps serving what it started with.

## What stays the same

`php:ini` still edits the same files: the native runtime reads the shared, per-version, mail and xdebug fragments through `PHP_INI_SCAN_DIR`, so a setting applies whichever runtime you are on. Xdebug works, shipped as a loadable extension alongside the binary. `dump()` and `dd()` capture works too, with the bridge reading its assets from their host copies. nginx keeps serving and terminating TLS, so HTTPS, domains, worktrees and site groups are untouched.

## What is captured

`dump()` and `dd()` work: the debug bridge is plain PHP and reads its assets from their host copies. Xdebug and pcov ship as loadable extensions beside the binary, so `xdebug:on` and coverage runs behave as they always have, and SPX is compiled in, so the profiler works too.

The Debug window's **query lens does not capture until your build ships the collector**. Those queries come from `lerd_devtools`, an engine-level extension the PHP image compiles in. The runtime loads it automatically when it is present beside the binary, and until then the site doctor says so rather than leaving the lens silently empty. `lerd php:runtime container` restores it in the meantime.

## Known gaps

The extension set is fixed at build time, so `lerd php:ext add` has nothing to add to. The build carries the same set the image does, including intl, imagick, mongodb, redis, soap, xsl and SPX, so this bites only a project needing something outside it. The site doctor reports the drift before you hit it at runtime, and container mode remains the escape hatch.

Extensions that cannot be compiled into a static binary, Xdebug and pcov among them, ship as loadable `.so` files beside it instead. That is a packaging detail rather than a limitation: both load and behave normally.

`lerd php:ext` and `lerd php:pkg` refuse here, and `lerd shell` says there is no container to enter rather than starting one.

A version with no published native build cannot be installed at all, and the native runtime needs PHP 8.1 or newer. Switching names the sites standing in the way rather than failing one at a time.
