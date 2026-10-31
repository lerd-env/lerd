# Community plugins

A plugin adds behaviour to lerd without touching lerd itself: CLI commands, tools and a skill for AI assistants, a page on the dashboard's rail, a tab on every site, commands in a site's Commands menu, API endpoints, and hooks that run when something happens. A plugin is a folder holding a `plugin.yaml` and the files it names, and every one of those surfaces is served by running one of its executables, so a plugin can be written in anything that runs on the machine: a shell script, Python, Node, a compiled binary. PHP is the one exception, since lerd runs it in containers.

Frameworks and services are not plugins. They are declarative definitions that live in the [framework](../usage/frameworks.md) and [service](../usage/services.md) stores and reach every install on their own. A plugin is code you install yourself.

---

## Getting started

```bash
lerd plugin new hello       # scaffold a working plugin from the boilerplate
lerd plugin list            # installed plugins, and any that failed to load
lerd hello:greet shop       # run one of its commands
```

`lerd plugin new` writes the boilerplate to `~/.local/share/lerd/plugins/<name>/`. It exercises every surface apart from [services and frameworks](#services-and-frameworks), so the fastest way to write a plugin is to delete what you don't need from it. Reload the dashboard to see its rail page and site tab.

| Command | What it does |
|---|---|
| `lerd plugin new <name>` | Create a plugin from the boilerplate, ready to edit |
| `lerd plugin search [term]` | List the approved plugins in the catalog |
| `lerd plugin install <name>` | Install an approved plugin at the commit it was approved at |
| `lerd plugin update [name...]` | Move catalog plugins to their newly approved commit |
| `lerd plugin install <path\|git-url>` | Install a plugin from a directory or a git repository, outside the catalog |
| `lerd plugin remove <name>` | Remove a plugin; the data it stored is kept |
| `lerd plugin list` | List installed plugins, with the reason any of them failed to load |
| `lerd plugin page <name> on\|off` | Switch a plugin's page script on or off for the site in this directory |
| `lerd plugin card <name> on\|off` | Switch a plugin's Overview card on or off for the site in this directory |

The dashboard does the same on its **Plugins** page, which the puzzle-piece button at the end of the rail's plugin group opens, as do the command palette's **Plugins** and **Browse plugins** and, on a phone, the **Apps** page. **Installed** shows each plugin as a card with whether it loaded; opening one shows what it adds, what it requires, where it came from and its last failure, with **Update** and **Remove**, broken plugins included. The **Add a plugin** card that ends the list opens **Browse**, and **Install from a folder or git** above it opens a dialog that takes a folder path or a git URL. Because installing puts code on the machine, a remote dashboard only shows it once `lerd remote-control full-access on` is set.

A plugin that fails to load is skipped and listed with its error; it never stops lerd or the other plugins.

An installed plugin's card opens with its icon, name, version and author over four tabs. **Overview** shows its screenshots, where it came from and what it requires. **Features** names everything it adds: each command with its usage, each site command, AI tool, page, tab and hook. **Documentation**, when the plugin has docs, shows its own instructions. **Logs** lists its recent failures in full, newest first, each with a **Report to the author** link when it came from the catalog. lerd keeps the last 50 in a log file per plugin, `plugin-failures/<name>.log` in its data folder.

### Building one with an AI assistant

Ask your assistant for the plugin you want. lerd's MCP server has a `plugin` tool whose `guide` teaches it how a plugin is written, the rules it follows and how to check each surface works; `new` scaffolds the boilerplate and `list` shows what loaded and why anything did not. Claude Code also gets the guide as `plugins.md` beside lerd's skill when you run `lerd mcp:inject`. The assistant can install a plugin from a folder; installing from a git URL or the catalog stays a command you run.

### The catalog

[plugins.lerd.sh](https://plugins.lerd.sh) lists the plugins approved in the [lerd-env/plugins](https://github.com/lerd-env/plugins) registry, and the dashboard's **Browse** tab shows the same list as cards with each plugin's first screenshot, filtered by category, searched and sorted, a page at a time. Each card names the version the catalog approved beside the plugin's name, read from that commit's plugin.yaml, and so does its detail; a catalog from before versions were listed shows none. Opening a card shows all its screenshots and details, with the Install button. Each listing names a repository and the exact commit a maintainer approved, and that commit is the only one lerd installs: it fetches that commit alone, checks it is the one it got, and keeps no git history beside it, so a repository that moves on later changes nothing on your machine. When a newer commit is approved, the dashboard says so: the Plugins button on the rail carries a count, the plugin's card shows `↑` with the new version, and its detail shows the version it moves from and to, with a **What changed** link to GitHub's comparison of the two commits. **Update** in the detail, **Update all** on the Installed page or `lerd plugin update` moves to it, keeping the data the plugin stored. The dashboard re-reads the catalog every hour for this, and raises one notification per newly approved commit, under the same Updates setting as lerd's own; nothing on the page waits on that check.

To be listed, a plugin meets the registry's [approval terms](https://github.com/lerd-env/plugins/blob/main/SUBMITTING.md). Among them, everything it runs is a script a reviewer can read, in shell, Python, Node or similar: compiled binaries are not listed, though a plugin you install from a folder or your own repository can be a binary too.

lerd reads the catalog at most once a day from the command line and every hour while the dashboard runs, and keeps the last copy, so browsing and installed plugins work offline. To try another index, such as a registry pull request's, set `LERD_PLUGIN_CATALOG` to its `index.json` URL for the command: `LERD_PLUGIN_CATALOG=http://127.0.0.1:8000/index.json lerd plugin search`. That index is cached on its own, so the real catalog is never mixed in. Plugins installed from a folder or a repository of your own show as not listed and are never updated from the catalog.

#### When a plugin is pulled

A plugin removed from the catalog is simply no longer offered: what you installed keeps running, and gets no more updates. A plugin pulled for a security problem is **revoked** instead, with a reason. On its next catalog check, lerd stops a revoked plugin you have installed: its commands, hooks, pages, tabs, tools and endpoints stop running, it shows as broken on the Plugins page with the reason, and the dashboard shows a banner on every page until you answer, plus one notification. **Remove** uninstalls it; **Keep using it**, or `lerd plugin keep <name>`, runs it again if you trust it, until a later revocation asks again. A revocation can name the commits it covers, so once a fixed commit is approved, an install at an older one is stopped and offered the update.

A listing means a maintainer read the plugin's code at one commit. It is not a warranty or a security audit: a plugin is its author's work, it runs as you, and lerd and its maintainers are not responsible for what an installed plugin does.

---

## plugin.yaml

Every key except `api_version` and `name` is optional. Paths are relative to the plugin's folder and may not point outside it; every `run` must be an executable file.

```yaml
api_version: 1
name: hello                 # lowercase letters, digits and dashes; must match the folder name
lerd: ">=1.37"              # the oldest lerd it runs on; lerd plugin new fills in the one that wrote it
docs: DOCS.md               # instructions on its Documentation tab in the dashboard, read offline
icon: icon.svg              # single-colour SVG on its card; its rail page's icon when omitted
screenshots: [docs/tab.png] # images shown on its card in the dashboard
platforms: [linux, macos]   # where it runs; both when omitted
notifications: true         # it sends notifications, once the user allows it
version: 0.1.0
description: A starting point for a lerd plugin

commands:                   # lerd hello:greet
  - name: greet
    usage: Print a greeting for a site
    run: bin/greet          # a shell script; each file's first line picks its language
  - name: sites
    usage: List lerd's sites through its API
    run: bin/sites.js       # Node
  - name: report
    usage: Summarise a site's slowest requests
    run: bin/report.py      # Python, found on the machine's path

site_commands:              # a site's Commands menu and the command palette
  - name: greet
    label: Greet
    description: Print a greeting for this site
    confirm: false          # ask before running
    run: bin/greet
    timeout: 45m            # longer than the 30 minute default, up to 1h

hooks:                      # event -> executable
  site.linked: bin/on-event

mcp:
  skill: SKILL.md           # installed by lerd mcp:inject as .claude/skills/lerd-hello/SKILL.md
  tools:
    - name: greet           # exposed as hello_greet
      description: Return a greeting for a lerd site
      input_schema:         # JSON Schema for the arguments, passed to the assistant as is
        type: object
        properties:
          site: {type: string}
      run: bin/greet

api:                        # served at /api/plugins/hello/greet
  - path: greet
    methods: [GET]          # GET when omitted
    run: bin/greet
    loopback_only: false    # true keeps it to the lerd host, like lerd's own host actions
    timeout: 2m             # longer than the 30 second default, up to 10m

requires: [jq]              # commands its scripts call, beyond the interpreters lerd reads itself

provides:                   # jobs the plugin takes over from lerd, see below
  worktree_database: bin/db

ui:                         # files live under ui/, the only folder the dashboard serves
  page:     {title: Hello, icon: ui/icon.svg, entry: ui/index.html}
  site_tab: {title: Hello, entry: ui/site.html}
  overview_card: {title: Hello, entry: ui/card.html}   # on a site's Overview once switched on for that site
  page_script: ui/page.js   # runs on a site's pages once switched on for that site
```

A manifest written for a newer lerd still loads as long as its `api_version` is one this lerd knows: unknown keys are ignored. An unknown value in a known key, such as a hook for an event this lerd doesn't have, refuses the plugin with a message naming it.

### Docs and screenshots

`docs` names a markdown file in the plugin, `DOCS.md` in the boilerplate, which the dashboard renders on the plugin's **Documentation** tab from the installed copy, so it reads without a network. It is for the person who installed the plugin: what it does, how to use each part and what to set up. The README stays the repository's page, free to say anything else. `screenshots` lists images in the plugin, png, jpg, gif or webp, shown on its card. `icon` is a single-colour SVG drawn on the plugin's card, in the theme's colour like lerd's own icons; a plugin without one shows its rail page's icon, or a puzzle piece.

Only the plugin's `ui/` folder, its `icon` and the files listed under `screenshots` are served over HTTP, so an image in the docs shows when it is one of those, and a relative link to another file in the plugin is shown as plain text. Raw HTML in the docs is dropped. A `docs` file that is missing or not markdown, or a screenshot that is missing or not an image, refuses the plugin with a message naming it.

### Which lerd a plugin runs on

`lerd` names the oldest release the plugin needs, compared on major and minor. Give it a lower bound only, never an upper one: every later lerd keeps loading the plugin, so it never needs a new version just because lerd shipped one. A lerd older than the bound refuses to install or load it and says which release it needs, and the Browse tab shows the requirement in place of the Install button. An approved update that needs a newer lerd than yours is not offered until you update lerd. A beta counts as the release it leads to, so 1.38.0-beta.1 installs a plugin that needs `>=1.38`, and a lerd built from source past its last tag installs any plugin, since it already holds the next release.

A change that would break existing plugins raises `api_version` instead, which is the one thing that can make a plugin stop loading on a newer lerd.

### Which platforms a plugin runs on

`platforms` lists where the plugin runs, `linux`, `macos` or both, and a plugin that leaves it out runs on both. Set it when a script depends on one of them, such as GNU `sed -i` or a tool only Linux has. On Windows lerd runs inside WSL, so plugins there run on `linux`. On a platform the plugin doesn't list, lerd refuses to install or load it and says where it runs, the Browse tab shows that in place of the Install button, and an update is not offered. Any value other than `linux` or `macos` is refused by name.

---

## Services and frameworks

A plugin can bring services and frameworks of its own, written exactly like the ones in the [service](https://github.com/lerd-env/services) and [framework](https://github.com/lerd-env/frameworks) stores, one file each:

```
hello/
  plugin.yaml
  services/queuebox.yaml      # name: queuebox
  frameworks/acme.yaml        # name: acme
```

A service shows up in the service picker beside lerd's own and is installed from there like any other; installing the plugin starts nothing. A framework is detected, listed by `lerd framework list` with the source `plugin`, and serves sites like a framework of your own. Each file's `name` must match its file name, and a framework comes as a single `<name>.yaml` with no per-version files.

A plugin that ships a service must say in its README where each one comes from: the image it runs and the project that publishes it, so whoever installs the plugin knows what they are pulling before they pull it. The catalog only approves plugins that do.

A plugin only adds names. One that ships a service or framework lerd already has, from itself, the store, your own definitions or another plugin, is refused at install with the names it clashes on. Should the store publish the same name later, the store's definition wins. A service installed from a plugin's definition is marked as that plugin's, as `[plugin <name>]` in `lerd service list` and "from plugin" on its dashboard card, and can still be removed on its own like any other. Removing the plugin takes its definitions away and stops and removes every service installed from them, so nothing it brought keeps running; each service's data is kept, the way `lerd service remove` keeps it, and goes with `lerd service remove <name> --purge` once the plugin is gone.

---

## Taking over a job from lerd

A plugin can declare itself in charge of a job lerd normally does, under `provides`. Lerd then hands that job to the plugin instead of doing it, and only one installed plugin can be in charge of a job: installing a second one is refused with the name of the first. A lerd that doesn't know a job refuses the plugin and names the job.

### worktree_database

Whenever a worktree gets a database of its own, from `lerd worktree add`, `lerd db:isolate`, the dashboard or an assistant, lerd runs the plugin's executable instead of creating and copying the database itself. It is meant for tools that branch a database copy-on-write, where lerd's own copy is a full dump and import.

The executable runs as `bin/db create` or `bin/db drop`, with the request as JSON on stdin and each field also as `LERD_<FIELD>`:

| Field | create | drop |
|---|---|---|
| `site`, `branch` | the site and the worktree's branch | same |
| `service` | the lerd service holding the parent's database, like `mysql` | same |
| `database` | the name lerd would have used | the database to drop |
| `parent_database` | the parent site's database | |
| `source` | `empty`, `main`, or the branch whose isolated database to copy | |
| `site_path`, `worktree_path` | the two checkouts | |

`create` prints a JSON object with the `database` it made and, when it serves that database somewhere other than the parent's service, the `host` to reach it on. Lerd writes both into the worktree's env file under the keys the framework declares, so env stays with the site, and records the plugin as the database's owner. Any other field in the answer is refused by name. Sharing the parent's database again, or removing the worktree, runs `drop` with the database `create` answered.

When `create` fails, the worktree keeps sharing the parent's database and the error carries what the plugin wrote to stderr. Lerd never falls back to a full copy on its own. When `drop` fails, the worktree stays on its own database and lerd keeps the record of it, so nothing is left behind unaccounted for. A database kept from a removed worktree is reconnected to only through the plugin that made it: with that plugin removed or no longer in charge, reusing it is refused and the record stays, rather than an empty database taking its place. Lerd's database tools, such as snapshots and the database shell, still work only on databases a lerd service holds.

---

## What a plugin's executable receives

Every surface runs the file the same way. Context arrives in environment variables, and anything structured arrives as JSON on stdin.

| Variable | Value |
|---|---|
| `LERD_PLUGIN_NAME` | The plugin's name |
| `LERD_PLUGIN_DIR` | The plugin's folder |
| `LERD_PLUGIN_DATA` | A writable folder of its own, kept when the plugin is removed or reinstalled |
| `LERD_SURFACE` | Who ran it: `cli`, `site_command`, `hook`, `mcp` or `api` |
| `LERD_BIN` | The lerd binary, for calling lerd commands |
| `LERD_API_URL`, `LERD_API_TOKEN` | Access to lerd's API, see below |
| `LERD_SITE`, `LERD_SITE_PATH`, `LERD_DOMAIN` | The site, when the surface has one |

Per surface:

- **CLI commands** get their arguments on argv and the terminal's stdin, stdout and exit code. Run inside a linked project, they also get the site.
- **Site commands** run in the site's directory with `LERD_BRANCH` set when a worktree is selected, and stream their output to the dashboard's run modal like any other command.
- **MCP tools** get the tool's arguments as JSON on stdin; stdout is the result, and a non-zero exit is reported to the assistant as an error. A `site` argument hands over the site.
- **API endpoints** get the request body on stdin and `LERD_METHOD` and `LERD_QUERY`; a `site` query parameter hands over the site. A zero exit answers 200 with stdout, served as JSON when it parses as JSON, and any other exit answers 500 with stderr.
- **Hooks** get `LERD_EVENT` and the event's payload, both as JSON on stdin and as `LERD_<KEY>` variables.

### How long it may run

Every surface but the CLI stops an executable that runs past its limit, together with everything it started, and records it as a failure, so a stuck script never holds up the dashboard, an assistant or the operation that fired a hook. A CLI command has no limit, since it runs in your terminal where Ctrl-C is yours.

| Surface | Default | `timeout:` up to |
|---|---|---|
| Hooks | 30s | fixed |
| API endpoints | 30s | 10m |
| MCP tools | 2m | 10m |
| Site commands | 30m | 1h |
| Jobs under `provides` | 5m | fixed |

`timeout` takes a duration like `90s` or `10m` on a single site command, MCP tool or API endpoint. A value past the ceiling refuses the plugin with a message naming it.

### Which language

Lerd runs the file itself, on this machine, so its first line picks the interpreter: `#!/bin/sh`, `#!/usr/bin/env python3` or `#!/usr/bin/env node`. PHP is the exception: lerd keeps PHP in its containers rather than on the machine, so a script whose first line names `php` is refused, with a message saying so.

Lerd reads that first line from every file the manifest runs, and adds anything else the scripts call that you list under `requires`, such as `jq` or `git`. Together they are the plugin's requirements, shown on its card in **Browse** and in the installed list. A plugin whose requirements this machine lacks is refused at install with the names it is missing, and one that loses a requirement later shows as broken with the same reason. Lerd's own bin folder is always on the path a plugin runs with, so a `node` lerd installed is found even when the dashboard's service started with a bare one.

---

## Events

| Event | Payload |
|---|---|
| `site.linked` | `site`, `site_path`, `domain` |
| `site.unlinked` | `site`, `site_path`, `domain` |
| `site.secured` | site keys, `secured` (`true` or `false`) |
| `site.php_changed` | site keys, `php_version` |
| `worktree.added` | site keys, `worktree` |
| `worktree.removed` | site keys, `worktree` |
| `service.started` | `service` |
| `service.stopped` | `service` |
| `lerd.started` | none |
| `lerd.stopped` | none |

Events fire once, from wherever the change was made: the CLI, the dashboard, the TUI or an AI assistant. Hooks run one after another once the operation has succeeded; a hook's output and failures go to `hooks.log` in its data folder and never fail the operation. A hook that runs longer than 30 seconds is stopped.

---

## Calling lerd from a plugin

Every plugin process gets `LERD_API_URL` and `LERD_API_TOKEN`, and may call the same API the dashboard uses by sending the token in the `X-Lerd-Plugin-Token` header:

```bash
curl -fsS -H "X-Lerd-Plugin-Token: $LERD_API_TOKEN" "$LERD_API_URL/api/sites"
```

The token is the plugin's own: lerd derives it from the plugin's name and a secret only your user can read, so a call can be traced to the plugin that made it. For anything else, `$LERD_BIN` runs lerd's CLI.

`/api/services` gives each service's `label`, the name the dashboard shows when its definition sets one (capitalize the `name` when it is absent), and its `domain` when lerd serves it on one, so a plugin listing services needs neither the dashboard's naming rules nor lerd's config files.

### Telling the user something

A hook runs where nobody watches, so a plugin that needs the user to act sends a notification instead. It says so in its manifest:

```yaml
notifications: true
```

and lerd asks the user whether to allow it when the plugin is installed, or when an update first adds the key: in the terminal for `lerd plugin install` and `lerd plugin update`, and in the dashboard's Plugins page otherwise, including for one installed from a terminal that could not be asked. The answer holds across updates and is forgotten when the plugin is removed. Each plugin that asks has its own switch on its card and under **Plugin messages** in the notification settings, and `lerd plugin notifications <name> on|off` sets it from the terminal, so a plugin that is too loud is silenced without the rest. The answer is kept in lerd's own config, and a request carrying the plugin's token cannot give it.

Once allowed, it sends with the same token:

```bash
curl -fsS -X POST -H "X-Lerd-Plugin-Token: $LERD_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Hosts file out of date","body":"myapp.test is missing from the hosts file.","tag":"hosts"}' \
  "$LERD_API_URL/api/notifications/plugin"
```

| Field | | |
| --- | --- | --- |
| `title` | required | At most 100 characters. Shown as `<plugin>: <title>`, so a message never passes for lerd's own |
| `body` | optional | At most 300 characters |
| `url` | optional | A dashboard link starting with `#`, such as `#sites/myapp`, opened when the notification is clicked |
| `tag` | optional | `a-z`, `0-9` and `-`, at most 64. In the browser and Web Push, a newer message with the same tag replaces the older one. The dashboard drops a repeat with the same tag within two seconds, and desktop notifications show each message. Without a tag, every message stands alone |

It is delivered like every other notification, to the dashboard, Web Push or the desktop, under its own **Plugin messages** category the user can switch off. The plugin is the one the token belongs to. lerd answers `204` once it is sent, or when notifications are off; `403` when the manifest does not declare `notifications` or the user has not allowed it; `400` for a field it does not know or a value out of bounds; `401` without a valid token; `429` past ten messages a minute from the same plugin. Older lerd answers `404`, so a plugin that relies on it declares the release that added it in `lerd:`.

---

## Dashboard pages and tabs

A rail page opens in the dashboard's panel, the same place service dashboards open, under its own rule on the rail below lerd's launchers; a site tab sits beside Overview, Logs and the rest, and receives `site`, `domain` and `branch` as query parameters. An Overview card is a section of its own on a site's Overview, below the services, and receives the same parameters. It is off on every site until switched on from the plugin icon in the site's header or with `lerd plugin card <name> on` in the project, and it grows to the height of its page, which the boilerplate's `ui/lerd.js` reports for it.

Plugin pages run in a sandboxed frame with no access to the dashboard, so a plugin page can never act with the dashboard's standing. It reaches its own API endpoints through the bridge in the boilerplate's `ui/lerd.js`:

```js
const text = await lerd.api('greet?site=' + encodeURIComponent(lerd.context.site));
await lerd.api('save', { method: 'POST', body: { key: 'value' } });
```

`lerd.api` only reaches `/api/plugins/<this plugin>/`.

### Speaking the user's language

A page also receives the dashboard's language as `lang`, like `en`, `ro` or `zh-TW`: `lerd.context.lang` in the boilerplate's `ui/lerd.js`, and the page's `<html lang>` when it sets none of its own. A plugin that ships translations picks its strings by it, falling back to English. Switching the dashboard's language reloads the dashboard, and the page with it.

### Looking like lerd

Every plugin page the dashboard serves starts with lerd's own look, in the dashboard's light or dark mode and its theme's accent, following the user as they switch. Plain elements already match: a `<button>` is lerd's 28px control, `<input>`, `<select>` and `<textarea>` are its fields, a `<table>` has its uppercase headers and row lines, and checkboxes take the accent. Mark the main button `class="primary"` and a destructive one `class="danger"`.

For the rest, use the dashboard's own pieces:

| Class | What it draws |
|---|---|
| `lerd-label` | The small uppercase heading above a block |
| `lerd-section` | A block with the dashboard's spacing below it |
| `lerd-card` | A bordered card; add `flush` for one holding a table or rows |
| `lerd-row` | A row in a flush card, with a line between rows |
| `lerd-toolbar` | A row of buttons; a `spacer` inside pushes what follows to the right |
| `lerd-tabs` | Underlined tabs; mark the open one `aria-selected="true"` |
| `lerd-pill` | A method, status or count; `ok`, `warn`, `bad` or `accent` colour it |
| `lerd-empty` | A centred empty state, its title in a `<strong>` |
| `lerd-muted`, `lerd-mono` | Secondary text, and code-styled text |
| `clickable`, `selected` on a `<tr>` | A table row that opens something, and the one that is open |
| `<dialog>` | A dialog like the dashboard's, opened with `showModal()`, its buttons in a `lerd-toolbar` |

Every rule has no weight, so any CSS the page writes wins over it; keep the page's own CSS to layout. The colours are custom properties too: `--lerd-accent`, `--lerd-on-accent`, `--lerd-text`, `--lerd-text-muted`, `--lerd-card`, `--lerd-line`, `--lerd-ok`, `--lerd-warn` and `--lerd-bad`, and the root has the `dark` class in dark mode. Plugins in the catalog are asked to build their pages from these, so every plugin looks like part of lerd.
---

## Scripts on site pages

A plugin's `page_script` runs on a site's own pages, for a toolbar, a debug bar or anything else that belongs on the page rather than in the dashboard. It is off everywhere until switched on for a site, from the plugin icon in the site's header or with `lerd plugin page <name> on` in the project. lerd adds it before `</head>` through nginx, the same way it adds browser logs.

The script runs in the site's origin, so it reaches its plugin's endpoints with a plain same-origin fetch under `/_lerd/plugins/<name>/api/`:

```js
const res = await fetch('/_lerd/plugins/hello/api/greet');
```

Those calls hand the endpoint the site, and when the page is a worktree's they run in that worktree's checkout with `LERD_BRANCH` set. They are only answered for a site the plugin is switched on for, only from that site's own pages, and never for a `loopback_only` endpoint, since a phone on the LAN may be the one showing the page. The script shares the page with the app, so draw inside a shadow root as the boilerplate's `ui/page.js` does.

---

## Access and security

A plugin runs as you, with everything you can do, so install only plugins you trust; lerd and its maintainers are not responsible for what one does. Its API endpoints sit behind the same rules as lerd's own: open on the lerd host, closed to the LAN unless [remote control](../reference/commands.md#full-lan-exposure-dns-based) is on and the browser has signed in, and an endpoint marked `loopback_only` stays closed to remote dashboards until `lerd remote-control full-access on`. Only the plugin's `ui/` folder, its icon and its listed screenshots are served over HTTP, and a link among them that leads elsewhere is not followed.

---

## When a plugin fails

Lerd records each plugin's failures, whichever surface they came from: a hook, a command, a site command, an API endpoint, an MCP tool or a job it is in charge of. The last 50 stay in the plugin's own log file, `plugin-failures/<name>.log` in lerd's data folder. Its card on the **Plugins** page shows an amber dot after a failure, and its **Logs** tab lists them in full, newest first. For a plugin installed from the catalog, each one has a **Report to the author** link that opens a new issue on its repository, filled in with the plugin's commit, your lerd version and the error. A failing `lerd <plugin>:<command>` prints the same link in the terminal.

A failure you were not watching, such as a hook the watcher ran, also raises a **Plugin failures** notification, which can be switched off in the notification settings like any other. The same failure again within the hour stays quiet. Reports go to the plugin's author rather than to lerd, since only they can fix their plugin; the issue says where to send it on if lerd turns out to be at fault.

---

## When lerd can't do it yet

If your plugin needs an event lerd does not fire or a job it cannot hand over yet, [open an issue](https://github.com/lerd-env/lerd/issues) describing the plugin and what it needs, or add it yourself following [Extending the Plugin System](../contributing/plugins.md). Don't work around the gap by editing lerd's configuration files or reaching into its containers: a plugin that does is not approved for the catalog, and the next lerd release can break it.
