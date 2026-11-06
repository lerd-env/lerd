---
title: Uninstall Lerd
description: How to uninstall Lerd completely on Linux, macOS and Windows (WSL2), what lerd uninstall removes, what it keeps, and how to clean up what is left.
---

# Uninstall Lerd

To uninstall Lerd, run `lerd uninstall`. It stops every container, removes the systemd or launchd units, the DNS setup, the binary and its `PATH` entry, then asks whether to delete your data, the MCP integration, the local CA and the images Lerd built. Your project folders are never touched.

```bash
lerd uninstall              # asks before each optional step
lerd uninstall --force      # yes to everything, a full cleanup
lerd uninstall --keep-data  # no prompts, keeps config, databases and images for a reinstall
```

Without a terminal and without one of those flags, `lerd uninstall` stops before changing anything, so a script never removes Lerd by accident.

## What the prompts remove

1. **Config and data**: `~/.config/lerd` and `~/.local/share/lerd`, which hold `sites.yaml`, certificates, bundled binaries and every service's databases. Global npm packages installed through Lerd's `npm` are offered a reinstall into your own prefix first.
2. **MCP integration**: Lerd's registration in Claude Code, Cursor, Windsurf and Junie, plus the skill and rule files it wrote, globally and in every linked site.
3. **The Lerd CA**: removed from the system and browser trust stores. The CA files themselves stay in the CA directory.
4. **Lerd-built images**: `lerd-php*-fpm:local` and `lerd-custom-*:local`. Images pulled from a registry, such as MySQL or Redis, stay, since your data lives in bind mounts rather than in them.

`--force` answers yes to all four. `--keep-data` answers no to 1 and 4 and yes to 2 and 3.

## Linux

It stops the watcher before it touches DNS, since the watcher would otherwise repair the setup it sees disappear, and if the watcher cannot be stopped, the uninstall ends there without changing anything. `lerd uninstall` also removes the `lerd` Podman network, the watcher, the desktop entry, the `lerd0` DNS interface and its NetworkManager rule, and `/etc/sysctl.d/99-lerd-ports.conf`. It restores systemd-resolved's fallback DNS servers if Lerd turned them off. On Omarchy it takes the Lerd Glance widget off the bar.

If Lerd came from a package, the binary stays for the package manager to remove, and `lerd uninstall` prints the command:

```bash
sudo apt remove lerd    # Debian, Ubuntu
sudo dnf remove lerd    # Fedora
```

On Fedora and Ubuntu 26.04, which build the initramfs with dracut, run the `sudo dracut -f` it prints so the lowered port setting stops applying at boot.

## macOS

`lerd uninstall` removes the launchd agents, the `.test` resolver, the containers, the Lerd app in `~/Applications` and the binary. With Homebrew, finish with:

```bash
brew uninstall lerd
```

The Podman machine stays. Remove it with `podman machine rm` if nothing else uses it.

## Windows (WSL2)

Run `lerd uninstall` inside the distro. It also cleans the Windows side: `lerd.exe` and the `php`, `composer` and `node` shims with their `PATH` entry, the start at login, the Quick access pins, the desktop app's autostart and `lerd://` handler, and, in one administrator prompt, the `.test` DNS rule and the `lerd LAN` firewall rule.

Two things are left for you:

- **The Lerd CA** in the Windows trust store. Open `certmgr.msc`, go to **Trusted Root Certification Authorities**, and delete it.
- **The desktop app** in `C:\Program Files\Lerd`. If you used `Install-Lerd.exe`, remove **Lerd** from Add/Remove Programs, which can also delete the WSL distro. After an `install.ps1` install, delete the folder yourself.

To remove the distro and everything in it, run `wsl --unregister <distro>` from PowerShell, for example `Ubuntu-26.04`. That deletes your projects inside it too.

## Leftover files

If Podman was already gone when you uninstalled, service data written inside the rootless namespace can survive. [Uninstall leaves the data directory behind](/troubleshooting#uninstall-leaves-the-data-directory-behind) shows how to remove it. Containers and images in Podman that Lerd did not build are never removed.

To install again later, follow [Installation](/getting-started/installation). A `--keep-data` uninstall brings your sites and databases back as they were.
