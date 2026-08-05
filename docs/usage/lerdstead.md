# Declarative Sites (lerdstead.yml)

If you're coming from Laravel Homestead, you're used to declaring your whole machine in one file and re-provisioning to converge on it. `lerd apply` brings that workflow to lerd: a `lerdstead.yml` file lists the projects this machine serves, and applying it links what's missing, updates what drifted, and unlinks what you removed from the file.

## Commands

| Command | Description |
|---|---|
| `lerd apply` | Reconcile sites and services against `~/.config/lerd/lerdstead.yml` |
| `lerd apply <file>` | Reconcile against an explicit file (handy for dotfiles repos) |
| `lerd apply --yes` | Also unlink sites removed from the file without asking |

## The file

The default location is `~/.config/lerd/lerdstead.yml`, next to `config.yaml`. Every key except `path` is optional:

```yaml
sites:
  - path: ~/code/blog
    domains: [blog, admin.blog]   # without the TLD, like .lerd.yaml
    php_version: "8.3"
    secured: true
    services: [mysql, redis]

  - path: ~/code/shop             # everything auto-detected, like lerd link

services: [mysql@8.4]             # global presets to keep installed and running
park: [~/code/clients]            # directories to keep parked
```

## What applying does

Running `lerd apply` walks the file top to bottom:

1. **Park** each directory under `park:` (same as `lerd park`).
2. **Ensure global services** under `services:` are installed and running. Use `name@version` to pin a preset version.
3. **Converge each site.** A path lerd doesn't know yet goes through the normal link pipeline, so the project's `.lerd.yaml`, framework detection, and required services all apply exactly as they would for `lerd link`. A site that already exists is updated in place: domains, PHP version, HTTPS state, and services are each brought to the declared value, and anything already matching is left untouched.
4. **Prune.** A site that was provisioned from the file and has since been removed from it is unlinked, after a confirmation prompt (`--yes` skips it; a non-interactive run without `--yes` only reports what it would remove). Sites you linked manually are never pruned.

Applying is idempotent: running it twice in a row does nothing the second time.

## How it interacts with .lerd.yaml

The two files answer different questions. `.lerd.yaml` is committed to the project's repo and describes the project: its framework, workers, env wiring, containers. `lerdstead.yml` is machine config and describes *which* projects this machine serves. On overlap (domains, PHP version, HTTPS, services) the lerdstead entry wins, the same way an explicit `lerd link` argument would, but nothing from `lerdstead.yml` is ever written into the project's `.lerd.yaml`.

A few details worth knowing:

- `secured` is tri-state: `true` enables HTTPS, `false` disables it, and leaving the key out keeps whatever the site currently has, so a manual `lerd secure` isn't undone by a file that never mentions it.
- A declared domain that already belongs to another site is skipped with a warning, never stolen.
- `php_version` is clamped to the framework's supported range, like every other PHP switch.
- Once a path appears in the file, that site counts as file-managed and becomes prunable when you remove it later. This mirrors Homestead: the file is the source of truth for what it lists.
