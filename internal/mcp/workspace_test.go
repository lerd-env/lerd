package mcp

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// isolateWorkspaceConfig points config.LoadGlobal/SaveGlobal at a temp dir so a
// workspace mutation never touches the developer's own config.yaml.
func isolateWorkspaceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func callWorkspace(t *testing.T, args map[string]any) map[string]any {
	t.Helper()
	h, ok := groupDispatch["workspace"][args["action"].(string)]
	if !ok {
		t.Fatalf("no handler for workspace action %q", args["action"])
	}
	res, rpcErr := h(args)
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	out, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if out["isError"] == true {
		t.Fatalf("tool error: %v", out["content"])
	}
	return out
}

func loadGlobalOrFail(t *testing.T) *config.GlobalConfig {
	t.Helper()
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	return cfg
}

func workspaceText(t *testing.T, res map[string]any) string {
	t.Helper()
	content, ok := res["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("no content in result: %v", res)
	}
	text, _ := content[0]["text"].(string)
	return text
}

// TestWorkspaceTool_isRegistered guards the whole point of the tool: a workspace
// is unreachable to an assistant unless it is both advertised and routable.
func TestWorkspaceTool_isRegistered(t *testing.T) {
	found := false
	for _, name := range ToolNames() {
		if name == "workspace" {
			found = true
		}
	}
	if !found {
		t.Error("workspace is missing from the tool list")
	}
	for _, action := range []string{"list", "create", "rename", "delete", "assign", "move"} {
		if _, ok := groupDispatch["workspace"][action]; !ok {
			t.Errorf("workspace dispatch missing %q", action)
		}
	}
}

func TestWorkspaceCreateRenameDelete(t *testing.T) {
	isolateWorkspaceConfig(t)

	callWorkspace(t, map[string]any{"action": "create", "name": "clients"})
	if got := loadGlobalOrFail(t).WorkspaceNames(); len(got) != 1 || got[0] != "clients" {
		t.Fatalf("after create, workspaces = %v", got)
	}

	callWorkspace(t, map[string]any{"action": "rename", "name": "clients", "new_name": "agency"})
	if got := loadGlobalOrFail(t).WorkspaceNames(); len(got) != 1 || got[0] != "agency" {
		t.Fatalf("after rename, workspaces = %v", got)
	}

	callWorkspace(t, map[string]any{"action": "delete", "name": "agency"})
	if got := loadGlobalOrFail(t).WorkspaceNames(); len(got) != 0 {
		t.Fatalf("after delete, workspaces = %v", got)
	}
}

// TestWorkspaceAssign_movesAndUngroups covers both directions: a site joins a
// workspace that is created on demand, and "none" takes it back out.
func TestWorkspaceAssign_movesAndUngroups(t *testing.T) {
	isolateWorkspaceConfig(t)

	callWorkspace(t, map[string]any{
		"action":    "assign",
		"sites":     []any{"shop"},
		"workspace": "clients",
	})
	if got := loadGlobalOrFail(t).WorkspaceOfSite("shop"); got != "clients" {
		t.Fatalf("shop should be in clients, got %q", got)
	}

	callWorkspace(t, map[string]any{
		"action":    "assign",
		"sites":     []any{"shop"},
		"workspace": "none",
	})
	if got := loadGlobalOrFail(t).WorkspaceOfSite("shop"); got != "" {
		t.Fatalf(`"none" should ungroup shop, got %q`, got)
	}
}

func TestWorkspaceMove_reorders(t *testing.T) {
	isolateWorkspaceConfig(t)

	for _, name := range []string{"a", "b", "c"} {
		callWorkspace(t, map[string]any{"action": "create", "name": name})
	}
	callWorkspace(t, map[string]any{"action": "move", "name": "c", "position": 0})

	got := loadGlobalOrFail(t).WorkspaceNames()
	want := []string{"c", "a", "b"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("after move, workspaces = %v, want %v", got, want)
		}
	}
}

func TestWorkspaceList_reportsMembership(t *testing.T) {
	isolateWorkspaceConfig(t)

	callWorkspace(t, map[string]any{"action": "create", "name": "clients"})
	text := workspaceText(t, callWorkspace(t, map[string]any{"action": "list"}))
	if !strings.Contains(text, "clients") {
		t.Errorf("list should report the workspace, got:\n%s", text)
	}
}

// TestWorkspaceCreate_rejectsReserved keeps the CLI's ungroup sentinel out of
// the config, where it would shadow the "none" argument assign relies on.
func TestWorkspaceCreate_rejectsReserved(t *testing.T) {
	isolateWorkspaceConfig(t)

	res, _ := groupDispatch["workspace"]["create"](map[string]any{"action": "create", "name": "none"})
	out, _ := res.(map[string]any)
	if out["isError"] != true {
		t.Errorf(`creating a workspace named "none" should be an error, got %v`, out)
	}
}
