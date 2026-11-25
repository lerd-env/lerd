---
title: Moving databases from DBngin or Herd
description: 'Move the MySQL and PostgreSQL databases you run in DBngin or Laravel Herd into lerd: export, import, point each site at lerd, check every row arrived, and handle databases too big for the default VM.'
head:
  - - meta
    - name: keywords
      content: dbngin alternative, dbngin to lerd, herd database migration, move mysql databases, import large mysql dump, podman machine memory, innodb_buffer_pool_size
---

# Moving databases from DBngin or Herd

Lerd can use a database server you already run, such as DBngin or the database services in Laravel Herd Pro, by [adding it as an external server](../usage/services.md#adding-an-external-database-server). That is the quickest way to get going. Once you are settled, moving the databases into lerd's own MySQL or PostgreSQL means one less app to run, one place for snapshots and worktree databases, and no second server holding the usual port.

This page moves them across one site at a time, checks every table arrived with the same number of rows, and keeps a dump of each before the old server goes. Databases of a few gigabytes or more need some memory planning first, covered in [Large databases](#large-databases).

## 1. Take stock

Note which servers you run, their engine and port, and which database each site uses. DBngin and Herd both default to user `root` with no password on the engine's usual port, so a second server of the same engine is often on `3307` or `5433`.

Lerd reaches the old server with its own client tools, which neither app puts on your `PATH`. Find the folder that holds them:

```bash
find /Users/Shared/DBngin -name mysqldump                    # DBngin
find ~/Library/Application\ Support/Herd -name mysqldump     # Herd
```

Use `pg_dump` in place of `mysqldump` for a PostgreSQL server.

## 2. Add the old server to lerd

```bash
lerd service external dbngin --engine mysql --port 3306 --user root \
  --tools /Users/Shared/DBngin/mysql/8.4.7_arm64/bin
```

Lerd connects first and saves nothing if that fails. Add each server under its own name, `dbngin80` and `dbngin84` for example, with the tools folder of its own version. Nothing changes on the server, and your sites keep using it until you point them elsewhere.

Ask it how big each database is, to see whether any count as [large](#large-databases):

```bash
echo "SELECT table_schema, ROUND(SUM(data_length + index_length) / 1024 / 1024) AS mb
        FROM information_schema.tables GROUP BY 1 ORDER BY 2 DESC" | lerd db:shell -s dbngin
```

## 3. Export each database

Anything that writes to the database after the export starts is left behind on the old server, and changes to existing rows don't show up in a row count. Pause the site first, which stops its queue, schedule and other workers and puts a landing page in front of it, and stop anything else you run against the database yourself:

```bash
lerd pause
```

The site stays paused until it is checked in step 6. Then, from the project directory:

```bash
mkdir -p ~/db-backups
lerd db:export -s dbngin -d shop -o ~/db-backups/shop.sql
```

The export carries routines, triggers and events, the same as every dump lerd takes. It is plain SQL, so a 15 GB database writes a file of about that size; leave room on the disk for the dump and for the imported copy.

## 4. Point the site at lerd

How you do this depends on how the site reaches the old server today:

- **Through `lerd db:use dbngin`**: change `dbngin` to `mysql` (or `postgres`) under `services:` in `.lerd.yaml`, and under `db:` too if the file has a `service: dbngin` there, then run `lerd env`. The `db:` block wins over everything else when lerd's database commands look for the site's database, so one left behind keeps them pointed at the old server.
- **Through [`.env.lerd_override`](../features/env-setup.md#using-your-own-service-instead-of-lerd-s-container)**: delete the `DB_*` lines and the database from `LERD_EXTERNAL_SERVICES`, then run `lerd env`.
- **Through `.env` alone**: run `lerd env`.

`lerd env` writes lerd's connection into the env file and creates an empty database named after the site. When the old database isn't named after the site, such as `shop_production` for a site called `shop`, keep its name with a personal override before running `lerd env`:

```bash
lerd env:override DB_DATABASE=shop_production
```

Check where the site points now with `grep DB_ .env`: the host is lerd's, and `DB_DATABASE` is the name to import into.

## 5. Import

```bash
lerd db:import -s mysql -d shop --fresh ~/db-backups/shop.sql
```

`--fresh` drops and recreates the database before loading, so a second attempt starts clean rather than colliding with half a first one. That is why the service and database are named outright: a command left to work out its own target drops whatever the project points at, and naming them means it can only be lerd's.

## 6. Check every row arrived

Save this as `counts.sql`. It counts the rows in every table of the current database:

The binary sort keeps the tables in the same order whatever character set each side's client uses, so `post_tag` and `posts` don't swap places and read as a difference.

```sql
SET SESSION group_concat_max_len = 1000000;
SELECT GROUP_CONCAT(CONCAT('SELECT ''', table_name, ''' AS tbl, COUNT(*) AS n FROM `', table_name, '`') SEPARATOR ' UNION ALL ')
  INTO @counts
  FROM information_schema.tables
 WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE';
SET @counts = CONCAT('SELECT * FROM (', @counts, ') c ORDER BY CAST(tbl AS BINARY)');
PREPARE counts FROM @counts;
EXECUTE counts;
```

Run it on both sides and compare, naming the database you exported in step 3 on the old server and the `DB_DATABASE` you imported into on lerd's:

```bash
lerd db:shell -s dbngin -d shop < counts.sql > before.txt &&
  lerd db:shell -s mysql -d shop < counts.sql > after.txt &&
  test -s before.txt &&
  diff before.txt after.txt && echo "every table matches"
```

Each step only runs when the one before it worked, so a misspelt name or a query that fails stops it there rather than comparing two empty files.

For PostgreSQL, use this `counts.sql` with `-s postgres`:

```sql
SELECT table_schema || '.' || table_name AS tbl,
       (xpath('/row/n/text()', query_to_xml(format('SELECT count(*) AS n FROM %I.%I', table_schema, table_name), false, true, '')))[1]::text::bigint AS n
  FROM information_schema.tables
 WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema')
 ORDER BY 1;
```

Once the counts match, resume the site with `lerd unpause`, load a few pages, and run your migrations' status command to confirm the schema is where you left it. Database clients such as TablePlus or Sequel Ace reach lerd's server with the [service credentials](../usage/services.md#service-credentials).

## 7. Keep the dump

Compress the dump and keep it until you are sure nothing is missing. `lerd db:import` reads a `.sql.gz` the same as a `.sql`, so putting it back later is the same command:

```bash
gzip ~/db-backups/shop.sql
```

A database no site uses can stop at step 3: export it and keep the file.

## 8. Stop the old server

Once every site is across, check nothing still holds a connection to the old server:

```bash
echo "SELECT user, host, db, command, time FROM information_schema.processlist WHERE id <> CONNECTION_ID()" | lerd db:shell -s dbngin
```

A database client left open from days ago is the usual culprit. Then remove the server from lerd, which leaves its data alone, and quit the app:

```bash
lerd service remove dbngin
```

Uninstall DBngin by moving it to the Bin. It keeps its servers and data in `/Users/Shared/DBngin`, which you can delete once you no longer need the dumps' source. In Herd, stop the service and remove it from the Services tab.

If lerd's MySQL moved to another port because the old server held `3306`, put it back now that the port is free:

```bash
lerd service port mysql --reset
```

That rewrites the env of every site that reaches MySQL over the published port, which is every site under the [native runtime](../features/native-runtime.md), and names them before it does. See [Moving a service's published host port](../usage/services.md#moving-a-service-s-published-host-port).

## Large databases

A big import is a test of memory before anything else. On macOS every lerd container runs inside the Podman Machine VM, which starts with 3 to 6 GB depending on the Mac's memory. MySQL shares it with PHP, Redis, a search engine and anything else running, and when the VM runs out, its kernel kills the biggest process, which during an import is MySQL.

### What it looks like

The import stops partway with one of:

```
ERROR 2002 (HY000): Can't connect to local MySQL server through socket '/var/run/mysqld/mysqld.sock' (111)
Lost connection to MySQL server during query
```

and sites briefly report `MySQL server has gone away` while the container restarts. Check whether the VM killed it:

```bash
lerd machine memory
```

It counts the kills since the VM booted and suggests a size. `Lost connection` with no kill counted is usually a single statement over `max_allowed_packet` instead, covered in [Large dumps and `max_allowed_packet`](../usage/database.md#large-dumps-and-max-allowed-packet).

### Before you import

**Give the VM more memory.** It is a ceiling, not a reservation, so a bigger VM costs nothing while it is idle. Pick a size in GB up to half the Mac's memory, which is as far as lerd lets the VM go:

```bash
lerd machine memory 8
```

That is the most a 16 GB Mac can give it; 12 GB needs a Mac with 24 GB or more, and anything over half is refused. Setting a size restarts the VM and every container with it. After an import that ran out of memory, `lerd machine memory` on its own suggests the next size to try. See [`lerd machine memory`](../usage/lifecycle.md#lerd-machine-memory-macos). On Linux there is no VM, so this step does not apply.

**Size MySQL's buffer pool to fit.** The buffer pool is MySQL's main cache and the largest thing it holds in memory. Set it in lerd's tuning file, which survives updates and reinstalls:

```bash
lerd service config mysql
```

```ini
[mysqld]
innodb_buffer_pool_size = 2G
```

Saving restarts MySQL. Keep it to around a quarter of the VM's memory, 2G on an 8 GB VM, leaving room for every other container. A larger pool makes a large import faster, but one too large for the VM is what gets MySQL killed: a 15 GB database imported in about a quarter of an hour into a 6 GB VM with the pool at 512 MB, after an attempt at 2 GB ran out of memory with a search engine running alongside.

**Stop what you do not need** for the length of the import, such as `lerd service stop elasticsearch`.

**Optionally, write less to disk during the import.** MySQL flushes its log on every commit by default. Relaxing that for the import speeds it up, at the cost of the last second of writes if MySQL crashed meanwhile, which a re-run with `--fresh` covers anyway:

```bash
echo "SET GLOBAL innodb_flush_log_at_trx_commit = 2" | lerd db:shell -s mysql
lerd db:import -s mysql -d big --fresh ~/db-backups/big.sql
echo "SET GLOBAL innodb_flush_log_at_trx_commit = 1" | lerd db:shell -s mysql
```

A `SET GLOBAL` lasts until MySQL restarts, so a restart puts it back as well.

Import the large databases last, after the small ones are across and checked, so a failure costs only the one that failed.

## Next steps

- [Database](../usage/database.md), every `db:*` command, snapshots and imports
- [Services](../usage/services.md), ports and credentials for lerd's own servers
- [Native runtime](../features/native-runtime.md), PHP on the host on macOS
