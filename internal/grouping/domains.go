package grouping

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/geodro/lerd/internal/webserver"
)

// Indirection points so ApplyDomainChange can be driven without podman or nginx.
var (
	writeHostsFn  = podman.WriteContainerHosts
	reloadNginxFn = webserver.ReloadAndSettle
	syncEnvFn     = siteops.SyncEnvIfPrimaryChanged
)

// ApplyDomainChange brings everything that follows a site's domains in line
// once the registry holds the new list: vhost, certificate SANs, container
// hosts, nginx, the .env and its worktrees, and a group's secondaries. Only a
// vhost failure is fatal; the rest come back as warnings, since the domain
// change itself has already been recorded.
func ApplyDomainChange(site *config.Site, oldPrimary string) (warnings []error, err error) {
	if err := regenerateVhostFn(site, oldPrimary); err != nil {
		return nil, err
	}
	warn := func(what string, err error) {
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", what, err))
		}
	}
	// The worktree-aware reissue keeps <branch>.<primary> SANs a bare
	// reissue would drop.
	if site.Secured {
		warn("reissuing certificate", reissueCertFn(*site))
	}
	warn("updating container hosts file", writeHostsFn())
	warn("reloading nginx", reloadNginxFn())
	warn("syncing .env to new primary domain", syncEnvFn(site, oldPrimary))
	if site.IsGroupMain() {
		warn("cascading group domain change", CascadeMainDomainChange(site))
	}
	return warnings, nil
}
