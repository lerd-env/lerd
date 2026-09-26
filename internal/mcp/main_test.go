package mcp

import (
	"os"
	"testing"
)

// TestMain keeps the exec tools off the developer's own services: waking the
// services of a project a test points at would start real containers.
func TestMain(m *testing.M) {
	wakeSiteServicesMCP = func(string) error { return nil }
	os.Exit(m.Run())
}
