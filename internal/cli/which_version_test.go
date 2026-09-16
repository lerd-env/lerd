package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// which reports what the site runs on, and for a linked site that is the version
// in the registry, because the vhost is generated from it. Re-detecting instead
// made which disagree with sites, the dashboard and nginx the moment a newer
// PHP was built, and it could name a prerelease nothing was serving from.
func TestWhichPHPVersionPrefersTheRegistry(t *testing.T) {
	site := &config.Site{Name: "shop", PHPVersion: "8.4"}
	if got := whichPHPVersion(site, "8.6"); got != "8.4" {
		t.Errorf("registry version should win, got %q", got)
	}

	// A site linked before lerd recorded a version falls back to detection
	// rather than showing nothing.
	bare := &config.Site{Name: "shop"}
	if got := whichPHPVersion(bare, "8.6"); got != "8.6" {
		t.Errorf("detection should fill in when the registry has none, got %q", got)
	}
}
