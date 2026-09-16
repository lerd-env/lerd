package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestSitesToRefreshForPortMove covers which sites follow a service's published
// port when it moves. A native-runtime site reaches services over loopback just
// like a host-proxy one, so leaving it out strands its .env on the old port.
func TestSitesToRefreshForPortMove(t *testing.T) {
	sites := []config.Site{
		{Name: "api", Path: "/p/api", HostPort: 3100, Framework: "laravel"},
		{Name: "shop", Path: "/p/shop", Framework: "laravel"},
		{Name: "fp", Path: "/p/fp", Runtime: "frankenphp", Framework: "laravel"},
		{Name: "bare", Path: "/p/bare", HostPort: 3200},
	}

	native := refreshPaths(sitesToRefreshForPortMove(sites, config.PHPRuntimeNative))
	want := []string{"/p/api", "/p/shop"}
	if !sameStrings(native, want) {
		t.Errorf("native runtime = %v, want %v", native, want)
	}

	container := refreshPaths(sitesToRefreshForPortMove(sites, config.PHPRuntimeContainer))
	if !sameStrings(container, []string{"/p/api"}) {
		t.Errorf("container runtime = %v, want [/p/api]", container)
	}
}

func refreshPaths(sites []config.Site) []string {
	out := make([]string, 0, len(sites))
	for _, s := range sites {
		out = append(out, s.Path)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
