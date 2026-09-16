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

// The case the first version of this diff let through, found by a lane A rerun.
// A backup taken while a site was mid-FrankenPHP-switch carries
// runtime: frankenphp. The diff compared only what a site is, not how it is
// served, so it reported "nothing would change", restored without asking, and
// repointed the vhost at a per-site container that had already been removed.
// The site then 502'd indefinitely with nothing said.
func TestDiffSiteRegistries_namesARuntimeMove(t *testing.T) {
	cur := []config.Site{{Name: "demo", PHPVersion: "8.5"}} // "" is the shared FPM pool
	bak := []config.Site{{Name: "demo", PHPVersion: "8.5", Runtime: "frankenphp"}}

	got := strings.Join(diffSiteRegistries(cur, bak), "\n")
	if !strings.Contains(got, "runtime fpm → frankenphp") {
		t.Errorf("diff should name the runtime move, got: %q", got)
	}
}

// Two sites on the shared pool must not read as a change just because the
// runtime field is empty on both.
func TestDiffSiteRegistries_quietWhenBothOnTheSharedPool(t *testing.T) {
	sites := []config.Site{{Name: "demo", PHPVersion: "8.5"}}
	if got := diffSiteRegistries(sites, sites); len(got) != 0 {
		t.Errorf("identical sites produced %v", got)
	}
}

// An unset port on both sides is not a change, or every diff would carry noise.
func TestDiffSiteRegistries_ignoresUnsetPorts(t *testing.T) {
	cur := []config.Site{{Name: "demo", ContainerPort: 0, HostPort: 0}}
	bak := []config.Site{{Name: "demo", ContainerPort: 0, HostPort: 0}}
	if got := diffSiteRegistries(cur, bak); len(got) != 0 {
		t.Errorf("unset ports reported as a change: %v", got)
	}
	cur2 := []config.Site{{Name: "demo", HostPort: 0}}
	bak2 := []config.Site{{Name: "demo", HostPort: 8080}}
	if got := strings.Join(diffSiteRegistries(cur2, bak2), "\n"); !strings.Contains(got, "host port") {
		t.Errorf("a real port move should be named, got: %q", got)
	}
}
