package cli

import (
	"path/filepath"
	"strconv"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/nginx"
	phpDet "github.com/geodro/lerd/internal/php"
)

// regenNginxVhost regenerates the nginx vhost for the site so proxy blocks are updated.
func regenNginxVhost(siteName, sitePath string) {
	site, err := config.FindSite(siteName)
	if err != nil {
		return
	}

	// Custom container and host-proxy sites handle proxying through their own
	// vhost, so the PHP-specific vhost regeneration is not needed.
	if site.IsCustomContainer() || site.IsHostProxy() {
		return
	}

	phpVer := site.PHPVersion
	if detected, detErr := phpDet.DetectVersion(sitePath); detErr == nil && detected != "" {
		phpVer = detected
	}
	var vhostErr error
	if site.Secured {
		if vhostErr = nginx.GenerateSSLVhost(*site, phpVer); vhostErr == nil {
			vhostErr = nginx.InstallSSLVhost(site.PrimaryDomain())
		}
	} else {
		vhostErr = nginx.GenerateVhost(*site, phpVer)
	}
	if vhostErr == nil {
		_ = nginx.Reload()
	}
}

// assignWorkerProxyPort finds the lowest unused port >= defaultPort for the given
// env key across all linked sites.
// assignWorkerProxyPort finds the lowest unused port >= defaultPort.
// It scans ALL proxy port env keys across ALL sites to prevent collisions
// between different workers and different frameworks.
func assignWorkerProxyPort(sitePath, envKey string, defaultPort int) int {
	if defaultPort == 0 {
		defaultPort = 8080
	}
	used := map[int]bool{}
	reg, err := config.LoadSites()
	if err != nil {
		return defaultPort
	}

	// Collect all proxy port env key names from every framework definition.
	proxyPortKeys := map[string]bool{envKey: true}
	for _, s := range reg.Sites {
		if s.Framework == "" {
			continue
		}
		fw, ok := config.GetFramework(s.Framework)
		if !ok {
			continue
		}
		for _, w := range fw.Workers {
			if w.Proxy != nil && w.Proxy.PortEnvKey != "" {
				proxyPortKeys[w.Proxy.PortEnvKey] = true
			}
		}
	}

	// Scan all sites for all proxy port values to build the used set.
	for _, s := range reg.Sites {
		if filepath.Clean(s.Path) == filepath.Clean(sitePath) {
			continue
		}
		for key := range proxyPortKeys {
			if v := envfile.ReadKey(filepath.Join(s.Path, ".env"), key); v != "" {
				if p, err := strconv.Atoi(v); err == nil {
					used[p] = true
				}
			}
		}
	}

	port := defaultPort
	for used[port] {
		port++
	}
	return port
}
