package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The case that made this necessary: a backup taken while the site was pinned
// to 8.3 restored over a working 8.5 site and put it back on 500, with nothing
// printed about the version moving.
func TestDiffSiteRegistries_namesAMovedPHPPin(t *testing.T) {
	cur := []config.Site{{Name: "demo", PHPVersion: "8.5", Domains: []string{"demo.test"}}}
	bak := []config.Site{{Name: "demo", PHPVersion: "8.3", Domains: []string{"demo.test"}}}

	got := strings.Join(diffSiteRegistries(cur, bak), "\n")
	if !strings.Contains(got, "PHP 8.5 → 8.3") {
		t.Errorf("diff should name the PHP move, got: %q", got)
	}
}

func TestDiffSiteRegistries_reportsAddedAndRemovedSites(t *testing.T) {
	cur := []config.Site{{Name: "demo"}, {Name: "gone-after"}}
	bak := []config.Site{{Name: "demo"}, {Name: "back-again"}}

	got := strings.Join(diffSiteRegistries(cur, bak), "\n")
	if !strings.Contains(got, "gone-after: removed from the registry") {
		t.Errorf("a site the backup lacks should read as removed, got: %q", got)
	}
	if !strings.Contains(got, "back-again: added back to the registry") {
		t.Errorf("a site only the backup has should read as added, got: %q", got)
	}
}

func TestDiffSiteRegistries_quietWhenNothingMoves(t *testing.T) {
	sites := []config.Site{{Name: "demo", PHPVersion: "8.5", Secured: true, Domains: []string{"demo.test"}}}
	if got := diffSiteRegistries(sites, sites); len(got) != 0 {
		t.Errorf("an identical registry should produce no lines, got: %v", got)
	}
}

// TLS and domains decide whether a site answers on the scheme the app writes
// into its env, so both belong in what the user is asked to agree to.
func TestDiffSiteRegistries_namesTLSAndDomainMoves(t *testing.T) {
	cur := []config.Site{{Name: "demo", Secured: true, Domains: []string{"demo.test"}}}
	bak := []config.Site{{Name: "demo", Secured: false, Domains: []string{"old.test"}}}

	got := strings.Join(diffSiteRegistries(cur, bak), "\n")
	if !strings.Contains(got, "TLS true → false") {
		t.Errorf("diff should name the TLS move, got: %q", got)
	}
	if !strings.Contains(got, "domains demo.test → old.test") {
		t.Errorf("diff should name the domain move, got: %q", got)
	}
}
