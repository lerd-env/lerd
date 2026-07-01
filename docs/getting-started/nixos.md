# NixOS

Lerd runs on NixOS through the community [`lerd-nixos`](https://github.com/lerd-env/lerd-nixos) flake, which packages the `lerd` binary and documents the handful of system settings the stack needs.

Lerd runs PHP-FPM, nginx, a DNS resolver, and services (MySQL, Redis, …) as rootless Podman containers managed by systemd user services (Quadlets), plus two helpers (`lerd-ui`, `lerd-watcher`) that run the host `lerd` binary. That design assumes a Debian/Ubuntu-style host, so on NixOS a few declarative settings replace the imperative setup the installer does elsewhere. This page is a complete, NixOS-only runbook from a fresh install.

::: warning Community-maintained
The `lerd-nixos` flake lives under the [lerd-env](https://github.com/lerd-env) organisation and adapts lerd to NixOS's declarative model. It's built and tested on `x86_64-linux`. The DNS, cert, and systemd integration below is what makes lerd coexist with NixOS cleanly, and it isn't part of the standard installer.
:::

## Try it without installing

```bash
nix run github:lerd-env/lerd-nixos -- --help
```

## Add it to your flake

The flake exposes a package (`packages.x86_64-linux.lerd`, also `default`) and an overlay (`overlays.default`).

### 1. Add the input

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    lerd = {
      url = "github:lerd-env/lerd-nixos";
      # Build lerd against your own nixpkgs instead of the one it pins,
      # so you don't download a second copy of nixpkgs:
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, lerd, ... }: {
    nixosConfigurations.nixos = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        ./configuration.nix
        { environment.systemPackages = [ lerd.packages.x86_64-linux.default ]; }
      ];
    };
  };
}
```

If your `environment.systemPackages` lives inside a module file instead of inline, pass `lerd` through `specialArgs = { inherit lerd; };` and reference `lerd.packages.${pkgs.system}.default` there.

### Or use the overlay

```nix
{ pkgs, lerd, ... }:
{
  nixpkgs.overlays = [ lerd.overlays.default ];
  environment.systemPackages = [ pkgs.lerd ];
}
```

## NixOS system configuration

Add the following to `configuration.nix`. Each block is explained in the comments, most of it is required, and the conditional items are called out as such. Replace `youruser` with your username throughout.

```nix
{ config, pkgs, ... }:

{
  # 1. Rootless Podman — lerd runs everything in containers.
  virtualisation.podman.enable = true;
  virtualisation.containers.enable = true;

  # 2. Move Podman's default subnet pool off 10.x.
  #    REQUIRED ONLY IF a route on your machine claims 10.0.0.0/8 (common with
  #    corporate VPNs — check `ip route`). Podman's default pool lives in 10.x,
  #    and an overlapping route makes network creation fail with
  #    "could not find free subnet from subnet pools". Harmless to keep even
  #    without a VPN.
  virtualisation.containers.containersConf.settings.network.default_subnet_pools = [
    { base = "172.20.0.0/16"; size = 24; }
  ];

  # 3. Let rootless nginx bind 80/443. Without this, lerd asks for sudo to set
  #    the sysctl at runtime on every install; declaring it makes it permanent.
  boot.kernel.sysctl."net.ipv4.ip_unprivileged_port_start" = 80;

  # 4. Keep lerd's user containers alive outside an active graphical session
  #    (otherwise systemd-logind tears them down on lock/logout).
  users.users.youruser.linger = true;

  # 5. DNS for *.test — owned by NixOS, NOT by lerd (see notes below).
  #    Routes ONLY *.test to lerd's DNS container on 127.0.0.1:5300; everything
  #    else stays on your normal resolver, so a stopped lerd-dns can only ever
  #    break .test, never the whole internet.
  services.resolved.enable = true;
  services.resolved.settings.Resolve = {
    DNS = "127.0.0.1:5300";
    Domains = [ "~test" ];
  };
  networking.networkmanager.dns = "systemd-resolved";  # if you use NetworkManager

  # 6. Trust lerd's mkcert root CA system-wide (curl, PHP, Node, …).
  #    The file doesn't exist yet on a fresh install — add this line AFTER the
  #    "First-time lerd setup" step below generates and copies it in.
  security.pki.certificateFiles = [ ./certs/lerd-rootCA.pem ];

  # 7. Point lerd's host-binary services at the Nix profile.
  #    lerd-ui and lerd-watcher exec `~/.local/bin/lerd`, where lerd would
  #    self-install on other distros. On NixOS the binary comes from the Nix
  #    profile, so without this symlink both fail with status=203/EXEC.
  #    /run/current-system/sw/bin/lerd tracks the current generation, so this
  #    survives nixos-rebuild and lerd updates.
  systemd.user.tmpfiles.rules = [
    "L %h/.local/bin/lerd - - - - /run/current-system/sw/bin/lerd"
  ];

  # 8. OPTIONAL host PHP + Composer (for editor tooling and `lerd new`'s
  #    initial scaffold). lerd serves your app's PHP from containers regardless
  #    of this — pin the per-project version with `lerd isolate <ver>`.
  environment.systemPackages = with pkgs; [ php84 php84Packages.composer ];
}
```

### Why DNS is configured this way

This is the part that bites people. Lerd is built for Debian/Ubuntu/Arch and, by default, **imperatively rewrites your system resolver** (a systemd-resolved drop-in and/or a NetworkManager dispatcher script) to point *all* DNS at its container. On those distros it works; on NixOS it fights the declarative config and, if the lerd-dns container isn't up, can take down **all** name resolution (you'll see `Could not resolve host: cache.nixos.org` even during `nixos-rebuild`).

The config above sidesteps that: **NixOS** owns the resolver and routes only `~test` to lerd-dns. Because that routing lives in the main `resolved.conf` (which lerd never edits), it's the stable anchor. The consequence is that you should **decline** lerd's offer to configure DNS, see the next section.

## First-time lerd setup

Do this once, in order. You need working internet DNS first, and if it's already broken from an earlier attempt, see [DNS is completely broken](#dns-is-completely-broken).

1. **Apply the system config** (without the cert line #6 yet, since the file doesn't exist):

   ```bash
   sudo nixos-rebuild switch
   ```

2. **Run the installer.** Use `--no-ipv6`, it keeps the lerd network IPv4-only, which avoids a second class of subnet-pool errors on the IPv6 dual-stack bridge:

   ```bash
   lerd install --no-ipv6
   ```

   This generates the mkcert root CA at `~/.local/share/mkcert/rootCA.pem`, writes the container Quadlets, and starts dns/nginx/php-fpm.

   ::: warning Decline the DNS prompt
   When it prints **"Configuring NetworkManager dispatcher for .test DNS resolution"** and asks for sudo, press **Ctrl+C to decline.** Your NixOS config (block #5) already resolves `.test`, so lerd doesn't need to touch DNS. Declining keeps it out of your resolver permanently.
   :::

3. **Trust the CA.** Copy it into your config repo and enable line #6:

   ```bash
   mkdir -p /etc/nixos/certs   # or wherever your flake lives
   cp ~/.local/share/mkcert/rootCA.pem /etc/nixos/certs/lerd-rootCA.pem
   ```

   Uncomment/add `security.pki.certificateFiles = [ ./certs/lerd-rootCA.pem ];`, then:

   ```bash
   sudo nixos-rebuild switch
   ```

   The CA cert is public and safe to commit; the matching **private key** stays in `~/.local/share/mkcert/`, do not copy or commit it.

4. **Verify** the whole stack:

   ```bash
   resolvectl query app.test                       # → 127.0.0.1
   getent hosts cache.nixos.org                     # internet still resolves
   curl -sI https://lerd.test                       # TLS verifies against system CA
   systemctl --user is-active lerd-ui lerd-watcher  # → active (thanks to block #7)
   ```

Firefox/Chromium use their own certificate stores. Many pick up the mkcert CA automatically; if a browser still warns, set `security.enterprise_roots.enabled` to `true` in `about:config` (it then reads the system store), or import `certs/lerd-rootCA.pem` under Settings → Certificates → Authorities.

## Create a Laravel project

```bash
lerd new myapp        # composer create-project laravel/laravel ./myapp
cd myapp
lerd isolate 8.4      # pin PHP version for this project (optional; 8.5 also available)
lerd link             # register ./myapp → myapp.test
lerd setup            # composer/npm install, .env, key, migrate, assets
lerd open             # open https://myapp.test
```

Handy follow-ups: `lerd which` (resolved PHP/docroot), `lerd php artisan …`, `lerd db:create`, `lerd db:shell`, `lerd logs`, `lerd tui`.

## Troubleshooting

### "could not find free subnet from subnet pools"

A route on your machine (usually a VPN) overlaps Podman's default `10.x` pool. Confirm with `ip route` (look for a `10.0.0.0/8` line), then apply block #2 above and rebuild. If a half-created network is left over, run `podman network rm lerd` before re-running `lerd install --no-ipv6`.

### DNS is completely broken

Symptom: nothing resolves (`Could not resolve host: cache.nixos.org`), often right after a lerd command or reboot, and possibly blocking `nixos-rebuild`. This is lerd's imperative DNS setup. Recover at runtime:

```bash
# Remove lerd's resolver hooks
sudo rm -f /etc/systemd/resolved.conf.d/lerd.conf \
           /etc/NetworkManager/conf.d/lerd.conf \
           /etc/NetworkManager/dnsmasq.d/lerd.conf \
           /etc/NetworkManager/dispatcher.d/99-lerd-dns

# Reset and restart whichever resolver you run
sudo resolvectl revert <iface>          # e.g. enp6s0; ignore errors if resolved is off
sudo systemctl restart systemd-resolved || sudo systemctl restart NetworkManager

getent hosts cache.nixos.org
```

If it's still down and you need to rebuild, force a resolver temporarily:

```bash
sudo rm -f /etc/resolv.conf
printf 'nameserver 192.168.0.1\nnameserver 8.8.8.8\n' | sudo tee /etc/resolv.conf
```

The permanent fix is the NixOS DNS config (block #5) plus declining lerd's DNS prompt. Once that's in place this shouldn't recur. The only things that re-touch DNS are `lerd install` / `lerd start` (decline the prompt) and the lerd watcher (which only acts when `.test` is already broken).

### "could not find … lerd-nginx" on `lerd link`

`lerd link` only *reloads* nginx; the container is created by `lerd install`. If an earlier install failed partway, re-run `lerd install --no-ipv6` (idempotent), then `lerd start`.

### `lerd-ui` / `lerd-watcher` fail with status=203/EXEC

Their unit templates hardcode `ExecStart=%h/.local/bin/lerd …`, which doesn't exist on NixOS. Block #7 fixes it declaratively. To apply without a reboot:

```bash
mkdir -p ~/.local/bin
ln -sf /run/current-system/sw/bin/lerd ~/.local/bin/lerd
systemctl --user reset-failed lerd-ui lerd-watcher   # clears the start-limit lockout
systemctl --user restart lerd-ui lerd-watcher
systemctl --user is-active lerd-ui lerd-watcher       # → active
```

Editing the unit files instead won't survive, `lerd install` regenerates them from embedded templates, so the symlink is the durable fix.

### A container won't start (`start lerd-… failed`)

Check the user service directly:

```bash
systemctl --user status lerd-<name> --no-pager
journalctl --user -xeu lerd-<name>.service --no-pager -n 50
```

Most "failed to start" cases on a fresh install trace back to missing **linger** (block #4) or the **unprivileged port** sysctl (block #3).
