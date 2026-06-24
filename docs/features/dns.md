# DNS

By default lerd runs a small dnsmasq container, `lerd-dns`, and points the host resolver at it so every site under `*.test` resolves to `127.0.0.1` without any `/etc/hosts` editing. This is the recommended setup and the only mode that supports HTTPS.

## Disabling lerd-managed DNS

Some users would rather not have lerd touch the system resolver, particularly on hosts where another local DNS pipeline (corporate VPN resolver, custom split-horizon setup, strict `systemd-resolved` config) gets confused by the dnsmasq tweak. Answering `n` to the install prompt picks the disabled mode:

```
--> Let lerd manage DNS for local sites (No: use *.localhost, no dnsmasq, no HTTPS)? [Y/n] n
```

When DNS is disabled lerd will:

- skip the `lerd-dns` container, the dnsmasq config, and the sudoers rule
- skip the mkcert root CA install (no trusted CA, no HTTPS)
- leave NetworkManager / `/etc/resolver` untouched
- write its config with `dns.tld: localhost` so newly created sites use a TLD that the system resolver libraries hardwire to `127.0.0.1` per RFC 6761

In this mode your sites are reachable at `http://<name>.localhost`. HTTPS is intentionally unavailable, the `lerd init` wizard skips the "Enable HTTPS?" question, the dashboard replaces the per-site HTTPS toggle with a muted lock icon that explains HTTPS needs lerd-managed DNS, `lerd secure` refuses with a clear message, and the API endpoint returns the same. `lerd dns:check` reports `DNS managed externally` instead of probing, the dashboard DNS panel shows a `disabled` pill, the System tab drops the DNS row, and the tray shows a muted dot for DNS so you do not get nagged that the container is missing.

## LAN exposure in disabled-DNS mode

`*.localhost` is hardwired to `127.0.0.1` on every device per RFC 6761, so a remote machine cannot resolve your sites by name no matter what. The dashboard, on the other hand, listens on `0.0.0.0:7073` regardless and is gated by HTTP Basic auth. To make that reachable from another device on the LAN, lerd combines the two steps into a single button:

- Open the dashboard, System tab, lerd panel
- The "Remote dashboard access" section in disabled-DNS mode shows a single "Enable dashboard on LAN" button
- It opens a credentials modal, persists the username and bcrypt-hashed password, and flips `lan:expose` in one go

From the CLI the same flow runs as `lerd lan:expose`, which prompts inline for credentials when none are stored yet. The traditional "LAN exposure" panel that talks about exposing sites is hidden in disabled-DNS mode because the only thing the LAN flag actually unlocks here is the dashboard.

For sites, use `lerd lan:share` per project. That assigns a stable port and runs a host-level reverse proxy that rewrites the `Host:` header, so a remote device can reach the site at `http://<host-ip>:<port>` without any DNS setup. `lerd remote-setup` is unavailable in disabled-DNS mode because it relies on the dnsmasq forwarder.

## Switching modes

The mode lives in `~/.config/lerd/config.yaml` under the `dns` key:

```yaml
dns:
  enabled: true
  tld: test
```

To flip an existing install, re-run `lerd install` and answer the DNS prompt with the opposite choice. The installer detects the TLD change, lists the affected sites, and offers to migrate everything in one pass:

- stored domains in the registry and `.lerd.yaml`
- each project's `.env` `APP_URL` plus `VITE_REVERB_*` keys
- git-worktree vhosts and per-worktree `.env` files
- stale primary vhost confs and (when disabling) the previous TLS cert and key

The lerd-dns service itself is also torn down on the disable transition, `systemctl stop` plus quadlet remove on Linux, `launchctl bootout` plus plist remove on macOS. NetworkManager / `/etc/resolver` entries from the previous run are left in place because removing them needs sudo and they are inert when dnsmasq is no longer running. Run `lerd-cleanup` (macOS) or remove the dropins manually if you want a fully clean system.

Custom TLDs (anything other than `test` or `localhost`) are preserved across toggles, lerd only flips the canonical defaults.

## Pinning the upstream DNS

For everything that is not `*.test`, the lerd-dns dnsmasq forwards queries to your system's upstream DNS servers. lerd auto-detects those from `systemd-resolved`, `/etc/resolv.conf`, or NetworkManager. On some setups the detection runs before DHCP has handed out the real resolver and captures the `systemd-resolved` fallback servers instead (`9.9.9.9`, `1.1.1.1`, `8.8.8.8`), so internal hostnames served by your LAN resolver stop resolving.

When that happens, pin the upstream yourself under the `dns` key:

```yaml
dns:
  enabled: true
  tld: test
  upstream:
    - 192.168.100.129
```

Entries are plain IPs; an optional `#port` suffix is supported (e.g. `192.168.100.129#5353`). When `upstream` is set it takes precedence over auto-detection everywhere, both when lerd writes the dnsmasq config and when the NetworkManager dispatcher rewrites it after a network change. Re-run `lerd install` (or restart lerd-dns) to apply it, then confirm with `cat ~/.local/share/lerd/dnsmasq/lerd.conf`.

## Reacting to network changes

lerd reacts to host network changes on its own, so the resolver and any LAN exposure keep working when you switch Wi-Fi, dock, or get a new DHCP lease without you re-running anything.

- **Upstream re-detection (Linux).** A NetworkManager dispatcher hook re-resolves the upstream DNS servers and rewrites the dnsmasq config after a connection comes up. A pinned `dns.upstream` always wins over what it detects.
- **LAN-IP healing.** When you expose a site to the LAN (see [LAN sharing](../usage/lan-sharing.md)) and the host's LAN IP later changes, `lerd-watcher` notices the drift, re-renders the `lan:expose` mapping to the current IP, and restarts `lerd-dns` so the exposed hostnames keep pointing at the right address. This runs even while lerd is otherwise idle.
- **macOS network watcher.** On macOS the watcher subscribes to the kernel's `PF_ROUTE` socket and triggers the same healing the moment an interface or route changes, rather than waiting for the next poll.

This is why a manual `lerd remote-setup` re-run after an IP change is usually no longer necessary.
