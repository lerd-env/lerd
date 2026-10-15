# Plugins

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

The dashboard does the same under **System → Plugins**: it lists every installed plugin with what it adds, installs one from a folder path or a git URL, and removes one, broken ones included. Because installing puts code on the machine, a remote dashboard only shows it once `lerd remote-control full-access on` is set.

A plugin that fails to load is skipped and listed with its error; it never stops lerd or the other plugins.

### The catalog

[plugins.lerd.sh](https://plugins.lerd.sh) lists the plugins approved in the [lerd-env/plugins](https://github.com/lerd-env/plugins) registry, and the dashboard's **Browse** tab shows the same list with an Install button. Each listing names a repository and the exact commit a maintainer approved, and that commit is the only one lerd installs: it fetches that commit alone, checks it is the one it got, and keeps no git history beside it, so a repository that moves on later changes nothing on your machine. When a newer commit is approved, the plugin shows an **Update** button and `lerd plugin update` moves to it, keeping the data the plugin stored.

To be listed, a plugin meets the registry's [approval terms](https://github.com/lerd-env/plugins/blob/main/SUBMITTING.md). Among them, everything it runs is a script a reviewer can read, in shell, Python, Node or similar: compiled binaries are not listed, though a plugin you install from a folder or your own repository can be a binary too.

lerd reads the catalog at most once a day and keeps the last copy, so browsing and installed plugins work offline. Plugins installed from a folder or a repository of your own show as not listed and are never updated from the catalog.

---

## plugin.yaml

Every key except `api_version` and `name` is optional. Paths are relative to the plugin's folder and may not point outside it; every `run` must be an executable file.

```yaml
api_version: 1
name: hello                 # lowercase letters, digits and dashes; must match the folder name
lerd: ">=1.37"              # the oldest lerd it runs on; lerd plugin new fills in the one that wrote it
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
  page_script: ui/page.js   # runs on a site's pages once switched on for that site
```

A manifest written for a newer lerd still loads as long as its `api_version` is one this lerd knows: unknown keys are ignored. An unknown value in a known key, such as a hook for an event this lerd doesn't have, refuses the plugin with a message naming it.

### Which lerd a plugin runs on

`lerd` names the oldest release the plugin needs, compared on major and minor. Give it a lower bound only, never an upper one: every later lerd keeps loading the plugin, so it never needs a new version just because lerd shipped one. A lerd older than the bound refuses to install or load it and says which release it needs, and the Browse tab shows the requirement in place of the Install button. An approved update that needs a newer lerd than yours is not offered until you update lerd.

A change that would break existing plugins raises `api_version` instead, which is the one thing that can make a plugin stop loading on a newer lerd.

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

A plugin only adds names. One that ships a service or framework lerd already has, from itself, the store, your own definitions or another plugin, is refused at install with the names it clashes on. Should the store publish the same name later, the store's definition wins. Removing the plugin takes its definitions away, while a service you installed from it stays installed until you remove it.

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

When `create` fails, the worktree keeps sharing the parent's database and the error carries what the plugin wrote to stderr. Lerd never falls back to a full copy on its own. Lerd's database tools, such as snapshots and the database shell, still work only on databases a lerd service holds.

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

Every surface but the CLI stops an executable that runs past its limit and records it as a failure, so a stuck script never holds up the dashboard, an assistant or the operation that fired a hook. A CLI command has no limit, since it runs in your terminal where Ctrl-C is yours.

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

---

## Dashboard pages and tabs

A rail page opens in the dashboard's panel, the same place service dashboards open; a site tab sits beside Overview, Logs and the rest, and receives `site`, `domain` and `branch` as query parameters.

Plugin pages run in a sandboxed frame with no access to the dashboard, so a plugin page can never act with the dashboard's standing. It reaches its own API endpoints through the bridge in the boilerplate's `ui/lerd.js`:

```js
const text = await lerd.api('greet?site=' + encodeURIComponent(lerd.context.site));
await lerd.api('save', { method: 'POST', body: { key: 'value' } });
```

`lerd.api` only reaches `/api/plugins/<this plugin>/`. The rail icon is drawn as a mask in the rail's own colour, so ship a single-colour SVG.

---

## Scripts on site pages

A plugin's `page_script` runs on a site's own pages, for a toolbar, a debug bar or anything else that belongs on the page rather than in the dashboard. It is off everywhere until switched on for a site, from the plugin icon in the site's header or with `lerd plugin page <name> on` in the project. lerd adds it before `</head>` through nginx, the same way it adds browser logs.

The script runs in the site's origin, so it reaches its plugin's endpoints with a plain same-origin fetch under `/_lerd/plugins/<name>/api/`:

```js
const res = await fetch('/_lerd/plugins/hello/api/greet');
```

Those calls hand the endpoint the site, and the worktree when the page is a worktree's. They are only answered for a site the plugin is switched on for, only from that site's own pages, and never for a `loopback_only` endpoint, since a phone on the LAN may be the one showing the page. The script shares the page with the app, so draw inside a shadow root as the boilerplate's `ui/page.js` does.

---

## Access and security

A plugin runs as you, with everything you can do, so install only plugins you trust. Its API endpoints sit behind the same rules as lerd's own: open on the lerd host, closed to the LAN unless [remote control](../reference/commands.md#full-lan-exposure-dns-based) is on and the browser has signed in, and an endpoint marked `loopback_only` stays closed to remote dashboards until `lerd remote-control full-access on`. Only the plugin's `ui/` folder is served over HTTP.

---

## When a plugin fails

Lerd keeps each plugin's latest failure, whichever surface it came from: a hook, a command, a site command, an API endpoint, an MCP tool or a job it is in charge of. Its row in **System > Plugins** shows what failed and the first line of the error, and for a plugin installed from the catalog a **Report to the author** link opens a new issue on its repository, filled in with the plugin's commit, your lerd version and the error. A failing `lerd <plugin>:<command>` prints the same link in the terminal.

A failure you were not watching, such as a hook the watcher ran, also raises a **Plugin failures** notification, which can be switched off in the notification settings like any other. The same failure again within the hour stays quiet. Reports go to the plugin's author rather than to lerd, since only they can fix their plugin; the issue says where to send it on if lerd turns out to be at fault.

---

## When lerd can't do it yet

If your plugin needs an event lerd does not fire or a job it cannot hand over yet, [open an issue](https://github.com/lerd-env/lerd/issues) describing the plugin and what it needs, or add it yourself following [Extending the Plugin System](../contributing/plugins.md). Don't work around the gap by editing lerd's configuration files or reaching into its containers: a plugin that does is not approved for the catalog, and the next lerd release can break it.
