# Browser logs

A JavaScript error thrown on a page lerd serves normally only ever reaches that browser's own console. Browser logs collect those errors in the dashboard, next to what the PHP side of the same site captured, and hands them to AI assistants over MCP, so an assistant debugging a page can read why it broke in the browser instead of asking you to copy the console.

Each site **opts in**, because capture changes the HTML of the site's pages, and a site's pages carry the script only while [debug capture](dumps.md) is on, so the debug switch is the only global one. It covers PHP-FPM sites, host-proxy sites (a dev server lerd proxies to, such as Vite, Next.js or Rails) and custom-container sites. Turn it on for a site with `lerd browser-logs on` in its directory or `lerd browser-logs on <site>`, the browser icon in the site's header, the button on its empty Debug → Browser tab, or `browser_toggle` with a `site` via MCP. `lerd browser-logs off` takes it back out.

## What is captured

Uncaught errors and unhandled promise rejections are always reported while capture is on. Each one carries its message, stack, `file:line` and the page URL. Per site you can also report:

- `console.error` and `console.warn` (both on by default)
- failed `fetch` and `XMLHttpRequest` calls, choosing any of 4xx, 5xx and `failed` (all off by default). `failed` is a request that got no response at all. The browser does not tell a page why, so a CORS rejection, a refused connection, a DNS failure and being offline all arrive the same way; lerd records the method and URL and flags a cross-origin request, where a missing CORS header is the usual cause. An aborted request is not reported.
- resources (off by default): an `<img>`, `<script>`, stylesheet or other element whose file failed to load
- page views (on by default): the first load and every SPA navigation through `history.pushState`, `replaceState`, the back and forward buttons or a hash change

Every page view starts a new group in the dashboard, so an error a single-page app throws after navigating sits under the page it happened on, not under the one the app was first loaded on.

The script stops after 50 events per page view and drops an identical report that arrives within a second of the last one, so a loop that throws on every frame cannot flood the dashboard while the same error caused again a moment later is still reported.

When the script loads it writes one line to the page's console naming what it reports, so you can see at a glance that capture is live.

## How it works

While debug capture is on, the vhost of every site that opted in gets two things: an nginx `sub_filter` that inserts `<script src="/_lerd/browser.js"></script>` before `</head>`, and two exact locations that proxy the script and the report endpoint to lerd-ui. The script is served per site with that site's settings baked in, so changing a setting takes effect on the next page load with no nginx reload. Only the debug switch and turning a site on or off rewrite vhosts, and the debug switch touches nginx only when some site opted in.

The script sends what it catches with `navigator.sendBeacon` to `/_lerd/browser` on the site's own origin, so there is no CORS involved. nginx names the site in a header it sets itself, and lerd-ui only accepts the report when nginx handed it over, so a page cannot claim to be another site and a client on the LAN cannot post to lerd-ui directly. A report is also refused unless the browser says the site's own page sent it (`Sec-Fetch-Site: same-origin`, or a matching `Origin` from an older browser), so another website open in the same browser cannot plant events in the dashboard or in what an AI assistant reads. Reports land in the same in-memory buffer as the debug lenses and are gone when lerd-ui restarts.

Because `console.error` and `console.warn` are wrapped, the script carries an inline source map that marks it as library code. DevTools then attributes a console message to your code's own line rather than to lerd's wrapper.

## Where to see it

- **Site → Debug → Browser** lists the site's events grouped per page view, with the stack and user agent one click away. The **Settings** button opens the site's capture settings, and **Clear** empties the browser events while keeping what the PHP side captured.
- **System → Debug bridge → Browser** shows every site's events, with a site filter.
- **MCP**: `diag` with `action: "browser_events"` returns the site's page views, newest first, each with what happened on it: errors with their stack and `file:line:col`, rejections, console messages, failed requests and configured events. It also returns a count per type, whether debug capture is on and whether the site opted in, and a `hint` that says why the list is empty, so no events is never mistaken for a page without errors. `types` narrows it, for example to `["error", "rejection", "network"]`, alongside `site`, `branch`, `since` and `limit`.

## Per-site settings

Each site's settings are kept under `browser_logs` in lerd's site registry (`~/.local/share/lerd/sites.yaml`), never in the project, since turning capture on is one developer's choice, like the debug switch it works under. Change them from the site's Browser settings, `lerd browser-logs`, or MCP. The fields are:

```yaml
browser_logs:
  enabled: true          # opts the site in, off by default
  console: [error, warn] # console levels to report
  network: [5xx, failed] # failed requests to report: 4xx, 5xx, failed (no response)
  resources: false       # report elements whose file failed to load
  events: []             # DOM events to report, see Frontend library events
  presets: {}            # presets switched on or off, see Presets (detected ones are on)
  navigation: true       # report page views, SPA navigations included
```

## Frontend library events

Most frontend frameworks need nothing extra. React, Vue, Svelte, Angular, Alpine and Stimulus either let an error escape as an uncaught error or log it with `console.error`, and both are captured already. Vue and Alpine.js log their warnings with `console.warn`, which is reported by default too.

Some libraries report a failure as a DOM event of their own instead. List those under `browser_logs.events` and each one is reported as an `event` in the Browser lens. `event` is the DOM event name, `label` names it in the dashboard, and `message` is an optional dot path into the event whose value becomes the message. A path that ends on an object or a list is shown as JSON, with an error as its name and message, a response as its status and URL (an axios response, which Inertia's events carry, as its status, method and URL), a DOM element as its tag and id, a Map or Set by its contents and a cycle as `[Circular]`, kept to three levels and twenty entries; point at a scalar such as `detail.response.status` when one value says it all. Only names and paths can be set, never code, because the script runs on every page of the site. Events are caught whether they are dispatched on `window`, `document` or an element, and whether or not they bubble. The same list can be edited from the site's Browser settings.

### Presets

The framework store's package definitions declare the DOM events a library reports failures through, on the composer and npm packages that ship it, so the packages a project requires are what detect a preset:

| Preset | Reports the events |
|---|---|
| Inertia.js | `inertia:invalid`, `inertia:exception` |
| Hotwire Turbo | `turbo:fetch-request-error`, `turbo:frame-missing` |
| htmx | `htmx:responseError`, `htmx:sendError`, `htmx:swapError` |
| Vite | `vite:preloadError` |

A library that reports through the browser's own channels needs no preset, only the right box under **What to report**: Vue and Alpine.js log their warnings with `console.warn`, on by default, and a failed Livewire update arrives as a 4xx or 5xx response. React, Svelte, Angular and Stimulus need nothing at all, since their errors escape as uncaught errors or go to `console.error`, which are captured by default.

A preset is **on by default when the project uses its library**: detected from its composer and npm packages, its events are reported without any setup. Every other preset is off. The site's Browser settings list the presets the project uses, each with a checkbox to switch it off, and `lerd browser-logs preset on <name>` or MCP switches on one lerd did not detect, such as a library loaded from a CDN; and the choice is recorded under `presets` as the preset's name and `true` or `false`. A preset's events are added when the script is served rather than copied into the site's own `events`, which hold only the ones you write yourself; when both name the same event, yours wins.

The CLI and MCP see the same presets and detection results:

```bash
lerd browser-logs presets             # presets for the site in this directory, marking detected and active ones
lerd browser-logs preset off inertia  # stop reporting a detected preset's events
lerd browser-logs preset on htmx      # report a preset the project was not detected using
```

Over MCP, `diag` with `action: "browser_presets"` and a `site` lists the presets with `detected` and `active`, and adding `preset` with `enable: true` or `false` switches one on or off.

### Writing events yourself

The presets are the same entries the examples below show, so a library without a preset is set up the same way.

Inertia, for a response that was not an Inertia response (usually an error page) and for an unexpected failure:

```yaml
browser_logs:
  events:
    - event: inertia:invalid
      label: Inertia invalid response
      message: detail.response
    - event: inertia:exception
      label: Inertia exception
      message: detail.exception.message
```

Turbo (Hotwire, Symfony UX Turbo), for a request that failed and for a frame response without the frame it should replace:

```yaml
browser_logs:
  events:
    - event: turbo:fetch-request-error
      label: Turbo request failed
      message: detail.error.message
    - event: turbo:frame-missing
      label: Turbo frame missing
      message: detail.response.status
```

htmx, for an error response, a request that could not be sent and a swap that failed:

```yaml
browser_logs:
  events:
    - event: htmx:responseError
      label: htmx error response
      message: detail.xhr.status
    - event: htmx:sendError
      label: htmx request not sent
    - event: htmx:swapError
      label: htmx swap failed
```

Vite, for a lazily loaded chunk that failed to load, typically after a deploy or a restarted dev server:

```yaml
browser_logs:
  events:
    - event: vite:preloadError
      label: Vite chunk failed to load
      message: payload.message
```

## Limits

- FrankenPHP sites and the worktrees of host-proxy sites are not covered yet, and a sleeping host-proxy site keeps its waking page until it is back.
- For host-proxy and custom-container sites lerd asks the backend for an uncompressed response, since nginx cannot rewrite a compressed body. On a PHP-FPM site, a response the app compresses itself is passed through unchanged.
- A page without `</head>` gets no script.
- A strict nonce-based Content Security Policy blocks the injected script. A policy that allows `script-src 'self'` is fine.
- Production bundles without source maps give stacks that point into minified files. Vite's dev server serves modules unbundled and reads fine.
- The script wraps `window.fetch` and `XMLHttpRequest` only when a network class is turned on for the site.
- The reason a request got no response stays in the browser's own console; a page cannot read it.
