# NativePHP walkthrough

End-to-end: a Laravel app that opens as a desktop window on your machine, or installs onto an Android emulator or an iOS simulator, while lerd keeps serving the same codebase at `https://myapp.test`.

::: info Prerequisites
You've already run `lerd install` once on this machine. If not, see [Installation](installation.md).
:::

[NativePHP](https://nativephp.com) is two products with two toolchains, and a project picks one:

| Package | Builds | Needs on your machine |
|---|---|---|
| `nativephp/electron` | A desktop app (macOS, Windows, Linux) | Nothing beyond lerd |
| `nativephp/mobile` | An iOS and Android app | Xcode for iOS, the Android SDK and a JDK for Android |

Both register the same `native:*` artisan commands, so don't install both in one project. lerd gates its own workers, commands and health checks on which package is present, so a project only ever sees the half it has.

---

## 1. Create the project

```bash
cd ~/Lerd
lerd new myapp --framework=laravel
cd myapp
lerd setup
```

Nothing here is NativePHP-specific yet. It is an ordinary Laravel site, linked, served at `myapp.test`, with its database created.

---

## 2. Desktop: add Electron

```bash
lerd composer require nativephp/electron
lerd run native:install
```

`native:install` is doing more than the artisan command of the same name. lerd runs every framework command on your host, but `php` on the host is lerd's shim, which routes straight back into the container, and the container has no display for Electron to draw into. So the command installs Electron's JavaScript dependencies on the host, unpacks the PHP binary NativePHP ships for your platform, and then runs `artisan native:install` through *that* binary. From there the platform-specific parts of NativePHP behave exactly as they do outside lerd.

You can still type the commands the NativePHP documentation gives you. The framework definition declares that the `native:` commands have to leave the container and which binary runs them, so `lerd artisan native:run` and `php artisan native:run` both reach that binary on your host rather than the shim, under the Node version your site is on. If the runtime is not installed yet they say so and name `native:install` instead of failing somewhere inside a container.

Start the app:

```bash
lerd worker start native
```

An Electron window opens running your app. The `native` worker is a [host worker](../usage/framework-workers.md), supervised by systemd on Linux and launchd on macOS, and it appears on the site's card in the dashboard and in the TUI alongside the queue and scheduler. Stopping it closes the window:

```bash
lerd worker stop native
```

Closing the window from the app itself ends `native:serve` with it, so the worker stops and its toggle turns off on its own. That is why the definition declares `restart: on-failure` rather than `always`: an always-restart worker would reopen the window you just closed. A crash still brings the app back, which is what you want mid-edit.

::: tip The toggle follows the process, not the window
If Electron loses its window but its own process survives, which is what a renderer crash looks like, lerd still reports the worker as running, because `native:serve` genuinely is. Neither of NativePHP's ports can tell you otherwise: it picks the PHP server from 8100-9000 and its API from 4000-5000 at boot and writes neither anywhere lerd could read, so there is nothing to probe. Toggle the worker off and on to get the window back.
:::


Package a binary when you're ready:

```bash
lerd run native:build
```

That one opens a terminal, because it asks which platform and architecture to target.

---

## 3. Mobile: add iOS and Android

```bash
lerd composer require nativephp/mobile
lerd run native:install
```

The mobile `native:install` asks for confirmation first, because it adds `nativephp/php-bin` to your `composer.json` as a dev dependency. That package is where the platform PHP binaries live, and the mobile toolchain needs one on your host for the same reason the desktop one does: `native:run` shells out to Gradle and adb, or to xcodebuild and simctl, and none of those exist inside a container. lerd unpacks the binary that matches your site's PHP version, which is also the version `composer.lock` was resolved against, and NativePHP checks that they agree.

Then build and launch:

```bash
lerd run native:run
```

It asks which platform, then which simulator, emulator or connected device, and builds from there. The first Android build downloads the NDK and a Gradle distribution, so give it a while; later builds are much faster.

To open the generated native project in Xcode or Android Studio:

```bash
lerd run native:open
```

### What each platform needs

| Platform | Requirement |
|---|---|
| Android | The Android SDK, and `JAVA_HOME` pointing at a JDK 17 or newer. Android Studio's bundled JDK works. |
| iOS | Xcode, plus a simulator runtime matching the SDK Xcode builds against. |

Gradle takes the JDK from `JAVA_HOME`, or the first `java` on `PATH` when that is unset, and an older one fails the build outright with `Your current JDK is located in ...`. Export it in your shell profile so both your terminal and lerd see it:

```bash
export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
```

The iOS one catches people out. Xcode ships an SDK but not necessarily the matching simulator runtime, and `xcodebuild` then reports `Unable to find a destination matching the provided destination specifier` while listing only physical devices as ineligible. Install the runtime and it resolves:

```bash
xcodebuild -downloadPlatform iOS
```

It is a multi-gigabyte download, which is why lerd reports the gap rather than filling it. Apple's asset service sometimes drops the transfer partway with `Download was cancelled`; re-running the command picks it back up.

---

## 4. The site keeps its URL

Linking a NativePHP project still gives you `myapp.test`, and on mobile that is more than a convenience.

`native:run` takes a `--start-url`, so the app can open on any route rather than `/`. `native:jump` goes further: it serves a development build to a real phone over the network and prints a QR code to scan, which is the same problem [LAN sharing](../usage/lan-sharing.md) already solves for browsers:

```bash
lerd run native:jump
```

It runs in a terminal rather than as a worker, and the QR code is why: a background unit has nowhere to draw one. It also asks which network interface to advertise when a machine has more than one, which podman and libvirt bridges make almost every development machine, and a unit has nobody to answer.

This one command does not run on the PHP binary NativePHP bundles. Jump's websocket bridge is Workerman, which needs the posix and pcntl extensions, and neither the Linux nor the macOS build of `php-bin` carries them, so the bridge dies on its first line: the phone loads the landing page over HTTP and then drops the session a few seconds later with nothing to connect to. The binary is a static build with dynamic loading compiled out, so the extensions cannot be added to it either. lerd runs Jump with its own PHP instead. On an install using the native runtime that is the binary already serving your sites, so nothing extra is downloaded; otherwise it is a pinned static build matching your project's PHP version, fetched to `~/.local/share/lerd/bin` the first time a project needs it, and lerd prints a line saying so. The toolchain commands are unaffected: `native:run` and the rest keep using the bundled binary, which has everything they need.

The `.test` domain itself stays useful throughout, for hitting a route with curl and for the parts of the app that are still an ordinary web app. The native runtime serves its own copy on its own port, so the two never collide.

---

## 5. Verify

```bash
lerd site:doctor
```

Three checks are specific to NativePHP, and each is gated on the package, so a plain Laravel project never sees them:

| Check | Fires when | Fix |
|---|---|---|
| **NativePHP Runtime** | The package is installed but the host runtime isn't unpacked, which is how a fresh clone arrives | `native:install` |
| **NativePHP Mobile Toolchain** | Neither Xcode nor the Android SDK is present | Reported only, since installing either is yours to do |
| **NativePHP Android JDK** | The Android SDK is present but the JDK Gradle would pick is older than 17 | Reported only, set `JAVA_HOME` |

---

## What just happened

| Command | What it did |
|---|---|
| `lerd link` | Registered `myapp.test` with nginx + dnsmasq, document root `public/` |
| `lerd run native:install` | Installed the native toolchain's own dependencies and unpacked the host PHP binary that drives them |
| `lerd worker start native` | Launched the Electron window as a supervised host worker |
| `lerd run native:run` | Built the app and installed it on a simulator, emulator or device |
| `lerd run native:jump` | Served a development build to a phone over the network |

---

## Quick commands

| Command | Product | What it does |
|---|---|---|
| `native:install` | both | Install the native runtime for this machine (mobile is confirm-gated) |
| `native:build` | desktop | Package the desktop app as a binary |
| `native:run` | mobile | Build and launch on a simulator, emulator or device (it picks the platform your machine can build for), [pinned](../usage/framework-commands.md#pinned-commands) to the site's control row |
| `native:open` | mobile | Open the Xcode or Android Studio project |

| `native:jump` | mobile | Serve to a phone on your network and draw the QR code |

Desktop also has a worker, `native`, because `native:serve` is a daemon: it supervises the Electron window and its toggle sits beside the queue and scheduler on the site's card. Mobile has none, and not by omission. Nothing in that flow is long-running in a way a background unit can carry: `native:run` builds and launches once, `native:open` hands off to Xcode or Android Studio, and Jump needs a terminal. All four are in the site's Commands panel instead.

Every other `native:*` command works through the host binary directly, for example:

```bash
vendor/nativephp/php-bin/bin/host/php artisan native:debug
```

---

## Next steps

- [Frameworks & Workers](../usage/frameworks.md): how the framework definition drives all of the above
- [Framework Workers](../usage/framework-workers.md): what `host: true` means and how host workers are supervised
- [LAN sharing](../usage/lan-sharing.md): reaching your site from a phone on the same network
- [AI Integration (MCP)](../features/mcp.md): drive lerd from Claude Code, Cursor, etc.
