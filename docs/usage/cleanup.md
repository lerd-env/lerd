# Disk cleanup

Local PHP development on podman accumulates reclaimable image data. Every PHP image rebuild re-points the fixed `:local` tag and leaves the old image dangling, every Containerfile hash bump (from a lerd update) strands the previous base image, and every service upgrade leaves the old version's image behind. Left alone, a machine that only ever kicked the tyres can end up tens of gigabytes deep.

`lerd cleanup` reclaims that space. Unlike a blunt `podman system prune -a`, it knows which images are load-bearing, so it can never eat a database or an image something still depends on. The default tier does reach beyond lerd's own images to unused catalog images and dangling leftovers on the host; `--safe` scopes it strictly to images lerd built.

## What it removes

By default (and automatically) cleanup reclaims everything below. Pass `--safe` to drop back to the conservative sweep that removes only images provably built by lerd.

**Orphaned lerd images** (always removed, even with `--safe`):

- **Orphaned PHP build images**: the old `lerd-php<ver>-fpm:local` / `lerd-frankenphp<ver>:local` image a rebuild left dangling when it re-pointed the tag.
- **Orphaned base images**: a pre-built `lerd-php*-fpm-base` image nothing live is built on: an old Containerfile hash, or a PHP version you no longer have installed. Whether a base is still in use is decided by **layer ancestry** (is its top layer part of any live image?), so a base the current PHP image is built on is always kept, never untagged into a needless re-pull.

**Unused images** (the deep tier, default):

- Any tagged image no container holds and nothing lerd knows about references. That covers the obvious case, an old `mysql:8.0` after you upgraded to `8.4`, and the one that used to be invisible: the base layer of a custom container. A site whose `Containerfile.lerd` says `FROM golang:1.25` pulls that 890 MB base once, keeps it across every rebuild, and strands it the moment the Containerfile picks a different base or the site goes away. It is not a service image and lerd's pull ledger never recorded it, so no narrower rule could ever reach it.
- Each service's **current image and its one-back rollback target are kept**, so a rollback still works, as is every installed quadlet's image, including the PHP-FPM image of a site that happens to be stopped.
- This tier does **not** require that lerd pulled the image, so on a machine that also runs podman for other work it can reclaim something you pulled yourself once its containers are gone. That is deliberate: an unreferenced image is the single largest thing a mixed-workload machine accumulates. Run `--safe` if you park stopped containers' images for other stacks; anything reclaimed here comes back with a `podman pull`.

**Dangling images** (the deep tier, default):

- Every untagged `<none>` image left behind by repeated rebuilds and re-pulls, including old upstream images that lost their tag when a newer digest was pulled. A dangling image is unreferenced by definition, so removing it frees disk and strands nothing. This is the bulk of what a long-lived install accumulates.

## What it never touches

- **Named data volumes**: your databases are never in scope.
- **Any tagged image in use**: an image a running container uses, and each installed service's current image and one-back rollback target, are always kept.
- **lerd's own tool images**: the `alpine` it runs the IPv6 network probe with, with `--pull never`, so losing it would quietly downgrade the probe. It leaves no container behind, so nothing else marks it as live.
- **A tagged image outside the service catalog**, in the unattended tiers and under `--safe`: your own application images, another tool's base images. The interactive default does reap these once nothing references them, which is the whole point of the tier.
- With **`--safe`**, only images provably built by lerd (a `dev.lerd.*` label or the `lerd-php*-fpm-base` repo name) are removed, and nothing else is touched at all.

The default reaches further than `--safe` in two places: it also removes **dangling** (untagged) images, and **every unused tagged image regardless of who pulled it**. The unattended daily sweep never does either, it stays strictly on catalog images lerd recorded pulling. Use `--safe` if you want cleanup scoped to lerd alone. Removal is reference-count safe throughout: shared layers stay on disk, and an image that turns out to be in use is skipped rather than forced.

Both the CLI preview and the dashboard modal itemise every image before anything is removed, so you always see an image of your own listed by name and size before confirming.

## Commands

```bash
lerd cleanup              # preview, confirm, then reclaim orphaned lerd, unused, and dangling images
lerd cleanup --dry-run    # show what would be reclaimed and the size, remove nothing
lerd cleanup --safe       # only reclaim images provably built by lerd, keep unused and dangling images
lerd cleanup --yes        # skip the confirmation prompt (for scripts)
```

Alongside images, cleanup reclaims the config a preset rendered for a service that is no longer installed (`~/.local/share/lerd/service-files/<name>/`). Removing a service clears its own, so what turns up here was left by an older lerd. A directory is only listed when nothing answers for the name any more: no service definition, no default preset, and no installed quadlet. Your databases live elsewhere, under `~/.local/share/lerd/data/<name>/`, and cleanup never touches them.

Reported sizes are the disk each removal actually frees, not the size the image reports. Those are different numbers on a PHP stack: four builds sharing one base each report the full size of that base, so adding them up counts it four times. lerd takes the layer-aware figure podman itself computes, which charges a shared layer to nobody, and an image a live image is still built on is never listed at all. The preview therefore reads low rather than high, and the total after a reclaim is measured against the image store on both sides rather than added up from the estimates. `lerd doctor` shows the reclaimable total as a read-only line so you discover the bloat early.

On macOS that space comes back inside the Podman Machine VM, whose disk image is sparse and only ever grows, so nothing returns to the host until you run [`lerd machine reclaim`](lifecycle.md). Cleanup says so once it has run.

The dashboard surfaces this too. The resources widget shows a **disk** figure alongside live CPU and memory: the space lerd's own images occupy right now, counting each shared layer once. Click it for the breakdown: every image lerd's stack holds, heaviest first, each marked in use or idle. It counts the whole stack, not just the service catalog, so nginx, a site's stripe-listen sidecar and the throw-away probe images are all in the total. **Reclaimable** appears next to it only when there is something to reclaim, split underneath into how much of it is lerd's own leftovers and how much is everything else on the machine. A **Clean up** button opens a confirmation modal that lists the images that would be removed, grouped the same way and named by image ref, with the space each returns, before it runs the deep reclaim. Confirming re-inspects the host and applies that fresh plan, so a modal left open across a rebuild never asks podman to remove an image that has since become live. Cleanup only ever reclaims images and never touches a named data volume, and the watcher already runs it unattended every day, so it does not sit in the same class as migrate, remove, or reinstall, which stay CLI-only. The TUI stays informative only.

## Automatic cleanup

Cleanup is on by default and safe, so the disk doesn't grow on its own:

- **On rebuild / service change**: a PHP rebuild (`lerd use`, `lerd php:rebuild`, `lerd php:ext`/`php:pkg`, a `lerd update` that bumps the Containerfile) reclaims the image it just superseded immediately. A `lerd service update` or `lerd service remove` reclaims that service's now-unused versions, scoped to that one service.
- **Daily backstop**: the `lerd-watcher` runs a managed sweep about once a day (throttled by a timestamp so a restarting watcher can't sweep more often), catching lerd's own orphaned build images and old service versions that fell out of the one-back rollback window. It keeps every tagged image in use (the current image and the rollback target) and never removes an image lerd didn't pull, so it stays safe unattended. The wider reap that also clears foreign untagged leftovers and any unused tagged image, a build base or a copy you pulled yourself included, is left to the interactive `lerd cleanup`, so nothing running another podman workload is surprised by an unattended prune.

Toggle automatic cleanup with `lerd cleanup auto on` / `lerd cleanup auto off` (or set `auto_cleanup` in [`~/.config/lerd/config.yaml`](../configuration.md)); `lerd cleanup auto status` shows the current state. When off, `lerd cleanup` stays available on demand.

```bash
lerd cleanup auto off       # disable the automatic sweep and event-driven reaping
lerd cleanup auto on        # re-enable (the default)
lerd cleanup auto status    # show whether automatic cleanup is on
```
