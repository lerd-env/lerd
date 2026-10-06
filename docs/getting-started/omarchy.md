---
title: PHP development on Omarchy
description: 'Omarchy ships an opinionated Arch and Hyprland desktop, but no PHP stack. Lerd adds one: automatic .test domains, HTTPS, per-project PHP 7.4 to 8.5, shared MySQL, PostgreSQL and Redis, rootless Podman, plus a native Omarchy bar widget.'
head:
  - - script
    - type: application/ld+json
    - |
      {
        "@context": "https://schema.org",
        "@type": "FAQPage",
        "mainEntity": [
          {
            "@type": "Question",
            "name": "Does Omarchy come with a PHP development environment?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "No. Omarchy gives you the desktop, the terminal and the editor, but there is no bundled web server, no PHP-FPM and no local domain routing. You install a PHP environment on top of it. Lerd is built for exactly that: one install gives every project a .test domain, HTTPS, its own PHP version and the databases it needs."
            }
          },
          {
            "@type": "Question",
            "name": "How do I install Lerd on Omarchy?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Run curl -fsSL https://lerd.sh/install.sh | bash. Omarchy is Arch-based, so the installer takes the Arch path: it checks for Podman 4.5 or newer, offers to install the missing prerequisites, wires the system resolver for .test domains and installs the binary to ~/.local/bin/lerd. Then run lerd link inside a project."
            }
          },
          {
            "@type": "Question",
            "name": "Does the Lerd tray icon work on Omarchy's Hyprland?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Yes. Lerd's tray unit is wired to graphical-session.target, which Omarchy reaches because it launches Hyprland through uwsm. Bare Hyprland started without uwsm never reaches that target and needs a one-line change to the unit."
            }
          },
          {
            "@type": "Question",
            "name": "Is there an Omarchy bar widget for Lerd?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "Yes. lerd Glance is an optional plugin for the Omarchy Quattro shell that puts sites, services, workers, DNS and container memory in the desktop bar, with a coloured dot only when something needs attention. Install it with omarchy plugin add https://github.com/lerd-env/lerd-omarchy-glance.git --enable."
            }
          },
          {
            "@type": "Question",
            "name": "Does Lerd need Docker on Omarchy?",
            "acceptedAnswer": {
              "@type": "Answer",
              "text": "No. Lerd runs nginx, PHP-FPM and services as rootless Podman containers under your own user account, managed by systemd user units. Nothing runs as root and there is no daemon. If you already use Docker on Omarchy for other work, the two coexist."
            }
          }
        ]
      }
---

# PHP development on Omarchy

[Omarchy](https://omarchy.org) hands you a finished Arch and Hyprland desktop: the terminal, the editor, the keybindings and the theming are all decided for you. What it deliberately does not decide is your web stack. There is no nginx, no PHP-FPM, no `.test` routing and no local TLS, so the first day of any PHP work on a fresh Omarchy box is the one part of the setup that is still yours to solve.

**Lerd is that missing half.** One install, and every project gets a `.test` URL, a trusted certificate, the PHP version it asks for and the databases it needs, all as rootless Podman containers under your own user. It fits the Omarchy shape: no sudo after install, nothing written into system directories, everything under `~/.config` and `~/.local/share`, and a bar widget so the state of the stack is on screen instead of behind a command.

```bash
curl -fsSL https://lerd.sh/install.sh | bash
cd ~/code/myapp
lerd link
```

Your project is live at `https://myapp.test`. That is the whole setup.

## Why it suits Omarchy

| What Omarchy already gives you | What Lerd adds on top |
|---|---|
| Arch with a curated package set, Hyprland launched through `uwsm` | nginx, PHP-FPM and services as rootless Podman containers, managed by systemd user units |
| A terminal, an editor, and language toolchains you install yourself | PHP 7.4 and 8.0 to 8.5, picked per project by `lerd isolate 8.4` or read from `composer.json` |
| No local web server, no vhost management | An nginx vhost generated on `lerd link`, with [overrides](/usage/nginx-overrides) when a project needs them |
| No local domain routing | Automatic `.test` domains through lerd's built-in DNS server wired into systemd-resolved, no `/etc/hosts` edits |
| No local TLS | `lerd secure`, a real mkcert certificate trusted by your system and browsers |
| Docker available if you install it | Rootless Podman, no daemon, no root, no `docker-compose.yml` per project |
| A themed bar with a plugin system | A [native bar widget](/features/omarchy-glance) showing sites, services, workers, DNS and container memory |
| A tray that follows `graphical-session.target` | A [system tray](/features/system-tray) that autostarts there, because Omarchy runs Hyprland under `uwsm` |

## Installing on Omarchy

Omarchy is Arch-based, so the installer takes the Arch path and Podman is already new enough. Arch ships podman 4.5 or newer out of the box, which is Lerd's [minimum](/getting-started/requirements#podman-4-5-minimum).

```bash
curl -fsSL https://lerd.sh/install.sh | bash
```

The installer checks the prerequisites, offers to install anything missing, points the system resolver at the `.test` domains (the one place it needs `sudo`) and installs the binary to `~/.local/bin/lerd`. Nothing goes into `/usr/local`.

Two Arch defaults are worth knowing about, because they differ from the Debian and Fedora side:

**`crun` is not the default runtime.** Arch defaults to `runc`. Both work, but `crun` is lighter and purpose-built for rootless containers, and `lerd doctor` will tell you it is missing:

```bash
sudo pacman -S crun
```

**`nss` provides `certutil`.** mkcert needs it to install the CA into Chrome and Firefox, so `lerd secure` produces a certificate the browser accepts with no warning page:

```bash
sudo pacman -S nss
```

Then start it and check the environment:

```bash
lerd start
lerd doctor
```

`lerd doctor` is the fastest way to confirm the resolver, the trust store, linger and the container runtime all landed correctly. If `systemctl --user` units do not survive logout, run `loginctl enable-linger $USER` once.

## The bar widget

[lerd Glance](/features/omarchy-glance) is an optional plugin for the Omarchy Quattro shell. It carries Lerd's mark in the bar and stays quiet while everything is healthy, showing a coloured dot only when a service, a worker or DNS needs attention.

```bash
omarchy plugin add https://github.com/lerd-env/lerd-omarchy-glance.git --enable
```

Clicking opens a panel with the site and service counts, one row per worker type, nginx and `.test` resolution status, the running PHP versions, the containers with their versions and ports, and total CPU and memory across the whole environment. Two buttons: open the dashboard, and clean up reclaimable disk space when there is any.

It reads the local dashboard API on `127.0.0.1:7073` and sends nothing anywhere else.

## The tray, and the `uwsm` detail

Lerd's tray unit is wired to `graphical-session.target`. Omarchy launches Hyprland through `uwsm`, which reaches that target on login, so [the tray](/features/system-tray) autostarts with no extra work.

This is worth knowing if you ever move off Omarchy's session setup: bare Hyprland, Sway or i3 started without `uwsm` never reaches `graphical-session.target`, and the tray will not autostart. Either run the compositor under `uwsm`, or change `WantedBy=graphical-session.target` to `WantedBy=default.target` in `~/.config/systemd/user/lerd-tray.service`. Every other Lerd unit uses `default.target` and is unaffected either way.

## `.test` domains on a systemd-resolved-only host

Omarchy uses systemd-resolved without NetworkManager, which used to be the awkward case: `.test` resolution could stop working when there was no network link for systemd-resolved to hang the route on.

Lerd now keeps an always-up dummy interface, `lerd0`, that carries the `~test` route. Because that link never goes down, systemd-resolved keeps forwarding `.test` to the Lerd resolver even with no network connection at all. It is created by a small system service, `lerd-dns-link.service`, which starts on every boot, so it survives reboots and applies on your next `lerd start` with nothing to run by hand.

If you would rather not touch the system resolver at all, pick the `.localhost` mode at install time. `.localhost` resolves to loopback by convention, so no resolver configuration is involved and the `dnsmasq` and `certutil` prerequisites are skipped entirely.

## A first project

```bash
lerd new myapp          # scaffolds through the framework's own installer, then links it
cd myapp
lerd service start mysql
lerd env                # rewrites .env to match the running services
```

Or take a repo you already have:

```bash
cd ~/code/existing-app
lerd link
```

Lerd detects the framework, picks the PHP version from `composer.json`, writes the vhost, registers the `.test` hostname and provisions the certificate. Use `lerd init` instead if you would rather choose the PHP version, HTTPS and services through a wizard and commit the answers to [`.lerd.yaml`](/configuration#per-project-config-lerd-yaml).

Services are shared rather than per project, which is why several running sites cost around 200 MB of RAM rather than a full stack each. MySQL, PostgreSQL, Redis, Meilisearch, MongoDB, S3-compatible storage and Mailpit are all a `lerd service start` away.

## Beyond PHP

Omarchy attracts people working across several stacks at once. Lerd serves non-PHP projects through a [`Containerfile.lerd`](/getting-started/containers) in the project root: Node, Python, Go and Rails apps get the same `.test` domain, the same certificate and the same dashboard row as a PHP site. A Rails app on `blog.test` and a Laravel app on `shop.test` sit side by side with one nginx in front of both.

## Frequently asked questions

**Does Omarchy include a PHP environment?**
No. It gives you the desktop, the terminal and the editor. The web stack is yours to install, which is what Lerd is for.

**Is Lerd in the AUR?**
Not yet. Install with the [one-line installer](/getting-started/installation#one-line-installer-recommended), which handles the Arch prerequisites, or with [Homebrew on Linux](/getting-started/installation#install-via-homebrew) if you already use it. `lerd update` self-replaces the binary on a script install.

**Will it fight with Docker?**
No. Lerd uses rootless Podman with no daemon, so Docker can stay installed and running for other work.

**Does it need sudo?**
Once, at install, to point the system resolver at the `.test` domains. Everything after that runs as your own user. The `.localhost` mode skips even that.

**Does it survive a reboot?**
Yes, provided `loginctl enable-linger $USER` is set, which the installer handles. The DNS route is re-created by `lerd-dns-link.service` on every boot.

**Does it theme with Omarchy?**
The bar widget follows the Quattro shell's own styling. The [web dashboard](/features/web-ui) has its own light and dark themes and follows your browser.

## Next steps

- [Requirements](/getting-started/requirements) and [installation](/getting-started/installation)
- [Quick start](/getting-started/quick-start), a project served in two commands
- [Omarchy bar widget](/features/omarchy-glance), the full panel reference
- [Comparison](/getting-started/comparison) against Laravel Herd, Sail, DDEV and Lando
