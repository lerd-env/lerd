package watcher

import "testing"

func TestWireOSDNSDeps_linuxRepairsRootlessNetworking(t *testing.T) {
	var deps dnsWatchDeps
	wireOSDNSDeps(&deps, "test")
	if deps.resyncContainerDNS == nil || deps.repairNginx == nil || deps.repairDNS == nil {
		t.Error("Linux should wire the container DNS, nginx and DNS repairs")
	}
	if deps.healMachine != nil {
		t.Error("Linux has no podman machine to heal")
	}
}
