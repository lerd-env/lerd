package ui

import (
	"fmt"
	"slices"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
)

// removeSiteService takes a service off a site: out of .lerd.yaml, then out of
// the env file, so the site stops listing it. The service and its data stay.
func removeSiteService(site *config.Site, name string) error {
	if err := config.RemoveProjectService(site.Path, name); err != nil {
		return err
	}
	if err := config.UnwireProjectService(site.Path, name); err != nil {
		return err
	}
	return config.SetSiteServiceDeclined(site.Name, name, true)
}

// declareSiteService records in .lerd.yaml a service the site reaches only
// through its env file. It is already wired, so nothing is installed or rewired.
func declareSiteService(site *config.Site, name string) error {
	e := siteinfo.Enrich(*site, siteinfo.EnrichServices)
	if !slices.Contains(e.Services, name) || slices.Contains(e.DeclaredServices, name) {
		return fmt.Errorf("%q is not a service %s uses without declaring it", name, site.Name)
	}
	svc := config.ProjectService{Name: name}
	if !config.IsDefaultPreset(name) {
		svc.Preset = name
	}
	if err := config.AddProjectServices(site.Path, []config.ProjectService{svc}); err != nil {
		return err
	}
	return config.SetSiteServiceDeclined(site.Name, name, false)
}
