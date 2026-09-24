# Workspaces

Once you have more than a handful of sites, one flat list stops being useful. Workspaces let you group sites the way you actually think about them, separating client work from experiments.

A workspace is purely organisational. It never touches nginx, domains, certificates or `.env`, and it never changes how a site is served. It is also not the same thing as a [site group](site-groups.md), which binds a main site's subdomains together and does rewrite vhosts and certificates. A site can belong to a group and a workspace at the same time.

## Commands

| Command | Description |
|---|---|
| `lerd workspace add <name>` | Create an empty workspace |
| `lerd workspace rename <old> <new>` | Rename a workspace, keeping its sites |
| `lerd workspace rm <name>` | Delete a workspace; its sites become ungrouped |
| `lerd workspace assign <site> <workspace\|none>` | Move a site into a workspace, or out of one with `none` |
| `lerd workspace move <name> <position>` | Reposition a workspace in the display order (`0` is first) |
| `lerd workspace list` | List the workspaces and their sites |

---

## Where they live

Workspaces are a personal preference rather than project state, so they live in your global config at `~/.config/lerd/config.yaml` and are never written to `.lerd.yaml` or the site registry:

```yaml
workspaces:
  - name: Client Work
    sites: [astrolov, acme]
  - name: Side Projects
    sites: [blog]
```

A site that appears in no workspace is ungrouped. An empty workspace is fine and survives a restart, so you can create one before you have anything to put in it. The order of the list is the order the sections are shown in. Unlinking a site drops it from its workspace, so a different project linked under the same name later starts out ungrouped.

Only a group main is ever written to the list. A [group secondary](site-groups.md) always displays in its main's workspace, so it has no membership of its own and `lerd workspace assign` will point you at the main instead. The name `none` is reserved: it is how you ungroup a site from the command line, and it labels the ungrouped option in the picker.

## In the web UI

The sites sidebar renders one collapsible section per workspace, followed by the ungrouped sites and then the paused ones. Collapse state is remembered per browser.

Drag a site row between sections to move it. Dragging a [site group](site-groups.md) main carries its secondaries with it, since a secondary always shows in its main's workspace. Drag a workspace header to reorder the sections; that moves whole blocks and never changes the order of sites within them. Rename and delete live in the menu on each header, and deleting a workspace only ungroups its sites, it never removes them. The **Add workspace** button sits next to the sort control at the bottom of the list.

Each site's detail header also has a workspace picker, which can create a new workspace and move the site into it in one step.

The Sites Overview groups its tiles by workspace too. Empty workspaces are hidden there, since the sidebar is where you manage them, and each tile still shows its framework as a badge. Until you create your first workspace the overview keeps grouping by framework, the way it always has.

## In the TUI

Press `o` in the sites pane to cycle the sort order until it reads `sort: workspace`. Sites are then listed under a header per workspace, with the ungrouped ones trailing. The TUI shows workspaces but does not edit them; use the web UI or `lerd workspace`.

## Streaming mode

When you share your screen, a stream or a meetup talk can show client projects you would rather keep to yourself. Mark those sites or whole workspaces private, then turn streaming mode on while you share.

To mark something private, pick **Hide while streaming** in the menu on a workspace header, or in a site's overflow menu. A private workspace takes every site in it along, and a [group secondary](site-groups.md) follows its main. The flags are stored as `private: true` on the workspace in `config.yaml` and on the site in the registry.

Turn the mode on with the eye button before the running count on the Sites card of the dashboard and in the Sites Overview header, or from a shell:

| Command | Description |
|---|---|
| `lerd streaming on` | Hide private sites and workspaces |
| `lerd streaming off` | Show them again |

On Linux under Wayland lerd can also switch it for you. Run `lerd streaming auto on` and the watcher turns streaming mode on as soon as a screen share starts, then off again when the share ends. It never turns off a mode you switched on yourself, and switching it off by hand during a share sticks until the next one. Detection reads PipeWire: every Wayland share goes through the desktop portal, which publishes the captured screen as a video source, so it works the same for a browser call, OBS or a meeting app. Recording counts as well when the recorder goes through PipeWire, as OBS, Spectacle and GNOME's built-in recorder do, and so does a virtual camera published over PipeWire. Plasma's taskbar previews use the same streams but are ignored. Recorders that grab the screen directly, such as wf-recorder or gpu-screen-recorder, cannot be seen. X11 sessions and macOS have no way to see a share, so there the toggle stays manual.

| Command | Description |
|---|---|
| `lerd streaming auto on` | Turn streaming mode on while the screen is shared |
| `lerd streaming auto off` | Stop following screen shares |

While it is on, the private sites and workspaces are left out of what lerd-ui sends to the browser, so they are absent from the sidebar, the dashboard, the overview and the command palette on every open dashboard, and from the TUI. Rearranging the sidebar in the meantime leaves them where they were. The CLI and the MCP tools still see everything, and logs or debug captures from a private site are not filtered, so keep those panels closed while you share.
