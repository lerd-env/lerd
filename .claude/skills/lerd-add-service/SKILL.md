---
name: lerd-add-service
description: Add or edit a lerd service preset (database, cache, search engine, admin dashboard) as YAML in the lerd-services store. Use whenever the task is to add a new service, a new version of a service, or wire a service into projects — never add services in Go.
---

# Add a lerd service preset

Services are submitted to the **lerd-env/services** repo
(https://github.com/lerd-env/services) as `services/<name>.yaml`,
one file per service. They are **data, not Go**. lerd ships only the default stack
(mysql, postgres, redis, meilisearch, rustfs, mailpit); everything else is a preset
there and reaches every install within ~24h with no binary release.

The `lerd-services/` directory in the lerd repo is a local checkout you can edit
and test against, but the pull request goes to **lerd-env/services**.

## Procedure

1. **Find the closest existing preset and copy it.** The existing YAML is the
   schema of record — do not invent fields. For a Redis-alike copy `valkey.yaml`;
   for a database copy `mariadb.yaml` or `mongo.yaml`; for an admin dashboard
   copy `phpmyadmin.yaml` / `pgadmin.yaml`.

2. **Fill the core fields** (see `valkey.yaml` for the minimal shape):
   - `name`, `description`, `family` (family groups alternates + admin UIs)
   - `image` (pin a specific tag), `ports` (`"host:container"`)
   - `data_dir` for the persistent volume
   - `env_vars` — the host/port/credentials injected into a linked site's `.env`
   - `connection_url` where applicable

3. **Avoid host-port collisions.** If the service shares a protocol/port with a
   default (e.g. Valkey vs Redis on 6379), publish it on a shifted host port so
   both can coexist, and note why in a short comment. lerd also auto-shifts
   collisions, but pick a sane default.

4. **Declare dependencies and mounted config** if the preset needs them (an admin
   dashboard depends on its database family; some presets mount a generated
   config file for auto-login). Copy the pattern from the matching existing preset.

5. **Update the store README table** in the lerd-env/services `README.md` so the
   new service is listed.

6. **Validate end-to-end** with a real lerd install:
   ```bash
   lerd service search <name>
   lerd service preset <name>
   ```
   Confirm the container starts, the port is reachable, and a linked site's `.env`
   gets the expected vars.

## Rules

- The PR goes to **lerd-env/services**, not the lerd binary repo.
- One service per file. No Go changes. No new mergers.
- Pin image tags; never rely on `latest`.
- Keep `description` one line; it shows in `lerd service search`.
