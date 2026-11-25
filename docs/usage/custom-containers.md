---
title: Custom Containers
description: Run non-PHP sites with per-project Containerfiles
---

# Custom Containers

Lerd can serve sites that use their own container instead of the shared PHP-FPM image. This lets you run Node.js, Python, Ruby, Go, or any other runtime alongside your PHP sites, with full access to services, HTTPS, LAN sharing, and workers.

## Quick start

1. Create a `Containerfile.lerd` in your project root:

```dockerfile
FROM node:20-alpine
RUN npm install -g nodemon
CMD ["npm", "run", "start:dev"]
```

> The project directory is bind-mounted at runtime, so no `WORKDIR` or `COPY` is needed. Install any global CLI tools (nodemon, ts-node, etc.) in the image itself.

2. Run `lerd init`:

```bash
cd ~/projects/nestapp
lerd init
```

When no PHP project is detected and a `Containerfile.lerd` exists, the wizard switches to custom container mode and asks for the container port, containerfile path, HTTPS, and services. The answers are saved to `.lerd.yaml`.

Alternatively, write `.lerd.yaml` manually:

```yaml
domains:
  - nestapp
container:
  port: 3000
services:
  - mysql
  - redis
```

3. Link the site:

```bash
lerd link
```

Lerd builds the image, creates a dedicated container, and configures nginx to reverse-proxy to it.

> **Important:** `lerd link` must be called **after** both files exist. Calling it without the `container:` section in `.lerd.yaml` registers the project as a PHP-FPM site instead. If that happened, run `lerd unlink` first, then set up the files and link again. If you haven't written `.lerd.yaml` yet, run `lerd init` instead of writing it by hand, it detects the `Containerfile.lerd` and runs the custom container wizard for you.

## Configuration

The `container` section in `.lerd.yaml` accepts these fields:

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `port` | yes | | Port the app listens on inside the container |
| `containerfile` | no | `Containerfile.lerd` | Path to the Containerfile (relative to project root) |
| `build_context` | no | `.` | Build context directory (relative to project root) |
| `target` | no | (last stage) | Stage to build in a multi-stage Containerfile, passed as `podman build --target`. Useful when your Containerfile has separate `development` and `production` stages and you want lerd to build the dev one locally. |
| `ssl` | no | `false` | Set to `true` if the app serves HTTPS on its port (nginx will `proxy_pass https://` with `proxy_ssl_verify off`) |
| `trust_ca` | no | `false` | Add lerd's CA to the image's trust store, so code in the container can call other `.test` sites over HTTPS |

## How it works

When you `lerd link` a project with a `container` section:

1. The image is built from your Containerfile and tagged `lerd-custom-{sitename}:local`
2. A systemd quadlet is written so the container starts automatically
3. The container joins the `lerd` network (same as PHP-FPM and services)
4. Nginx is configured to `proxy_pass` to the container instead of `fastcgi_pass`
5. Your project directory is bind-mounted into the container

`lerd php`, `lerd composer` and `lerd console` exec into this container when the
image carries PHP, so a project that pins its own runtime gets that runtime for
its development commands too, not just for the requests it serves. lerd probes
the image once and remembers the answer against the image ID, so a rebuild is
re-checked and nothing shells out on the path every `lerd php` takes.

An image with no PHP in it, which is the usual case for a Node, Python or Go
site, keeps the shared PHP container: the project is visible there through the
home mount, and execing `php` into an image that has none fails at the runtime
rather than with anything you can act on. Composer runs from lerd's own pinned
phar either way, so lerd's bin directory is mounted read-only into the
container.

## Services

Services work exactly the same as for PHP sites. Containers on the `lerd` network can reach services by name:

- MySQL: `lerd-mysql:3306`
- Redis: `lerd-redis:6379`
- PostgreSQL: `lerd-postgres:5432`

## HTTPS

`lerd secure` and `lerd unsecure` work with custom container sites. The nginx vhost is regenerated with SSL termination, and your app continues to receive plain HTTP from nginx.

If your app itself serves HTTPS on its port (e.g. it has its own TLS cert), set `ssl: true` under `container:` so nginx proxies via HTTPS:

```yaml
container:
  port: 3000
  ssl: true
```

Nginx will use `proxy_pass https://` and skip certificate verification (`proxy_ssl_verify off`) since the container cert is self-signed. Run `lerd site:doctor` to confirm the setting is recognised.

### Calling other lerd sites over HTTPS

Code in the container does not trust lerd's certificates by default, so a request from one site to another, such as `https://api.test`, fails with a certificate error. Set `trust_ca: true` and link the site again:

```yaml
container:
  port: 80
  trust_ca: true
```

Lerd adds its CA to the image's trust store as a last build step, on top of the public ones, so there is no certificate to copy into the project. It covers Debian, Alpine and Fedora based images, and sets `NODE_EXTRA_CA_CERTS` for Node, which does not read the system store. The image needs a shell, so a `scratch` or distroless one fails to build. Programs with a trust store of their own, such as Java or Python's `requests`, have to be pointed at it separately. The certificate is at `/usr/local/share/ca-certificates/lerd-mkcert.crt`.

## Hot reload

The project directory is bind-mounted into the container at the same absolute path, so file edits on the host are immediately visible inside the container. However, **filesystem watch events (inotify) do not fire** across the virtiofs mount boundary that Podman Machine uses on macOS. File watchers that rely on inotify (nodemon's default, Vite, webpack, etc.) will not detect changes.

Use polling instead:

| Tool | Polling flag |
|------|-------------|
| nodemon | `--legacy-watch` |
| Vite | `--watch` (already polls) or set `server.watch.usePolling: true` in `vite.config` |
| NestJS | `nest start --watch` uses nodemon, add `--legacy-watch` via `nodemon.json`: `{"legacyWatch": true}` |
| webpack | `watchOptions: { poll: 1000 }` in webpack config |

Example `package.json`:

```json
{
  "scripts": {
    "start:dev": "nodemon --legacy-watch src/main.js"
  }
}
```

The polling interval is typically 1–2 seconds, which is fine for dev.

## Workers

Define workers in `.lerd.yaml` using `custom_workers`:

```yaml
container:
  port: 3000
custom_workers:
  dev-server:
    label: Dev Server
    command: npm run start:dev
    restart: always
  queue:
    label: Queue Worker
    command: node dist/queue.js
    restart: on-failure
```

Workers exec into the custom container, so they have access to the same filesystem and environment.

Worker config fields:

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `label` | no | worker name | Display name in the dashboard |
| `command` | yes | | Shell command to run inside the container |
| `restart` | no | `always` | `always` or `on-failure` |
| `schedule` | no | | systemd OnCalendar expression for timer-based workers |
| `conflicts_with` | no | | Worker names to stop before starting this one |

Worker definitions stay in `custom_workers` permanently. The `workers` list tracks which are active and is synced by start/stop commands.

## LAN sharing

LAN sharing works transparently since it proxies through nginx.

## Pausing

`lerd pause` stops the custom container and all workers, replacing the nginx vhost with a landing page. `lerd unpause` starts the container back up and restores workers.

## Restarting

`lerd restart` restarts the custom container without rebuilding the image. Useful after config changes inside the container.

## Unlinking

`lerd unlink` stops the custom container, removes the quadlet, and cleans up the image.

## Rebuilding

`lerd rebuild` builds a new image from the Containerfile, restarts the container on it, and only then removes the old image, so the site keeps serving while the build runs:

```bash
lerd rebuild
```

`lerd link` rebuilds the image when it is out of date and restarts the container when the image changed. With a `build_context` in a directory of its own, such as `.lerd`, it runs the build every time and Podman's cache decides what is out of date. A context that holds the project itself, including the default, is only rebuilt when the Containerfile or `target` changed, since every source edit would otherwise rebuild the image. Run `lerd rebuild` after changing a file the image copies from there.

When a build fails, the site keeps serving the image built before it. `lerd sites`, the dashboard, the TUI and the MCP site list mark it as a failed build until one succeeds.

## Sharing a Containerfile between projects

Paths in `container` are relative to the project and may point outside it, so several projects can build from one Containerfile:

```yaml
container:
  port: 80
  containerfile: ../shared/Containerfile
  build_context: ../shared
```

Each site still gets an image and a container of its own, and Podman reuses the build layers they have in common. The container starts in the project's directory, so a start script in the shared image can use `$PWD` to find it. After changing a file in `../shared`, run `lerd link` in each project to pick it up.

When one project needs something extra, build the shared image once:

```bash
podman build -t localhost/php73-apache ../shared
```

Then start that project's own `Containerfile.lerd` from it:

```dockerfile
FROM localhost/php73-apache
RUN docker-php-ext-install calendar
```

Lerd does not track that base image. After building it again, run `lerd rebuild` in the projects that start from it.

## CLI commands

| Command | Description |
|---------|-------------|
| `lerd link` | Build image, create container, generate nginx vhost |
| `lerd unlink` | Stop container, remove image, quadlet, and vhost |
| `lerd secure` / `lerd unsecure` | Toggle HTTPS |
| `lerd pause` / `lerd unpause` | Pause/resume the site and container |
| `lerd restart` | Restart the container |
| `lerd rebuild` | Rebuild image from Containerfile and restart |
| `lerd worker start <name>` | Start a custom worker |
| `lerd worker stop <name>` | Stop a custom worker |
| `lerd worker list` | List available workers and status |
