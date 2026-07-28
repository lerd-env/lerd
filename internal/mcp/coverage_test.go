package mcp

import (
	"strings"
	"testing"
)

// contentText returns a tool result's single text block, for the handlers that
// answer with a message rather than JSON.
func contentText(t *testing.T, result any) string {
	t.Helper()
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result is not a map")
	}
	content, ok := m["content"].([]map[string]any)
	if !ok || len(content) != 1 {
		t.Fatal("result has no content block")
	}
	text, ok := content[0]["text"].(string)
	if !ok {
		t.Fatal("content block has no text")
	}
	return text
}

// actionsOf returns a tool's advertised actions.
func actionsOf(t *testing.T, tool string) []string {
	t.Helper()
	acts, ok := ToolActions()[tool]
	if !ok {
		t.Fatalf("tool %q is not registered", tool)
	}
	return acts
}

func hasAction(t *testing.T, tool, action string) bool {
	t.Helper()
	for _, a := range actionsOf(t, tool) {
		if a == action {
			return true
		}
	}
	return false
}

// A dump that needs postgis or pgvector reaches for a type and lerd creates the
// extension for it, but an agent could not see what an engine offers or add one
// up front, so `lerd db:extension` had no counterpart here.
func TestDBToolExposesExtensions(t *testing.T) {
	for _, action := range []string{"extension_list", "extension_add"} {
		if !hasAction(t, "db", action) {
			t.Errorf("db tool is missing the %q action", action)
		}
	}
}

// A service preset declares what it holds. Databases have their own tool; every
// other kind (RustFS buckets and anything a preset declares later) was reachable
// only from the dashboard.
func TestServiceToolExposesEntities(t *testing.T) {
	for _, action := range []string{"entities", "entity_action"} {
		if !hasAction(t, "service", action) {
			t.Errorf("service tool is missing the %q action", action)
		}
	}
}

// The reference described the fnm/nvm split but gave no way to act on it.
func TestRuntimeToolExposesNodeManager(t *testing.T) {
	if !hasAction(t, "runtime", "node_manager") {
		t.Error("runtime tool is missing the \"node_manager\" action")
	}
}

// Every advertised action must route somewhere, or a client reading the manifest
// gets an "unknown action" for something the schema promised.
func TestEveryAdvertisedActionDispatches(t *testing.T) {
	for tool, actions := range ToolActions() {
		handlers, ok := groupDispatch[tool]
		if !ok {
			continue // worktree and workspace route through their own dispatchers
		}
		for _, a := range actions {
			if _, ok := handlers[a]; !ok {
				t.Errorf("tool %q advertises action %q with no handler", tool, a)
			}
		}
	}
}

// The description is what a model reads before it picks an action, so a new
// action that never reaches it is invisible in practice.
func TestNewActionsAppearInToolDescriptions(t *testing.T) {
	descs := map[string]string{}
	for _, tl := range toolList() {
		descs[tl.Name] = tl.Description
	}
	for tool, action := range map[string]string{
		"db": "extension_list", "service": "entities", "runtime": "node_manager",
	} {
		if !strings.Contains(descs[tool], action) {
			t.Errorf("%q is missing from the %q tool description", action, tool)
		}
	}
}

// A tool that only appears in the manifest is not a feature. These drive the
// handlers themselves, so a registered action that cannot run is caught here
// rather than by the first agent to try it.
func TestNodeManagerReportsWithoutSwitching(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	res, rerr := execNodeManager(map[string]any{})
	if rerr != nil {
		t.Fatalf("execNodeManager: %v", rerr.Message)
	}
	var got struct {
		Manager string `json:"manager"`
	}
	decodeContent(t, res, &got)
	if got.Manager != "fnm" && got.Manager != "nvm" {
		t.Errorf("manager = %q, want fnm or nvm", got.Manager)
	}
}

func TestNodeManagerRejectsAnUnknownManager(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	res, rerr := execNodeManager(map[string]any{"manager": "yarn"})
	if rerr != nil {
		t.Fatalf("execNodeManager: %v", rerr.Message)
	}
	if !strings.Contains(contentText(t, res), "unknown manager") {
		t.Errorf("expected an unknown-manager error, got %q", contentText(t, res))
	}
}

// A service with no declared entities must say so rather than return an empty
// list an agent would read as "this service holds nothing".
func TestServiceEntitiesReportsWhenNoneAreDeclared(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	res, rerr := execServiceEntities(map[string]any{"name": "mysql"})
	if rerr != nil {
		t.Fatalf("execServiceEntities: %v", rerr.Message)
	}
	if !strings.Contains(contentText(t, res), "declares no entities") {
		t.Errorf("expected a no-entities message, got %q", contentText(t, res))
	}
}

func TestServiceEntityActionRefusesStreamingActions(t *testing.T) {
	for _, action := range []string{"export", "import"} {
		res, rerr := execServiceEntityAction(map[string]any{
			"name": "rustfs", "kind": "buckets", "entity": "media", "entity_action": action,
		})
		if rerr != nil {
			t.Fatalf("execServiceEntityAction: %v", rerr.Message)
		}
		if !strings.Contains(contentText(t, res), "stream a file") {
			t.Errorf("%s: expected the streaming refusal, got %q", action, contentText(t, res))
		}
	}
}

func TestDBExtensionAddRequiresAName(t *testing.T) {
	res, rerr := execDBExtensionAdd(map[string]any{"path": t.TempDir()})
	if rerr != nil {
		t.Fatalf("execDBExtensionAdd: %v", rerr.Message)
	}
	if !strings.Contains(contentText(t, res), "extension is required") {
		t.Errorf("expected the missing-argument error, got %q", contentText(t, res))
	}
}
