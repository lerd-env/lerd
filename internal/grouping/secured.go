package grouping

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteops"
)

// Package vars so the invariant pass can be tested without issuing certs,
// writing vhosts or reaching nginx.
var (
	setSecuredFn = siteops.SetSecured
	dnsManagedFn = func() bool { cfg, _ := config.LoadGlobal(); return cfg.DNSManaged() }
)

// EnforceSecondarySecured secures every group secondary whose main is secured.
// A group main serves "<main> *.<main>", so a secondary left on plain HTTP has
// no 443 block and the main's wildcard answers its subdomain instead (#811).
//
// Returns the secondaries it changed. A failure on one is reported but does not
// stop the rest: a partially repaired group still serves more sites correctly
// than an aborted pass. No-op when DNS is disabled, where nothing has HTTPS.
func EnforceSecondarySecured() ([]string, error) {
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

	var changed []string
	var failures []string
	for _, s := range reg.Sites {
		if s.Secured || !s.IsGroupSecondary() || !securedMains[s.Group] {
			continue
		}
		if err := setSecuredFn(&s, true); err != nil {
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
