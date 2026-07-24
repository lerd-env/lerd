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
	os.Exit(m.Run())
}
