# Sharing Sites

`lerd share` exposes the current site via a public tunnel. Requires [ngrok](https://ngrok.com/download), [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/), or [Expose](https://expose.dev) to be installed, or an ngrok auth token so lerd can run ngrok as a container. A tool installed with Homebrew, or dropped into lerd's own `bin` directory, is found by the dashboard too, which does not inherit your shell's `PATH`.

For sharing on your local network instead of the public internet, see [LAN sharing](lan-sharing.md).

| Command | Description |
|---|---|
| `lerd share` | Share the current site (auto-detects ngrok, cloudflared, or Expose) |
| `lerd share <name>` | Share a named site |
| `lerd share --ngrok` | Force ngrok |
| `lerd share --cloudflare` | Force Cloudflare Tunnel (cloudflared) |
| `lerd share --expose` | Force Expose |
| `lerd share --localhost-run` | Force localhost.run (SSH, no signup) |
| `lerd share --serveo` | Force serveo.net (SSH, no signup) |
| `lerd share --pinggy` | Force Pinggy (SSH, no signup) |
| `lerd share --domain <hostname>` | Serve on your own hostname: a Cloudflare-managed one (implies Cloudflare Tunnel), or with `--ngrok` a domain reserved on your ngrok account |
| `lerd share --token <token>` | Auth token for this run, overriding the stored one (ngrok, or Pinggy with `--pinggy`) |
| `lerd share --ngrok-args "<flags>"` | Flags passed straight to ngrok for this run, overriding the stored ones |
| `lerd share:tool [tool]` | Show or set the default tunnel tool (`ngrok`, `cloudflare`, `expose`, `serveo`, `localhost-run`, `pinggy`, or `auto`) |
| `lerd share:domain [domain]` | Show or set the base domain a Cloudflare share is served under (`none` forgets it) |
| `lerd share:token [provider] [token]` | Show whether auth tokens are stored, or set one (`none` forgets it); a bare token means ngrok |
| `lerd share:ngrok-args [flags]` | Show or set the extra flags every ngrok share passes to ngrok (`none` forgets them) |

The three SSH tunnels need nothing installed beyond `ssh` itself and no account. Pinggy's free tier hands out an ephemeral URL per run; a token from the [Pinggy dashboard](https://dashboard.pinggy.io), stored with `lerd share:token pinggy <token>`, gives it a stable subdomain instead.

Every tunnel is served through a small local proxy rather than pointed straight at nginx. The proxy sets the `Host` nginx routes on, dials a secured site over HTTPS without tripping on the local mkcert certificate, and rewrites the site's own `.test` domain out of redirects and asset URLs so the public hostname survives. Without it a secured site answers the first request with a redirect to its `.test` address, which means nothing to whoever opened the public URL.

That rewrite covers more than the plain form of an address. JSON escapes its slashes, so the same URL reaches the browser as `https:\/\/site.test\/path` inside a payload embedded in the page or returned from an XHR, and it is matched in that form too. An external redirect does not always travel in a `Location` header either, since a framework can hand its own client one through a header of its own, so `Content-Location` and `X-Inertia-Location` are rewritten alongside it. Rewritten URLs always come back over `https`, because the tunnel is TLS and a plain-http one would be refused as mixed content.

Expose names a tunnel after the host it is pointed at, and every lerd share goes through the local proxy on loopback, so lerd asks Expose for the site's own label instead: `payments.test` is shared as `payments`, and a worktree of it as `feature-payments`. Custom subdomains are an Expose Pro feature, so a free account is told the request was ignored and gets a random subdomain, which still beats a share that fails outright because the proxy's port number was already taken as a name.

## ngrok without installing it

ngrok is the one tool with a published image, so lerd can run it from `ngrok/ngrok:latest` under podman when the binary is not on the machine. A container carries none of the host's ngrok configuration, so this needs an auth token:

```bash
lerd share:token 2abcXYZ...   # get one at https://dashboard.ngrok.com/get-started/your-authtoken
lerd share                     # ngrok is now an option even with nothing installed
```

The token authenticates an installed ngrok too, so a binary that was never run through `ngrok config add-authtoken` works the same way. `lerd share --token` overrides the stored token for a single run without replacing it.

An installed tool always wins over the image: pulling one is the slower route to the same URL. The container only stands in when nothing is installed, where it ranks ahead of the signup-free SSH tools because storing a token is a deliberate choice of ngrok.

The token is a credential. It is stored in `~/.config/lerd/config.yaml`, which is tightened to owner-only the moment a token is saved, it is passed to the container through the environment rather than the command line so it cannot be read off `ps`, and it is never printed back or returned by the dashboard's API. In the dashboard, the cog next to ngrok in the share menu sets and clears it.

How the container reaches that local proxy depends on the platform, because the proxy is a host process either way. On Linux the container shares the host's own network namespace, so the proxy really is on loopback and ngrok is given the port. On macOS the container runs inside the podman machine VM, whose loopback is not the host's, so it is pointed at `host.containers.internal` instead. Sharing the VM's network namespace there would dial the VM, where nothing is listening, and every request would come back as ngrok's `ERR_NGROK_8012`.

The container runs as `lerd-ngrok-<site>` (with the branch appended for a worktree), so a running tunnel appears alongside lerd's other containers in the dashboard's resource usage. The name is also how it is cleaned up. A container does not die with the process that started it: podman's supervisor is reparented out of the client's process tree and cgroup, so killing `lerd-ui` outright would otherwise leave the site publicly tunnelled. Stopping a tunnel removes the container by name rather than signalling a process, and every `lerd-ui` start sweeps any tunnel container a previous run left behind.

## ngrok on your own terms

ngrok has features lerd has no setting of its own for, and rather than grow a
lerd flag per ngrok flag they are passed straight through:

```bash
lerd share:ngrok-args --host-header=rewrite                    # every ngrok share
lerd share:ngrok-args --traffic-policy-file=/home/me/pol.yml   # a traffic policy
lerd share --ngrok-args "--compression"                        # this run only
lerd share:ngrok-args none                                     # forget them
```

Stored flags apply to every ngrok share, from the CLI and from the dashboard
alike, which is where the cog next to ngrok in the share menu sets them next to
the auth token. `--ngrok-args` wins for a single run without replacing them.

Two kinds of flag are refused rather than accepted and quietly undone. `--log`
and `--log-format` are lerd's: a dashboard share reads the public URL out of
ngrok's JSON log, and a run that reshapes that log never reports one. `--url`,
`--domain` and `--hostname` are refused alongside `lerd share --domain`, because
a share that silently ignored one of them would hand back a URL nobody is
expecting. A file a flag points at has to exist, so a mistyped path fails at the
command rather than as an opaque ngrok start error.

On a machine without ngrok installed, where lerd runs the published image, a
host path means nothing inside the container. lerd mounts the file the flag
points at read-only and repoints the flag at the mount, so the same stored flags
work on both routes.

### A reserved ngrok domain

`--domain` means the same thing for ngrok as it does for Cloudflare Tunnel, a
public URL that survives the next run, except the domain has to be reserved on
your ngrok account first:

```bash
lerd share --ngrok --domain myapp.ngrok.app
```

ngrok is asked for explicitly here on purpose. A Cloudflare named tunnel is
created on the spot, so `--domain` on its own still means Cloudflare; an ngrok
domain that was never reserved would only fail once the tunnel starts. The base
domain from `lerd share:domain` stays Cloudflare-only, since no other tool can
hand out a subdomain of a domain you own.

## Tunnels from the dashboard

The same tunnels can be started from the [web UI](../features/web-ui.md)'s share menu: hover the wifi button in a site's header and pick a tool (or the auto entry, which follows the same detection order and `share:tool` default as the CLI). The dashboard waits for the tool's public URL and shows it next to the domain with a hover-QR. A tunnel started from the UI belongs to the `lerd-ui` daemon, so it ends when you stop it or when the daemon shuts down, and it is not restored on restart. If the daemon is killed outright rather than asked to stop, the next start reaps whatever tunnel survived, so a public URL never outlives the dashboard that owns it.

A shared site is marked in the sites list too: a violet globe against the row while a public tunnel is up, a teal wifi icon while it is on the LAN, both captioned with the address. A share on one of the site's worktrees counts for the row, since the list has one row per site.

A `lerd share` running in a terminal shows up there as well. The CLI records the share while it runs and clears the record on the way out, so the dashboard reflects it like one of its own, labelled as started from the CLI. Stopping it from the dashboard signals that `lerd share` to exit. A share whose process is killed outright leaves its record behind; the dashboard drops it as soon as it notices the process is gone.

## Sharing a worktree

Run `lerd share` from inside a git worktree and lerd tunnels that branch's own domain (`<branch>.<site>.test`), not the parent checkout's. The worktree inherits the parent's registration, so there is nothing to link first: the command resolves the parent site and the branch you are standing in. The same is true of `lerd lan:share`, which assigns the branch its own LAN port, and of `lerd open`, which opens the branch domain.

A worktree tunnel is independent of the parent's. Both can run at once, each on its own public URL, and stopping one leaves the other alone. In the dashboard, switch to the worktree's tab and the share menu acts on that branch.

Naming a site explicitly (`lerd share myapp`) always means the site itself, never one of its branches.

## Default tunnel tool

Auto-detection picks the first installed tool, which may not be the one you want. `lerd share:tool cloudflare` pins the default; from then on a bare `lerd share` uses Cloudflare Tunnel even with ngrok installed. A tool flag still overrides the default per run, and `lerd share:tool auto` restores auto-detection.

## Sharing on your own domain

Quick tunnels hand out a fresh random `trycloudflare.com` URL on every run. When you need a stable URL (sending a client the same link twice, webhook or OAuth callback targets), pass `--domain` with a hostname whose DNS is managed by Cloudflare:

```bash
lerd share --domain dev.example.com
```

Custom hostnames are a Cloudflare Tunnel feature, so `--domain` selects that tool on its own. You never need `--cloudflare` alongside it, and it wins over a different default set with `lerd share:tool`. Combining it with another tool flag is rejected rather than silently ignored.

### A base domain, so you never type the hostname

`--domain` takes a full hostname and applies to one run. Set a base domain instead and every Cloudflare share is served under it, with lerd composing the hostname from the site name:

```bash
lerd share:domain example.com   # myapp.test is shared on myapp.example.com
lerd share:domain               # show the current one
lerd share:domain none          # forget it, back to quick tunnels
```

The hostname follows the site's own domain rather than the folder the project sits in, so a `scorediviner.test` served out of a `score-diviner` directory is shared on `scorediviner.example.com`.

The dashboard asks the first time you pick Cloudflare Tunnel from the share menu: type the base domain, or skip it for a quick tunnel. Tick **Remember this answer** and it stops asking, whichever way you answered. The cog next to the Cloudflare entry reopens that dialog whenever you want to change the domain, or clear it so lerd asks again.

A worktree's subdomain flattens into one label, so `feat-login.myapp.test` is shared on `feat-login-myapp.example.com`: a Cloudflare certificate covers one level of subdomain and no more.

`lerd share --domain` still wins for a single run, and the other tunnel tools ignore the base domain: no other one can hand out a subdomain of a domain you own.

### How the named tunnel is set up

On the first run cloudflared opens a browser window to authorize your Cloudflare account (a one-time login that writes `~/.cloudflared/cert.pem`). The dashboard has no terminal to run that login in, so a share from the UI stops and tells you to run `cloudflared tunnel login` once. lerd then creates a named tunnel called `lerd-<site>`, routes the hostname to it with a CNAME record, and starts the tunnel. Later runs reuse the same tunnel and hostname, so the URL never changes. Re-routing a hostname that already points at the same tunnel is a no-op; if the record exists but points somewhere else, lerd leaves it alone and prints a note asking you to check it.

A freshly created DNS record takes a moment to become visible. If you open the URL in the first seconds and your resolver caches the miss, it can keep answering NXDOMAIN for up to 30 minutes even though the tunnel is healthy.

A local reverse proxy rewrites the `Host` header to the site's domain so nginx routes to the correct vhost. Response `Location` headers and HTML/CSS/JS/JSON body references to the local domain are also rewritten to the public tunnel URL, so redirects and asset links work correctly in the browser.

When the tunnel forwards an `X-Forwarded-Host` header (the public hostname the visitor actually typed), lerd's generated vhosts propagate it into `HTTP_HOST`, `SERVER_NAME`, and the `HTTP_X_FORWARDED_*` family, so PHP apps that build absolute URLs from `$_SERVER` or Laravel's `url()` helper return the public URL instead of the local `.test` one. See [Nginx Overrides](./nginx-overrides.md#forwarded-headers-and-tunneling) for the full mapping, and for how to drop per-site snippets under `~/.local/share/lerd/nginx/custom.d/` without losing them on the next `lerd update`.
