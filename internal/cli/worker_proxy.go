package cli

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/envpass"
	"github.com/geodro/lerd/internal/freeport"
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

// pinnedWorkerPort returns the port lerd owns for this worker, allocating one on
// first use and recording it on the site. A pinned port belongs to lerd rather
// than to the project's .env, so it is kept clear of every other site's pinned
// ports and dev servers, and it survives restarts: the vhost proxies to it and
// the worker is handed the same number.
func pinnedWorkerPort(siteName, workerName string, defaultPort int) int {
	if defaultPort == 0 {
		defaultPort = 8080
	}
	reg, err := config.LoadSites()
	if err != nil {
		return defaultPort
	}

	used := map[int]bool{}
	for _, s := range reg.Sites {
		if s.Name == siteName {
			if p := s.WorkerPorts[workerName]; p > 0 {
				return p
			}
		}
		for name, p := range s.WorkerPorts {
			if s.Name == siteName && name == workerName {
				continue
			}
			used[p] = true
		}
		if s.DevServerPort != 0 {
			used[s.DevServerPort] = true
		}
		for _, p := range s.WorktreeDevPorts {
			used[p] = true
		}
	}

	port := freeport.FirstFree(defaultPort, func(p int) bool {
		return used[p] || !freeport.Bindable(p)
	})
	if port == 0 {
		port = defaultPort
	}

	site, err := config.FindSite(siteName)
	if err != nil {
		return port
	}
	if site.WorkerPorts == nil {
		site.WorkerPorts = map[string]int{}
	}
	site.WorkerPorts[workerName] = port
	if err := config.AddSite(*site); err != nil {
		return port
	}
	return port
}

// withPinnedWorkerPort hands the worker the port lerd picked, through the key
// the definition names, so the tool's own config reads it from the environment
// rather than lerd guessing a flag the command may not take.
//
// It goes through env(1) rather than a bare KEY=value prefix, because a prefix
// is shell syntax and the command does not always start a shell line: a host
// worker is spliced after the version manager's `fnm exec -- `, where the
// assignment would be read as the name of the program to run.
func withPinnedWorkerPort(siteName, workerName string, w config.FrameworkWorker, command string) string {
	if !w.Proxy.PinnedPort() || w.Proxy.PortEnvKey == "" {
		return command
	}
	port := strconv.Itoa(pinnedWorkerPort(siteName, workerName, w.Proxy.DefaultPort))
	key := w.Proxy.PortEnvKey
	// A command that takes the port as an argument writes it as $KEY, and the
	// shell expands that before env(1) has set anything, so the tool would be
	// handed an empty string. Fill those in directly, and still export the key
	// for a tool that reads its own config at runtime.
	command = strings.NewReplacer("${"+key+"}", port, "$"+key, port).Replace(command)
	// A command that re-enters the container through lerd's own shims (php
	// artisan, and anything it starts) gets a clean environment there, so the
	// key is named for passthrough as well: the shim forwards what it is told
	// to forward, and the tool reads the same port on either side.
	return "env " + key + "=" + port + " " + envpass.EnvVar + "=" + key + " " + command
}
