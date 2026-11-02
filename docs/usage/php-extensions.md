---
title: "PHP extensions"
description: "The PHP extensions Lerd bundles, and how to add any other extension or Alpine package with lerd php:ext and lerd php:pkg, plus locales and ODBC drivers."
---

# PHP extensions

Lerd's PHP images ship about thirty extensions, `intl`, `gd`, `imagick`, `redis`, `mongodb` and the PDO drivers among them. To add one that is missing, run `lerd php:ext add <name>`: it is added to every PHP version and the current one is rebuilt. System libraries go in with `lerd php:pkg add`.

Related: [PHP versions per project](/usage/php-versions), [php.ini overrides](/usage/php-ini), [Composer](/usage/composer).

## Commands

| Command | Description |
|---|---|
| `lerd php:ext add <ext> [--apk-deps "pkg ..."]` | Add a custom PHP extension to every PHP image and rebuild the current version; `--apk-deps` lists extra Alpine packages the extension needs to build |
| `lerd php:ext remove <ext>` | Remove a custom PHP extension from every PHP image and rebuild |
| `lerd php:ext list` | List your declared extensions, and what each PHP version's image actually loaded |
| `lerd php:pkg add <package...>` | Install extra Alpine packages into every FPM image and rebuild the current version |
| `lerd php:pkg remove <package...>` | Remove extra Alpine packages from every FPM image and rebuild |
| `lerd php:pkg list` | List your declared packages, and what each PHP version's image actually installed |

## Custom extensions

The default lerd FPM image ships ~30 extensions covering the vast majority of Laravel projects (`bcmath`, `bz2`, `calendar`, `curl`, `dba`, `exif`, `ftp`, `gd`, `gmp`, `igbinary`, `imagick`, `intl`, `ldap`, `mbstring`, `mongodb`, `mysqli`, `odbc`, `opcache`, `pcntl`, `pdo_mysql`, `pdo_odbc`, `pdo_pgsql`, `pdo_sqlite`, `redis`, `soap`, `shmop`, `sockets`, `sqlite3`, `sysvmsg`, `sysvsem`, `sysvshm`, `xdebug`, `xsl`, `zip`, and more).

Two of those names are version-gated, because the image genuinely cannot build them everywhere: `random` is a PHP core extension only from 8.2, and `mongodb` builds only on 8.1 and up. On older versions they are not part of the bundle, and `lerd park` warns when a project requires one rather than staying quiet and letting `composer install` fail its platform check.

To add an extension that isn't in the bundle:

```bash
lerd php:ext add swoole
```

An extension the image already ships is refused, since the bundle is the better build of it. The image compiles those extensions as part of its own configure run, with flags a standalone rebuild on top of the image cannot pass, so redeclaring one would quietly replace it with a lesser build: `ftp` declared this way came back without FTPS support and no `ftp_ssl_connect`. Builds skip any bundled name left over in an existing declared set for the same reason, and an image built while one was still being rebuilt reads as out of date, so the next build puts the image's own version back with nothing to run by hand.

Extensions belong to you, not to a PHP version. One declared set applies to every PHP image lerd builds, so a site that changes version keeps them. The version you are on is rebuilt and verified straight away; other installed versions carry the old set until they are rebuilt, and lerd says which ones those are.

Those deferred versions are rebuilt by the next command that touches them, which is usually `lerd use`, `lerd link`, `lerd fetch`, `lerd unpause` or `lerd start`. When that happens to a version whose container is already running, lerd restarts the container onto the image it just built, so the running PHP always matches what `lerd php:ext list` and the dashboard report for it.

Extensions are persisted in `~/.config/lerd/config.yaml` under `php.extensions`, so they survive `lerd php:rebuild`.

After the rebuild, lerd checks that the extension actually loaded (`php -m`); if the PECL build failed, `lerd php:ext add` exits with an error and removes the extension from the config again, rather than reporting success for an extension that isn't there. A rebuild that fails outright is reverted the same way, so a name that cannot build is not left declared and retried by every command after it.

### What each version actually loaded

A declared set cannot always be honoured. `mongodb` does not build below 8.1, and the legacy 7.4 and 8.0 images are Alpine 3.16, where some packages do not exist. lerd records what each version's image really loaded, verified after its build, and never advertises what an image does not have:

```bash
lerd php:ext list
```

```
Declared, for every PHP version:
  - mongodb
  - swoole

Per version:
  PHP 7.4  swoole (cannot load: mongodb)
  PHP 8.1  image predates this set, run 'lerd php:rebuild 8.1'
  PHP 8.4  mongodb, swoole
  PHP 8.5  mongodb, swoole
```

The three states are different problems. An image that **predates the set** was built before you declared something, and a rebuild fixes it. Something an image **cannot load** did not build on that version, and a rebuild will not change that. A version with no image at all is not listed. Nothing is reported as present unless that image really has it.

The dashboard shows the same thing, plus every module the image loads, under **System → PHP → Extensions**. The module list is `php -m` read from the image itself, so it is what your code will actually see; it is fetched when you open the tab and cached against the image, so a rebuild refreshes it and nothing else pays for it. The TUI's System view carries a shorter `Extras · PHP <version>` line per version, from the same recorded data.

Changing a site's PHP version is exactly when it would silently lose an extension, so lerd checks the target image at that moment. An image built before you declared something can be brought up to date with a rebuild:

```
$ lerd isolate 8.3
 ✓ PHP pinned to 8.3
 ⚠ PHP 8.3's image predates your custom extensions and packages
       run 'lerd php:rebuild 8.3' to bring it up to date
```

An extension that genuinely cannot build on that version is reported differently, because no rebuild will fix it.

One that loaded on **no version at all** is reported differently again. A version boundary shows up on some versions and not others, so an extension missing from every one of them is usually the build failing rather than the versions refusing it, and `lerd php:ext list` says so instead of reading it as a capability gap.

Some extensions need extra Alpine packages to compile. lerd already knows the ones for `imap` (`imap-dev krb5-dev openssl-dev c-client`); for anything else, pass them with `--apk-deps`:

```bash
lerd php:ext add ssh2 --apk-deps "libssh2-dev"
lerd php:ext add imap                                  # deps known to lerd, no flag needed
```

The packages are saved alongside the extension in `~/.config/lerd/config.yaml` (under `php.ext_apk_deps`), so they reapply on every `lerd php:rebuild`.

```bash
lerd php:ext list                # show your custom extensions and their apk deps
lerd php:ext remove swoole       # remove from every version and rebuild
```

### Prebuilt Zend extensions (ionCube)

`php:ext add` builds from source, through PECL or the PHP source tree, so an extension that is only ever shipped as a ready-made binary cannot go through it. ionCube Loader is the usual one: `lerd php:ext add ioncube` fails because there is nothing to build.

Such a loader needs no rebuild at all. Your home directory is mounted into every PHP container at the same path, so download the loader there and point the version's `php.ini` at it. The images are Alpine, so take the `lin-musl` build for your architecture and the file matching the PHP version:

```bash
lerd php:ini 8.5
```

```ini
zend_extension = "/home/you/.php/ioncube_loader_lin-musl_8.5.so"
```

Use the per-version file rather than the shared one, since each loader only works on the PHP version it was built for. Saving restarts that version's FPM, and `lerd php -v` run from a site on that version should then mention the ionCube Loader. CLI commands run in the same container, and so do workers in the default exec mode, so they pick it up too.

On macOS with [`lerd workers mode container`](/usage/worker-runtime), each worker runs in its own container that mounts only the project folder, so a loader in your home directory is not visible to it. Keep the loader inside the project there, and restart the site's workers after editing the ini, since saving it only restarts FPM.

## Locales and internationalisation

The FPM image is Alpine-based, so it uses musl libc rather than glibc. Two consequences worth knowing:

- **`ext-intl` (`NumberFormatter`, `IntlDateFormatter`, Laravel's `Number::currency()`, `money` formatting) works for every locale.** The image bundles ICU's full CLDR locale database (`icu-data-full`), so `new NumberFormatter('nl_NL', NumberFormatter::CURRENCY)` correctly produces `€ 13.943,20`. This is the recommended way to do locale-aware formatting and it does not depend on the system locale at all.
- **The C-library `setlocale()` / `localeconv()` path stays in the C locale.** musl does not implement locale-specific `LC_NUMERIC` / `LC_MONETARY` rules, so `setlocale(LC_ALL, 'nl_NL')` will return a value but `localeconv()` keeps returning `.` / empty separators, and `number_format()` without explicit separators won't switch. Pass separators explicitly (`number_format($n, 2, ',', '.')`) or use `ext-intl`.

If a library you depend on calls `setlocale()` and branches on whether it succeeded, adding the `musl-locales` / `musl-locales-lang` apk packages makes the call return a value, but it still will not change number or currency formatting.

## Composer.json detection

When you run `lerd park` or `lerd link`, Lerd reads `composer.json` and warns if any `ext-*` requirements are not covered by the bundled or installed extension set:

```
[!] my-app requires PHP extensions not in the image: swoole
    Run: lerd php:ext add swoole
```

An extension can also be one the site's PHP version cannot have at all, rather than one that is merely absent from the image. `random` is a PHP core extension only from 8.2, and `mongodb` builds only on 8.1 and up, so on an older site no image rebuild can supply them and `lerd php:ext add` would fail after several minutes of building. Lerd says so up front instead of offering an install that cannot work:

```
[!] my-app requires ext-random, which is not available on PHP 8.1 (first shipped on 8.2)
    lerd php:ext add cannot build it. Move the site to PHP 8.2 or newer, or require a polyfill package instead.
```

A requirement can also fail even though the extension is in the image, because composer names a few extensions differently from the name they are installed under. Composer builds its `ext-*` names from the module name PHP reports, and OPcache reports itself as `Zend OPcache`, so composer publishes `ext-zend-opcache` and never `ext-opcache`. A `composer.json` asking for `ext-opcache` therefore fails its platform check on `composer install` even though OPcache is loaded, and `lerd php:ext add opcache` will not help because nothing is actually missing. Lerd recognises both spellings and tells you which one composer wants:

```
[!] my-app requires ext-opcache, which composer publishes as ext-zend-opcache
    The extension is in the image; composer install will still fail its platform check.
    Require ext-zend-opcache in composer.json instead.
```

## ODBC databases

The image ships the ODBC stack whole: unixODBC as the driver manager, `ext-odbc` for the `odbc_*` functions and their `odbc.defaultlrl` / `odbc.defaultbinmode` settings, and `ext-pdo_odbc` so `new PDO('odbc:...')` and any framework connection built on it work with nothing to declare and no extension to add.

What the image cannot ship is the driver itself. Every ODBC driver (SAP HANA, Oracle, SQL Server, Snowflake) is vendor-licensed and downloaded from the vendor, so you bring the `.so` and register it. The images carry `gcompat` and `libstdc++` for exactly that moment: vendor drivers are built against glibc while lerd's images are Alpine/musl, and without the shim unixODBC cannot load them at all.

### Turning it on

There is nothing to enable. The extensions are part of the image recipe, so a version whose image is built from the current recipe has them, and one built before it does not until it is rebuilt.

`lerd update` does that rebuild for you: the recipe changed, so the install step that follows an update rebuilds every PHP image, pulling the matching [pre-built base](/usage/php-versions#pre-built-images) rather than compiling. To force it, or to bring one version forward on its own:

```bash
lerd php:rebuild            # every installed version
lerd php:rebuild 8.5        # just this one
```

Check what landed from inside the container:

```bash
lerd shell
php -m | grep -i odbc                      # odbc, PDO_ODBC
php -r 'print_r(PDO::getAvailableDrivers());'
```

The dashboard shows the same list under **System → PHP → Extensions**, read from the image itself.

Two site kinds need one more step. An Octane site's FrankenPHP image is rebuilt by the same `lerd php:rebuild`. A site with its own `Containerfile.lerd` builds `FROM` the base image, so it keeps the image it has until you rebuild it:

```bash
cd my-project && lerd rebuild
```

That is also the moment such a project can drop its own `pdo_odbc` build steps, since the base it builds on carries them now.

### Registering a driver

A driver is named in `/etc/odbcinst.ini`, the driver manager's registry. lerd generates that file from the drivers you register and mounts it read-only into every PHP container, so one registration covers every version and every site:

```bash
lerd php:odbc add HDBODBC ~/sap/hdbclient/libodbcHDB.so --description "SAP HANA"
lerd php:odbc list
lerd php:odbc remove HDBODBC
```

The driver file itself stays where the vendor's installer put it; lerd only points at it. Anything under your home directory is already visible inside the containers at the same path, and a driver elsewhere has its directory mounted along with the registry.

Until you register something, lerd mounts no registry at all, so a site whose own image writes `/etc/odbcinst.ini` keeps the drivers it baked in. Once a driver is registered, lerd's registry is what every container reads, and a driver baked into a site image has to be registered here too.

`add` then reads the driver back from inside the image and says what it found, because a registration that looks fine on the host is exactly the one that fails later as a connection error with nothing in it:

```
$ lerd php:odbc add HDBODBC ~/sap/hdbclient/libodbcHDB.so
 › registering ODBC driver HDBODBC
   /Users/you/sap/hdbclient/libodbcHDB.so
 ⚠ PHP 8.5 cannot load the driver, gcompat does not carry these glibc symbols:
   qfcvt_r, qecvt_r, fcvt_r, backtrace, __strcpy_chk, gethostent_r, ...
   a driver needing more than gcompat has to be patched in a per-site
   Containerfile, see the ODBC section of the PHP docs
 ✓ driver HDBODBC registered for every PHP version, but the PHP 8.5 image cannot load it yet
```

The three ways a registration fails are worth telling apart, because unixODBC reports all of them at connect time as `Can't open lib '...' : file not found`, whatever the real cause. The container cannot see the path; the driver needs a library the image does not have; or every library resolved but one of them is musl standing in for glibc and does not carry a symbol the driver wants. Only the last one lists symbols, and those symbols are the shopping list for the shim below.

`lerd php:odbc list` re-runs the check, so it reports what each driver does now rather than what it did when you registered it.

A fourth outcome is worth reading carefully, because it is not a verdict on the driver. When the check cannot run at all, say the runtime is down or the version has no image yet, the registration still stands but nothing was established about it:

```
 ✓ driver HDBODBC registered, but it was not read back from an image, so run
   'lerd php:odbc list' once the runtime is up to see whether it loads
```

`list` says `not checked` for the same reason. Neither means the driver is broken, only that no image has been asked yet.

### Where a driver may live

The driver stays where the vendor put it, with one refusal. A driver sitting directly in a directory the container keeps its own libraries in is turned away, because lerd mounts the directory holding a driver into every PHP container and the host's copy would cover the container's own:

```
$ lerd php:odbc add PgOdbc /usr/lib/psqlodbcw.so
 ✗ driver "/usr/lib/psqlodbcw.so" sits in /usr/lib, which the PHP container keeps
   its own libraries in: mounting it would take the container down. Copy the driver
   somewhere of its own (say ~/odbc-drivers) and register that path
```

That is only the handful of directories the runtime itself occupies, `/usr/lib`, `/lib`, `/etc`, `/var` and their neighbours. A vendor client unpacked under `/opt` is the normal case and mounts as it is, and so does `/usr/lib64`, which the Alpine image does not have. Distributions that package an ODBC driver into `/usr/lib` are the ones to copy out.

A driver already reachable inside the container gets no mount of its own. Anything under your home directory arrives through the home mount, and anything inside a parked project arrives through that project's, which keeps it writable where a read-only mount of lerd's own would not.

If the driver directory later disappears, through an uninstall or a disk that is not plugged in, the registry entry stays and `list` reports the driver as one the container cannot see. The mount is dropped from the quadlets the next time the watcher starts, so the containers do not stay stuck on a path that has gone.

### Drivers that need more than gcompat

Some vendor clients want glibc symbols `gcompat` does not provide, and a few need patching before the loader will touch them at all. Those are prepared in a per-site [custom image](/usage/php#custom-image-containerfile), where the driver can be unpacked, patched and registered in one place.

SAP HANA is the fullest version of this problem, so it makes the best example. Its client is a glibc build, its ODBC library needs thirteen symbols musl does not have, and SAP's own installer cannot run on Alpine at all: `hdbinst` runs on a bundled perl built against glibc, which `gcompat` does not satisfy. The payloads inside the archive are plain tarballs, though, so they can be unpacked directly.

Download the Linux client for your architecture from SAP (it is licensed, so it is not something lerd or your image registry can ship) and drop the tarball in `docker/hana/` next to a small C file supplying the missing symbols:

```c
// docker/hana/glibc-shim.c — one stub per symbol `lerd php:odbc add` listed.
#include <string.h>
#include <stdlib.h>

void *__memcpy_chk(void *d, const void *s, size_t n, size_t dl) { (void)dl; return memcpy(d, s, n); }
char *__strcpy_chk(char *d, const char *s, size_t dl) { (void)dl; return strcpy(d, s); }
int backtrace(void **b, int size) { (void)b; (void)size; return 0; }
// ... qfcvt_r, qecvt_r, fcvt_r, gethostent_r, getpwent_r, getservent_r,
// getspent_r, pthread_mutexattr_setkind_np, __register_atfork
```

```dockerfile
# Containerfile.lerd
FROM lerd-php85-fpm:local

# SAP ships glibc builds and the base is Alpine/musl. gcompat covers most of the
# gap; the shim supplies what it does not, and patchelf wires it in as a
# DT_NEEDED entry with an $ORIGIN rpath so whatever dlopens the driver, unixODBC
# here, pulls the shim in with it.
COPY docker/hana/ /tmp/hana/

RUN set -eux; \
    apk add --no-cache --virtual .hana-build patchelf build-base; \
    mkdir -p /tmp/hana/x /usr/sap/hdbclient; \
    tar -xzf /tmp/hana/hanaclient-*.tar.gz -C /tmp/hana/x; \
    tar -xzf /tmp/hana/x/client/client/ODBC.TGZ -C /usr/sap/hdbclient; \
    cc -shared -fPIC -O2 -o /usr/sap/hdbclient/libhdbglibcshim.so /tmp/hana/glibc-shim.c; \
    patchelf --add-needed libhdbglibcshim.so --set-rpath '$ORIGIN' /usr/sap/hdbclient/libodbcHDB.so; \
    ldd /usr/sap/hdbclient/libodbcHDB.so; \
    printf '[HDBODBC]\nDescription=SAP HANA\nDriver=/usr/sap/hdbclient/libodbcHDB.so\n' > /etc/odbcinst.ini; \
    apk del .hana-build; \
    rm -rf /tmp/hana

ENV LD_LIBRARY_PATH=/usr/sap/hdbclient
```

The `ldd` in the middle is deliberate: it runs during the build, so a shim that is still a symbol short fails visibly there rather than as a connection error weeks later.

```yaml
# .lerd.yaml — no port, so the site is served by fastcgi from this image
domains:
  - myapp
container:
  containerfile: Containerfile.lerd
```

Then `lerd rebuild`, and the DSN resolves through the name registered in the image:

```php
// config/database.php
'hana' => [
    'driver'   => 'odbc',
    'dsn'      => 'odbc:Driver={HDBODBC};ServerNode=hana.example:30015;char_as_utf8=true;',
    'username' => env('HANA_USERNAME'),
    'password' => env('HANA_PASSWORD'),
],
```

Laravel has no `odbc` database driver of its own, so a connection like this one needs a connector registered for it in a service provider, extending `Illuminate\Database\Connectors\Connector` and returning a PDO built from the `dsn` key. That part is your application's, not lerd's; what the image owes you is a driver the PDO constructor can actually open.

A site whose image registers a driver this way keeps working exactly as before, because lerd mounts its own registry over `/etc/odbcinst.ini` only once you register something with `lerd php:odbc`. If you do both, lerd's registry is what the container reads, so register the in-image driver there too.

unixODBC's own tools come with the image, so `lerd shell` is where a registration is checked by hand: `odbcinst -q -d` lists the drivers the manager can see, `isql -v HDBODBC user password` connects without PHP in the way, and `ldd /usr/sap/hdbclient/libodbcHDB.so` is the one that explains a driver that will not open. Its `Error relocating ...: symbol not found` lines are each a function the shim has to supply, and a driver whose libraries all resolve can still fail on every one of them.
