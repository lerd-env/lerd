---
title: "lerd's own web server"
description: "Serve your sites with lerd's built-in web server instead of nginx, switch between the two, and customise it per site or for every site with a small YAML file."
---

# lerd's own web server

lerd serves sites with nginx by default. It also ships a web server of its own, written in Go and built into the `lerd` binary, which you can switch to at any time and switch back from just as easily. Your sites, domains, certificates, workers and services stay exactly as they are; only the process answering on ports 80 and 443 changes.

It reads the site registry directly instead of generated config files, so a link, a domain change or a pause takes effect on the next request with nothing to regenerate or reload.

## Switching

```bash
lerd web-server        # shows which one serves: nginx or go
lerd web-server go     # serve every site with lerd's own web server
lerd web-server nginx  # back to nginx
```

The same switch is on the dashboard under **System → Web server → Change web server**.

A switch pulls what the new server needs, stops the old one and removes its container, starts the new one and waits for it to answer. If it does not answer, lerd puts the previous server back on its own and tells you why, so a failed switch never leaves your sites unserved. Switching back to nginx rebuilds every nginx vhost from the registry first, since sites linked in the meantime have none.

While lerd's own web server serves, nginx is not just idle but gone: its container and its unit are removed, the dashboard, the tray and the TUI name the web server rather than nginx, and `lerd nginx` points you at `lerd web-server` instead. The container is called `lerd-web`.

Two things stop a switch to lerd's own web server and tell you which:

- **macOS.** On macOS the web server runs inside the podman machine, where the macOS `lerd` binary cannot run. For now lerd's own web server is Linux only, WSL included.
- **Raw nginx overrides.** A per-site override in `custom.d/` or a global one in `http.d/` is nginx config, which lerd's own web server cannot read. Rather than silently dropping what you wrote, the switch names each file; move what they do into the settings below, remove them, and switch again.

## What stays the same

Everything a site relies on is served the way the nginx vhost serves it:

- PHP-FPM sites, with the same FastCGI variables, front-controller routing, request timeout (and the hour granted while Xdebug debugs) and the framework's own FastCGI parameters.
- FrankenPHP, custom container and host-proxy sites, proxied with the same headers.
- HTTPS with each site's own certificate, the redirect from HTTP, and a moved HTTPS port.
- Worktrees on their own subdomain, worker paths and subdomains (Reverb, Vite and the like), the dev server prefix, and WebSocket upgrades through all of them.
- Site groups, paused sites, and the wake from idle-suspend, which holds the request until the site is back and answers it with the app's own response.
- Service domains with their CORS answers, the dashboard on `lerd.localhost`, the SPX profiler, browser logs, page plugins and request stats.
- LAN exposure, which stays loopback only until you run `lerd lan:expose`, and LAN and public shares.

## Settings

lerd's own web server is customised with a small YAML file per site, and one for every site:

```bash
lerd web-server edit            # the site in this directory
lerd web-server edit --branch x # a worktree of it
lerd web-server edit --global   # every site
```

`show` prints a file (`--path` prints where it lives) and `reset` deletes it, keeping its backups. On the dashboard the same files open from a site's settings button and from **System → Web server → Config**. The MCP `site_nginx` tool edits a site's file with `scope: web`.

```yaml
# Response headers, on every response
headers:
  X-Frame-Options: DENY

# A path, and everything under it, answered with a redirect
redirects:
  - from: /old
    to: /new
    status: 301        # 301, 302 (default), 307 or 308

# A path, and everything under it, sent to another server
proxies:
  - path: /api
    to: http://host.containers.internal:9000

# A path, and everything under it, served from another directory, PHP included
roots:
  - path: /setup
    root: "{{root}}"   # {{root}} is the project, {{public}} its document root

# Largest request body, 0 for no limit; a larger one answers 413
max_body_size: 100m

# Extra variables for PHP
fastcgi_params:
  APP_FLAG: "on"
```

A site's own file is applied after the global one and wins on the same header or parameter. The files are read on every request, so a save takes effect at once. A key lerd does not know, or a value it cannot serve, is refused with the file named, and the editors roll the save back rather than leave the site running on a file it cannot read.

A framework that needs a path served from elsewhere declares it in its store definition under `web.roots`, the counterpart of an nginx snippet; see [framework definitions](framework-definitions.md#serving-rules-for-lerds-own-web-server).

## Logs

The web server's logs, on the dashboard and with `lerd logs`, carry what went wrong, such as a PHP-FPM that did not answer or a certificate that could not be read. Requests themselves go to the request stats, as they do with nginx.
