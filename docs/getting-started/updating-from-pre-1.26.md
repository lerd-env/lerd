# Updating from a version before 1.26

If `lerd update` stops with this, you're on a build from before the project moved to the [lerd-env](https://github.com/lerd-env) organisation:

```
could not fetch latest version: unexpected release URL format: https://github.com/lerd-env/lerd/releases/latest
```

That build can no longer find releases on its own. Re-run the installer once and every later update goes back to working normally.

## Are you affected

```bash
lerd --version
```

Anything below **1.26.0** is affected. Version 1.26.0 is where lerd learned to follow the move by itself, so 1.26.0 and newer update without any of this.

You may also see nothing at all rather than an error. The "update available" notice in `lerd status` and `lerd doctor` fails quietly by design, so an old install simply stops telling you that new versions exist.

## What's actually broken

Only the version lookup. Everything else an old binary fetches still resolves, because GitHub forwards the old links: downloading a release archive, fetching the framework and service stores, reading the changelog, and pulling the prebuilt PHP base images from GHCR all keep working. Your sites, certificates, services and data are untouched by this, and the install keeps serving normally. It just can't discover a newer release.

The lookup asks GitHub for `/releases/latest` and reads the `Location` header of the redirect it gets back, expecting a URL that ends in `/tag/v1.32.0`. After the move that header points at the new organisation's `/releases/latest` instead of at a tag, and the old parser has nothing to read a version out of. Version 1.26.0 replaced those hardcoded links with a layer that tries each known location in order and remembers whichever answered, which is why the move is invisible from that release on.

## Fix it

### Script installs in `~/.local/bin`

Re-run the one-line installer. It replaces the binary with the current release and then runs `lerd install` to reapply the Quadlet units, DNS and certificate setup, which matters when you're crossing several releases at once. Your config, sites, certificates and service data are left alone.

::: code-group

```bash [curl]
curl -fsSL https://lerd.sh/install.sh | bash
```

```bash [wget]
wget -qO- https://lerd.sh/install.sh | bash
```

:::

If you keep the standalone installer around, `lerd-installer --update` swaps the binary too, but it stops there. Follow it with `lerd install` yourself.

### Homebrew on macOS

The tap moved along with everything else, and Homebrew follows the redirect, so the ordinary upgrade still works:

```bash
brew update
brew upgrade lerd
```

To stop leaning on that redirect, point Homebrew at the tap's real home afterwards:

```bash
brew untap geodro/lerd
brew tap lerd-env/lerd
```

### apt and dnf

Packaged installs aren't affected. The Ubuntu PPA lives on Launchpad and the Fedora package on COPR, neither of which moved:

```bash
sudo apt upgrade      # Ubuntu, Debian
sudo dnf upgrade      # Fedora
```

An old `lerd update` still errors out on a packaged install, because it looks up the release before it gets as far as telling you to use your package manager. The package manager is the right route regardless.

### By hand

If you'd rather not pipe a script anywhere, take the archive for your platform from the [releases page](https://github.com/lerd-env/lerd/releases/latest), then:

```bash
tar -xzf lerd_<version>_linux_amd64.tar.gz
install -m 755 lerd ~/.local/bin/lerd
install -m 755 lerd-tray ~/.local/bin/lerd-tray   # only if the archive has one
lerd install
```

## Confirm it worked

```bash
lerd --version
lerd update
```

The version should be 1.26.0 or newer, and `lerd update` should now report that you're already on the latest release instead of failing. From here on `lerd update` handles itself, including any future move.
