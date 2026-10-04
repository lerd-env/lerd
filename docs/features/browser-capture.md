# Browser capture

A JavaScript error thrown on a page lerd serves normally only ever reaches that browser's own console. Browser capture collects those errors in the dashboard, next to what the PHP side of the same site captured, and hands them to AI assistants over MCP, so an assistant debugging a page can read why it broke in the browser instead of asking you to copy the console.

The feature is **off by default**, because turning it on changes the HTML of every site it covers: PHP-FPM sites, host-proxy sites (a dev server lerd proxies to, such as Vite, Next.js or Rails) and custom-container sites. Enable it with `lerd browser-capture on`, the browser toggle in the Sites header or the System Health card, or `browser_toggle` via MCP. It has its own switch and works without debug capture.

## What is captured

Uncaught errors and unhandled promise rejections are always reported while capture is on. Each one carries its message, stack, `file:line` and the page URL. Per site you can also report:

- `console.error` (on by default) and `console.warn` (off by default)
- failed `fetch` and `XMLHttpRequest` calls, choosing any of 4xx, 5xx and `failed` (all off by default). `failed` is a request that got no response at all. The browser does not tell a page why, so a CORS rejection, a refused connection, a DNS failure and being offline all arrive the same way; lerd records the method and URL and flags a cross-origin request, where a missing CORS header is the usual cause. An aborted request is not reported.
- resources (off by default): an `<img>`, `<script>`, stylesheet or other element whose file failed to load
- page views (on by default): the first load and every SPA navigation through `history.pushState`, `replaceState`, the back and forward buttons or a hash change

Every page view starts a new group in the dashboard, so an error a single-page app throws after navigating sits under the page it happened on, not under the one the app was first loaded on.

The script stops after 50 events per page view and drops repeats of the same error, so a loop that throws on every frame cannot flood the dashboard.

When the script loads it writes one line to the page's console naming what it reports, so you can see at a glance that capture is live. Set `verbose: true` to also log each capture there.

## How it works

When the toggle is on, the vhost of every covered site gets two things: an nginx `sub_filter` that inserts `<script src="/_lerd/browser.js"></script>` before `</head>`, and two exact locations that proxy the script and the report endpoint to lerd-ui. The script is served per site with that site's settings baked in, so changing a setting takes effect on the next page load with no nginx reload. Only the global toggle and a changed route rewrite vhosts.

The script sends what it catches with `navigator.sendBeacon` to `/_lerd/browser` on the site's own origin, so there is no CORS involved. nginx names the site in a header it sets itself, and lerd-ui only accepts the report when nginx handed it over, so a page cannot claim to be another site and a client on the LAN cannot post to lerd-ui directly. Reports land in the same in-memory buffer as the debug lenses and are gone when lerd-ui restarts.

Because `console.error` and `console.warn` are wrapped, the script carries an inline source map that marks it as library code. DevTools then attributes a console message to your code's own line rather than to lerd's wrapper.

## Where to see it

- **Site → Debug → Browser** lists the site's events grouped per page view, with the stack and user agent one click away. The **Settings** button opens the site's capture settings, and **Clear** empties the browser events while keeping what the PHP side captured.
- **System → Debug bridge → Browser** shows every site's events, with a site filter.
- **MCP**: `diag` with `action: "browser_events"` returns the site's page views, newest first, each with what happened on it: errors with their stack and `file:line:col`, rejections, console messages, failed requests and configured events. It also returns a count per type, whether capture is on for lerd and for the site, and a `hint` that says why the list is empty, so no events is never mistaken for a page without errors. `types` narrows it, for example to `["error", "rejection", "network"]`, alongside `site`, `branch`, `since` and `limit`.

## Per-site settings

Settings live under `browser_capture` in the project's `.lerd.yaml`:

```yaml
browser_capture:
  enabled: true          # false turns capture off for this site only
  console: [error, warn] # console levels to report
  network: [5xx, failed] # failed requests to report: 4xx, 5xx, failed (no response)
  resources: false       # report elements whose file failed to load
  events: []             # DOM events to report, see Frontend library events
  presets: []            # store presets added to the site, see Presets
  navigation: true       # report page views, SPA navigations included
  verbose: false         # log each capture to the page's console
  route: /_lerd/browser  # where the script and endpoint are served
```

The dashboard writes to `.lerd.yaml` when the project has one. A project without one keeps its settings in lerd's site registry instead, so changing a setting never creates the file.

`route` is for an app that serves something under `/_lerd` itself. The script is served at the route plus `.js` and reports go to the route, and nginx maps both onto lerd-ui's fixed paths. It must be an absolute path of plain segments, such as `/__dev/js-errors`.

## Frontend library events

Most frontend frameworks need nothing extra. React, Vue, Svelte, Angular, Alpine and Stimulus either let an error escape as an uncaught error or log it with `console.error`, and both are captured already. Vue and Alpine.js log their warnings with `console.warn`, so turn that level on to see them.

Some libraries report a failure as a DOM event of their own instead. List those under `browser_capture.events` and each one is reported as an `event` in the Browser lens. `event` is the DOM event name, `label` names it in the dashboard, and `message` is an optional dot path into the event whose value becomes the message. A path that ends on an object or a list is shown as JSON, with an error as its name and message, a response as its status and URL, a DOM element as its tag and id, a Map or Set by its contents and a cycle as `[Circular]`, kept to three levels and twenty entries; point at a scalar such as `detail.response.status` when one value says it all. Only names and paths can be set, never code, because the script runs on every page of the site. Events are caught whether they are dispatched on `window`, `document` or an element, and whether or not they bubble. The same list can be edited from the site's Browser settings.

### Presets

The framework store publishes ready-made presets for libraries that report failures as DOM events of their own, each with the composer and npm packages that show a project uses the library:

| Preset | Adds the events |
|---|---|
| Inertia.js | `inertia:invalid`, `inertia:exception` |
| Hotwire Turbo | `turbo:fetch-request-error`, `turbo:frame-missing` |
| htmx | `htmx:responseError`, `htmx:sendError`, `htmx:swapError` |
| Vite | `vite:preloadError` |

A library that reports through the browser's own channels needs no preset, only the right box under **What to report**: Vue and Alpine.js log their warnings with `console.warn`, and a failed Livewire update arrives as a 4xx or 5xx response. React, Svelte, Angular and Stimulus need nothing at all, since their errors escape as uncaught errors or go to `console.error`, which are captured by default.

The site's Browser settings show the presets that are added or detected in the project, and **Show n more** lists the rest. Adding a preset copies its events into the site's own list, where they can be edited like any other, and records the preset's name under `presets`. Removing it takes out its events again, except one another added preset also declares. Nothing is turned on behind your back: a preset only applies once you add it.

The CLI and MCP see the same presets and detection results:

```bash
lerd browser-capture presets              # presets for the site in this directory, marking detected and added ones
lerd browser-capture preset add inertia   # add a preset's events
lerd browser-capture preset remove inertia
```

Over MCP, `diag` with `action: "browser_presets"` and a `site` lists the presets with `detected` and `applied`, and adding `preset` with `enable: true` or `false` adds or removes one.

### Writing events yourself

The presets are the same entries the examples below show, so a library without a preset is set up the same way.

Inertia, for a response that was not an Inertia response (usually an error page) and for an unexpected failure:

```yaml
browser_capture:
  events:
    - event: inertia:invalid
      label: Inertia invalid response
      message: detail.response.status
    - event: inertia:exception
      label: Inertia exception
      message: detail.exception.message
```

Turbo (Hotwire, Symfony UX Turbo), for a request that failed and for a frame response without the frame it should replace:

```yaml
browser_capture:
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
browser_capture:
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
browser_capture:
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
