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
