package serviceops

import (
	"os"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestMain(m *testing.M) {
	// init wires ServiceRunning for production; unit tests want the installed
	// member list unless a case sets the seam itself.
	config.ServiceRunning = nil
	// The port guard asks podman whether a service's container runs; a test
	// reads "absent" unless it says otherwise, never the machine's own podman.
	ensureContainerRunning = func(string) (bool, bool) { return false, true }
	os.Exit(m.Run())
}
