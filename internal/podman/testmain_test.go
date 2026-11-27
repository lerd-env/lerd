package podman

import (
	"os"
	"testing"
)

// Quadlet writes consult the host for ::1 to decide whether IPv6 publish lines
// are safe. Pin it so the suite asserts the dual-stack shape everywhere,
// regardless of the machine or CI container the tests run on.
func TestMain(m *testing.M) {
	hostIPv6LoopbackFn = func() bool { return true }
	// Keep the live podman ps out of tests that fake inspect through execCommand.
	runningPsFn = func() (string, error) { return "", nil }
	// Builds must not pick up the added or distrusted roots of whoever runs the suite.
	hostCAConfPath = os.DevNull
	hostAddedCASources = nil
	os.Exit(m.Run())
}
