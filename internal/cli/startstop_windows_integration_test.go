//go:build windows

package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/podman"
)

// TestRealMachineStartup drives ensurePodmanMachineRunning against the real
// Podman on this host. It creates and starts a Hyper-V machine, downloads an
// image and needs an elevated shell, so it only runs when asked for. It must not
// redirect XDG_DATA_HOME: Podman honours it, and the machine disk would land in
// a temp dir the test then deletes.
func TestRealMachineStartup(t *testing.T) {
	if os.Getenv("LERD_TEST_REAL_MACHINE") != "1" {
		t.Skip("set LERD_TEST_REAL_MACHINE=1 to create and start a real Podman machine")
	}

	if err := ensurePodmanMachineRunning(); err != nil {
		t.Fatalf("ensurePodmanMachineRunning: %v", err)
	}
	if !machineAlreadyUsable() {
		t.Fatal("podman ps fails after startup")
	}
	name, running, rootful := selectedMachineState()
	if name == "" || !running || !rootful {
		t.Errorf("machine state = name %q running %v rootful %v, want a running rootful machine", name, running, rootful)
	}
	out, err := podman.Cmd("machine", "inspect", "--format", "{{.Rootful}} {{.Resources.Memory}}", name).Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("machine %s: rootful/memory = %s", name, strings.TrimSpace(string(out)))
}
