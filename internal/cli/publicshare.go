package cli

import (
	"fmt"
	"net/http"
	"sync"

	gitpkg "github.com/geodro/lerd/internal/git"

	"github.com/geodro/lerd/internal/config"
)

// A public share is the reverse-proxy twin of a LAN share: the same in-process
// Host-rewriting proxy (startLANShareProxy) on a stable 0.0.0.0 port, but reached
// through the user's own reverse proxy instead of by LAN IP. The user points a
// wildcard subdomain they control at "<site>.<base>" -> this machine's public
// port, and the proxy rewrites Host so nginx serves the site's normal .test
// vhost. Nothing is added to the site's domains, nginx server_name, or certs; it
// is purely a runtime share, started and stopped from the share menu.

const publicSharePortBase = 9300

var (
	publicShareMu      sync.Mutex
	publicShareServers = map[string]*http.Server{} // key -> running proxy
)

// publicShareKey / worktree key mirror the LAN share server keys.
func publicWorktreeKey(siteName, branch string) string { return siteName + "@" + branch }

// PublicBaseDomain returns the configured public base, empty when none is set.
func PublicBaseDomain() string {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return ""
	}
	return cfg.Share.PublicBaseDomain
}

// SetPublicBaseDomain validates and stores the public base domain. Clearing it
// (empty) stops every running public share, since their URLs no longer exist.
func SetPublicBaseDomain(base string) (string, error) {
	normalized, err := config.NormalizePublicBase(base)
	if err != nil {
		return "", err
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return "", err
	}
	cfg.Share.PublicBaseDomain = normalized
	if err := config.SaveGlobal(cfg); err != nil {
		return "", err
	}
	if normalized == "" {
		StopAllPublicShares()
	}
	return normalized, nil
}

// PublicShareStart starts the site's public share proxy and returns the public
// hostname it is reached at ("<site>.<base>"). Idempotent. Needs a base
// configured, since without one there is no URL to hand out.
func PublicShareStart(siteName string) (string, error) {
	base := PublicBaseDomain()
	if base == "" {
		return "", fmt.Errorf("no public base domain is set: configure one in the share menu first")
	}
	site, err := config.FindSite(siteName)
	if err != nil {
		return "", err
	}
	if site.Paused {
		return "", fmt.Errorf("site %q is paused", siteName)
	}
	if TunnelActive(siteName, "") || LANShareRunning(siteName) {
		return "", errShareBusy
	}

	port := site.PublicPort
	if port == 0 {
		port = assignPublicSharePort(siteName, "")
		site.PublicPort = port
		if err := config.AddSite(*site); err != nil {
			return "", fmt.Errorf("saving public port: %w", err)
		}
	}

	host := config.PublicShareHost(siteName, base)
	publicShareMu.Lock()
	if _, running := publicShareServers[siteName]; running {
		publicShareMu.Unlock()
		return host, nil
	}
	publicShareMu.Unlock()

	srv, err := startPublicShareProxyFor(site.PrimaryDomain(), port, site.Secured)
	if err != nil {
		return "", err
	}
	publicShareMu.Lock()
	publicShareServers[siteName] = srv
	publicShareMu.Unlock()
	return host, nil
}

// PublicShareStop stops the site's public share proxy and clears its port.
func PublicShareStop(siteName string) error {
	closePublicShareServer(siteName)
	site, err := config.FindSite(siteName)
	if err != nil {
		return err
	}
	if site.PublicPort == 0 {
		return nil
	}
	site.PublicPort = 0
	return config.AddSite(*site)
}

// PublicShareStartWorktree starts the public share for a worktree, served on the
// flat "<site>-<branch>.<base>" host, and returns it. The proxy targets
// "<branch>.<parent>.<tld>" so nginx routes to the worktree vhost.
func PublicShareStartWorktree(siteName, branch string) (string, error) {
	base := PublicBaseDomain()
	if base == "" {
		return "", fmt.Errorf("no public base domain is set: configure one in the share menu first")
	}
	site, err := config.FindSite(siteName)
	if err != nil {
		return "", err
	}
	if site.Paused {
		return "", fmt.Errorf("site %q is paused", siteName)
	}
	if TunnelActive(siteName, branch) || LANShareWorktreeRunning(siteName, branch) {
		return "", errShareBusy
	}

	port := site.WorktreePublicPorts[branch]
	if port == 0 {
		port = assignPublicSharePort(siteName, branch)
		if err := setWorktreePublicPort(siteName, branch, port); err != nil {
			return "", err
		}
	}

	host := config.PublicShareWorktreeHost(siteName, gitpkg.SanitizeBranch(branch), base)
	key := publicWorktreeKey(siteName, branch)
	publicShareMu.Lock()
	if _, running := publicShareServers[key]; running {
		publicShareMu.Unlock()
		return host, nil
	}
	publicShareMu.Unlock()

	worktreeDomain := branch + "." + site.PrimaryDomain()
	srv, err := startPublicShareProxyFor(worktreeDomain, port, site.Secured)
	if err != nil {
		return "", err
	}
	publicShareMu.Lock()
	publicShareServers[key] = srv
	publicShareMu.Unlock()
	return host, nil
}

// PublicShareStopWorktree stops a worktree's public share and clears its port.
func PublicShareStopWorktree(siteName, branch string) error {
	closePublicShareServer(publicWorktreeKey(siteName, branch))
	return setWorktreePublicPort(siteName, branch, 0)
}

// PublicShareRunning reports whether the site's public share proxy is bound.
func PublicShareRunning(siteName string) bool {
	publicShareMu.Lock()
	defer publicShareMu.Unlock()
	_, ok := publicShareServers[siteName]
	return ok
}

// PublicShareWorktreeRunning reports whether a worktree's public share is bound.
func PublicShareWorktreeRunning(siteName, branch string) bool {
	publicShareMu.Lock()
	defer publicShareMu.Unlock()
	_, ok := publicShareServers[publicWorktreeKey(siteName, branch)]
	return ok
}

// RestorePublicShareProxies restarts public share proxies for every site (and
// worktree) that has a public port stored. Called once when the UI server starts.
func RestorePublicShareProxies() {
	if PublicBaseDomain() == "" {
		return
	}
	reg, err := config.LoadSites()
	if err != nil {
		return
	}
	for _, s := range reg.Sites {
		if s.Paused {
			continue
		}
		if s.PublicPort != 0 {
			if srv, err := startPublicShareProxyFor(s.PrimaryDomain(), s.PublicPort, s.Secured); err == nil {
				publicShareMu.Lock()
				publicShareServers[s.Name] = srv
				publicShareMu.Unlock()
			}
		}
		for branch, port := range s.WorktreePublicPorts {
			if port == 0 {
				continue
			}
			srv, err := startPublicShareProxyFor(branch+"."+s.PrimaryDomain(), port, s.Secured)
			if err != nil {
				continue
			}
			publicShareMu.Lock()
			publicShareServers[publicWorktreeKey(s.Name, branch)] = srv
			publicShareMu.Unlock()
		}
	}
}

// StopAllPublicShares closes every running public share proxy without touching
// the stored ports, so a restart brings them back. Called on daemon shutdown.
func StopAllPublicShares() {
	publicShareMu.Lock()
	servers := publicShareServers
	publicShareServers = map[string]*http.Server{}
	publicShareMu.Unlock()
	for _, srv := range servers {
		srv.Close()
	}
}

// startPublicShareProxyFor starts the shared LAN-share proxy for a public share.
// The proxy derives the public-facing host from the incoming Host header, so the
// same implementation serves a LAN IP or a public hostname alike.
func startPublicShareProxyFor(domain string, port int, secured bool) (*http.Server, error) {
	cfg, _ := config.LoadGlobal()
	httpPort, httpsPort := 80, 443
	if cfg != nil {
		if cfg.Nginx.HTTPPort != 0 {
			httpPort = cfg.Nginx.HTTPPort
		}
		if cfg.Nginx.HTTPSPort != 0 {
			httpsPort = cfg.Nginx.HTTPSPort
		}
	}
	return startLANShareProxy(domain, port, httpPort, httpsPort, secured)
}

func closePublicShareServer(key string) {
	publicShareMu.Lock()
	srv, running := publicShareServers[key]
	if running {
		delete(publicShareServers, key)
	}
	publicShareMu.Unlock()
	if running {
		srv.Close()
	}
}

// setWorktreePublicPort writes (or clears, when port is 0) a worktree's public
// port on the site registry.
func setWorktreePublicPort(siteName, branch string, port int) error {
	site, err := config.FindSite(siteName)
	if err != nil {
		return err
	}
	if port == 0 {
		delete(site.WorktreePublicPorts, branch)
	} else {
		if site.WorktreePublicPorts == nil {
			site.WorktreePublicPorts = map[string]int{}
		}
		site.WorktreePublicPorts[branch] = port
	}
	return config.AddSite(*site)
}

// assignPublicSharePort returns the lowest unused public-share port, scanning
// every site and worktree public port, excluding the one being assigned.
func assignPublicSharePort(excludeSite, excludeBranch string) int {
	used := map[int]bool{}
	if reg, err := config.LoadSites(); err == nil {
		for _, s := range reg.Sites {
			if s.PublicPort != 0 && !(s.Name == excludeSite && excludeBranch == "") {
				used[s.PublicPort] = true
			}
			for br, p := range s.WorktreePublicPorts {
				if p != 0 && !(s.Name == excludeSite && br == excludeBranch) {
					used[p] = true
				}
			}
		}
	}
	port := publicSharePortBase
	for used[port] {
		port++
	}
	return port
}
