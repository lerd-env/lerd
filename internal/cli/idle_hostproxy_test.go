package cli

import (
	"strings"
	"testing"
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
