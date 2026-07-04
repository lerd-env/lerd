---
name: lerd-add-framework
description: Add or extend a lerd framework definition (a new PHP framework or a new major version) as versioned YAML in the lerd-frameworks store. Use whenever the task involves framework detection, per-framework workers, env wiring, doctor checks, custom commands, or scaffolding — never branch on a framework name in Go.
---

# Add a lerd framework definition

Frameworks are submitted to the **lerd-env/frameworks** repo
(https://github.com/lerd-env/frameworks) as `frameworks/<name>/<version>.yaml`,
one file per major version. lerd is
framework-agnostic: no Go code knows a framework's name. Everything a framework
needs is declared here as data and ships to every install within ~24h with no
binary release.

The `lerd-frameworks/` directory in the lerd repo is a local checkout you can edit
and test against, but the pull request goes to **lerd-env/frameworks**.

## Procedure

1. **Copy the closest existing definition.** `laravel/12.yaml` is the most
   complete reference (workers, env services, doctor, commands, tinker). For a
   new major version of an existing framework, copy the previous version file and
   adjust. The existing YAML is the schema of record — do not invent fields.

2. **Fill the sections that apply** (all keyed by real examples in the store):
   - `name`, `version`, `label`, `public_dir`, `create` (scaffold command)
   - `php.min` / `php.max` — the versions the framework supports
   - `detect` — marker file, lockfile, or `composer:` package that identifies it,
     including the major version
   - `env` — `.env` file/format, `key_generation`, and the `services` map that
     wires each service (mysql, postgres, redis, mailpit, …) with detection rules
     and the vars to inject. **Env vars belong to the site, declared here — never
     on workers.**
   - `workers` — queue, schedule, and any framework-specific long-runners. Each
     has `command`, `restart`, optional `check`/`exclude_check` (composer package
     or file gating), `conflicts_with`, `proxy`, `per_worktree`, `host`. **New
     workers go here, not in Go.**
   - `setup` — post-link steps (migrate, storage:link…) with sensible `default:`
   - `doctor.checks` — declarative health checks (`env_combo`, `symlink`,
     `command`) with a `fix:` command and a human `detail:` string
   - `commands` — dashboard/TUI custom commands with `icon` and `confirm` where
     destructive
   - `tinker`, `logs`, `console`, `composer`, `npm`

3. **Update `frameworks/index.json`** if the store requires it (check how the
   existing entries are registered), and the README table in the
   lerd-env/frameworks `README.md`.

4. **Validate end-to-end** against a real project:
   ```bash
   lerd link           # in a project of that framework — detection must fire
   lerd site:doctor    # the declared checks must run
   ```
   Confirm detection, PHP pinning, `.env` wiring, workers, and doctor all behave.

## Rules

- The PR goes to **lerd-env/frameworks**, not the lerd binary repo.
- Data only. If you're tempted to write Go that branches on the framework name,
  the logic belongs in this YAML instead.
- Version the file by major version; keep detection specific enough to pick the
  right one.
