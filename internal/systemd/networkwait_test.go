package systemd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNetworkWaitStalls(t *testing.T) {
	tests := []struct {
		name       string
		loadState  string
		dropIn     bool
		target     string
		wantStalls bool
	}{
		{
			name:       "atomic image: target never activates",
			loadState:  "loaded",
			target:     "inactive",
			wantStalls: true,
		},
		{
			name:      "ordinary host: target is active",
			loadState: "loaded",
			target:    "active",
		},
		{
			name:      "already neutralised by lerd",
			loadState: "loaded",
			dropIn:    true,
			target:    "inactive",
		},
		{
			name:      "podman too old to ship the wait unit",
			loadState: "not-found",
			target:    "inactive",
		},
		{
			name:      "user already masked the unit",
			loadState: "masked",
			target:    "inactive",
		},
		{
			// Mid-boot the target can still be on its way up. Only act on a
			// settled "inactive", never on a target that may yet come online.
			name:      "target still coming up",
			loadState: "loaded",
			target:    "activating",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := networkWaitStalls(tt.loadState, tt.dropIn, tt.target); got != tt.wantStalls {
				t.Errorf("networkWaitStalls(%q, %v, %q) = %v, want %v",
					tt.loadState, tt.dropIn, tt.target, got, tt.wantStalls)
			}
		})
	}
}

func TestNetworkWaitDropInOverridesExecStart(t *testing.T) {
	// Resetting ExecStart before reassigning it is what makes the override
	// replace podman's poll loop instead of appending a second command.
	execStarts := 0
	for _, line := range strings.Split(networkWaitDropIn, "\n") {
		if strings.HasPrefix(line, "ExecStart=") {
			execStarts++
		}
	}
	if execStarts != 2 {
		t.Fatalf("drop-in has %d ExecStart= lines, want 2 (a reset then the no-op)", execStarts)
	}
	if !strings.Contains(networkWaitDropIn, "ExecStart=\nExecStart=") {
		t.Error("drop-in must reset ExecStart= to empty before reassigning it")
	}
	if !strings.Contains(networkWaitDropIn, "[Service]") {
		t.Error("drop-in must carry a [Service] section")
	}
}

func TestNetworkWaitDropInPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path := NetworkWaitDropInPath()
	if got := filepath.Base(filepath.Dir(path)); got != "podman-user-wait-network-online.service.d" {
		t.Errorf("drop-in parent dir = %q, want the wait unit's .d directory", got)
	}
	if got := filepath.Base(path); got != "10-lerd-no-network-wait.conf" {
		t.Errorf("drop-in file = %q", got)
	}
}
