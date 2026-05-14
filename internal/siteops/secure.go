package siteops

import (
	"fmt"
	"net/http"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/nginx"
)

// Indirection points so tests can swap in inert stubs without touching mkcert,
// nginx, podman, or the daemon HTTP API. Production code uses the real impls.
var (
	secureCertFn   = certs.SecureSite
	unsecureCertFn = certs.UnsecureSite
	nginxReloadFn  = nginx.Reload
	notifyDaemonFn = defaultNotifyDaemon
)

// defaultNotifyDaemon posts an action to the running lerd-ui daemon HTTP
// API. Best-effort: if the daemon isn't running, the systemd services it
// would have refreshed (Stripe listener, LAN share proxy) aren't being
// supervised anyway, so silently skipping the notification is correct.
func defaultNotifyDaemon(domain, action string) error {
	resp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:7073/api/sites/%s/%s", domain, action),
		"application/json", nil,
	)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// SetSecured toggles the site's TLS state and runs every step the toggle
// depends on. It is the single source of truth for "what happens when a
// site is secured/unsecured"; CLI, UI, and MCP all call this with no
// per-caller variation so a new step added here applies everywhere.
//
// Steps:
//  1. Issue or remove the certificate (also regenerates the nginx vhost on disk).
//  2. Persist site.Secured to the registry.
//  3. Sync APP_URL and VITE_REVERB_HOST/SCHEME/PORT in the project's .env.
//  4. Update the per-project .lerd.yaml secured flag.
//  5. Reload nginx so the new vhost takes effect.
//  6. Notify the daemon to refresh dependent listeners (Stripe webhook URL,
//     LAN share proxy backend port). The daemon owns the in-process state
//     for these, so even callers running inside the daemon hit the same
//     HTTP endpoints; a tiny loopback roundtrip is the cost of having one
//     identical post-toggle path.
func SetSecured(site *config.Site, secured bool) error {
	if secured {
		if err := secureCertFn(*site); err != nil {
			return fmt.Errorf("issuing certificate: %w", err)
		}
	} else {
		if err := unsecureCertFn(*site); err != nil {
			return fmt.Errorf("removing certificate: %w", err)
		}
	}
	site.Secured = secured
	if err := config.AddSite(*site); err != nil {
		return fmt.Errorf("updating site registry: %w", err)
	}
	_ = envfile.SyncPrimaryDomain(site.Path, site.PrimaryDomain(), secured)
	_ = config.SetProjectSecured(site.Path, secured)
	if err := nginxReloadFn(); err != nil {
		return fmt.Errorf("reloading nginx: %w", err)
	}
	_ = notifyDaemonFn(site.PrimaryDomain(), "stripe:refresh")
	_ = notifyDaemonFn(site.PrimaryDomain(), "lan:refresh")
	return nil
}
