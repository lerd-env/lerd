package phpsets

import (
	"os"
	"testing"
)

// The runtime is read from the machine's own config by default, so a developer
// on the native runtime would take the native path through every test written
// for the image one. Pinned to the image runtime here; the tests that are about
// the host build set it themselves.
func TestMain(m *testing.M) {
	nativeRuntimeFn = func() bool { return false }
	os.Exit(m.Run())
}
