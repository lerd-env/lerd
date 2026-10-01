package lifecycle

import (
	"fmt"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
)

// TeardownOnLogout is true on macOS, the one platform with something to lose.
// The Podman Machine VM outlives the session, and a database killed with it
// mid-write replays its write-ahead log for minutes on the next start.
const TeardownOnLogout = true

// StopPodmanMachine stops the running Podman Machine VM, so it is shut down
// cleanly rather than killed with containers still writing. A database killed
// mid-write replays its write-ahead log for minutes on the next start.
func StopPodmanMachine() {
	out, err := podman.Cmd("machine", "list", "--format", "{{.Name}}\t{{.Running}}").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != "true" {
			continue
		}
		name := strings.TrimSuffix(fields[0], "*")
		feedback.Line(fmt.Sprintf("Stopping Podman Machine (%s)…", name))
		cmd := podman.Cmd("machine", "stop", name)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			feedback.Warn("podman machine stop: %v", err)
		}
	}
}

// BatchStopContainers stops all running lerd-* containers in two podman calls
// (stop then rm) so the Podman Machine socket isn't flooded by N individual
// stop requests. After this returns the individual StopUnit calls find no
// containers and go straight to launchctl bootout.
func BatchStopContainers(_ []string) {
	// Query only running containers with name prefix "lerd-" to avoid passing
	// non-existent names (native services like lerd-dns have no container).
	out, err := podman.Run("ps", "--format", "{{.Names}}", "--filter", "name=^lerd-")
	if err != nil || strings.TrimSpace(out) == "" {
		return
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if n := strings.TrimSpace(line); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return
	}
	// No -t: podman then honours each container's own --stop-timeout, so a
	// database keeps the longer grace its definition asked for instead of every
	// container being held to one flat window.
	podman.RunSilent(append([]string{"stop"}, names...)...)     //nolint:errcheck
	podman.RunSilent(append([]string{"rm", "-f"}, names...)...) //nolint:errcheck
}
