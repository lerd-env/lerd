package watcher

// wireOSDNSDeps wires no resume repairs on Windows yet: containers take DNS from
// the podman machine VM, and the machine heal is macOS's.
func wireOSDNSDeps(*dnsWatchDeps, string) {}
