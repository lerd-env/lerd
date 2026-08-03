# Installation

## Linux

::: warning Requires systemd
Lerd runs every container as a Podman Quadlet and every worker as a systemd user service, so a systemd-based distro is required. OpenRC (Gentoo, Artix-openrc, Alpine), runit (Void, Artix-runit), s6, and sysvinit-based distros (Devuan) are not supported.

Tested and known-good: Ubuntu, Fedora, Arch, Debian, Mint, Pop!_OS, openSUSE, CachyOS, Omarchy. Any systemd distro should work.
:::

### One-line installer (recommended)

::: code-group

```bash [curl]
curl -fsSL https://lerd.sh/install.sh | bash
```

```bash [wget]
wget -qO- https://lerd.sh/install.sh | bash
```

```bash [From source]
git clone https://github.com/lerd-env/lerd
cd lerd
make build
make install            # installs to ~/.local/bin/lerd
make install-installer  # installs lerd-installer to ~/.local/bin/
```

:::

The installer will:

- Check and offer to install missing prerequisites (Podman, NetworkManager, unzip)
- Download the latest `lerd` binary for your architecture (amd64 / arm64)
- Install it to `~/.local/bin/lerd`
- Add `~/.local/bin` to your shell's `PATH` (bash, zsh, or fish)
- Automatically run `lerd install` to complete environment setup

::: info Setup asks for sudo once, up front
Everything `lerd install` needs root for happens in one step at the very start, before any downloading or container work: the unprivileged-port sysctl so nginx can bind 80 and 443, systemd linger so your containers survive logout, and a passwordless sudoers rule for the DNS resolver operations. It runs as `sudo lerd bootstrap --system`, the same command the apt package runs as root, so both routes apply identical settings. The mkcert CA is trusted in the system store the same way once it has been generated.

Reinstalling for an update or a test reuses what is already in place and does not ask again, and if a step cannot run through `sudo` it falls back to prompting for each one separately. Uninstalling takes the sudoers rule and the CA back out, so both last exactly as long as lerd does.
:::

After install, reload your shell or open a new terminal so `PATH` takes effect.

`lerd install` will:

1. Check that the host ports lerd binds first (HTTP 80, HTTPS 443, DNS 5300) are free
2. Create XDG config and data directories
3. Create the `lerd` Podman network
4. Download static binaries: Composer, fnm, mkcert
5. Install the mkcert CA into your system trust store
6. Write and start the `lerd-dns` and `lerd-nginx` Podman Quadlet containers
7. Enable the `lerd-watcher` background service (auto-discovers new projects)
8. Add `~/.local/share/lerd/bin` to your shell's `PATH`

The downloaded tools are pinned to explicit versions, so a fresh install always gets the same Composer, fnm and mkcert regardless of what upstream shipped that day. The pins live in a small manifest published in the lerd repository: the binary fetches it before downloading and falls back to its embedded copy when offline, so a broken pin can be fixed for every install without waiting for a release. Downloads retry transient network and server errors with a short backoff, and a stalled transfer is cancelled and retried instead of hanging, so a momentary CDN hiccup doesn't abort the install. Already-installed tools are never touched by an upgrade; `lerd status` shows their versions and `lerd tools:update` brings them to the current pins when you want that.

::: info Running alongside Laravel Herd or another local stack
If another tool is already serving sites on ports 80/443 (Laravel Herd, a system nginx/Apache) or holding the DNS port, install prints a warning naming each busy port and how to find the process. Install still continues, so stop the other stack to free the ports first, otherwise `lerd-nginx` and `lerd-dns` will fail to start.
:::

---

### Install from a local build

If you built from source and want to skip the GitHub download:

```bash
make build
bash install.sh --local ./build/lerd
```

---

### Install via apt (Ubuntu/Debian)

Lerd is published to a Launchpad PPA, so you can install and update it with your package manager:

```bash
sudo add-apt-repository ppa:lerd/lerd
sudo apt update
sudo apt install lerd
```

The package installs the binary to `/usr/bin/lerd` and finishes setup automatically: its maintainer script enables the unprivileged-port sysctl and systemd linger, then runs `lerd install` for your user, so the stack comes up straight away and again at every boot. `.test` DNS and HTTPS are configured with no prompt because the package sets up the sudoers rule and trusts the mkcert CA as root.

Updates come through apt like any other package:

```bash
sudo apt upgrade
```

A package-installed lerd lives under `/usr`, so `lerd update` (which self-replaces a `~/.local/bin` install) detects it and defers to your package manager instead of fighting it.

The setup steps behind the package are not Debian-specific: `lerd bootstrap` recognises the Debian, Fedora, Arch and openSUSE trust store layouts and picks whichever the system uses, so the same flow will serve future rpm and AUR packages. On distros with no writable system trust store (NixOS), it prints where the CA lives so you can trust it declaratively.

It is also not package-specific. A normal `lerd install` runs the same `lerd bootstrap` steps through `sudo` rather than through a maintainer script, so the machine ends up in the same state however you installed.

---

### Update

```bash
lerd update
```

Fetches the latest release from GitHub, downloads the binary for your architecture, and atomically replaces the running binary. No restart needed.

You can also re-run the installer:

::: code-group

```bash [curl]
curl -fsSL https://lerd.sh/install.sh | bash -s -- --update
```

```bash [wget]
wget -qO- https://lerd.sh/install.sh | bash -s -- --update
```

:::

---

### Uninstall

```bash
lerd uninstall
```

Stops all containers, disables and removes Quadlet units, removes the watcher service, removes the binary, tears down the `lerd` podman network (including aardvark-dns runtime state), and cleans up the `PATH` entry from your shell config.

Four opt-in prompts before finishing:

1. **Remove all config and data**: deletes `~/.config/lerd` and `~/.local/share/lerd` (takes your `sites.yaml`, bundled binaries, TLS certs, and all service data with it). Global npm packages that the `npm` shim installed into lerd's managed prefix are not silently lost: when a system npm exists you're offered a reinstall into your own prefix first, and otherwise the exact `npm install -g …` line to run afterwards is printed.
2. **Remove MCP integration**: unregisters lerd from Claude Code, Cursor, Windsurf, and Junie at user scope, removes `~/.claude/skills/lerd/`, `~/.cursor/rules/lerd.mdc`, and strips the lerd block from `~/.junie/guidelines.md`. Also runs across every registered site to clean the same files per-project.
3. **Uninstall mkcert CA**: runs `mkcert -uninstall` so browsers and OS trust stores stop trusting the lerd CA that `install` originally added.
4. **Purge lerd-built container images**: removes `lerd-php*-fpm:local`, `lerd-custom-*:local`, and `lerd-dnsmasq:local`. Upstream pulled images (mysql/redis/postgres/etc.) are deliberately left alone; they're expensive to re-pull and your database/app data lives in host bind mounts, not inside the images, so nothing is lost by keeping them.

To answer yes to every prompt without interaction:

```bash
lerd uninstall --force
```

The installer's own `--uninstall` stops the user units and removes the binary, but the DNS setup lives outside your home directory and only lerd can take it back out: the `lerd0` link unit, the NetworkManager rules and dispatcher, the drop-in that empties `FallbackDNS`, and the passwordless sudoers rule the DNS operations run under. So when it finds that configuration it offers to run `lerd dns:disable` first, and prints the root commands to clear it by hand if you decline or the binary has already gone.

---

### Check prerequisites only

```bash
bash install.sh --check
```

---

## macOS

### One-line installer (recommended)

::: code-group

```bash [curl]
curl -fsSL https://lerd.sh/install.sh | bash
```

```bash [wget]
wget -qO- https://lerd.sh/install.sh | bash
```

:::

The same installer powers Linux and macOS. On macOS it will:

- Check for the `podman` CLI and offer to `brew install podman` if it's missing
- Download the latest `darwin` binary for your architecture (amd64 / arm64)
- Install it to `~/.local/bin/lerd` and add that directory to your `PATH`
- Automatically run `lerd install`, which starts Podman Machine, mkcert, DNS, and nginx

::: info Homebrew is only used for Podman
The installer itself doesn't require Homebrew. It's used only to install the `podman` dependency when it isn't already present, so you can also install Podman by any other means beforehand.
:::

### Install via Homebrew (alternative)

```bash
brew install lerd-env/lerd/lerd
lerd install
```

Podman is installed automatically as a Homebrew dependency.

::: warning Untrusted tap
Recent Homebrew versions refuse to load formulae from third-party taps until they're trusted. If you see `Refusing to load formula ... from untrusted tap`, run `brew trust lerd-env/lerd` once, then retry.
:::

### Update

```bash
lerd update
```

If you installed via Homebrew instead, update with `brew upgrade lerd && lerd install`.

If you're running a local development build (a `git describe` version like `1.25.0-6-g7d03`), the one-line installer and `--update` detect it and ask before replacing it with a release binary, so an ahead-of-release build isn't overwritten silently. Decline to keep your build, or reinstall one explicitly with `install.sh --local <path>`.

### Uninstall

```bash
lerd uninstall                                    # tears down launchd agents, DNS resolver, containers
curl -fsSL https://lerd.sh/install.sh | bash -s -- --uninstall
```

Run `lerd uninstall` first (while the binary is still present) so the DNS resolver and Podman state are cleaned up, then the installer's `--uninstall` removes the launchd agents and the binary. If you installed via Homebrew, finish with `brew uninstall lerd` instead of the second command. On macOS the installer detects when the binary is still present and pauses to remind you to run `lerd uninstall` first, since the DNS resolver (`/etc/resolver/test`, removed with sudo) and the Podman machine are unreachable once the binary is gone; if it can't reach a terminal it prints the manual removal commands at the end instead.

## Windows (beta)

There is no native Windows build. Lerd runs on Windows through WSL2, where the standard Linux build works unchanged once systemd and rootless Podman are set up. Windows support is **beta**, it works well for daily development but gets less testing than native Linux or macOS. See the [Windows (WSL2) guide](./wsl2) for the full walkthrough, including the `events_logger` Podman tweak and the mkcert root CA export to the Windows trust store.

## NixOS

NixOS's declarative model doesn't fit the one-line installer's imperative DNS and self-install steps, so the community [`lerd-nixos`](https://github.com/lerd-env/lerd-nixos) flake packages the `lerd` binary and provides the `configuration.nix` blocks the stack needs (rootless Podman, `*.test`-only DNS routing, the mkcert CA, and the systemd fixes for `lerd-ui` / `lerd-watcher`). See the [NixOS guide](./nixos) for the complete runbook from a fresh install.

## Desktop app (optional)

The dashboard runs in any browser, but [Lerd Desktop](https://github.com/lerd-env/lerd-desktop) wraps it in a dedicated window with [native desktop notifications](../features/notifications) for captured mail, worker failures and finished operations. It is optional and entirely separate from the lerd install itself, which keeps working unchanged without it.

It ships for Linux as a Flatpak:

```bash
flatpak install --user https://lerd.sh/lerd.flatpakref
```

Update it with `flatpak update`. Once it is installed, `lerd dashboard` and the tray's **Open Dashboard** open the app instead of a browser tab, and clicking a native notification opens it through its `lerd://` scheme. The one-line installer also offers to set it up for you on Linux.
