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

## When to use LAN sharing vs full LAN exposure

| | `lerd lan:share` | `lerd lan:expose` |
|---|---|---|
| Scope | One site at a time | All sites at once |
| Client DNS setup | Not required, plain `IP:port` | Required (forward `.test` to lerd dnsmasq) |
| Client cert trust | Not required | Required for HTTPS sites |
| External tools | None | None |
| Persists across restarts | Yes (port saved in `sites.yaml`) | Yes (`lan.exposed` in `config.yaml`) |
| Use case | Quick demo to someone on the same wifi | [full remote development setup](remote-development.md) (laptop + server) |
