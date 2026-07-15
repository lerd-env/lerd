package systemd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// podmanNetworkWaitUnit is the unit podman's quadlet generator makes every
// rootless container Want= and After=. It polls the system's
// network-online.target and gives up after 90 seconds.
const podmanNetworkWaitUnit = "podman-user-wait-network-online.service"

const networkWaitDropInFile = "10-lerd-no-network-wait.conf"

// networkWaitDropIn turns podman's wait into a no-op. network-online.target is
// only reached when some unit pulls it in; on atomic images (Fedora Silverblue
// and friends) nothing does, so the wait can only ever time out, and it drags
// every lerd container start, and the boot itself, out by the full 90 seconds.
// systemd cannot drop an inherited After=, so the wait unit is overridden here
// rather than removed from the quadlets.
const networkWaitDropIn = `# Written by lerd.
# network-online.target never activates on this host, so podman's wait unit can
# only time out, stalling every container start and boot by 90s. Lerd publishes
# on loopback and needs no routable network, so the wait is skipped.
[Service]
ExecStart=
ExecStart=/bin/true
`

// NetworkWaitDropInPath returns the path of lerd's override for podman's
// network-online wait unit.
func NetworkWaitDropInPath() string {
	return filepath.Join(config.SystemdUserDir(), podmanNetworkWaitUnit+".d", networkWaitDropInFile)
}

// networkWaitStalls decides, from the wait unit's load state, whether lerd has
// already overridden it, and the current state of network-online.target,
// whether container starts on this host are paying the 90s timeout.
func networkWaitStalls(loadState string, dropInPresent bool, targetState string) bool {
	if loadState != "loaded" || dropInPresent {
		return false
	}
	return targetState == "inactive" || targetState == "failed"
}

// NetworkWaitStalls reports whether podman's network-online wait unit stalls
// container starts on this host.
func NetworkWaitStalls() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := os.Stat(NetworkWaitDropInPath())
	return networkWaitStalls(unitLoadState(podmanNetworkWaitUnit), err == nil, networkOnlineTargetState())
}

// EnsureNoNetworkWaitStall installs the override when podman's wait unit would
// otherwise stall every container start. Reports whether it wrote the drop-in.
func EnsureNoNetworkWaitStall() (bool, error) {
	if !NetworkWaitStalls() {
		return false, nil
	}

	path := NetworkWaitDropInPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	config.GuardRealWrite(path)
	if err := os.WriteFile(path, []byte(networkWaitDropIn), 0644); err != nil {
		return false, err
	}

	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	_ = exec.Command("systemctl", "--user", "reset-failed", podmanNetworkWaitUnit).Run()
	return true, nil
}

// unitLoadState returns systemd's LoadState for a user unit ("loaded",
// "not-found", "masked"), or "" when systemctl can't be reached.
func unitLoadState(unit string) string {
	out, err := exec.Command("systemctl", "--user", "show", unit, "-p", "LoadState", "--value").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// networkOnlineTargetState returns the ActiveState of the system's
// network-online.target. systemctl is-active exits non-zero for every state
// but "active", so the output is what matters, not the exit code.
func networkOnlineTargetState() string {
	out, _ := exec.Command("systemctl", "is-active", "network-online.target").Output()
	return strings.TrimSpace(string(out))
}
