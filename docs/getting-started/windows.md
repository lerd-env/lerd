---
title: Lerd for Windows
description: Install Lerd on Windows with one installer. Lerd runs inside WSL2 and gives Windows browsers, terminals and IDEs .test domains, trusted HTTPS, PHP and Composer.
---

# Lerd for Windows

Lerd runs on Windows through WSL2 (Windows 10 version 2004 or newer, Windows 11 22H2 or newer recommended, since older versions have no mirrored networking and `.test` sites may not open from Windows), and `Install-Lerd.exe` sets the whole thing up for you: WSL, an Ubuntu distro, Podman, Lerd and the Windows integration. Your sites then open at `https://<site>.test` in Windows browsers, with a trusted certificate, and `lerd`, `php` and `composer` work from any Windows terminal.

## Install

Pick one:

- **Installer.** Download [Install-Lerd.exe](https://github.com/lerd-env/lerd-desktop/releases/latest/download/Install-Lerd.exe) and run it. It asks for a Linux username and password, restarts Windows once if WSL needs enabling, and carries on by itself after you sign back in.
- **PowerShell.** Run this in a normal (not administrator) PowerShell window. It takes the same steps and asks for administrator rights only where Windows needs them:

  ```powershell
  irm https://lerd.sh/install.ps1 | iex
  ```

- **Existing WSL distro.** Inside Ubuntu 22.04+ or Debian 12+ with systemd, install Lerd the Linux way, then run `lerd wsl:setup` to wire it into Windows:

  ```bash
  curl -fsSL https://lerd.sh/install.sh | bash
  lerd wsl:setup
  ```

Running the installer again after a failure resumes where it stopped. Lerd is removed from Add/Remove Programs like any other app.

## How it works on Windows

Lerd is built on systemd user services and rootless Podman, which only exist on Linux, so on Windows it runs the standard Linux build inside WSL2 rather than a separate Windows port. What you get on the Windows side:

- `.test` domains, wildcards included, resolved for every Windows browser.
- Lerd's certificate authority trusted by Windows, so HTTPS shows no warnings.
- `lerd.exe`, `php.exe`, `composer.exe` and `node.exe` on your PATH, ready for PhpStorm or VS Code.
- The Lerd desktop app in the tray, starting Lerd at login.
- Database clients such as TablePlus or HeidiSQL connecting on `127.0.0.1`.

Keep projects on the Linux disk (`\\wsl.localhost\Ubuntu\home\<you>`), not under `C:\`, where file access is many times slower.

## Next steps

- [Windows (WSL2) guide](./wsl2) documents every step the installer takes, for doing it by hand or fixing a drifted setup.
- [Quick Start](./quick-start) links your first site.
- [Uninstall](/getting-started/uninstall#windows-wsl2) removes Lerd from Windows.
- Follow new releases on the [Lerd desktop releases page](https://github.com/lerd-env/lerd-desktop/releases) and the [changelog](/changelog).
