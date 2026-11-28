package nginx

import (
	"os"
	"testing"
)

// TestMain keeps the vhost tests off the developer's own idle-suspend state: a
// site sharing a name with one asleep on this machine would otherwise render
// its waking vhost.
func TestMain(m *testing.M) {
	siteWaitsOnSleepingService = func(string) bool { return false }
	// The developer's own Homebrew nginx and port forward would move every native
	// test to the host; hostModeLegs turns them back on where a test wants it.
	hostNginxBinary = func() (string, bool) { return "", false }
	os.Exit(m.Run())
}
