# Disk cleanup

Local PHP development on podman accumulates reclaimable image data. Every PHP image rebuild re-points the fixed `:local` tag and leaves the old image dangling, every Containerfile hash bump (from a lerd update) strands the previous base image, and every service upgrade leaves the old version's image behind. Left alone, a machine that only ever kicked the tyres can end up tens of gigabytes deep.

`lerd cleanup` reclaims that space. Unlike a blunt `podman system prune -a`, it knows which images are load-bearing, so it can never eat a database or an image something still depends on. The default tier does reach beyond lerd's own images to unused catalog images and dangling leftovers on the host; `--safe` scopes it strictly to images lerd built.

## What it removes

By default (and automatically) cleanup reclaims everything below. Pass `--safe` to drop back to the conservative sweep that removes only images provably built by lerd.

**Orphaned lerd images** (always removed, even with `--safe`):

- **Orphaned PHP build images** — the old `lerd-php<ver>-fpm:local` / `lerd-frankenphp<ver>:local` image a rebuild left dangling when it re-pointed the tag.
- **Orphaned base images** — a pre-built `lerd-php*-fpm-base` image nothing live is built on: an old Containerfile hash, or a PHP version you no longer have installed. Whether a base is still in use is decided by **layer ancestry** (is its top layer part of any live image?), so a base the current PHP image is built on is always kept, never untagged into a needless re-pull.

**Unused service images** (the deep tier, default):

- A **catalog service image** that no installed service references any more and that no container currently holds, e.g. an old `mysql:8.0` after you upgraded to `8.4`. Each service's **current image and its one-back rollback target are kept**, so a rollback still works.
- This tier does **not** require that lerd pulled the image. Any image whose repo appears in the service catalog (`postgres`, `mysql`, `mariadb`, `redis`, `mongo`, `elasticsearch`, `valkey` and the rest) is in scope once nothing references it, including a copy you pulled yourself for a non-lerd project. That is deliberate: an unreferenced catalog image is the single largest thing a mixed-workload machine accumulates. If you park stopped containers' images for other stacks, run `--safe` instead; anything reclaimed here comes back with a `podman pull`.

**Dangling images** (the deep tier, default):

- Every untagged `<none>` image left behind by repeated rebuilds and re-pulls, including old upstream images that lost their tag when a newer digest was pulled. A dangling image is unreferenced by definition, so removing it frees disk and strands nothing. This is the bulk of what a long-lived install accumulates.

## What it never touches

- **Named data volumes** — your databases are never in scope.
- **Any tagged image in use** — an image a running container uses, and each installed service's current image and one-back rollback target, are always kept.
- **A tagged image outside the service catalog** — your own application images, another tool's base images, anything still carrying a tag whose repo lerd doesn't recognise as a service. (Untagged leftovers are a different matter, the deep tier reaps those wherever they came from.)
- With **`--safe`**, only images provably built by lerd (a `dev.lerd.*` label or the `lerd-php*-fpm-base` repo name) are removed, and nothing else is touched at all.

The default reaches further than `--safe` in two places: it also removes **dangling** (untagged) images, and **unreferenced catalog images regardless of who pulled them**. On a machine that also runs podman for non-lerd projects, that means the interactive `lerd cleanup` can reclaim a `postgres` or `redis` you pulled for another stack once its containers are gone. The unattended daily sweep never does either of these, it stays strictly on images lerd pulled. Use `--safe` if you want cleanup scoped to lerd alone. Removal is reference-count safe throughout: shared layers stay on disk, and an image that turns out to be in use is skipped rather than forced.

Both the CLI preview and the dashboard modal itemise every image before anything is removed, so you always see a catalog image of your own listed by name and size before confirming.

## Commands

```bash
lerd cleanup              # preview, confirm, then reclaim orphaned lerd, unused service, and dangling images
lerd cleanup --dry-run    # show what would be reclaimed and the size, remove nothing
lerd cleanup --safe       # only reclaim images provably built by lerd, keep unused service and dangling images
lerd cleanup --yes        # skip the confirmation prompt (for scripts)
```

Reported sizes are an estimate of the disk each removal frees. An image a live image is still built on is never listed (removing it is impossible and would free nothing), so cleanup never promises space it can't reclaim, though images that share layers with each other can add up to less than their sizes suggest. `lerd doctor` shows the reclaimable total as a read-only line so you discover the bloat early.

The dashboard surfaces this too. The resources widget shows the reclaimable total alongside live CPU and memory, and a **Clean up** button opens a confirmation modal that lists the images that would be removed and the space they would return, mirroring the preview above, before it runs the deep reclaim. Confirming re-inspects the host and applies that fresh plan, so a modal left open across a rebuild never asks podman to remove an image that has since become live. Cleanup only ever reclaims images and never touches a named data volume, and the watcher already runs it unattended every day, so it does not sit in the same class as migrate, remove, or reinstall, which stay CLI-only. The TUI stays informative only.

## Automatic cleanup

Cleanup is on by default and safe, so the disk doesn't grow on its own:

- **On rebuild / service change** — a PHP rebuild (`lerd use`, `lerd php:rebuild`, `lerd php:ext`/`php:pkg`, a `lerd update` that bumps the Containerfile) reclaims the image it just superseded immediately. A `lerd service update` or `lerd service remove` reclaims that service's now-unused versions, scoped to that one service.
- **Daily backstop** — the `lerd-watcher` runs a managed sweep about once a day (throttled by a timestamp so a restarting watcher can't sweep more often), catching lerd's own orphaned build images and old service versions that fell out of the one-back rollback window. It keeps every tagged image in use (the current image and the rollback target) and never removes an image lerd didn't pull, so it stays safe unattended. The wider reap that also clears foreign untagged leftovers and unreferenced catalog images you pulled yourself is left to the interactive `lerd cleanup`, so nothing running another podman workload is surprised by an unattended prune.

Toggle automatic cleanup with `lerd cleanup auto on` / `lerd cleanup auto off` (or set `auto_cleanup` in [`~/.config/lerd/config.yaml`](../configuration.md)); `lerd cleanup auto status` shows the current state. When off, `lerd cleanup` stays available on demand.

```bash
lerd cleanup auto off       # disable the automatic sweep and event-driven reaping
lerd cleanup auto on        # re-enable (the default)
lerd cleanup auto status    # show whether automatic cleanup is on
```
