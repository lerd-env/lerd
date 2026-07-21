# Database

Database commands work with any project type: Laravel, Symfony, NestJS, Next.js, or any other framework. lerd automatically detects which database service to use through a resolution chain described below.

## Commands

| Command | Description |
|---|---|
| `lerd db:create [name]` | Create a database and a `<name>_testing` database |
| `lerd db:import [-s service] [-d name] <file.sql>` | Import a SQL dump |
| `lerd db:export [-s service] [-d name] [-o file.sql]` | Export a database to a SQL dump |
| `lerd db:shell [-s service] [-d name]` | Open an interactive MySQL or PostgreSQL shell |
| `lerd db:snapshot [name] [-A]` | Create a named, restorable snapshot of a database |
| `lerd db:snapshots [--all]` | List stored snapshots |
| `lerd db:restore <name> [-A] [-f]` | Restore a database from a stored snapshot |
| `lerd db:snapshot:rm <name> [-A]` | Delete a stored snapshot |
| `lerd db create [name]` | Same as `db:create` (subcommand form) |
| `lerd db import [-s service] [-d name] <file.sql>` | Same as `db:import` (subcommand form) |
| `lerd db export [-s service] [-d name]` | Same as `db:export` (subcommand form) |
| `lerd db shell [-s service] [-d name]` | Same as `db:shell` (subcommand form) |
| `lerd db snapshot [name]` | Same as `db:snapshot` (subcommand form) |
| `lerd db snapshots` | Same as `db:snapshots` (subcommand form) |
| `lerd db restore <name>` | Same as `db:restore` (subcommand form) |
| `lerd db snapshot:rm <name>` | Same as `db:snapshot:rm` (subcommand form) |

### Flags

| Flag | Short | Description |
|---|---|---|
| `--service <name>` | `-s` | Target a specific lerd service (e.g. `mysql`, `postgres`, `mysql-5-7`) |
| `--database <name>` | `-d` | Override the database name |
| `--output <file>` | `-o` | Output file for `db:export` (default: `<database>.sql`) |
| `--all-databases` | `-A` | Snapshot or restore every database in the service at once |
| `--force` | `-f` | Skip the `db:restore` confirmation prompt |
| `--all` | | List snapshots across every database on the service (`db:snapshots`) |

A named snapshot (`lerd db:snapshot nightly`) gets a UTC timestamp appended to its name, e.g. `nightly-20260719-135558`, so taking the same name twice never collides. Reference the full stamped name shown by `db:snapshots` when restoring or removing it.

---

## Databases tab (web UI)

Each database engine's detail page in the web UI (Services → pick MySQL, MariaDB, PostgreSQL or MongoDB) opens on a **Databases** tab that shows the databases inside that engine as a grid of cards, each with its on-disk size. It surfaces the same operations as the CLI without leaving the browser:

- **Create** a database inline from the field above the grid. Names accepted here are limited to letters, digits, underscores and dashes, up to 64 characters, which covers every name lerd generates and keeps the value safe to use as both a path segment and a SQL identifier.
- **Export** a database to a `.sql` dump, or **import** a dump into one, from the card. An import reports itself while it runs: the card shows the dump's name with a progress bar, then a spinner once the last byte is in and the engine is still replaying it, and finally either a confirmation that fades away or the engine's own error, so a dump that fails halfway says why instead of quietly stopping. A load that the engine accepted while still complaining ends on an amber warning with the error count and the most frequent complaints, because `psql` exits 0 even when every statement in a dump failed and a silent green tick over a half-empty database is worse than no feedback at all.
- **Snapshots** are managed on the card of the database they belong to: take a snapshot, restore one (with a confirmation, since a restore overwrites the current data), delete one (also confirmed), or download one as a plain `.sql` dump. Taking, restoring and deleting all report what they are doing in the snapshots modal and leave a confirmation or the engine's error behind, so a slow restore of a large database is visibly working rather than apparently frozen. A snapshot is keyed on the engine and database it was taken from, never on a site, so it lives with the database rather than on the site page. A named snapshot gets a UTC timestamp appended (`nightly-20260719-135558`), so repeated snapshots of one name never collide; the list shows the parsed time and sorts newest first.
- **Copy connection string** builds a ready-to-paste DSN for that specific database, which works whether or not an admin UI is installed.
- **Open in the admin UI** appears on the card when an admin tool is installed for the engine, and opens it straight to this database when the tool supports a per-database URL (phpMyAdmin and Adminer for MySQL/MariaDB, Mongo Express for MongoDB). pgAdmin has no such URL, so it opens at its root.
- **The linked site**, when a site owns the database, is shown as a link on the card that jumps to that site. A `<name>_testing` database links to the same site as `<name>`. A worktree's isolated database is shown under the branch's own domain (`staging.astrolov.test` for the `staging` branch of `astrolov.test`), so it reads as that branch's data rather than as a stray database of the parent site, and the link still opens the parent site's page.
- **A `<name>_testing` database shares the card of the `<name>` database it tests**, rather than taking a second card of its own for what is usually an empty database. The card header carries an App/Testing segment, and the name, size, linked site and every action below it act on whichever half is selected, so an export, an import, a snapshot or a drop always applies to the database currently shown. Dropping one half leaves the other in place. A `_testing` database whose matching database does not exist keeps an ordinary card of its own.

The same "open in the admin tool" affordance is on the database service card in a site's own overview (a database-icon button), so from a site you can jump straight into that site's database in phpMyAdmin, Adminer or Mongo Express.

Document engines like MongoDB list their databases and expose the connection string and admin link, but the SQL-only operations (create, export, import, snapshots) are hidden for them since those act through SQL clients. A stopped engine shows a prompt to start it rather than an empty grid.

Which databases an engine advertises, and their sizes, comes from an `introspect.list_databases` command declared in the engine's [service preset](service-presets.md), so a newly added engine works here as soon as its preset ships that query, with no lerd release. The size is the data you put there, not the engine's own overhead: every postgres database inherits roughly 7.5 MB of system catalogs from `template1`, so that baseline is netted off and an empty database reads as empty, the same as it does on MySQL.

## Service and database resolution

Every db command resolves which service to target and which database to use through the following chain (first match wins):

1. **`--service` flag**: explicit override, e.g. `lerd db:shell --service postgres`
2. **`.lerd.yaml` `db:` block**: declared in the project root, works even on unlinked sites
3. **Framework definition**: lerd detects the framework and uses its service detection rules against the framework's env file (e.g. `.env.local` for Symfony)
4. **`.env` key inference**: reads `DB_CONNECTION`, `DB_TYPE`, `TYPEORM_CONNECTION`, `DATABASE_URL`, or `DB_PORT` from `.env`
5. **Error**: with instructions listing all options above

The `--database` flag overrides the database name at any resolution level.

### `.lerd.yaml` `db:` block

Add a `db:` block to `.lerd.yaml` to set a persistent default for the project. Useful for non-PHP projects that don't have a lerd framework definition.

```yaml
db:
  service: postgres
  database: myapp
```

### Supported `.env` keys

When falling back to `.env` inference, lerd checks the following keys in order to determine the database type:

| Key | Frameworks |
|---|---|
| `DB_CONNECTION` | Laravel (`mysql`, `pgsql`, etc.) |
| `DB_TYPE` | TypeORM / NestJS (`postgres`, `mysql`, etc.) |
| `TYPEORM_CONNECTION` | TypeORM CLI |
| `DATABASE_URL` | Prisma, Drizzle, Symfony, Next.js (`postgresql://...`, `mysql://...`) |
| `DB_PORT` | Last resort: `5432` for postgres, `3306`/`3307` for mysql |

The database name is resolved from `DB_DATABASE`, `TYPEORM_DATABASE`, or the path component of `DATABASE_URL` (Prisma's `?schema=public` suffix is stripped automatically).

---

## `lerd db:create` name resolution

Name is resolved in this order (first match wins):

1. Explicit `[name]` argument
2. Database name from the resolution chain above
3. Project name derived from the registered site name (or directory name)

A `<name>_testing` database is always created alongside the main one. If a database already exists the command reports it instead of failing.

---

## Snapshots

Snapshots are named, restorable point-in-time copies of a database, stored inside lerd's own data directory. Use one as a safety net before a risky migration, a branch switch, or any destructive experiment, then roll back in a single command. Snapshots cover the SQL engines only: MySQL, MariaDB, and PostgreSQL.

```bash
lerd db:snapshot pre-migration       # snapshot the current project database
lerd db:snapshot                     # name omitted: auto-named snapshot-<timestamp>
lerd db:snapshots                    # list snapshots for this database
lerd db:restore pre-migration        # restore it (prompts for confirmation)
lerd db:snapshot:rm pre-migration    # delete it
```

Snapshots live under `~/.local/share/lerd/snapshots/<service>/`, one directory per snapshot holding a gzipped SQL dump and a `meta.json` sidecar. They are scoped to a `(service, database)` pair, so two projects can both keep a snapshot called `pre-migration` without colliding. The same service-and-database resolution chain as every other db command applies, so from inside a project directory the snapshot commands just work.

### Restoring

`lerd db:restore <name>` is destructive. A per-database restore **drops and recreates** the target database before loading the dump, so the restore is clean with no leftover tables. It prompts for confirmation; pass `--force` to skip the prompt (required when running non-interactively, e.g. in a script).

### All databases

Pass `--all-databases` (`-A`) to snapshot or restore every database in the service at once instead of a single one:

```bash
lerd db:snapshot --service mysql --all-databases nightly
lerd db:restore --service mysql --all-databases nightly
```

An all-databases restore drops and recreates every database contained in the snapshot, but leaves databases that aren't in the snapshot untouched.

### Reserved names

`db:snapshot` rejects names that look like command verbs (`list`, `rm`, `delete`, `restore`, …), so `lerd db snapshot list` errors with a hint instead of silently creating a snapshot literally named "list". Use `lerd db:snapshots` to list.

### Imports that finish with errors

`psql` exits 0 whether a dump loaded cleanly or every statement in it failed, so `lerd db:import`, `lerd db:restore` and a cross-version `service migrate` count what the engine wrote and end on a warning instead of "import complete" when it complained. The warning lists the most frequent complaints with their counts, which is usually enough to name the cause on sight: a flood of `invalid command \N` means a `COPY` block had no table to load into, so the failure is further up in whatever stopped that table from being created.

### Large dumps and `max_allowed_packet`

A big restore that dies partway with "Lost connection to MySQL server during query" is almost always a single SQL statement exceeding `max_allowed_packet`, which is enforced on both the client and the server. `lerd db:import`, `lerd db:restore`, and a cross-version `service migrate` all raise the client ceiling to 1G automatically, so the client is never the bottleneck, and the bundled MySQL config ships a `max_allowed_packet` of 256M on the server. If a dump has an even larger single statement, raise the server ceiling in the service **Config** tab (or the `zz-*.cnf` tuning file) under `[mysqld]` and run `lerd service restart <name>`:

```ini
[mysqld]
max_allowed_packet = 1G
```

When you restore with an external client instead (a GUI, a manual `mysql` call), raise the packet size there too, either with `mysql --max-allowed-packet=1G` or a matching `[client]` entry in the same tuning file.

---

## Picking a database for a Laravel project

The database for a Laravel project is configured through `.lerd.yaml` and applied to `.env` when `lerd env` runs (which the `lerd init` wizard calls automatically). The supported choices are:

| Choice | Service | `.env` keys written |
|---|---|---|
| `sqlite` | none (local file) | `DB_CONNECTION=sqlite`, `DB_DATABASE=database/database.sqlite` |
| `mysql` | `lerd-mysql` (Podman) | `DB_CONNECTION=mysql`, `DB_HOST=lerd-mysql`, `DB_PORT=3306`, `DB_DATABASE=<project>`, `DB_USERNAME=root`, `DB_PASSWORD=lerd` |
| `postgres` | `lerd-postgres` (Podman) | `DB_CONNECTION=pgsql`, `DB_HOST=lerd-postgres`, `DB_PORT=5432`, `DB_DATABASE=<project>`, `DB_USERNAME=postgres`, `DB_PASSWORD=lerd` |

Installed family alternates are valid picks too: `mariadb` / `mariadb-10-11`, `mysql-5-7`, `postgres-pgvector` / `postgres-17`, etc. They go through the same env-write + database-create flow as the built-ins, using the host and port from their preset. Install one first with `lerd service preset <name>`, then list it in `.lerd.yaml` under `services:` or pick it in the `lerd init` wizard.

For SQLite, the `database/database.sqlite` file is created automatically if it doesn't exist. No service is started.

For MySQL or PostgreSQL (and their family alternates), the matching `lerd-<service>` container is started if it isn't already, and the project database (plus a `_testing` variant) is created via `lerd db:create`.

You can change the choice at any time by editing the `services:` list in `.lerd.yaml` and re-running `lerd env`, or by running `lerd init --fresh` and picking a different database in the wizard.

---

## Moving sites between services

`lerd service migrate <service> <version>` upgrades one service in place (e.g. `postgres` from 16 to 18): the service keeps its name, so every site on it follows automatically and no `.env` changes. Use that when you want to move everyone off a major version at once. See [Service updates](service-updates.md#migrate-automated-dump-restore).

`lerd db:move` is the other half: when you run two services of the same family **side by side** (e.g. the canonical `postgres` and an installed `postgres-18` alternate), it moves selected sites from one to the other and repoints their `.env`. For each site it dumps the database from the source, creates and restores it on the target, then rewrites the site's `.env` `DB_HOST`/`DB_PORT` (the same code path as `lerd env`, so host-proxy sites get loopback host + published port). The source data is left intact as a safety net.

Run it without flags for an interactive wizard:

```bash
lerd db:move
# ? Move databases from which service?  postgres (3 sites)
# ? Move to which service?              postgres-18
# ? Which sites?                        [x] shop  [x] blog  [ ] api
```

Or script it:

```bash
lerd db:move --from postgres --to postgres-18 --all      # every site on postgres
lerd db:move --from postgres --to postgres-18 --site shop --site blog
lerd db:move --from postgres --to postgres-18 --all --force   # skip the confirmation prompt
```

Both services must already be installed and in the same family (`mysql`→`mysql-5-7`, `postgres`→`postgres-18`, etc.); cross-family moves are rejected. A site's current service is detected from its `.lerd.yaml` `services:`/`db:` entry, falling back to the `lerd-<service>` hostname in `.env`. The target's `_testing` database is recreated empty by the env step; only the primary database is copied. Because the source data is preserved, clean it up by hand once you're happy with the move (drop the old databases via `lerd db:shell --service <source>`, or reinstall/remove the old service).

The repoint reuses `lerd env`, so the site needs a detectable framework (Laravel, Symfony, etc.); if the env step fails the `.lerd.yaml` change is rolled back so the site stays on its original service.

---

## Non-PHP projects

For projects without a lerd framework definition (NestJS, Next.js, Go, etc.), db commands work without any lerd-specific configuration if the project's `.env` uses a recognised key:

```bash
# NestJS / TypeORM, DB_TYPE is sufficient
lerd db:shell

# Next.js / Prisma, DATABASE_URL is sufficient
lerd db:shell

# No .env at all, use --service
lerd db:shell --service postgres --database myapp

# Or declare it once in .lerd.yaml
# db:
#   service: postgres
#   database: myapp
lerd db:shell
```

## Client tools for external databases and IDEs

The `db:*` commands work against lerd's own service containers. When you need to dump or query a database that lives **outside** lerd, for example a managed cluster on DigitalOcean, or you want to point an IDE like PhpStorm at a real `mysqldump` executable, lerd exposes the client tools that already ship inside its database images as host shims.

A service declares which tools it exposes in its YAML, so the set grows with the store. Today: mysql and mariadb expose `mysql` and `mysqldump` (mariadb backed by the `mariadb`/`mariadb-dump` binaries); postgres and its pgvector/timescaledb variants expose `psql`, `pg_dump`, `pg_dumpall`, `pg_restore`; redis exposes `redis-cli`; valkey exposes `valkey-cli`; and mongo exposes `mongosh`, `mongodump`, `mongorestore`, `mongoexport`, `mongoimport`. Each becomes a shim on your PATH in `~/.local/share/lerd/bin`.

When you install a service, lerd installs its shims. If you do not already have the tool on your system there is nothing to shadow, so the shim is installed automatically. If you **do** already have the tool installed, lerd asks first (default no) because the shim sits ahead of your own binary on PATH. Removing a service removes its shims. A tool added to a service in the store reaches an already-installed service on the next `lerd update`, without a reinstall.

The shims pass every argument straight through, so targeting an external host is just a matter of supplying your own connection flags:

```bash
# Dump a managed database to a file in the current directory
mysqldump -h db.example.com -P 25060 -u doadmin -p yourdb > dump.sql

# Same for postgres
pg_dump -h db.example.com -p 25060 -U doadmin -d yourdb > dump.sql
```

Each tool runs in a throwaway container spun from the service's image, so nothing touches your running database container. Your home directory is mounted read-write, so the tool can read a CA cert and write its output anywhere under it, whether you use a shell redirect (`> dump.sql`), the tool's own `--result-file`/`-f` flag, or an IDE that fills one in. Output files are owned by you, not root.

Managed databases usually require TLS. Keep the CA file you pass with a flag like `--ssl-ca` somewhere under your home directory so the tool can read it:

```bash
mysqldump -h db.example.com -P 25060 -u doadmin -p --ssl-ca=ca.crt yourdb > dump.sql
```

When you give no host, the tool connects to a local lerd database with its admin credentials, so `pg_dump mydb` or `mysqldump mydb` just works. If you run it from a project directory, it targets that project's own database service, read from the project's `DB_HOST`, so a mariadb-backed project routes to your mariadb container rather than the default mysql one. Outside a project, or when the project's database is a different family than the tool, it falls back to the family's default service. Passing `-h` (an external host) turns all of this off and the shim forwards everything untouched. For scripted local dumps `lerd db:export` is still the tidier option; the raw shim is there for external databases and IDEs.

Run from inside a git worktree, the shim reads that checkout's own env file rather than the parent site's, so a branch with an isolated database dumps from its own schema even when the worktree lives inside the parent's directory. A worktree whose env was never rewritten keeps using the parent site's.

To point an IDE at a tool, use its shim path, for example `~/.local/share/lerd/bin/mysqldump`.

### Managing shims

List the shims your installed services expose and whether each is installed:

```bash
lerd shims
```

Add or remove an individual shim, for example if you declined it at install time and later want it, or you would rather keep your own binary on PATH:

```bash
lerd shims remove mysqldump   # take lerd's shim off your PATH
lerd shims add mysqldump      # put it back
```

The same per-tool toggles are on each database service's Tools tab in the web UI. When two services of the same family are installed (say mysql and mariadb, which both provide `mysqldump`), one owns the shim and runs it; the others show that tool disabled on their Tools tab so it is managed in one place.

## Recovering after a service reinstall

`lerd service reinstall <name> --reset-data` wipes the database server's data dir (rename-aside, recoverable) and then walks every active site that depends on the service to recreate the database it expects via `CREATE DATABASE IF NOT EXISTS`. Database name resolution is the same as `lerd env`: `.lerd.yaml` `db.database` first, then `.env` `DB_DATABASE`, then a name derived from the site name.

The DBs come back empty. The previous data lives next door as `~/.local/share/lerd/data/<name>.pre-remove-<timestamp>`. If you need the old contents, stop the service, rename the aside dir back over the new data dir, and start the service again.

If you only want to recreate a single missing database without wiping the whole server, use `lerd db:create` against the live service instead.
