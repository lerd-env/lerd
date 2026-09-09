package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A paused or ignored site keeps whatever vhost it has, a landing page or none
// at all. Rewriting one is not just pointless: an ignored secured site whose
// certificate has been removed still renders an SSL vhost naming that
// certificate, and one unloadable certificate fails the whole nginx config, so
// every reload after it is refused and every other site keeps serving the
// runtime it was on before.
func TestRuntimeSwitchLeavesPausedAndIgnoredSitesAlone(t *testing.T) {
	cases := []struct {
		name string
		site config.Site
		want bool
	}{
		{"a served site", config.Site{Name: "shop"}, true},
		{"paused", config.Site{Name: "shop", Paused: true}, false},
		{"ignored", config.Site{Name: "shop", Ignored: true}, false},
		{"ignored and secured", config.Site{Name: "shop", Ignored: true, Secured: true}, false},
		// Still nothing to move for the site types the shared pool never served.
		{"frankenphp", config.Site{Name: "shop", Runtime: "frankenphp"}, false},
		{"host proxy", config.Site{Name: "shop", HostPort: 3000}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := switchableToNative(&c.site); got != c.want {
				t.Errorf("switchableToNative = %v, want %v", got, c.want)
			}
		})
	}
}
