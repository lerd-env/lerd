package watcher

import "testing"

func TestWireOSDNSDeps_macOSHealsTheMachine(t *testing.T) {
	var deps dnsWatchDeps
	wireOSDNSDeps(&deps, "test")
	if deps.healMachine == nil {
		t.Error("macOS should wire the podman machine heal")
	}
	if deps.resyncContainerDNS != nil || deps.repairNginx != nil {
		t.Error("macOS containers get DNS from the VM, so there is nothing to re-sync")
	}
}
