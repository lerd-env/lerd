# LAN sharing

The quickest way to let another device on the same network reach one of your sites, no DNS setup, no external tools, no internet access required:

```bash
cd ~/Projects/myapp
lerd lan:share
```

```
Sharing myapp at http://192.168.1.42:9100
Other devices on the network can use that URL directly, no DNS setup needed.

█████████████████████████████████
█ ▄▄▄▄▄ █▀▄ █▄█ ▀▀ ▄█ ▄▄▄▄▄ █
...
Run 'lerd lan:unshare' to stop.
```

What it does:

- Assigns a stable port to the site (starting at 9100, incremented to avoid conflicts) and saves it in `sites.yaml`.
- Starts a host-level reverse proxy inside the lerd daemon (`lerd-ui`) listening on `0.0.0.0:<port>`.
- Rewrites the `Host` header on every request so nginx routes to the correct vhost.
- Rewrites absolute URLs (from `https://myapp.test/...` to `http://192.168.1.42:9100/...`) in HTML, CSS, and JS response bodies so assets and redirects work from the client device without a `.test` DNS resolver.
- Forwards `X-Forwarded-Port` to the upstream so framework URL builders (Ziggy, Symfony `Request::getSchemeAndHttpHost()`, etc.) emit the share port instead of nginx's listen port. URLs that frameworks compute from `SERVER_PORT` no longer leak `:443` into the rendered page.
- Prints a QR code you can scan to open the site on a phone.

The port is reused across restarts. Stop sharing with `lerd lan:unshare`. Toggling TLS on or off (`lerd secure` / `lerd unsecure`, or the dashboard padlock) automatically re-binds the share to the new backend so you do not need to manually restart it.

The dashboard shows the LAN URL next to the HTTPS toggle for each site. Hovering the URL shows a QR code inline.

## Vite, RustFS, and other loopback services

If your project runs a Vite dev server, lerd's share proxy reaches it transparently. URLs that the laravel-vite-plugin emits as `http://[::1]:5173/...` are rewritten in the response body to `http://<share>/__lerd_vite__/5173/...`, and the proxy forwards those paths to the local dev server. The Vite client's WebSocket handshake (HMR) is also routed through the same listener, so hot module reload works for the device viewing the share without any per-project config in `vite.config.js`. Transitive module imports (Vite-transformed JS that imports absolute paths like `/node_modules/...` or `/@vite/...`) reach Vite via the most recently observed Vite port for the share.

The same mechanism handles any other loopback service whose URLs leak into the page. Object stores like RustFS or MinIO running on `localhost:9000`, Mailpit on `localhost:8025`, or any other dev-time loopback URL gets rewritten and proxied automatically. URLs encoded into Inertia.js `data-page` JSON (where Laravel's `json_encode` escapes the forward slashes as `\/`) are caught by the same rewrite, so avatar images and other S3-style URLs load over the share without touching the client device's own localhost.

The `Referer` header is **not** trusted for routing decisions. Because the share listens on `0.0.0.0`, anyone else on the LAN could forge a `Referer` pointing at an arbitrary loopback port (SSH, the database, etc.). The proxy only dials a non-prefixed Vite-internal path against a port it learned from a genuine `/__lerd_vite__/<port>/` request.

## Public sharing through your own reverse proxy

A **public share** is the same mechanism as a LAN share, reached through a reverse proxy you run instead of by LAN IP. It suits a setup where a wildcard subdomain you control already points at this machine, for example a self-hosted netbird VPN or an nginx/Caddy in front of your dev box.

Configure it from a site's share menu in the dashboard (the wifi/globe button):

1. Click the cog next to **Public domain** and set a base domain you control, like `dev.example.com`. It must be a real domain (at least two labels); a bare TLD is refused, since lerd serves local development, it is not a host.
2. Point that base's wildcard (`*.dev.example.com`) at this machine in your own DNS or reverse proxy, forwarding each `<site>.<base>` to the share's port on the lerd host.
3. Click **Share via your reverse proxy**. lerd starts a Host-rewriting proxy on a stable `0.0.0.0` port (from 9300), and the menu shows `https://<site>.<base>`, a QR, and a stop, exactly like the LAN and tunnel shares.

Because it reuses the LAN share proxy, everything LAN sharing does works here too: nginx serves the site's normal `.test` vhost with the `Host` rewritten, response URLs are rewritten to the public hostname, and **Vite HMR works** through the same listener. Nothing is added to the site's domains, nginx `server_name`, or certificates; it is a runtime share, off until you start it and gone when you stop it. TLS is expected to terminate at your proxy, which forwards plain HTTP to the share port.

Worktrees share independently, served on the flat `<site>-<branch>.<base>` so a single `*.<base>` wildcard covers them. A site (or worktree) can be exposed only one way at a time: starting a public share is refused while a tunnel or LAN share is live for it, and vice versa.

## When to use LAN sharing vs full LAN exposure

| | `lerd lan:share` | `lerd lan:expose` |
|---|---|---|
| Scope | One site at a time | All sites at once |
| Client DNS setup | Not required, plain `IP:port` | Required (forward `.test` to lerd dnsmasq) |
| Client cert trust | Not required | Required for HTTPS sites |
| External tools | None | None |
| Persists across restarts | Yes (port saved in `sites.yaml`) | Yes (`lan.exposed` in `config.yaml`) |
| Use case | Quick demo to someone on the same wifi | [full remote development setup](remote-development.md) (laptop + server) |
