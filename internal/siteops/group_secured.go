package siteops

import (
	"errors"
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
)

// A group main serves "<main> *.<main>". nginx prefers an exact server_name over
// a wildcard, but only on the ports the exact block listens on, so a secondary
// left on plain HTTP has no 443 block and the main's wildcard answers its
// subdomain, serving the main's app (#811). Securing a main therefore secures
// its secondaries, and unsecuring a secondary under a secured main is refused.

var dnsManagedFn = func() bool { cfg, _ := config.LoadGlobal(); return cfg.DNSManaged() }

// securedMainOf returns the secured group main above site, or nil when site is
// not a secondary, its main is missing, or that main is on plain HTTP (which
// publishes no 443 wildcard and so cannot swallow the subdomain).
func securedMainOf(site *config.Site) *config.Site {
	if !site.IsGroupSecondary() {
		return nil
	}
	reg, err := config.LoadSites()
	if err != nil || reg == nil {
		return nil
	}
	for i := range reg.Sites {
		m := &reg.Sites[i]
		if m.IsGroupMain() && m.Group == site.Group && m.Secured {
			return m
		}
	}
	return nil
}

// secureSecondary secures one secondary. A reload against a stopped nginx is
// benign here: SetSecured has already written the cert, vhost and registry, and
// the config is what the next start reads.
func secureSecondary(s *config.Site) error {
	if err := SetSecured(s, true); err != nil && !errors.Is(err, nginx.ErrNotRunning) {
		return err
	}
	return nil
}

// cascadeGroupSecondaries secures every plain-HTTP secondary of main. Called
// when main itself is secured, so the wildcard it just gained on 443 cannot
// start answering a secondary's subdomain.
func cascadeGroupSecondaries(main *config.Site) ([]string, error) {
	reg, err := config.LoadSites()
	if err != nil || reg == nil {
		return nil, err
	}
	var changed, failures []string
	for _, s := range reg.Sites {
		if s.Secured || !s.IsGroupSecondary() || s.Group != main.Group {
			continue
		}
		if err := secureSecondary(&s); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", s.Name, err))
			continue
		}
		changed = append(changed, s.Name)
	}
	if len(failures) > 0 {
		return changed, fmt.Errorf("securing group secondaries: %s", strings.Join(failures, "; "))
	}
	return changed, nil
}

// EnforceGroupSecondaries repairs the invariant across the whole registry: it
// secures every secondary whose group main is secured. Install runs it as a
// reconcile, which is also what `lerd dns:enable` re-execs into, so an install
// that has already drifted into the broken state is repaired.
//
// Returns the secondaries it changed. A failure on one is reported but does not
// stop the rest: a partially repaired group still serves more sites correctly
// than an aborted pass. No-op when DNS is disabled, where nothing has HTTPS.
func EnforceGroupSecondaries() ([]string, error) {
	if !dnsManagedFn() {
		return nil, nil
	}
	reg, err := config.LoadSites()
	if err != nil || reg == nil {
		return nil, err
	}

	securedMains := map[string]bool{}
	for _, s := range reg.Sites {
		if s.IsGroupMain() && s.Secured {
			securedMains[s.Group] = true
		}
	}

	var changed, failures []string
	for _, s := range reg.Sites {
		if s.Secured || !s.IsGroupSecondary() || !securedMains[s.Group] {
			continue
		}
		if err := secureSecondary(&s); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", s.Name, err))
			continue
		}
		changed = append(changed, s.Name)
	}
	if len(failures) > 0 {
		return changed, fmt.Errorf("securing group secondaries: %s", strings.Join(failures, "; "))
	}
	return changed, nil
}
