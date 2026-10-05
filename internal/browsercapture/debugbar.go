package browsercapture

import "github.com/geodro/lerd/internal/config"

// SetDebugbar turns the debug bar on or off for one site. Its vhost carries
// the script tag, so a change rewrites it and reloads nginx.
func SetDebugbar(site config.Site, on bool) (Result, error) {
	if config.DebugbarFor(site) == on {
		return Result{Enabled: on, NoChange: true}, nil
	}
	if err := config.SaveDebugbar(site, on); err != nil {
		return Result{}, err
	}
	if !Capturable(site) {
		return Result{Enabled: on}, nil
	}
	updated, err := config.FindSite(site.Name)
	if err != nil {
		return Result{}, err
	}
	if err := regenerateSiteVhostFn(*updated); err != nil {
		return Result{}, err
	}
	return Result{Enabled: on}, nginxReloadFn()
}
