package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestWakingPageHTML_autoRefreshes guards the wake mechanism: the page a sleeping
// host-proxy site serves must auto-refresh, so once the access hit drives resume
// and the proxy vhost is restored, the browser reloads onto the live app without
// the user touching anything.
func TestWakingPageHTML_autoRefreshes(t *testing.T) {
	if !strings.Contains(wakingPageHTML, `http-equiv="refresh"`) {
		t.Error("waking page must carry a meta refresh so a resumed site reloads on its own")
	}
}

// TestHostProxyResumeWaitPort pins finding #8: idle-resume must wait for the dev
// server to rebind before restoring the proxy vhost, on a non-zero port, so it
// never skips the wait and flashes a bare 502. The committed proxy block's port is
// preferred, but when it carries none (an auto-allocated dev server) the wait falls
// back to the site's registered HostPort instead of 0.
func TestHostProxyResumeWaitPort(t *testing.T) {
	dir := t.TempDir()
	site := &config.Site{Name: "node-app", Domains: []string{"node-app.test"}, Path: dir, HostPort: 3000, HostCommand: "npm run dev"}

	if got := hostProxyResumeWaitPort(site); got != 3000 {
		t.Fatalf("with no committed proxy block, wait port = %d, want 3000 (HostPort)", got)
	}

	// Committed proxy block omits the port (auto-allocated dev server).
	if err := config.SaveProjectConfig(dir, &config.ProjectConfig{
		Proxy: &config.ProxyConfig{Command: "npm run dev"}, // Port left 0
	}); err != nil {
		t.Fatalf("SaveProjectConfig: %v", err)
	}
	if got := hostProxyResumeWaitPort(site); got != 3000 {
		t.Fatalf("with a port-less committed proxy block, wait port = %d, want 3000 (HostPort fallback, never 0)", got)
	}
}

// TestHostProxyVhostSwapApplies pins the gate the idle suspend/resume vhost swap
// rides on: only a host-proxy site whose worker set includes the app dev server
// swaps to (or restores from) the waking page; every other site or worker set is
// left alone.
func TestHostProxyVhostSwapApplies(t *testing.T) {
	app := hostProxyWorkerName
	cases := []struct {
		name        string
		isHostProxy bool
		workers     []string
		want        bool
	}{
		{"host-proxy with app", true, []string{app}, true},
		{"host-proxy with app among others", true, []string{"queue", app}, true},
		{"host-proxy without app", true, []string{"queue", "schedule"}, false},
		{"host-proxy empty set", true, nil, false},
		{"non-host-proxy with app", false, []string{app}, false},
		{"non-host-proxy without app", false, []string{"queue"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostProxyVhostSwapApplies(tc.isHostProxy, tc.workers); got != tc.want {
				t.Fatalf("hostProxyVhostSwapApplies(%v, %v) = %v, want %v",
					tc.isHostProxy, tc.workers, got, tc.want)
			}
		})
	}
}
