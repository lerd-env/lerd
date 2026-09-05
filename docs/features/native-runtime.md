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

FrankenPHP sites, custom containers and host-proxy sites are unaffected either way: they never used the shared FPM container.

## What stays the same

`php:ini` still edits the same files: the native runtime reads the shared, per-version, mail and xdebug fragments through `PHP_INI_SCAN_DIR`, so a setting applies whichever runtime you are on. Xdebug works, shipped as a loadable extension alongside the binary. `dump()` and `dd()` capture works too, with the bridge reading its assets from their host copies. nginx keeps serving and terminating TLS, so HTTPS, domains, worktrees and site groups are untouched.

## What is captured

`dump()` and `dd()` work: the debug bridge is plain PHP and reads its assets from their host copies. Xdebug and pcov ship as loadable extensions beside the binary, so `xdebug:on` and coverage runs behave as they always have, and SPX is compiled in, so the profiler works too.

The Debug window's **query lens does not capture until your build ships the collector**. Those queries come from `lerd_devtools`, an engine-level extension the PHP image compiles in. The runtime loads it automatically when it is present beside the binary, and until then the site doctor says so rather than leaving the lens silently empty. `lerd php:runtime container` restores it in the meantime.

## Known gaps

The extension set is fixed at build time, so `lerd php:ext add` has nothing to add to. The build carries the same set the image does, including intl, imagick, mongodb, redis, soap, xsl and SPX, so this bites only a project needing something outside it. The site doctor reports the drift before you hit it at runtime, and container mode remains the escape hatch.

Extensions that cannot be compiled into a static binary, Xdebug and pcov among them, ship as loadable `.so` files beside it instead. That is a packaging detail rather than a limitation: both load and behave normally.

`lerd php:ext` and `lerd php:pkg` refuse here, and `lerd shell` says there is no container to enter rather than starting one. Installing a PHP version from the dashboard also refuses: that needs a prebuilt native binary rather than a container image.
