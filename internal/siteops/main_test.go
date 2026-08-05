package siteops

import (
	"os"
	"testing"
)

// The secure path waits for nginx to serve the new scheme, which is a real
// request against a real domain. Tests drive that path with fixture sites, so
// the wait is stubbed out here rather than spending its timeout per test.
func TestMain(m *testing.M) {
	waitSchemeServedFn = func(string, bool) {}
	os.Exit(m.Run())
}
