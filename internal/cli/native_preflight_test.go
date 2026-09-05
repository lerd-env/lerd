package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The switch is install-wide, so a refusal should name every site standing in
// the way at once. Failing on the first one turns a single decision into a
// guessing game of repeated attempts.
func TestUnsupportedSitesNamesEveryBlocker(t *testing.T) {
	sites := []config.Site{
		{Name: "shop", PHPVersion: "8.4"},
		{Name: "legacy-cms", PHPVersion: "7.4"},
		{Name: "old-api", PHPVersion: "8.0"},
		{Name: "blog", PHPVersion: "8.2"},
		// Never served by the shared FPM container, so its version is irrelevant.
		{Name: "octane", PHPVersion: "7.4", Runtime: "frankenphp"},
	}
	msg := unsupportedSitesMessage(sites)
	if msg == "" {
		t.Fatal("expected a refusal naming the legacy sites")
	}
	for _, want := range []string{"legacy-cms", "7.4", "old-api", "8.0"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message should mention %q, got: %s", want, msg)
		}
	}
	for _, unwanted := range []string{"shop", "blog", "octane"} {
		if strings.Contains(msg, unwanted) {
			t.Errorf("message should not mention %q, got: %s", unwanted, msg)
		}
	}
}

func TestUnsupportedSitesSilentWhenAllSupported(t *testing.T) {
	if msg := unsupportedSitesMessage([]config.Site{{Name: "a", PHPVersion: "8.3"}}); msg != "" {
		t.Errorf("expected no refusal, got: %s", msg)
	}
}
