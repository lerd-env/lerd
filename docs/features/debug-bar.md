# Debug bar

The debug bar puts the Debug window's view of a request on the page itself. It sums up the request that served the page and the requests the page sends afterwards, and a click opens the same request view the dashboard has, without leaving the site. It works on every site lerd serves, whatever framework it runs, and needs nothing installed in the app.

The bar is **off by default** and switched on per site:

- `lerd debugbar on` (and `off`, `status`) in the project's folder
- the **Debug bar** toggle in the toolbar of the site's Requests lens, on its Debug tab
- `devtools: debugbar: true` in the project's `.lerd.yaml`, which `lerd debugbar` and the toggle write to when the project has one; without one the choice is kept in lerd's site registry

Editing `.lerd.yaml` by hand works too: lerd-watcher notices and rewrites the site's vhost within a few seconds.

## What it shows

Each chip opens the request view on its own tab:

- the request: method, path and status, a red dot when it failed
- how long it took, with a bar splitting the time into before PHP (nginx and the FPM queue), PHP itself and the database
- peak memory
- queries, flagged N+1 when the request has one, views, cache calls and log lines
- the tabs the app wrote through [`lerd/debug`](debug-package.md), two on the bar and the rest under **+N**
- who the request ran as
- **child requests**: the calls the page sent after it loaded, each with its status, time and when it went out, failed ones counted in red

A chip only shows when there is something behind it, so a quiet request makes a short bar. While the request is still running the bar shows that it is loading, and it keeps up with the calls the page sends, checking every few seconds while the page is visible.

The request view is the dashboard's own: the timeline, the request and response, stack traces, GraphQL, the database and everything else, and a child request or the page that sent one is a click away inside it.

## Minimising and moving it

The **×** minimises the bar to lerd's mark in a corner, and clicking the mark brings it back. Dragging the mark moves it to another corner, which the browser remembers and which then wins over lerd's setting. The bar starts minimised until someone opens it, and remembers that too.

## How it looks

**System → Debug bar** sets how the bar looks on every site that shows it:

- **Style**: a floating **dock** at the bottom of the page, or a **compact** strip along the top or bottom edge with the labels written out. A compact strip takes room of its own, so the page's content starts beside it instead of under it.
- **Edge**: the edge a compact strip sits on.
- **Minimised in**: the corner the bar minimises to, until someone drags it elsewhere.
- **Theme**: **System** follows the operating system, **Light** and **Dark** are fixed. The colours follow the dashboard's theme, imported and desktop themes included.

The settings are kept under `debugbar` in `~/.config/lerd/config.yaml`.

## Sites lerd proxies

On a host-proxy or custom-container site, a dev server lerd proxies to, no PHP request serves the page, so the bar follows the page view [browser capture](browser-capture.md) names instead, including every navigation of a single-page app. The bar needs browser capture on for such a site; until it has a page view to show it sits as the minimised mark.

## Code and the editor

From a page on this machine, the bar's paths open in your editor and its stack traces show the code around each line, just like the dashboard. Reading a site's code and opening the editor need three things to be true, since nginx cannot tell one local browser from another:

- the LAN is not exposed (`lerd lan:expose` off), so nginx only listens on this machine
- the request did not come through a tunnel, which marks what it forwards
- it is the page's own script asking, a same-origin request

With the LAN exposed, the bar still shows everything else, and paths are there to copy.

## How it works

When the bar is on for a site, its vhost inserts a script before `</head>`, next to browser capture's when that is on, and proxies one route under the capture route, `/_lerd/browser/bar/` by default, to lerd-ui. nginx names the site in a header it sets itself, and lerd-ui only answers for requests that site served or its pages sent, so one site's page cannot read another site's requests.

The bar lives in a shadow root on an element of its own, a layer over the whole page that lets clicks through: the page's styles cannot reach it, its styles cannot reach the page, and nothing the page stacks high, a chat widget say, covers it. lerd serves its fonts itself; nothing is fetched from a font host.
