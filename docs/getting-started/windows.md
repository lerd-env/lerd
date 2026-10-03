---
title: Windows (native, experimental)
description: Run lerd natively on Windows, using a Podman machine on Hyper-V or WSL2, a built-in DNS answerer and a Windows service manager.
---

# Windows (native, experimental)

Lerd builds and runs natively on Windows, with no Linux distro to manage. Containers run in a Podman machine on Hyper-V, or on WSL2 where Hyper-V is unavailable, `.test` names are answered by a small DNS server built into lerd, and lerd's own processes are supervised by a Windows service manager instead of systemd or launchd.

::: warning Experimental
The native build is under active development and is **not** ready for daily use. Sites and workers are not finished yet, see [What is missing](#what-is-missing). If you want something that works today, use [Windows (WSL2)](/getting-started/wsl2).
:::

## Requirements

- Windows 10 or 11. Lerd creates its Podman machine on **Hyper-V** when the host has it, and falls back to **WSL2** when it does not, which covers Windows Home.
- For Hyper-V (Pro, Enterprise or Education), enable the feature from an elevated PowerShell, then reboot:

  ```powershell
  Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All
  ```

- For the WSL2 fallback, install WSL from an elevated PowerShell, then reboot. Podman brings its own distro for the machine, so no Ubuntu is needed:

  ```powershell
  wsl --install --no-distribution
  ```

- The **Podman CLI** (not Podman Desktop). `lerd install` installs it for you when it is missing, see [Podman](#podman). Lerd drives `podman` and `podman machine` directly, and Podman Desktop would create a machine of its own.
- An elevated PowerShell for the first `lerd install`. Writing the DNS rule needs administrator rights, and so does creating a Hyper-V machine.

You do not have to work out which of these applies. On its first run `lerd install` checks the Windows edition and which backends are installed before it changes anything. If neither is ready it stops and prints the command for the one your edition supports, Hyper-V on Pro, Enterprise and Education with WSL2 as the alternative, WSL2 alone on Home, and asks you to reboot and run `lerd install` again. The same happens when WSL is installed but Windows cannot start virtual machines because the Host Compute Service is missing, in which case it prints the commands that repair the Virtual Machine Platform.

### Podman

When `podman` is not on the `PATH`, lerd first looks in the folders Podman's installers use, `%LOCALAPPDATA%\Programs\Podman` and `%ProgramFiles%\RedHat\Podman`, so a terminal opened before Podman was installed still finds it. When there is none, `lerd install` downloads the Podman MSI it pins, checks its sha256 and installs it silently with `msiexec`. The MSI installs for the current user, needs no elevation and adds Podman to your user `PATH` for new terminals. The installer's log is written to `%LOCALAPPDATA%\lerd\logs\podman-install.log`. If the install fails, lerd stops with the reason and the `winget install RedHat.Podman` command to install it yourself.

### Choosing the provider

When both are ready, Hyper-V is used. When WSL2 is already installed and the edition supports Hyper-V but it is not enabled, `lerd install` lays out both options and asks which to use, recommending Hyper-V:

| | Hyper-V (recommended) | WSL2 |
| --- | --- | --- |
| Getting started | enable the feature and reboot first | works right away |
| Creating the machine | needs an elevated shell | no elevation |
| Isolation | its own VM, `wsl --shutdown` leaves your sites running | shares the WSL2 VM, `wsl --shutdown` stops your sites |
| Memory | sized by lerd for the PC | set by your `.wslconfig` |

Choosing Hyper-V stops the install with the command to enable it, then you reboot and run `lerd install` again. Choosing WSL2 carries on and is saved, so you are not asked again. With no terminal to ask on, as in a scripted install, lerd carries on with WSL2.

### WSL 3 and cgroups

WSL 3 places the machine in a cgroup that does not hand the `pids` controller down, so with Podman's default systemd cgroup manager no container can start, failing with `crun: controller 'pids' is not available` ([podman#29749](https://github.com/podman-container-tools/podman/issues/29749)). On a WSL machine, `lerd install` and `lerd start` start a throwaway container first, and when it fails this way they switch the machine to the `cgroupfs` manager with a drop-in at `/etc/containers/containers.conf.d/90-lerd-wsl-cgroupfs.conf`. The drop-in lives on the machine's disk, so it is applied once, and a machine that runs containers fine is left untouched.

### Older WSL kernels

Podman 6 sets up container networks with netavark 2, whose nftables rules need the kernel's `NFT_FIB_INET` support. WSL kernels before 6.18 (WSL 2.x) are built without it, so every container fails with `netavark (exit code 1): nftables error: "nft" did not return successfully while applying ruleset`, image builds included. netavark 2 has no iptables backend to fall back on. The same throwaway container catches this, and `lerd install` and `lerd start` stop with the fix: run `wsl --update`, then `wsl --shutdown`, then `lerd install` again. WSL 3 ships a 6.18 kernel that works.

Lerd saves the provider it created the machine with as `machine.provider` in the global config, so it keeps using that machine even if Hyper-V is turned on or off later. To pick one yourself before the first install, set `CONTAINERS_MACHINE_PROVIDER` to `hyperv` or `wsl`, or set `machine.provider` in the config. Any other value is refused.

## How it works

| Piece | On Linux | On Windows |
| --- | --- | --- |
| Containers | rootless Podman | a rootful Podman machine on Hyper-V, or WSL2 without it |
| Service supervision | systemd user units | a Windows service manager, unit definitions under the lerd data dir |
| `.test` DNS | dnsmasq container | `lerd dns-serve`, a built-in answerer on `127.0.0.1:53` |
| DNS routing | systemd-resolved | a DNS Client NRPT rule for `.test` |
| Login start | `lerd autostart` | a `Run` registry entry, on by default and removed by `lerd autostart disable` |
| Shims on `PATH` | sh scripts, `PATH` set in the shell rc | `lerd.exe`, `php`, `composer`, `laravel` and the node shims in `%LOCALAPPDATA%\lerd\bin`, as `.cmd` for cmd and PowerShell and as sh scripts for Git Bash, added to the user `PATH` in the registry |
| Site paths in containers | the same path | `C:\Sites\app` becomes `/mnt/c/Sites/app` inside the machine |
| Drive sharing (Hyper-V) | not needed | `lerd p9-serve` in place of Podman's 9p server |

Config lives under `%APPDATA%\lerd` and data under `%LOCALAPPDATA%\lerd`. Setting `XDG_CONFIG_HOME` or `XDG_DATA_HOME` overrides both, which is how the test suite isolates itself.

`lerd install` can be run from wherever you downloaded `lerd.exe`. It copies itself, and `lerd-tray.exe` when that sits beside it, into `%LOCALAPPDATA%\lerd\bin` and finishes the install from there, so the services and shims point at a copy that stays put and the download can be deleted. A copy that is running is renamed to `lerd.exe.old-<time>` first, since Windows will not overwrite it, and the next install clears those.

Open a new terminal after `lerd install` so it picks up the `PATH` change. If `node` still runs a system install, a machine-wide `PATH` entry is ahead of the user one; `lerd doctor` flags it.

S3 signs every request with the current time, and RustFS refuses one more than 15 minutes off its own clock. When the Windows clock has drifted, `lerd env` reports that instead of a bare "Access Denied"; syncing the system time fixes it.

### Drive sharing on Hyper-V

A Hyper-V machine reaches `C:\` and your home folder through 9p, served on the Windows side by `podman machine server9p`. That server is built on hugelgupf/p9 v0.4.1, whose Windows backend keeps a handle open on every file it looks up and cannot replace an existing file, rename a folder that holds one, append to a file, lock one or set its times. In practice `composer install`, `lerd new`, Laravel's caches and log, and any SQLite database in the project fail, and the leaked handles make every request slower until the machine restarts.

`lerd start` swaps that server for its own on a Hyper-V machine. It stops lerd's containers, unmounts the shares inside the VM, stops Podman's server, starts `lerd p9-serve` with the same arguments on the same hvsock services, and mounts the shares again with Podman's own `client9p`. `lerd p9-serve` is built on a fork of hugelgupf/p9 that carries the fixes, which are on their way upstream (hugelgupf/p9#114). If Podman's server takes arguments lerd does not recognise, lerd leaves it running and warns; if its own server does not come up, it puts Podman's back. The server logs to `%LOCALAPPDATA%\lerd\logs\p9-serve.log` and exits with the machine. A machine started with `podman machine start` alone keeps Podman's server until the next `lerd start`.

NTFS has no POSIX owners or group and other bits, so the server reports files as `0644` and folders as `0755`, the way a Linux system with umask 022 creates them. Programs that refuse world-writable files, like MySQL with its config, accept them, and containers write as root, as they do on Linux.

WSL2 machines share the drives differently and are left alone.

### OPcache

Every file a request touches lives on that 9p share, and OPcache's default revalidation stats each one again once two seconds have passed, which is nearly every request while you code. On a Laravel app that is thousands of round trips to Windows and about 1.7 seconds a page. On Windows lerd starts PHP-FPM with `opcache.validate_timestamps=0` and drops cached files itself instead: when the watcher sees a save under a site's source directories, and after `lerd artisan`, it invalidates every cached script outside `vendor/`, which also covers templates and caches the framework compiles from your code; after `lerd composer` it clears the whole cache. A change shows up within about a second and a page takes around 0.15 seconds. FrankenPHP sites keep revalidating, since lerd has no way to reach their cache.

Template engines decide whether to recompile by comparing file times, so the Windows clock and the machine's have to agree. `lerd start` compares the two and warns when they are more than a minute apart; sync the Windows time when it does.

The DNS server reads the same `lerd.conf` a dnsmasq container would, so anything that rewrites that file keeps working. It answers `A` and `AAAA` for the configured TLD over UDP and TCP and refuses every other name, which is fine because the NRPT rule only sends `.test` queries to it.

## What is missing

- **Workers.** Queue, schedule, Horizon and the other framework workers are disabled on Windows. They bind-mount the site at its own path and run through shell guard scripts, which need the path mapping and a Windows script format.
- **Tool downloads.** phpantom has no Windows build wired in yet, so tinker autocomplete is unavailable.
- **Scheduled workers.** There is no timer equivalent in the service manager.
- **Unverified path mapping.** The `/mnt/c` mapping matches Podman's own default mount, but it has had little testing on either provider.

## Removing it

```powershell
lerd uninstall
podman machine rm -f
```

`lerd uninstall` removes the login entry and the `PATH` entry too. Windows will not delete the running `lerd.exe`, so the binary and the data directory that holds it are removed a few seconds after the command exits.

`lerd install` also writes `lerd-cleanup.ps1` next to the lerd binary, which removes containers, the login entry and the DNS rule if the binary is already gone.
