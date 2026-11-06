# HTTPS / TLS

Lerd runs its own local certificate authority, trusted by your system and browsers, so `.test` sites load over HTTPS without warnings. The CA lives where [mkcert](https://github.com/FiloSottile/mkcert) keeps one (`~/.local/share/mkcert` on Linux, `~/Library/Application Support/mkcert` on macOS, or `$CAROOT`) and uses mkcert's format, so a CA mkcert made, including one an older lerd install made through it, keeps signing and nothing is trusted again. Lerd signs the certificates itself and no longer downloads mkcert.

> [!NOTE]
> HTTPS is only available when lerd is managing DNS. If you installed in disabled-DNS mode (`dns.enabled: false`, sites under `*.localhost`), the root CA was never installed, the `lerd init` wizard skips the "Enable HTTPS?" question, the per-site HTTPS toggle in the dashboard becomes a muted lock icon, and `lerd secure` refuses with `HTTPS requires lerd-managed DNS, set dns.enabled: true and re-run lerd install`. A project whose committed `.lerd.yaml` carries `secured: true` is linked over http on a disabled-DNS install rather than registered as a non-functional HTTPS site. See [DNS](dns.md) for the toggle and how to switch back.

```bash
cd ~/Lerd/my-app
lerd secure
# Issues a cert for my-app.test, regenerates the SSL vhost, reloads nginx
# Updates APP_URL=https://my-app.test in .env if it exists
# Updates secured: true in .lerd.yaml if it exists
# Visit https://my-app.test with no certificate warning

lerd unsecure
# Removes the cert, switches back to HTTP vhost
# Updates APP_URL=http://my-app.test in .env if it exists
# Updates secured: false in .lerd.yaml if it exists
```

HTTPS can also be enabled during `lerd init` or `lerd setup`, the wizard asks the question upfront and applies it as part of the configuration step. A project with nothing committed yet, which is every project `lerd new` scaffolds, starts on the "Enable HTTPS?" answer already set to yes, so pressing enter through the wizard produces a site on https. A project that already has a `.lerd.yaml` keeps whatever that file asks for, and when the file never recorded the field the answer follows the site as it is registered today, so re-running the wizard on a site you deliberately left on http does not quietly turn it around.

Certificates are stored in `~/.local/share/lerd/certs/sites/`.

---

## Trust inside the PHP containers

The PHP-FPM and FrankenPHP images trust the mkcert CA too, so composer, curl and the app's own requests accept `.test` HTTPS from inside the container. They also follow the roots you have distrusted on the host: every `!` entry in the host's `/etc/ca-certificates.conf` is distrusted in the image as well, so a root pulled after an incident stays out of PHP while Alpine's `ca-certificates` still ships it. The image picks up a change on its next build, so run `lerd php:rebuild` after distrusting a root. A host without that file, such as macOS, carries nothing over.

---

## Browser trust (certutil / nss-tools)

Lerd installs the local CA into two places: the system trust store, used by curl, PHP, wget and openssl, and the browser NSS databases used by Firefox and Chrome/Chromium. Writing to the browser stores needs `certutil`, which ships in `nss-tools`. When `certutil` is missing the system store is still trusted, so command-line tools accept `.test` HTTPS, but browsers show a certificate warning. `lerd doctor` reports this under "browser HTTPS trust", and `lerd install` / `lerd dns:enable` print a note when it applies.

That check looks in the stores themselves rather than settling for `certutil` being installed, since the two can disagree: a CA that never reached a browser, or one that was removed from a profile afterwards, leaves every `.test` site warning on a machine that otherwise looks healthy. The certificate found in each store is compared against the CA lerd signs with today, so a CA regenerated after a profile imported the old one is reported rather than accepted for carrying the same name, and a CA stored under another nickname, such as one imported by hand, counts as present. Any store that comes up short is named in the warning, and `lerd dns:repair` installs the CA there. A machine with no browser store at all has nothing to check and passes.

On ordinary distributions install nss-tools and re-run `lerd dns:repair`:

```bash
sudo dnf install nss-tools        # Fedora
sudo apt install libnss3-tools    # Debian / Ubuntu
sudo pacman -S nss                # Arch
lerd dns:repair
```

On atomic images (Fedora Silverblue, Bazzite, Kinoite, CoreOS) the package has to be layered and the machine rebooted first:

```bash
rpm-ostree install nss-tools
systemctl reboot
lerd dns:repair
```

If you would rather not install a package at all, run `lerd dns:disable` to serve your sites over plain http on `*.localhost`, which needs no certificate.

> [!NOTE]
> On atomic desktops a Flatpak Firefox or Chrome keeps its own trust store inside the sandbox, which lerd cannot reach even once certutil is installed. A native (non-Flatpak) browser, or Chrome using the shared `~/.pki/nssdb`, picks up the CA normally.

---

## Automatic renewal

Each leaf certificate lasts 27 months, under the 825 days macOS and iOS accept. Lerd renews a secured site's certificate on its own before it lapses: whenever a certificate is within roughly 30 days of its `NotAfter` (or has already expired, gone missing, or been corrupted), the next ordinary `lerd start` or watcher pass reissues it in place. A still-valid certificate comfortably clear of that window is left untouched, so the renewal check is cheap and silent. `lerd status` continues to surface the same 30-day expiry warning under `[TLS Certificates]`, but you no longer need to act on it manually; a long-lived site that just keeps running self-heals its own certificate.

To reset the clock on demand, without toggling HTTPS off and on, run:

```bash
cd ~/Lerd/my-app
lerd secure --renew
# Reissues the certificate for my-app.test (covering worktree SANs), reloads nginx
```

`lerd secure --renew` only applies to already-secured sites; on an HTTP site it tells you to run `lerd secure` first. From an AI assistant the same action is available as the `tls_renew` action on the `site` MCP tool.

---

## From the Web UI

The Sites tab has an HTTPS toggle per site; clicking it runs `lerd secure` or `lerd unsecure` inline and updates the vhost without touching the terminal. If `.lerd.yaml` exists in the project, the `secured` field is updated there too so the state is preserved for future `lerd init` runs.

---

## Git worktrees

When a site has [git worktrees](git-worktrees.md), securing the parent automatically enables HTTPS for all its worktrees too. The parent's certificate is issued with `*.myapp.test` to cover worktree subdomains. When a new worktree is created on a secured site, the certificate is reissued to also include `*.branch.myapp.test` SANs, so deep subdomains like `app.branch.myapp.test` (common in multi-tenant apps) are covered without manual cert regeneration.

Unsecuring the parent switches all worktree vhosts back to HTTP and updates their `.env` files accordingly.

---

## Stripe listener

If a [Stripe webhook listener](../usage/stripe.md#stripe-listen) is running for the site, toggling HTTPS automatically restarts it so `--forward-to` points at the correct `http://` or `https://` URL. No manual intervention required.

---

## How it works

1. `lerd install` generates a local CA, unless one is already there, and installs it into the system trust store (NSS databases for Chrome/Firefox, and the system root store).
2. `lerd secure <site>` issues a certificate signed by that CA for `<site>.test` **and** `*.<site>.test` (wildcard), so all subdomain worktrees are covered. When worktrees exist, `*.branch.<site>.test` SANs are included so deep subdomains work too. The certificate is reissued automatically when new worktrees are created.
3. The nginx vhost is regenerated to listen on port 443 with the new cert, and port 80 redirects to HTTPS (302, not 301, so the redirect is not cached by browsers).
4. `APP_URL` in the project's `.env` (and any worktree `.env` files) is updated to `https://`.
5. If a `lerd stripe:listen` service is active for the site, it is restarted with the updated forwarding URL.
