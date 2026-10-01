//go:build windows

package cli

import "github.com/geodro/lerd/internal/unitlog"

func serviceStartHint(unit string) string {
	return "lerd start"
}

func serviceStatusHint(unit string) string {
	return "lerd start  |  logs: " + unitlog.LogHint(unit)
}

func dnsRestartHint() string {
	return "run 'lerd install' from an elevated shell to reconfigure DNS"
}

func podmanDaemonHint() string {
	return "podman machine start"
}
