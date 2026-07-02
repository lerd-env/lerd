package podman

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MachineHeal recovers a stalled Podman Machine (e.g. macOS post-sleep, when the
// VM and its gvproxy networking are suspended and every podman call blocks
// forever). Wired by the darwin CLI to restartPodmanMachineForHeal; nil on
// Linux and in tests, where there is no machine VM to restart.
var MachineHeal func()

// machineProbeTimeout bounds the "is the VM responsive" probe. A healthy machine
// answers `podman ps -q` in well under a second, so this only fires on a stall.
const machineProbeTimeout = 10 * time.Second

// healCooldown suppresses repeated stop/start cycles when the machine is
// genuinely dead, so a burst of MCP calls does not stop-start it in a loop.
const healCooldown = 2 * time.Minute

var (
	healMu     sync.Mutex
	lastHealAt time.Time
)

// EnsureMachineResponsive verifies the Podman Machine answers a cheap query
// within a timeout before an MCP handler shells out for real. On a stall it
// heals the VM once (subject to a cooldown) and re-probes, turning a post-sleep
// freeze into a self-healed retry or, if the machine is truly dead, a fast
// surfaced error instead of an unbounded hang. Fast no-op on a healthy machine
// and on Linux (no machine VM, MachineHeal nil).
func EnsureMachineResponsive() error {
	if machineResponds() {
		return nil
	}
	if MachineHeal == nil || !healOnceWithinCooldown() {
		return fmt.Errorf("podman machine is not responding (try: lerd start)")
	}
	if machineResponds() {
		return nil
	}
	return fmt.Errorf("podman machine did not recover after a restart (try: lerd machine reset)")
}

// healOnceWithinCooldown runs MachineHeal unless one ran within healCooldown,
// returning whether a heal actually ran. Guards against stop-start loops when
// the machine stays dead across successive calls.
func healOnceWithinCooldown() bool {
	if MachineHeal == nil {
		return false
	}
	healMu.Lock()
	if !lastHealAt.IsZero() && time.Since(lastHealAt) < healCooldown {
		healMu.Unlock()
		return false
	}
	lastHealAt = time.Now()
	healMu.Unlock()
	MachineHeal()
	return true
}

// machineResponds reports whether `podman ps -q` returns within the probe
// timeout. Any failure (deadline, dead socket, missing machine) counts as
// unresponsive, which is exactly when a heal is warranted.
func machineResponds() bool {
	ctx, cancel := context.WithTimeout(context.Background(), machineProbeTimeout)
	defer cancel()
	return CmdContext(ctx, "ps", "-q").Run() == nil
}
