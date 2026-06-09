package mcp

import (
	"sort"
	"testing"
)

// TestGroupDispatch_MatchesSchemaEnums guards the grouped-tool contract: every
// action advertised in a tool's schema enum must have a handler in groupDispatch,
// and every handler must be advertised. This catches enum/handler drift when
// actions are added, renamed, or removed (e.g. the worker mode_get/mode_set
// split). worktree is excluded: it routes through dispatchWorktree, not the table.
func TestGroupDispatch_MatchesSchemaEnums(t *testing.T) {
	for _, tool := range toolList() {
		if tool.Name == "worktree" {
			continue
		}
		handlers, ok := groupDispatch[tool.Name]
		if !ok {
			t.Errorf("tool %q has no groupDispatch entry", tool.Name)
			continue
		}
		enum := tool.InputSchema.Properties["action"].Enum
		if len(enum) == 0 {
			t.Errorf("tool %q schema has no action enum", tool.Name)
			continue
		}
		enumSet := map[string]bool{}
		for _, a := range enum {
			enumSet[a] = true
			if _, has := handlers[a]; !has {
				t.Errorf("tool %q: schema action %q has no handler", tool.Name, a)
			}
		}
		for a := range handlers {
			if !enumSet[a] {
				t.Errorf("tool %q: handler %q is not in the schema enum (undiscoverable)", tool.Name, a)
			}
		}
	}
}

// TestWorkerModeActions_split asserts the worker mode operations are exposed as
// distinct mode_get / mode_set actions. execWorkersMode re-reads args["action"]
// for its own get/set switch, so a single "mode" action is unreachable through
// the grouped router.
func TestWorkerModeActions_split(t *testing.T) {
	h := groupDispatch["worker"]
	for _, want := range []string{"mode_get", "mode_set"} {
		if _, ok := h[want]; !ok {
			t.Errorf("worker dispatch missing %q", want)
		}
	}
	if _, bad := h["mode"]; bad {
		t.Error(`worker still exposes bare "mode" action, which routes to execWorkersMode's default branch and always errors`)
	}
}

// TestToolList_tenGroups is a guardrail on the consolidated surface.
func TestToolList_tenGroups(t *testing.T) {
	want := []string{"db", "diag", "env", "exec", "framework", "runtime", "service", "site", "worker", "worktree"}
	got := make([]string, 0, len(toolList()))
	for _, tool := range toolList() {
		got = append(got, tool.Name)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("expected %d grouped tools, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool set mismatch: got %v, want %v", got, want)
			break
		}
	}
}
