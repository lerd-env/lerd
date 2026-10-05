package watcher

import "github.com/geodro/lerd/internal/config"

// wireOSDNSDeps turns on the podman machine heal. Containers get DNS from the
// VM, so there is no container DNS to re-sync, but a host suspend can stall the
// VM itself (issue #715); the resume tick restarts it so the MCP/exec path is
// healed before the next agent call. isStopped keeps a deliberate `lerd stop`
// from resurrecting it.
func wireOSDNSDeps(deps *dnsWatchDeps, _ string) {
	deps.healMachine = defaultHealMachine
	deps.isStopped = config.IsStopped
}
