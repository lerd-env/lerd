package ui

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/siteinfo"
)

// linkableServices are the installed services a site can be wired to: the ones
// with something to wire, that it does not use yet, and whose address would not
// overwrite the one of a service it does use.
func linkableServices(site *config.Site) []string {
	used := siteinfo.Enrich(*site, siteinfo.EnrichServices).Services
	taken := map[string]bool{}
	for _, name := range used {
		for _, key := range addressKeys(serviceEnv(name)) {
			taken[key] = true
		}
	}
	out := []string{}
	consider := func(name string, env []string) {
		if len(env) == 0 || slices.Contains(used, name) || slices.Contains(out, name) {
			return
		}
		if slices.ContainsFunc(addressKeys(env), func(key string) bool { return taken[key] }) {
			return
		}
		out = append(out, name)
	}
	for _, name := range siteinfo.KnownServices() {
		if serviceops.ServiceInstalled(name) {
			consider(name, config.DefaultPresetEnvVars(name))
		}
	}
	customs, _ := config.ListCustomServices()
	for _, svc := range customs {
		consider(svc.Name, svc.EnvVars)
	}
	return out
}

// linkSiteService wires a site to a service this machine already runs. Only what
// linkableServices offers is taken, so a crafted request cannot write an
// arbitrary service into a project's .lerd.yaml.
func linkSiteService(site *config.Site, name string) error {
	if !slices.Contains(linkableServices(site), name) {
		return fmt.Errorf("%q cannot be added to %s: it is not installed, the site already uses it, or it would replace a service the site uses", name, site.Name)
	}
	// Older projects list sqlite, which env would go on wiring over the database.
	if slices.ContainsFunc(serviceEnv(name), func(kv string) bool { return strings.HasPrefix(kv, "DB_CONNECTION=") }) {
		if proj, err := config.LoadProjectConfig(site.Path); err == nil && slices.Contains(proj.ServiceNames(), "sqlite") {
			if err := config.RemoveProjectService(site.Path, "sqlite"); err != nil {
				return err
			}
		}
	}
	return wireSiteService(site, cli.ProjectServiceFor(name, nil))
}

// serviceEnv is the env a service publishes. A name this machine has not
// installed is read from its preset.
func serviceEnv(name string) []string {
	if config.IsDefaultPreset(name) {
		return config.DefaultPresetEnvVars(name)
	}
	if svc, err := config.LoadCustomService(name); err == nil {
		return svc.EnvVars
	}
	p, err := config.LoadPreset(name)
	if err != nil {
		// mysql-8-4 is named after the preset it came from.
		p, err = config.LoadPreset(config.InferFamily(name))
	}
	if err != nil {
		return nil
	}
	return p.EnvVars
}

// addressKeys are the keys naming the service's container. Driver keys such as
// QUEUE_CONNECTION are left out: two services a site uses side by side share them.
func addressKeys(env []string) []string {
	var keys []string
	for _, kv := range env {
		if key, value, _ := strings.Cut(kv, "="); strings.Contains(value, "lerd-") {
			keys = append(keys, key)
		}
	}
	return keys
}

// handleSiteServiceLinkable answers the add-service picker. Worked out when it
// opens, since it reads every service definition and sites are listed often.
func handleSiteServiceLinkable(w http.ResponseWriter, domain string) {
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		writeJSON(w, SiteActionResponse{Error: "site not found: " + domain})
		return
	}
	writeJSON(w, map[string][]string{"services": linkableServices(site)})
}
