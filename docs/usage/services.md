# Services

## Built-in services

| Command | Description |
|---|---|
| `lerd service start <name>` | Start a service (auto-installs on first use) |
| `lerd service stop <name>` | Stop a service container |
| `lerd service restart <name>` | Restart a service container; refreshes the quadlet first so config edits land |
| `lerd service status <name>` | Show systemd unit status |
| `lerd service list` | All services with status, version, and an Update column |
| `lerd service search [query]` | Browse the external service-preset store; install a hit with `lerd service preset <name>` |
| `lerd service update <name> [tag]` | Pull a newer image and restart; tag selects an explicit upgrade target |
| `lerd service migrate <name> <version>` | SQL dump + restore for cross-version moves (mysql, mariadb, postgres); `<version>` is a preset version label such as `18` |
| `lerd service rollback <name>` | Swap back to the previously-running image (toggles) |
| `lerd service pin <name>` | Pin a service so it is never auto-stopped |
| `lerd service unpin <name>` | Unpin a service so it can be auto-stopped when unused |
| `lerd service expose <name> <host:container>` | Publish an extra port on a built-in service |
| `lerd service expose <name> <host:container> --remove` | Remove a previously exposed port |
| `lerd service port <name> <port>` | Move a service's primary published host port (e.g. free 3306 for a host server) |
| `lerd service port <name> <port> --container <cport>` | Move a specific mapping of a multi-port service (e.g. Mailpit's 8025 UI) |
| `lerd service port <name> --reset` | Reset a service to its preset default published port |

Available services: `mysql` (8.4 LTS canonical, 9.7 LTS / 5.7 alternates), `redis` (7-alpine), `postgres` (16 canonical with PostGIS, 17 / 18 alternates), `meilisearch` (v1.42), `rustfs` (S3-compatible), `mailpit` (SMTP catcher).

Default services are defined as YAML presets with `default: true` in the lerd binary. Adding or replacing a default service is a YAML edit, not a code change. Each preset declares its own `update_strategy` (patch / minor / rolling), whether `track_latest` should auto-bump fresh installs to the current upstream, whether `allow_major_upgrade` lets the cross-strategy upgrade button cross numeric majors, and where the engine records the version that wrote its data (`data_version_file`) so a data dir that outlives its config still gets a server that can open it. See [Service updates](service-updates.md) for the full update / upgrade / migrate / rollback flow.

`lerd service list` shows the version (derived from the image tag) and an Update column with green / amber / violet badges:

```
╭─────────────┬─────────┬────────┬────────╮
│ Service     │ Version │ Status │ Update │
├─────────────┼─────────┼────────┼────────┤
│ mailpit     │ latest  │ active │        │
│ meilisearch │ v1.42.1 │ active │        │
│ mysql       │ v8.4.9  │ active │        │
│ postgres    │ v16     │ active │        │
│ redis       │ v7.4.8  │ active │        │
│ rustfs      │ latest  │ active │        │
╰─────────────┴─────────┴────────┴────────╯
```

The Web UI, the TUI, and `lerd status` display the same labels. Services pinned to rolling tags (`latest`, `main`) show the tag verbatim. Services where an update is available show `→ <new-tag>`; cross-strategy upgrades show `⇧ <new-tag>` in amber.

### Exposing extra ports on bundled services

Bundled services publish a fixed set of ports by default. Use `lerd service expose` to bind additional host ports without recompiling or replacing the service. This works for any service lerd ships as a preset, both the default-stack ones (MySQL, PostgreSQL, Redis) and the optional ones you install on demand (Gotenberg, MongoDB, Elasticsearch, and so on). Only genuinely custom services you define yourself are excluded, since those declare their ports in their own YAML.

```bash
# Expose MySQL on an extra port (e.g. for a second GUI client using a different port)
lerd service expose mysql 13306:3306

# Remove the extra port
lerd service expose mysql --remove 13306:3306
```

Extra port mappings are persisted in `~/.config/lerd/config.yaml` under `services.<name>.extra_ports` and are applied automatically every time the service starts. If the service is already running when you run `expose`, it is restarted immediately to apply the change.

You can also edit `~/.config/lerd/config.yaml` directly:

```yaml
services:
  mysql:
    extra_ports:
      - "13306:3306"
```

Then apply with `lerd service restart mysql`.

You can also manage extra ports from the dashboard: open a service and switch to the **Ports** tab, then add or remove mappings alongside the published port. The CLI, dashboard, MCP and TUI all route through the same logic, so a change made on one surface shows up on the others.

### Moving a service's published host port

Each service publishes on a default host port (MySQL `3306`, PostgreSQL `5432`, Redis `6379`, and so on). Every member of a service family shares that one canonical port rather than pre-spacing itself, so a single database of any family lands on the familiar port. When lerd writes a service's quadlet and the port can't be bound, or another installed same-family service already holds it, it shifts the service to the next free port and records it, so the container comes up cleanly instead of failing to bind. The decision is made purely from port availability and what other lerd services already claim: lerd never inspects host files, sockets, or installed packages. It applies to every service, not just databases.

Move a port yourself, or undo an automatic shift, with `lerd service port`:

```bash
# Publish lerd-mysql on 3307 so a host-installed MySQL can keep 3306
lerd service port mysql 3307

# Go back to the preset default
lerd service port mysql --reset   # or: lerd service port mysql 0
```

The container-internal port never changes, so containerized apps (which reach the service by name over the `lerd` network) are unaffected. Only host clients pointed at the old published port need to follow. [Host-proxy sites](host-proxy.md) that connect over the published loopback port have their `.env` regenerated automatically when the port moves. A host-proxy site that is paused when the port moves is skipped at that moment and picks up the new port when it is next unpaused.

Some services publish more than one host port: Mailpit exposes SMTP on `1025` and its web UI on `8025`, RustFS the S3 API on `9000` and the console on `9001`, Selenium the WebDriver on `4444` and the noVNC view on `7900`. `lerd service port <name> <port>` moves the primary (first) mapping. To move any other published port, name the mapping by its container-internal port with `--container`:

```bash
# Move Mailpit's web UI off 8025 to 8026 (SMTP on 1025 is untouched)
lerd service port mailpit 8026 --container 8025

# Put it back
lerd service port mailpit --reset --container 8025
```

The dashboard link for a service always follows the port its dashboard is served on, so moving Mailpit's UI port re-points the dashboard and the "open dashboard" iframe automatically.

The chosen ports are persisted in `~/.config/lerd/config.yaml` and reapplied on every start: the primary under `services.<name>.published_port`, any other mapping under `services.<name>.published_ports` keyed by container port. Once a port is set, automatically or with `lerd service port`, it sticks: lerd never moves it again on its own, not even back to the default when that frees up later. Change it only with `lerd service port`.

Every published port can also be moved from the dashboard: a service's **Ports** tab lists one editable host-port field per published port (primary and secondary alike), each with a reset-to-default. The TUI shows the current published and extra ports read-only; editing stays in the CLI, dashboard and MCP.

::: warning Known limitation
The shift is decided at quadlet-write time, from whether the port can be bound right then. A host server that is installed but stopped at that moment leaves its port looking free, so lerd may take it and clash when that server next starts (for example at boot). This is the deliberate trade for not inspecting the host: a host database is usually running, and the failure is loud. Recover by moving lerd onto a free port with `lerd service port <name> <port>`.
:::

---

## Using a service you run on the host

Moving lerd's published port lets a host-installed server keep its own. The step past that is pointing a project at your server instead of lerd's container: the database you already have, with your data and your users, while lerd keeps managing everything else.

It takes two things, both in the project's personal, gitignored [`.env.lerd_override`](../features/env-setup.md#personal-overrides-envlerd_override): the connection values that point at your server, and the reserved `LERD_EXTERNAL_SERVICES` key that tells lerd to stay out of the way.

```dotenv
# .env.lerd_override: this project uses the MySQL installed on the machine
DB_HOST=host.containers.internal
DB_PORT=3306
DB_DATABASE=myapp
DB_USERNAME=myapp
DB_PASSWORD=secret

LERD_EXTERNAL_SERVICES=mysql
```

Run `lerd env` and those values are written into `.env`, last, over anything lerd computed. For a service named in `LERD_EXTERNAL_SERVICES` lerd still writes the connection variables your framework reads, but it does not start the container and does not create the project database or S3 bucket. The key is comma or space separated, so `LERD_EXTERNAL_SERVICES=mysql, redis` opts both out, and it is consumed by lerd rather than written into `.env`. The output names what it skipped:

```
Updating existing .env...
  Detected mysql        — applying lerd connection values
   mysql externally managed (.env.lerd_override) — not starting it
  Applying 5 override(s) from .env.lerd_override
Done.
```

Opting out does not stop a lerd service that is already running. Leave it (the two coexist once their ports differ, see above) or shut it down with `lerd service stop mysql`.

Use `host.containers.internal` as the host, never `127.0.0.1` or `localhost`. The app runs inside the PHP-FPM container, where loopback is the container itself; `host.containers.internal` is an entry lerd maintains that points at an address it has probed and found routable back to your machine. `lerd doctor` prints the one in force under **Container → Host connectivity**.

### What the host server has to allow

A container is not on your machine's loopback, and what that costs you depends on the platform.

**macOS.** gvproxy maps `host.containers.internal` to `192.168.127.254` and hands the connection to the host's loopback, so a server listening on `127.0.0.1` with `'user'@'localhost'` grants accepts it with nothing changed.

**Linux.** The connection arrives from a real, non-loopback address, so a distro package left at its defaults refuses it. On Ubuntu, `mysql-server` ships `bind-address = 127.0.0.1` in `/etc/mysql/mysql.conf.d/mysqld.cnf` and grants only for `localhost`, which is exactly the combination that produces a refused connection from a lerd site. Three things need attention:

1. **Listen past loopback.** MySQL and MariaDB: set `bind-address = 0.0.0.0` and restart the server. PostgreSQL: `listen_addresses = '*'` in `postgresql.conf`. Redis: comment out `bind 127.0.0.1` or add the address the container reaches.
2. **Grant from somewhere other than `localhost`.** `CREATE USER 'myapp'@'%'` rather than `'myapp'@'localhost'`; PostgreSQL needs a matching `host` line in `pg_hba.conf`. The address the server actually sees is your machine's own address on one of its interfaces, and which one it is depends on the podman network setup, so `%` is the practical choice on a development machine. If you would rather pin it, make one failed attempt and read it back out of the rejection: MySQL answers `Access denied for user 'myapp'@'192.168.122.139'`, and that address is the one to grant.
3. **Let it through the firewall.** ufw and firewalld both drop the port by default once they are enabled.

::: warning Binding wider than loopback
`bind-address = 0.0.0.0` exposes the server to every network the machine is on, not just to containers. On a laptop that joins untrusted networks, keep a firewall rule that allows the port only from the podman subnet, or bind to that bridge address specifically instead of to everything.
:::

### What still points at lerd's container

The `lerd db:*` commands resolve their target from the service, not from `DB_HOST`, so `db:shell`, `db:import`, `db:export` and the snapshot commands keep talking to `lerd-mysql` even while your site reads and writes your own server. Use your own `mysql` or `psql` client for a host-run database. Everything else, the site's `.env`, migrations, queue workers, and the app itself, goes to the host server.

---

## Service credentials

::: tip Two sets of hostnames
Services run as Podman containers on the `lerd` network. Two hostnames apply depending on where you're connecting from:

- **From host tools** (e.g. TablePlus, Redis CLI): use `127.0.0.1`
- **From your Laravel app** (PHP-FPM runs inside the `lerd` network): use container hostnames (e.g. `lerd-mysql`)

`lerd service start <name>` prints the correct `.env` variables to paste into your project.
:::

| Service | Default version | Host (host tools) | Host (Laravel `.env`) | Port | User | Password | DB |
|---|---|---|---|---|---|---|---|
| MySQL | 8.4 LTS (`mysql:8.4`) | 127.0.0.1 | lerd-mysql | 3306 | root | `lerd` | `lerd` |
| PostgreSQL | 16 + PostGIS 3.5 | 127.0.0.1 | lerd-postgres | 5432 | postgres | `lerd` | `lerd` |
| Redis | 7-alpine | 127.0.0.1 | lerd-redis | 6379 | - | - | - |
| Meilisearch | v1.42 | 127.0.0.1 | lerd-meilisearch | 7700 | - | - | - |
| RustFS | latest | 127.0.0.1 | lerd-rustfs | 9000 | `lerd` | `lerdpassword` | per-site bucket |
| Mailpit SMTP | latest | 127.0.0.1 | lerd-mailpit | 1025 | - | - | - |

Additional UIs:

- RustFS console: `http://127.0.0.1:9001`
- Mailpit web UI: `http://127.0.0.1:8025`

### Mailpit notifications

Captured emails can pop a notification with the subject and sender; clicking the notification opens the captured message in the Mailpit overlay. This is one of several notification kinds the dashboard supports, see [Notifications](../features/notifications.md) for the full list (worker failures, finished service operations, service updates, dumps) and how to configure them under **System → Notifications**.

### RustFS, per-site buckets

Mail sent through PHP's own `mail()` reaches Mailpit too, without any project configuration. The FPM image's `sendmail` is BusyBox's, which talks to `127.0.0.1:25` and finds nothing listening inside the container, so lerd writes a `sendmail_path` pointing at the mail catcher it runs and mounts it into every PHP container. That covers the frameworks that send through `mail()` rather than SMTP, Drupal and WordPress among them, which would otherwise report that mail could not be sent with nothing to show for it. A `sendmail_path` you set yourself in the shared or per-version `php.ini` wins, since lerd's file loads before both.

RustFS is an S3-compatible object storage service (a drop-in replacement for MinIO). When `lerd env` detects it is needed (via `FILESYSTEM_DISK=s3` or `AWS_ENDPOINT` in `.env`), it automatically:

1. Creates a bucket named after the site handle, sanitised to match the S3 naming rules (lowercase, digits, hyphens, dots only, max 63 chars). Underscores in the handle are rewritten as hyphens, so `admin_astrolov` becomes bucket `admin-astrolov`.
2. Sets the bucket to **public access** (suitable for local development)
3. Writes the correct `.env` values:

```ini
FILESYSTEM_DISK=s3
AWS_ACCESS_KEY_ID=lerd
AWS_SECRET_ACCESS_KEY=lerdpassword
AWS_DEFAULT_REGION=us-east-1
AWS_BUCKET=my-project
AWS_URL=http://localhost:9000/my-project
AWS_ENDPOINT=http://lerd-rustfs:9000
AWS_USE_PATH_STYLE_ENDPOINT=true
```

If a historical `AWS_BUCKET` value with underscores (or other S3-invalid characters) is present from an earlier lerd run or Sail import, `lerd env` will sanitise it in place on the next run.

`AWS_URL` points to the public bucket URL (browser-reachable). `AWS_ENDPOINT` is the internal container address used by PHP.

### Migrating from MinIO to RustFS

RustFS exposes the same S3 API as MinIO with the same default credentials, no application changes are needed after migration.

**Automatic prompt during `lerd update`**

If lerd detects an existing MinIO data directory (`~/.local/share/lerd/data/minio`) during `lerd update`, it will offer to migrate automatically:

```
==> MinIO detected, migrate to RustFS? [y/N]
```

Answering `y` runs the full migration in-place. The update continues regardless of your answer.

**Manual migration**

```bash
lerd minio:migrate
```

This command:

1. Stops the `lerd-minio` container (if running)
2. Removes the MinIO quadlet so it no longer auto-starts
3. Copies `~/.local/share/lerd/data/minio/` to `~/.local/share/lerd/data/rustfs/`
4. Updates `~/.config/lerd/config.yaml`: removes the `minio` entry and adds `rustfs`
5. Installs and starts the `lerd-rustfs` service

The original MinIO data directory is **not deleted**. Verify the migration works, then remove it manually:

```bash
rm -rf ~/.local/share/lerd/data/minio
```

---

## More

- [Service updates](service-updates.md): the Update / Upgrade / Migrate / Rollback flow, `update_strategy` / `track_latest` / `allow_major_upgrade` configuration, and recovery from failed migrations.
- [Service presets](service-presets.md): one-command installers for phpMyAdmin, pgAdmin, MongoDB, alternate MySQL / MariaDB versions, Selenium, and Stripe Mock.
- [Custom services](custom-services.md): YAML schema for your own OCI-based services, with env injection, placeholders, dependencies, and worked examples (Soketi, Stripe).
