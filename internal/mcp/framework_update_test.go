package mcp

import "testing"

// The framework MCP tool mirrors the CLI: install was dropped in favour of
// update as the single manual store-refresh action.
func TestFrameworkGroup_InstallReplacedByUpdate(t *testing.T) {
	fw := groupDispatch["framework"]
	if _, ok := fw["install"]; ok {
		t.Error("framework install action should be removed")
	}
	if _, ok := fw["update"]; !ok {
		t.Error("framework update action should be registered")
	}
}
