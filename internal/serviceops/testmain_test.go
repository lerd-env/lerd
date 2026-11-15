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
	// Ready markers and service locks live in the data dir, never the machine's.
	// A resume helper process keeps its parent's, which it shares the lock with.
	if os.Getenv("LERD_RESUME_DIR") != "" {
		os.Exit(m.Run())
	}
	data, err := os.MkdirTemp("", "serviceops-data-")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_DATA_HOME", data)
	code := m.Run()
	os.RemoveAll(data)
	os.Exit(code)
}
