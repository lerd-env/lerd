package cli

import "testing"

// install was removed: definitions auto-fetch on link and refresh in the
// background, so a manual install has no job. update is the sole manual
// store-refresh command.
func TestFrameworkCmd_InstallRemovedUpdateKept(t *testing.T) {
	names := map[string]bool{}
	for _, c := range NewFrameworkCmd().Commands() {
		names[c.Name()] = true
	}
	if names["install"] {
		t.Error("framework install should be removed")
	}
	if !names["update"] {
		t.Error("framework update should remain as the manual refresh command")
	}
}
