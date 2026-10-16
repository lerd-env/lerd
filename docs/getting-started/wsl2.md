# Windows (WSL2)

Lerd runs on Windows through WSL2. There is no native Windows build, the architecture leans on systemd user services and rootless Podman, both of which only exist on Linux. WSL2 with systemd enabled gives you a real Linux user session where the standard Linux build runs unchanged, install script, Quadlets, watcher and all.

On a Windows machine with nothing set up, `lerd-setup.exe` does all of it for you, and `install.ps1` does the same from a PowerShell window. If you already run WSL, `lerd wsl:setup` wires an existing install into Windows. The rest of this page documents each piece by hand, as the reference for what those two do and as the fallback.

::: warning Beta
Windows support via WSL2 is **beta**. The standard Linux build runs unchanged inside a systemd WSL2 session and is fine for daily development, but this path gets less testing than native Linux or macOS. Please report anything that misbehaves on the [issue tracker](https://github.com/lerd-env/lerd/issues).
:::

::: warning Supported distros
Ubuntu 22.04 or newer, or Debian 12 or newer, running on WSL 0.67.6 or newer (the release that brought official systemd support). Older WSL versions need workarounds like `genie` or `subsystemctl`, which are not tested here.
:::

## Install with lerd-setup.exe

Download `lerd-setup.exe` from the [latest release](https://github.com/lerd-env/lerd/releases/latest/download/lerd-setup.exe) and run it. It asks for administrator rights once, then:

1. Enables the two Windows features WSL 2 runs on and asks to restart. After you log back in it carries on by itself.
2. Installs WSL from Microsoft's GitHub release, then an Ubuntu 24.04 distro.
3. Creates your Linux user (it suggests your Windows name) and asks you to choose its password, which `sudo` asks for later.
4. Runs lerd's normal installer inside the distro, then `lerd wsl:setup`.
5. Restarts WSL and opens the dashboard in your browser.

Every step checks before it acts, so running it again after a failure picks up where it stopped. Windows shows a certificate warning when lerd's local CA is trusted, answer Yes.

## Install from PowerShell

For a console install, run this in a PowerShell window. It does not need to be opened as administrator:

```powershell
irm https://lerd.sh/install.ps1 | iex
```

It takes the same steps as `lerd-setup.exe` and leaves the same result: WSL, Ubuntu 24.04 with your Linux user, Podman, the Lerd desktop app under `C:\Program Files\Lerd`, lerd itself and `lerd wsl:setup`. It asks for the Linux user and its password, then for the local domain mode and whether lerd manages Node.js, the questions the wizard's pages ask. It runs as you and asks Windows for administrator rights only for the steps that need them: enabling the WSL features, installing the WSL package and unpacking the desktop app, plus the DNS rule `lerd wsl:setup` adds.

When the WSL features need a restart, it offers one and runs again by itself after you sign back in. Like the wizard, running it again after a failure resumes where it stopped. To test an unreleased build, set `$env:LERD_SETUP_LOCAL` to a folder holding `install.sh`, `lerd` and `lerd.exe`, and optionally `lerd-desktop-win32-<arch>.zip`, then run `.\install.ps1` from a checkout.

## What lerd sets up on the Windows side

`lerd wsl:setup`, which the installer runs for you, puts these in place:

- **`lerd` in any Windows terminal.** `lerd.exe` in `%LOCALAPPDATA%\lerd\bin`, on your PATH, runs lerd inside the distro from the folder you are in, including a project opened through `\\wsl.localhost\...`. Windows paths in its arguments, such as `C:\work\app` or `--configuration=\\wsl.localhost\Ubuntu\home\you\app\phpunit.xml`, are translated to the paths the distro sees.
- **`php`, `composer`, `node` and lerd's other shims.** `php.exe`, `composer.exe` and so on sit beside `lerd.exe` and run the same shim as inside the distro, so a Windows IDE can point its PHP interpreter, Composer or Node at them. They are rewritten on each `lerd wsl:setup`, so run it again after enabling a new shim.
- **lerd at login.** A background agent starts with Windows and boots the distro, so sites and the dashboard are up without a WSL window open. With the Lerd desktop app installed, the app starts the agent and starts it again if it stops; without it, a `lerd` entry under the user's Run key does. WSL's idle timeout is switched off in `%USERPROFILE%\.wslconfig`, which otherwise stops the distro, and every container with it, seconds after the last terminal closes. If the distro stops anyway, after a `wsl --shutdown`, a `wsl --update` or a crash, the agent boots it again the next time a Windows app looks up a site, so the page loads after a few seconds instead of failing to connect.
- **`.test` in Windows browsers, wildcards included.** The agent answers DNS for your site TLD on `127.0.0.1`, and a Windows DNS rule (NRPT) sends the TLD there. It is the one step that needs administrator rights. Answering DNS rather than writing hosts file lines is what keeps `*.branch.site.test` worktree domains working.
- **HTTPS trusted by Windows browsers.** lerd's local CA is imported into your Windows user's trust store.
- **Mirrored networking**, so Windows reaches WSL's loopback, plus systemd, the Podman `events_logger` fix and the Linux tray turned off, covered below.

It is idempotent, so run it again whenever something drifts. `lerd doctor` adds a `[WSL2]` section that re-checks the Linux side on demand.

## Working on your sites from Windows

Your sites live on the distro's own disk, not in `C:\Users\you`, and the two are not synced. Windows reaches the same files at `\\wsl.localhost\<distro>\home\<user>`, which is what everything below uses:

- **Explorer.** `lerd wsl:setup` pins your sites folder (`~/Lerd` and any other parked directory) to Quick access. It also shows under **Linux** in the sidebar.
- **Editor.** VS Code with the WSL extension (`code .` from a WSL terminal, or open the `\\wsl.localhost` folder), or PhpStorm opening that path with the WSL PHP interpreter. Both edit the files where they live.
- **Terminal.** `lerd` works in Windows Terminal from inside a `\\wsl.localhost` folder as well as from a WSL shell.
- **Browser.** `https://<site>.test` from any Windows browser, with the certificate trusted.

The dashboard's buttons and `lerd open` reach the Windows desktop too: a site opens in your default browser, a folder in Explorer, a terminal as a Windows Terminal tab on your distro's profile, and a file in an editor found on the Windows PATH through its own WSL launcher (Zed's or VS Code's `bin` folder), which opens it over WSL.

Keep projects on the Linux side. A folder under `C:\` works through `/mnt/c`, but file access there is roughly an order of magnitude slower, which composer and npm make obvious; see step 5.

## 1. Enable systemd inside WSL2

The whole architecture, Quadlet containers, watcher, UI, FPM, runs as systemd user units. Without systemd as PID 1, nothing starts.

Inside the WSL distro, create or edit `/etc/wsl.conf`:

```ini
[boot]
systemd=true

[user]
default=your-user
```

Then from PowerShell on Windows:

```powershell
wsl --shutdown
```

Reopen the WSL terminal and confirm both checks pass:

```bash
ps -p 1 -o comm=
# expected: systemd

systemctl --user status
# should respond without error
```

## 2. Mirrored networking (recommended)

Without mirrored networking, `http://yoursite.test` and `http://yoursite.localhost` only respond inside the WSL distro, the Chrome or Edge install on the Windows side cannot reach them. With mirrored networking the WSL2 network interface mirrors the Windows one and loopback works from both sides.

On Windows, create or edit `%USERPROFILE%\.wslconfig`:

```ini
[wsl2]
networkingMode=mirrored
dnsTunneling=true
firewall=true
autoProxy=true

[general]
instanceIdleTimeout=-1
```

`instanceIdleTimeout=-1` keeps the distro running after the last terminal closes; without it WSL stops it within seconds and every site goes down.

Then `wsl --shutdown` again.

::: info Requires WSL 2.0.0+ on Windows 11 22H2+
Without mirrored networking you can still reach the dashboard from a browser installed inside WSL (Firefox or Chromium from apt) or by hitting the WSL VM IP directly (`ip addr show eth0`), but day to day this is the setting that makes Windows browsers behave like the WSL distro is localhost.
:::

## 3. Install Podman

Podman in default WSL2 Ubuntu and Debian repos works, but a few defaults clash with how lerd drives the Quadlets. The cleanest path is to follow [this Podman on WSL2 setup gist](https://gist.github.com/GiovanniGrieco/94ab72099fa35bc307fda0e36b88f1bd), then apply the lerd specific tweak below.

```bash
sudo apt update
sudo apt install -y \
  podman uidmap fuse-overlayfs slirp4netns \
  curl git unzip libnss3-tools
```

`uidmap` and `slirp4netns` are what give you rootless containers. `fuse-overlayfs` is the fallback when the WSL kernel's overlayfs misbehaves. `libnss3-tools` provides the `certutil` mkcert needs.

::: danger Set `events_logger = "journald"` for Podman
Grieco's gist sets `events_logger = "file"` in `~/.config/containers/containers.conf`. Podman defaults to the `journald` log driver on systemd hosts, and refuses `--follow` when the log driver is journald but the events backend is file. Lerd's dashboard log views and `lerd logs` both call `podman logs --follow` under the hood, so every log pane ends up showing `Error: using --follow with the journald --log-driver but without the journald --events-backend (file) is not supported`.

Set `events_logger = "journald"` instead:

```ini
# ~/.config/containers/containers.conf
[engine]
events_logger = "journald"
cgroup_manager = "cgroupfs"
```

Keep `cgroup_manager = "cgroupfs"` from the gist, that part is correct for WSL2.
:::

::: warning Do not install Docker alongside Podman
Lerd is built exclusively for rootless Podman with Quadlet. Installing `docker.io` or `docker-ce` on the same WSL distro causes networking and cgroup conflicts.
:::

## 4. Run the lerd installer

```bash
curl -fsSL https://lerd.sh/install.sh | bash
```

When the installer asks **"Let lerd manage DNS for local sites?"**, both modes are viable on WSL2:

- **Yes (`.test` domains, lerd-dns, HTTPS)**: confirmed working on WSL2 Ubuntu by a community user. Picks up `systemd-resolved` or NetworkManager if you have one running, and falls back cleanly when neither is the active resolver.
- **No (`.localhost` domains, no DNS daemon)**: lighter path, no resolver wiring at all, `.localhost` resolves to loopback by RFC 6761. Good if you hit DNS issues with the `.test` mode.

If the installer fails to enable linger automatically, run it by hand and then restart the distro:

```bash
sudo loginctl enable-linger $USER
exit
# in PowerShell:
wsl --shutdown
```

## 5. Keep projects in `$HOME`, never in `/mnt/c/...`

This is the single biggest performance lever on WSL2. Bind mounts from `/mnt/c/...` into containers route through 9P, which is roughly an order of magnitude slower than the WSL2 ext4 filesystem. Measured on a fresh Laravel app, `composer dump-autoload` took 31 s on `/mnt/c` against 3.3 s on ext4, `artisan` commands ran 4 to 8 times slower, and a page took over a second whenever OPcache rechecked its files. File watchers also miss every change there, since inotify events don't cross the 9P mount, so Vite's hot reload stays silent. `lerd link`, `lerd park` and `lerd doctor` warn when a project sits on a Windows drive.

```bash
# Avoid
cd /mnt/c/Users/you/projects/myapp
lerd link

# Do
mkdir -p ~/projects
cd ~/projects
git clone git@github.com:org/myapp.git
cd myapp
lerd link
```

To move an existing project, copy it and link the copy: `cp -a /mnt/c/Users/you/projects/myapp ~/projects/`, then `lerd unlink` in the old folder and `lerd link` in the new one. Windows apps reach the copy at `\\wsl.localhost\<distro>\home\<you>\projects\myapp`, and editing it from there still fires the watchers.

If you edit from VS Code on Windows, use the Remote-WSL extension and launch from the WSL side with `code .` from inside `~/projects/myapp`. The Windows VS Code process will attach to the WSL server, but the files stay on ext4.

## 6. `.test` DNS and HTTPS

Inside the distro, WSL writes its own `/etc/resolv.conf` pointing straight at its DNS proxy, which bypasses systemd-resolved and so every `.test` route. lerd hands that file to systemd-resolved on install: it sets `generateResolvConf=false` in `/etc/wsl.conf`, keeps WSL's proxy as resolved's upstream in `/etc/systemd/resolved.conf.d/wsl-upstream.conf`, and from there takes the same path as on any Ubuntu machine. That drop-in is deliberately not removed by an uninstall, since the distro would be left with no upstream DNS.

Windows resolves `.test` through the lerd agent described above. To trust HTTPS by hand instead of through `lerd wsl:setup`, copy the CA out of WSL and import it:

```bash
cp "$(~/.local/share/lerd/bin/mkcert -CAROOT)/rootCA.pem" /mnt/c/Users/$USER/Desktop/lerd-rootCA.crt
```

Double-click `lerd-rootCA.crt`, pick **Place all certificates in the following store**, choose **Trusted Root Certification Authorities**, finish, and restart the browser.

## 7. The tray lives in the Windows desktop app

`lerd-tray` needs a graphical tray host implementing the `StatusNotifierItem` or `AppIndicator` protocol, and WSL2 does not provide one, so lerd disables it inside WSL. On Windows the tray belongs to the Lerd desktop app, which draws the menu the dashboard serves at `http://127.0.0.1:7073/api/tray` (see [Drawing the menu elsewhere](../features/system-tray.md#drawing-the-menu-elsewhere)). Without it, the CLI and the dashboard cover everything. **Open Dashboard** and `lerd dashboard` open the dashboard as an app when you installed it as one from Edge, Chrome, Vivaldi or Brave, and in your default browser otherwise.

## 8. Notifications

With the native sink (`lerd notify target native`), `lerd-ui` raises Windows toasts through PowerShell, titled **Lerd** with its logo, and nothing has to be open for them to appear. Clicking one opens the dashboard at the right view in your default Windows browser. Banners, sound and Do Not Disturb are set per app under **Settings → System → Notifications → Lerd**. `lerd uninstall` removes the app entry Windows keeps for them.

While the Lerd desktop app is running it shows these notifications itself, under the same name, logo and settings entry, and clicking one opens the app at the right view. `lerd-ui` hands them over through `/api/desktop/ws` and decides what is raised just as before: the global toggle, the category toggles and the focused-window rule all still apply. If the app is not running, the PowerShell toasts take over again.

## Verifying the install

Before running `lerd link` on your first project, sanity check that all the pieces are in place:

```bash
ps -p 1 -o comm=                                   # systemd
systemctl --user is-active default.target          # active
loginctl show-user $USER --property=Linger         # Linger=yes
podman info --format '{{.Host.Security.Rootless}}' # true
podman info --format '{{.Store.GraphDriverName}}'  # overlay
systemctl --user is-active lerd-ui                 # active
getent hosts lerd.localhost                        # ::1 / 127.0.0.1
```

If all of those come back green, `cd ~/projects/myapp && lerd link` behaves exactly the same as on native Linux.

## Known WSL2 quirks

::: details WSL prints "Nested virtualisation is not supported on this machine"
Windows itself is running in a virtual machine. It only means you cannot run a VM inside WSL, which lerd never needs.
:::

::: details Composer or npm install is painfully slow
The project is somewhere under `/mnt/c/...`. Move it into `~/projects/` and re-link.
:::

::: details `curl.exe` on Windows rejects a `.test` certificate that browsers accept
Windows' own curl insists on a certificate revocation check, which a local CA cannot answer. Browsers do not, so this is curl only; `curl.exe --ssl-no-revoke` gets past it.
:::

::: details `podman build` fails on overlay
`sudo apt install fuse-overlayfs && podman system reset` then run install again.
:::

::: details `cannot allocate memory` on large builds
Raise the WSL VM memory ceiling in `%USERPROFILE%\.wslconfig`:

```ini
[wsl2]
memory=8GB
```

Then `wsl --shutdown` and reopen.
:::

::: details Port 80 or 443 already in use
Usually a leftover nginx from Valet for Linux or a stale Docker Desktop service. Stop and disable the offending unit, then `lerd stop && lerd start`.
:::

## What about a native Windows build?

A native Windows port is not on the roadmap. The runtime depends on rootless Podman with Quadlet plus systemd user units, neither of which has a Windows equivalent. Maintaining a third fully separate runtime path next to Linux and the macOS launchd port is not realistic without sustained Windows-side help. WSL2 is the supported way to run lerd on Windows.
