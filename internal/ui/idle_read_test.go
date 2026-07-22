package ui

import (
	"testing"
	"time"

	"github.com/geodro/lerd/internal/siteinfo"
)

func TestIdleSiteIsIdle(t *testing.T) {
	now := time.Now()
	activity := map[string]int64{"myapp": now.Add(-time.Hour).Unix()}
	timeout := 30 * time.Minute

	cases := []struct {
		name                    string
		key                     string
		paused, exempt, enabled bool
		want                    bool
	}{
		{"idle past the timeout", "myapp", false, false, true, true},
		{"feature off", "myapp", false, false, false, false},
		{"paused", "myapp", true, false, true, false},
		{"exempt from suspension", "myapp", false, true, true, false},
		{"no activity recorded", "other", false, false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idleSiteIsIdle(activity, tc.key, tc.paused, tc.exempt, tc.enabled, timeout, now)
			if got != tc.want {
				t.Errorf("idleSiteIsIdle = %v, want %v", got, tc.want)
			}
		})
	}
}

// A proxy-only site is exempt from suspension, so the dashboard must not show it
// as sleeping; one with a supervised dev command still idles normally.
func TestEnrichedSiteIsProxyOnly(t *testing.T) {
	proxyOnly := siteinfo.EnrichedSite{HostPort: 3000}
	if !proxyOnly.IsProxyOnly() {
		t.Error("host-proxy site with no command should be proxy-only")
	}
	supervised := siteinfo.EnrichedSite{HostPort: 3000, HostCommand: "npm run dev"}
	if supervised.IsProxyOnly() {
		t.Error("host-proxy site with a command is not proxy-only")
	}
}
