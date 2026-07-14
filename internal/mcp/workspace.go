package mcp

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
)

// A workspace is a display-only grouping of sites (internal/config/workspaces.go).
// It never touches nginx, domains, certificates or .env, which is what separates
// it from the site groups the `site` tool's group_* actions manage. "none" is the
// ungroup sentinel, matching the CLI.
func workspaceTool() mcpTool {
	return mcpTool{
		Name:        "workspace",
		Description: `Group sites into named workspaces, shown in the dashboard sidebar and the TUI. action: list, create, rename, delete, assign (move sites in, or out with workspace "none"), move (reorder). Display-only: a workspace never touches nginx, domains, certificates or .env. To nest a site under another site's subdomain use the site tool's group_* actions instead.`,
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"action":    {Type: "string", Enum: []string{"list", "create", "rename", "delete", "assign", "move"}},
				"name":      {Type: "string", Description: "create/rename/delete/move: workspace name."},
				"new_name":  {Type: "string", Description: "rename: the new name."},
				"sites":     {Type: "array", Description: `assign: site names or domains, e.g. ["shop"].`},
				"workspace": {Type: "string", Description: `assign: target workspace, created if new. "none" ungroups.`},
				"position":  {Type: "integer", Description: "move: zero-based slot in the display order."},
			},
			Required: []string{"action"},
		},
	}
}

func execWorkspaceList(map[string]any) (any, *rpcError) {
	workspaces, err := config.ListWorkspaces()
	if err != nil {
		return toolErr("listing workspaces: " + err.Error()), nil
	}
	reg, err := config.LoadSites()
	if err != nil {
		return toolErr("loading sites: " + err.Error()), nil
	}

	grouped := map[string]bool{}
	type entry struct {
		Name  string   `json:"name"`
		Sites []string `json:"sites"`
	}
	out := make([]entry, 0, len(workspaces))
	for _, w := range workspaces {
		for _, s := range w.Sites {
			grouped[s] = true
		}
		out = append(out, entry{Name: w.Name, Sites: w.Sites})
	}

	// A site in no workspace is ungrouped, not an error; report it so an
	// assistant can see what is left to place. Group secondaries display under
	// their main and never hold a membership of their own.
	ungrouped := []string{}
	for _, s := range reg.Sites {
		if !grouped[s.Name] && s.GroupSubdomain == "" {
			ungrouped = append(ungrouped, s.Name)
		}
	}

	return toolJSON(map[string]any{"workspaces": out, "ungrouped": ungrouped}), nil
}

func execWorkspaceCreate(args map[string]any) (any, *rpcError) {
	name := strArg(args, "name")
	if name == "" {
		return toolErr("create: name is required"), nil
	}
	if err := config.AddWorkspace(name); err != nil {
		return toolErr("creating workspace: " + err.Error()), nil
	}
	return toolOK(fmt.Sprintf("Created workspace %q.", name)), nil
}

func execWorkspaceRename(args map[string]any) (any, *rpcError) {
	old, next := strArg(args, "name"), strArg(args, "new_name")
	if old == "" || next == "" {
		return toolErr("rename: name and new_name are required"), nil
	}
	if err := config.RenameWorkspace(old, next); err != nil {
		return toolErr("renaming workspace: " + err.Error()), nil
	}
	return toolOK(fmt.Sprintf("Renamed workspace %q to %q.", old, next)), nil
}

func execWorkspaceDelete(args map[string]any) (any, *rpcError) {
	name := strArg(args, "name")
	if name == "" {
		return toolErr("delete: name is required"), nil
	}
	if err := config.DeleteWorkspace(name); err != nil {
		return toolErr("deleting workspace: " + err.Error()), nil
	}
	return toolOK(fmt.Sprintf("Deleted workspace %q. Its sites are now ungrouped; no site was touched.", name)), nil
}

func execWorkspaceAssign(args map[string]any) (any, *rpcError) {
	sites := strSliceArg(args, "sites")
	if len(sites) == 0 {
		return toolErr("assign: sites is required"), nil
	}
	for i, s := range sites {
		sites[i] = config.ResolveSiteRef(s)
	}

	target := strArg(args, "workspace")
	if target == "none" {
		target = ""
	}
	if err := config.AssignSiteWorkspace(sites, target, true); err != nil {
		return toolErr("assigning sites: " + err.Error()), nil
	}
	if target == "" {
		return toolOK(fmt.Sprintf("Ungrouped %v.", sites)), nil
	}
	return toolOK(fmt.Sprintf("Moved %v into workspace %q.", sites, target)), nil
}

func execWorkspaceMove(args map[string]any) (any, *rpcError) {
	name := strArg(args, "name")
	if name == "" {
		return toolErr("move: name is required"), nil
	}
	if err := config.MoveWorkspace(name, intArg(args, "position", 0)); err != nil {
		return toolErr("moving workspace: " + err.Error()), nil
	}
	return toolOK(fmt.Sprintf("Moved workspace %q.", name)), nil
}
