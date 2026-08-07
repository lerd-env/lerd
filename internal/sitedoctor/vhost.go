package sitedoctor

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
)

// checkVhost reports whether the nginx vhost serving the site still matches
// what lerd would write for it. Everything else the doctor looks at is inside
// the project; this is the one file between a healthy project and a served one,
// and it is written once and never revisited, so a document root, an upstream
// or a domain that has moved since leaves the site answering out of a config
// nobody has looked at.
//
// Skipped for a path that is not a registered site (a worktree is served by a
// vhost of its own), and for a site whose vhost lerd swapped on purpose.
func checkVhost(path string) (Check, bool) {
	site, err := config.FindSiteByPath(path)
	if err != nil || site == nil {
		return Check{}, false
	}
	drift, err := nginx.VhostDrift(*site)
	if err != nil || !drift.Checked {
		return Check{}, false
	}
	c := Check{Name: "vhost", Status: StatusOK, Detail: "serving from the current config"}
	if drift.Drifted {
		c.Status = StatusWarn
		c.Detail = "the vhost " + drift.Detail + "; regenerate it to serve what lerd would write now"
		c.Fix = FixVhostRegenerate
	}
	return c, true
}

// FixVhost rewrites the site's vhost and reloads nginx, the fix behind the
// vhost check. It takes the project path so it reads the same way as the checks
// do, and is a no-op for a path that is not a registered site.
func FixVhost(path string) error {
	site, err := config.FindSiteByPath(path)
	if err != nil || site == nil {
		return err
	}
	if err := nginx.RegenerateVhost(*site); err != nil {
		return err
	}
	nginx.ReloadOrWarn("")
	return nil
}
