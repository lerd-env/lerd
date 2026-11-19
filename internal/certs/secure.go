package certs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/webserver"
)

// ErrDNSDisabled signals that the operation requires the lerd-managed DNS /
// mkcert CA stack, which the user has opted out of. Surfaces through the CLI
// `lerd secure` command and the dashboard HTTPS toggle.
var ErrDNSDisabled = fmt.Errorf("HTTPS requires lerd-managed DNS, set dns.enabled: true and re-run lerd install")

// RegenerateHostProxyWorktreeVhost regenerates the proxy vhost for a single
// worktree of a host-proxy site, switching it between HTTP and HTTPS. It is
// populated by the cli package (which owns host-proxy port allocation) so certs
// can do this without importing cli. nil in builds that don't link cli, in
// which case host-proxy worktree vhosts are left untouched.
var RegenerateHostProxyWorktreeVhost func(site config.Site, wtPath, wtDomain string, secured bool) error

// SecureSite issues a TLS certificate for the site and switches its nginx vhost to HTTPS.
func SecureSite(site config.Site) error {
	if cfg, _ := config.LoadGlobal(); !cfg.DNSManaged() {
		return ErrDNSDisabled
	}
	if err := issueCertWithWorktrees(site); err != nil {
		return fmt.Errorf("issuing certificate: %w", err)
	}

	https := site
	https.Secured = true
	if err := webserver.ApplySite(https); err != nil {
		return fmt.Errorf("writing SSL vhost: %w", err)
	}

	// Regenerate SSL vhosts and sync APP_URL + VITE_REVERB_* for worktrees.
	if worktrees, err := gitpkg.ServableWorktrees(site.Path, site.PrimaryDomain()); err == nil {
		for _, wt := range worktrees {
			if site.IsHostProxy() {
				// Host-proxy worktrees have no PHP/FPM; render the proxy vhost
				// pointing at the worktree's dev-server port instead of fastcgi.
				if RegenerateHostProxyWorktreeVhost != nil {
					_ = RegenerateHostProxyWorktreeVhost(site, wt.Path, wt.Domain, true)
				}
				config.SyncSiteURL(wt.Path, wt.Domain, true) //nolint:errcheck
				continue
			}
			effectivePHP := config.WorktreePHPVersion(wt.Path, site.PHPVersion)
			_ = webserver.ApplyWorktree(wt.Domain, wt.Path, effectivePHP, site.PrimaryDomain(), site.Name, wt.Branch, true)
			config.SyncSiteURL(wt.Path, wt.Domain, true) //nolint:errcheck
		}
	}

	return nil
}

// ReissueCertForWorktree reissues the site's TLS certificate to include
// wildcard SANs for all current worktree domains (*.branch.domain.test).
// Call this after a new worktree is created on a secured site so that
// subdomains like app.branch.domain.test are covered by the certificate.
func ReissueCertForWorktree(site config.Site) error {
	return issueCertWithWorktrees(site)
}

// EnsureCert reuses the site's existing certificate when it is still valid and
// clear of the reissue window, otherwise reissues one covering the site's own
// domains plus every current worktree domain. Unlike ReissueCertForWorktree it
// never forces, so it is cheap to call on every boot/watcher pass as the routine
// self-heal that keeps a long-lived secured site's leaf cert from expiring.
func EnsureCert(site config.Site) error {
	certsDir, domains := siteCertDomains(site)
	return IssueCert(site.PrimaryDomain(), domains, certsDir)
}

// siteCertDomains assembles the cert output directory and the full SAN list (the
// site's own domains plus every current worktree domain) shared by the reuse and
// force reissue paths so they cannot drift.
func siteCertDomains(site config.Site) (certsDir string, domains []string) {
	certsDir = filepath.Join(config.CertsDir(), "sites")
	var wtDomains []string
	if worktrees, err := gitpkg.ServableWorktrees(site.Path, site.PrimaryDomain()); err == nil {
		for _, wt := range worktrees {
			wtDomains = append(wtDomains, wt.Domain)
		}
	}
	return certsDir, WorktreeCertDomains(site.Domains, wtDomains)
}

// issueCertWithWorktrees detects all worktrees for the site and issues a
// certificate covering the site's own domains plus *.worktreeDomain for each
// worktree, so that deep subdomains (e.g. app.branch.domain.test) work. The
// reissue is atomic: a transient mkcert failure leaves the existing cert
// intact rather than tripping RepairVhosts into flipping the site to HTTP.
func issueCertWithWorktrees(site config.Site) error {
	certsDir, domains := siteCertDomains(site)
	return IssueCertForce(site.PrimaryDomain(), domains, certsDir)
}

// WorktreeCertDomains builds the full domain list for a certificate that covers
// the site's own domains plus all worktree domains. Each domain gets a wildcard
// entry via IssueCert, so worktree domains like branch.myapp.test produce
// *.branch.myapp.test SANs for deep subdomain coverage.
func WorktreeCertDomains(siteDomains []string, worktreeDomains []string) []string {
	domains := make([]string, len(siteDomains))
	copy(domains, siteDomains)
	domains = append(domains, worktreeDomains...)
	return domains
}

// UnsecureSite regenerates a plain HTTP vhost for the site, removing TLS.
func UnsecureSite(site config.Site) error {
	mainConf := filepath.Join(config.NginxConfD(), site.PrimaryDomain()+".conf")
	if err := os.Remove(mainConf); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing SSL vhost: %w", err)
	}

	plain := site
	plain.Secured = false
	if err := webserver.ApplySite(plain); err != nil {
		return fmt.Errorf("writing HTTP vhost: %w", err)
	}

	// Switch any worktree SSL vhosts back to plain HTTP and sync env.
	if worktrees, err := gitpkg.ServableWorktrees(site.Path, site.PrimaryDomain()); err == nil {
		for _, wt := range worktrees {
			if site.IsHostProxy() {
				if RegenerateHostProxyWorktreeVhost != nil {
					_ = RegenerateHostProxyWorktreeVhost(site, wt.Path, wt.Domain, false)
				}
				config.SyncSiteURL(wt.Path, wt.Domain, false) //nolint:errcheck
				continue
			}
			effectivePHP := config.WorktreePHPVersion(wt.Path, site.PHPVersion)
			_ = webserver.ApplyWorktree(wt.Domain, wt.Path, effectivePHP, "", site.Name, wt.Branch, false)
			config.SyncSiteURL(wt.Path, wt.Domain, false) //nolint:errcheck
		}
	}

	RemoveSiteCerts(site.PrimaryDomain())

	return nil
}

// RemoveSiteCerts deletes a domain's certificate pair. Site certs live in the
// sites/ subdirectory, so a caller building the path from CertsDir alone
// silently removes nothing.
func RemoveSiteCerts(domain string) {
	certsDir := filepath.Join(config.CertsDir(), "sites")
	os.Remove(filepath.Join(certsDir, domain+".crt")) //nolint:errcheck
	os.Remove(filepath.Join(certsDir, domain+".key")) //nolint:errcheck
}
