# Domains

Every linked site is served on one or more domains under the configured TLD (`.test` by default). This page covers how domain names are derived, adding extra domains, what happens when two sites want the same name, and overriding the `APP_URL` lerd writes to `.env`.

## Commands

| Command | Description |
|---|---|
| `lerd domain add <name>` | Add an additional domain to the current site |
| `lerd domain remove <name>` | Remove a domain from the current site |
| `lerd domain list` | List all domains for the current site |

---

## Domain naming

Directories with real TLDs are automatically normalised: dots are replaced with dashes and the TLD is stripped before appending `.test`.

For example: `admin.example.com` becomes `admin-example.test`

---

## Multiple domains

A site can respond to multiple domains. The argument to `lerd link` is the domain name without the `.test` TLD; it is appended automatically from the global config.

```bash
lerd link myapp                # links as myapp.test
```

After linking, you can add more domains:

```bash
lerd domain add api            # adds api.test
lerd domain add admin          # adds admin.test
lerd domain list
#   myapp.test (primary)
#   api.test
#   admin.test
lerd domain remove api         # removes api.test
```

Domains are stored in `.lerd.yaml` as an array (without the TLD) so the file stays portable across machines with different TLD configurations:

```yaml
domains:
  - myapp
  - admin
```

You can also manage domains from the web UI: click the pencil icon next to the domain in the site header to open the domain management modal. Changing the primary domain there also rewrites `APP_URL` in the project's `.env` to match the new primary, unless you have pinned a custom `app_url` (see [Custom `APP_URL`](#custom-app-url) below).

When a site is secured with HTTPS, the certificate is automatically reissued to cover all domains.

Subdomains (e.g. `anything.myapp.test`) are automatically routed to the same site. Git worktree subdomains take priority when they exist.

To route a subdomain to a **different** site instead (for example a separate admin app at `admin.myapp.test`), group the two sites rather than adding an alias. See [Site Groups](site-groups.md).

---

## Domain conflicts

A domain may only be claimed by one site at a time. When `lerd link`, the watcher's auto-registration, or a `.lerd.yaml`-driven re-link tries to register a domain that another site already owns, the conflicting domain is **filtered out** (not the whole site) and a warning is printed:

```
$ lerd link
  [WARN] domain "shared.test" already used by site "owner-app", skipped
Linked: clone-app -> clone-app.test (PHP 8.5, Node 22, Framework: laravel)
```

The site still gets registered with whatever domains survived the filter. If every requested domain is conflicted, lerd falls back to a freshly generated `<dirname>.<tld>` (with a numeric suffix to avoid name collisions).

`.lerd.yaml` is **never modified** when this happens; the original `domains:` list stays on disk so the conflict is visible to the UI and the entry self-heals on the next link if you remove the owning site. The web UI surfaces filtered domains in two places:

- The site detail header's domain pill shows an amber ⚠️ when one or more declared domains are filtered (`+N more` count includes them). Hovering reveals each conflicted entry with the owning site name.
- The Manage Domains modal lists conflicted entries at the top with a warning icon, the domain struck-through, a `used by <site>` pill, and a small trash button. Clicking the trash removes the entry from `.lerd.yaml` only; the registry, vhost, and certs are untouched.

The conflict check is **strict**: a domain is reserved regardless of TLS scheme. Two sites cannot share the same domain even if one runs HTTPS and the other HTTP; DNS and browser caches don't reliably disambiguate by scheme, and the resulting setup is fragile.

---

## Custom `APP_URL`

By default `lerd env` writes `APP_URL=<scheme>://<primary-domain>` to the project's `.env` on every run. If you need to override that (for example to add a path prefix, point at a staging hostname, or pin a specific protocol), set `app_url` in `.lerd.yaml` (committed, shared across machines) or in the per-machine site entry in `~/.local/share/lerd/sites.yaml`. The precedence chain is:

1. `.lerd.yaml` `app_url`: committed to the repo, takes effect on every machine.
2. `sites.yaml` `app_url`: per-machine override, useful when only one developer needs a different URL.
3. The default generator (`<scheme>://<primary-domain>`): used when neither override is set.

```yaml
# .lerd.yaml
domains:
  - myapp
app_url: http://myapp.test/api
```

`lerd env` reads the chain on every invocation, so editing the file and re-running `lerd setup` (or `lerd env` directly) is enough to apply the change. If the `.lerd.yaml` `app_url` happens to point at a domain that got filtered by the conflict check, lerd silently falls through to the next precedence level so you don't end up writing a `DB_HOST` of `lerd-mysql` next to an `APP_URL` that points at someone else's site.

---

## Unlinked domains

When you visit a `.test` domain that isn't linked to any site over **HTTP**, lerd shows a branded "Site Not Found" page with a link to the dashboard and a retry button. This replaces the browser's generic connection error.

For **HTTPS** the catch-all uses `ssl_reject_handshake on;`, so the browser sees a clean `ERR_SSL_UNRECOGNIZED_NAME_ALERT` connection error rather than a landing page. This is unavoidable: lerd cannot pre-issue a certificate covering arbitrary `*.test` hostnames because browsers (Chrome especially) reject TLD-level wildcard certificates with `ERR_CERT_COMMON_NAME_INVALID`. If you're hitting this on a domain you used to have linked, the fix is browser-side (clear site data / unregister the service worker), not server-side.
