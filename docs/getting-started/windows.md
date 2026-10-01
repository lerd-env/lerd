---
title: Windows (native, experimental)
description: Run lerd on Windows without WSL2, using a Hyper-V Podman machine, a built-in DNS answerer and a Windows service manager.
---

# Windows (native, experimental)

Lerd builds and runs natively on Windows, with no WSL2 and no Linux distro to manage. Containers run in a Podman machine on Hyper-V, `.test` names are answered by a small DNS server built into lerd, and lerd's own processes are supervised by a Windows service manager instead of systemd or launchd.

::: warning Experimental
The native build is under active development and is **not** ready for daily use. Sites, workers and the PHP and Composer shims are not finished yet, see [What is missing](#what-is-missing). If you want something that works today, use [Windows (WSL2)](/getting-started/wsl2).
:::

## Requirements

- Windows 10 or 11 Pro, Enterprise or Education. Hyper-V is not available on Home.
- **Hyper-V** enabled. From an elevated PowerShell, then reboot:

  ```powershell
  Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All
  ```

- The **Podman CLI** (not Podman Desktop) on your `PATH`. Lerd drives `podman` and `podman machine` directly, and Podman Desktop would create its own machine on the WSL provider.
- An elevated PowerShell for the first `lerd install`. Creating a Hyper-V machine and writing the DNS rule both need administrator rights.

## How it works

| Piece | On Linux | On Windows |
| --- | --- | --- |
| Containers | rootless Podman | a rootful Podman machine on Hyper-V |
| Service supervision | systemd user units | a Windows service manager, unit definitions under the lerd data dir |
| `.test` DNS | dnsmasq container | `lerd dns-serve`, a built-in answerer on `127.0.0.1:53` |
| DNS routing | systemd-resolved | a DNS Client NRPT rule for `.test` |
| Login start | `lerd autostart` | a `Run` registry entry, on by default |
| Site paths in containers | the same path | `C:\Sites\app` becomes `/mnt/c/Sites/app` inside the machine |

Config lives under `%APPDATA%\lerd` and data under `%LOCALAPPDATA%\lerd`. Setting `XDG_CONFIG_HOME` or `XDG_DATA_HOME` overrides both, which is how the test suite isolates itself.

The DNS server reads the same `lerd.conf` a dnsmasq container would, so anything that rewrites that file keeps working. It answers `A` and `AAAA` for the configured TLD over UDP and TCP and refuses every other name, which is fine because the NRPT rule only sends `.test` queries to it.

## What is missing

- **Workers.** Queue, schedule, Horizon and the other framework workers are disabled on Windows. They bind-mount the site at its own path and run through shell guard scripts, which need the path mapping and a Windows script format.
- **Shims.** The `php`, `composer` and `node` shims are shell scripts and have no Windows form yet.
- **Tool downloads.** mise, mkcert and phpantom have no Windows builds wired in yet.
- **Scheduled workers.** There is no timer equivalent in the service manager.
- **Unverified path mapping.** The `/mnt/c` mapping matches Podman's own default mount, but it has had little testing across providers.

## Removing it

```powershell
lerd uninstall
podman machine rm -f
```

`lerd install` also writes `lerd-cleanup.ps1` next to the lerd binary, which removes containers, the login entry and the DNS rule if the binary is already gone.
