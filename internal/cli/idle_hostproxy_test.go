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
