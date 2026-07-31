---
description: Install lerd and get a PHP site running on a local .test domain with HTTPS, on Linux, macOS or Windows via WSL2.
---

# Getting Started

Lerd is a local PHP development environment for Linux and macOS, with Windows supported through WSL2. It runs Nginx, PHP-FPM and your services as rootless Podman containers, so there is no Docker daemon, no sudo for day to day work, and nothing installed system wide.

If you just want a site running, read [Requirements](/getting-started/requirements) and then [Installation](/getting-started/installation). Together they take a few minutes.

## Install

- [Requirements](/getting-started/requirements) covers the supported distributions and the handful of packages lerd expects to find.
- [Installation](/getting-started/installation) is the main path, a single install script that sets up directories, the container network, DNS and certificates.
- [Windows (WSL2, beta)](/getting-started/wsl2) explains the systemd and mirrored networking setup Windows needs, most of which `lerd wsl:setup` does for you.
- [NixOS](/getting-started/nixos) documents the flake based route for immutable and declarative systems.
- [Quick Start](/getting-started/quick-start) is the short version once lerd is installed: link a directory, get a `.test` domain with HTTPS.

## Framework walkthroughs

Lerd detects your framework from the project itself and configures workers, environment wiring and health checks from a versioned store definition rather than hardcoded rules.

- [Laravel](/getting-started/laravel)
- [Symfony](/getting-started/symfony)
- [WordPress](/getting-started/wordpress)
- [Containers (Node, Python, Go, …)](/getting-started/containers) for stacks that are not PHP at all.

## Add-ons and context

- [Services](/getting-started/services) adds MongoDB, phpMyAdmin, Redis and the rest of the service presets.
- [Comparison](/getting-started/comparison) sets lerd against Herd, Laragon, DDEV, Lando and Sail.
- [Laravel Herd for Linux](/getting-started/herd-linux) is aimed at people who used Herd on a Mac and moved to Linux.
- [Laragon for Linux](/getting-started/laragon-linux) is aimed at people moving over from Windows.

Once you are set up, [Usage](/usage/sites) covers day to day site management and [Features](/features/web-ui) covers the web UI, TUI, MCP server and the rest.
