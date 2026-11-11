# Building a lerd plugin

Read this before writing a plugin for the user. A plugin adds behaviour to lerd on this machine: CLI commands, site commands, hooks on lerd's events, MCP tools, API endpoints, a dashboard page, a tab on every site, a card on a site's Overview, a script on a site's own pages, and services or frameworks of its own.

## Plugin or store

A new framework, a new framework version, or a service everyone could use belongs in lerd's stores, as YAML, not in a plugin: point the user at the lerd-env/frameworks and lerd-env/services repositories. A plugin is for behaviour that is code, or that only this user wants.

## Workflow

1. Pick the surfaces the request needs, and only those.
2. `plugin` `new` with a name (lowercase letters, digits, dashes). It writes a working boilerplate that uses every surface into `~/.local/share/lerd/plugins/<name>/` and returns the path.
3. Edit the files there in place. Delete every surface from `plugin.yaml`, and the files behind it, that the request does not need. lerd reads plugins on every call, so a change applies at once; the dashboard needs a reload to show a new page or tab.
4. `plugin` `list` after each edit. A plugin that does not load is listed with the reason, and nothing else of it runs.
5. Drive every surface you kept, as the user would, before saying it works (see Verifying below).

Building the plugin somewhere else, such as a repository the user will publish? Then `plugin` `install` with that folder's path copies it in. The installed copy does not follow the folder and an install never replaces one, so after a change run `lerd plugin remove <name>`, which keeps the plugin's data, and install again; or make the change in the installed folder too.

## plugin.yaml

Only `api_version` and `name` are required; `name` must match the folder. Paths are relative to the plugin folder and may not leave it. Every `run` is an executable file (chmod +x) whose first line names its interpreter.

```yaml
api_version: 1
name: notes
lerd: ">=1.38"              # oldest lerd it needs, a lower bound only; `new` fills it in
platforms: [linux, macos]   # omit for both; WSL counts as linux
version: 0.1.0
description: One line, shown in the dashboard and the catalog
docs: DOCS.md               # instructions shown on the plugin's Documentation tab, offline
icon: icon.svg              # single-colour SVG on its card; a rail page's icon stands in when omitted
screenshots: [docs/tab.png] # shown on its card in the dashboard; png, jpg, gif or webp

commands:                   # lerd notes:add
  - {name: add, usage: Add a note to this site, run: bin/add}
site_commands:              # a site's Commands menu
  - {name: export, label: Export notes, description: Write notes.md, run: bin/export, confirm: false}
hooks:                      # event -> executable
  site.linked: bin/on-event
mcp:
  skill: SKILL.md           # written beside lerd's own skill by lerd mcp:inject
  tools:                    # exposed as notes_add
    - name: add
      description: Add a note to a lerd site
      input_schema: {type: object, required: [site, text], properties: {site: {type: string}, text: {type: string}}}
      run: bin/add
api:                        # /api/plugins/notes/list
  - {path: list, methods: [GET], run: bin/list}
  - {path: wipe, methods: [POST], run: bin/wipe, loopback_only: true}
requires: [jq]              # commands the scripts call beyond their interpreters
ui:                         # only ui/ is served
  page: {title: Notes, icon: ui/icon.svg, entry: ui/index.html}
  site_tab: {title: Notes, entry: ui/site.html}
  overview_card: {title: Notes, entry: ui/card.html}
  page_script: ui/page.js
```

Events: `site.linked`, `site.unlinked`, `site.secured`, `site.php_changed`, `worktree.added`, `worktree.removed`, `service.started`, `service.stopped`, `lerd.started`, `lerd.stopped`. Any other name refuses the plugin.

A command or site command that ends the way a tool did can list the exits that are the tool's answer, such as `expected_exits: [101]` for `cargo check` finding code that does not compile. They reach the user as usual but are not recorded as the plugin failing; any other non-zero exit still is.

Time limits: hooks and API 30s, MCP tools 2m, site commands 30m, CLI none. `timeout:` raises one entry up to 10m (API, MCP) or 1h (site commands).

## What an executable gets

| | |
|---|---|
| `LERD_SURFACE` | `cli`, `site_command`, `hook`, `mcp` or `api` |
| `LERD_PLUGIN_NAME`, `LERD_PLUGIN_DIR` | the plugin and its folder |
| `LERD_PLUGIN_DATA` | its writable folder, kept across reinstalls; write nowhere else |
| `LERD_SITE`, `LERD_SITE_PATH`, `LERD_DOMAIN`, `LERD_BRANCH` | the site, when the surface has one |
| `LERD_BIN` | the lerd binary, for `$LERD_BIN php artisan ...` and every other lerd command |
| `LERD_API_URL`, `LERD_API_TOKEN` | lerd's own API; send the token as `X-Lerd-Plugin-Token` |

- CLI commands: argv, the terminal, the exit code. Run inside a linked project they get the site.
- Site commands: run in the site's folder (a worktree's when one is picked), output streams to the dashboard.
- MCP tools: the arguments as JSON on stdin; stdout is the answer, a non-zero exit is an error. A `site` argument hands over the site.
- API endpoints: the body on stdin, `LERD_METHOD`, `LERD_QUERY`; a `site` query parameter hands over the site. Exit 0 answers 200 with stdout (as JSON when it parses), anything else 500 with stderr.
- Hooks: `LERD_EVENT` and the payload, as JSON on stdin and as `LERD_<KEY>`. Output goes to `hooks.log` in the data folder and never fails the operation.

`curl -fsS -H "X-Lerd-Plugin-Token: $LERD_API_TOKEN" "$LERD_API_URL/api/sites"` lists the sites; `/api/services`, `/api/status`, `/api/stats` and `/api/disk` are the dashboard's own data.

To tell the user something, such as a command to run, declare `notifications: true` in plugin.yaml, then POST `{"title": "...", "body": "...", "url": "#sites/<site>", "tag": "..."}` to `$LERD_API_URL/api/notifications/plugin` with the token: only `title` is required (100 characters, `body` 300), `url` must start with `#`, a repeated `tag` replaces the earlier browser notification (desktop notifications show each), unknown fields are refused, and each plugin gets ten a minute. It reaches the user as a notification titled with the plugin's name, and only once they allowed it: lerd asks them at install, or at the update that adds the key, and answers `403` until then or after they switch it off, so treat a `403` as the user's choice and fall back to stderr. Never use it to report progress, and fall back to stderr when it fails, since older lerd answers 404.

## Rules

- Scripts only run on the machine, never in a container: shell, Python, Node, anything but PHP, which lerd refuses. PHP code runs through `$LERD_BIN php ...` in the site's folder.
- Write portable shell when `platforms` lists macos: no `sed -i`, `tac`, `\|` in sed, `readlink -f` or `date -d`.
- Mark `loopback_only: true` on every endpoint that runs code, changes files or restarts anything, so a dashboard on the LAN never reaches it.
- A page gets the dashboard's language as `lerd.context.lang` (`en`, `ro`, `zh-TW`...). English is enough; a plugin that adds translations picks its strings by it and falls back to English.
- Pages run in a sandboxed frame. Load the boilerplate's `ui/lerd.js` and call `lerd.api('<path>', {method, body})`; it reaches only this plugin's endpoints and resolves with the response text. A site tab or Overview card reads the site from `lerd.context.site`, and `domain` and `branch` beside it. `lerd.diff({path, original, modified, before, after})` opens the dashboard's own diff on two versions of a file, for a page that compares them, and `lerd.view({path, content, label})` shows one file read only; older lerd ignores both.
- An Overview card is a short summary, not a page: it grows to its content up to 800px, so keep it to a row or two and leave the detail to a site tab. It shows only on sites where the user ran `lerd plugin card <name> on` or switched it on in the site's header.
- Treat every value from the API, a site or a file as untrusted in a page: set `textContent`, or escape it before it goes into `innerHTML`.
- A page script shares the site's page with the app: draw inside a closed shadow root, as the boilerplate's `ui/page.js` does, and fetch only `/_lerd/plugins/<name>/api/...`. It is off until the user runs `lerd plugin page <name> on` in the project.
- Pages get lerd's look on their own, and must keep it: plain `<button>`, `<input>`, `<table>` and checkboxes already match the dashboard and its theme. Build the rest from lerd's classes: `lerd-label` for a section heading, `lerd-card` (`flush` for a table or rows) with `lerd-row` rows, `lerd-toolbar`, `lerd-tabs` with `aria-selected="true"`, `lerd-pill` (`ok`, `warn`, `bad`, `accent`), `lerd-empty`, `lerd-muted` and `lerd-mono`; `class="primary"` on the main button. Ask before anything destructive with a `<dialog>` opened by `showModal()`, its buttons in a `lerd-toolbar`, which looks like lerd's own dialogs, rather than `confirm()`. Write CSS for layout only, never colours, borders or fonts of your own. Ship a single-colour SVG icon, since the rail draws it as a mask.
- A service goes in `services/<name>.yaml`, written like a lerd-services preset, with the image pinned to a version and its source named in the plugin's README. A framework goes in `frameworks/<name>.yaml`. Names lerd already has are refused.
- `dump()` from PHP goes to the dashboard's Dumps tab while lerd's debug bridge is on, so print with `echo` or `print_r` where the output is the answer.

## Verifying

- CLI: `lerd <name>:<command>` in a project and outside one.
- MCP tools: call them; lerd lists them as `<name>_<tool>` once the plugin loads, though the client may need its tool list refreshed.
- API: `curl -fsS http://127.0.0.1:7073/api/plugins/<name>/<path>?site=<site>`.
- Hooks: cause the event (`lerd service start <svc>`, `lerd link`) and read `hooks.log` in the data folder.
- Pages, tabs and cards: open the dashboard and check the frame shows real data with no console errors; for a card, `lerd plugin card <name> on` in the project first; for a page script, `lerd plugin page <name> on` and load the site.
- Failures from any surface show on the plugin's card on the dashboard's Plugins page and in `plugin` `list`.

## Docs and screenshots

Write `DOCS.md` for the person who just installed the plugin: what it does, every command and tab, and anything to set up. It is the plugin's Documentation tab in the dashboard and works offline, so do not leave the boilerplate's text in it. Keep it apart from the README, which is the repository's page. Relative images in it show only when they are screenshots or under `ui/`, since those are the only plugin files lerd serves, and relative links to other files are shown as plain text. When the plugin adds a page, tab, card or page script, take a screenshot of it, list it under `screenshots`, and check it shows on the plugin's card.

## Sharing it

To list a plugin on plugins.lerd.sh, it goes in its own public repository and is submitted to lerd-env/plugins under the terms in its SUBMITTING.md: scripts a reviewer can read, no compiled binaries, pinned images, and screenshots when it adds UI.
