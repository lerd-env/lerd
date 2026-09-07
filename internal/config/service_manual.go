package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
)

func manuallyStartedServicesFile() string {
	return filepath.Join(DataDir(), "manually-started-services.yaml")
}

// ServiceIsManuallyStarted returns true if the service was explicitly started by
// the user (via `lerd service start` or the dashboard), making it exempt from
// auto-stop when no sites reference it.
func ServiceIsManuallyStarted(name string) bool {
	return serviceSetContains(manuallyStartedServicesFile(), name)
}

// SetServiceManuallyStarted marks or clears the manually-started flag for the
// named service.
func SetServiceManuallyStarted(name string, v bool) error {
	return serviceSetUpdate(manuallyStartedServicesFile(), name, v)
}

func pinnedServicesFile() string {
	return filepath.Join(DataDir(), "pinned-services.yaml")
}

// ServiceIsPinned returns true if the service has been pinned by the user,
// meaning it will never be auto-stopped even when no sites reference it.
func ServiceIsPinned(name string) bool {
	return serviceSetContains(pinnedServicesFile(), name)
}

// SetServicePinned marks or clears the pinned flag for the named service.
func SetServicePinned(name string, v bool) error {
	return serviceSetUpdate(pinnedServicesFile(), name, v)
}

// CountSitesUsingPHP returns how many non-ignored, non-paused sites are
// registered with the given PHP version.
func CountSitesUsingPHP(version string) int {
	reg, err := LoadSites()
	if err != nil {
		return 0
	}
	count := 0
	for _, s := range reg.Sites {
		if s.Ignored || s.Paused {
			continue
		}
		if s.PHPVersion == version {
			count++
		}
	}
	return count
}

// SitesUsingService returns the active (non-ignored, non-paused) sites whose
// .lerd.yaml lists the service or whose env file references lerd-{name}. The
// env file is the one the framework declares, so a project keeping its
// configuration in wp-config.php or app/etc/env.php counts the same as one
// keeping it in .env.
func SitesUsingService(name string) []Site {
	reg, err := LoadSites()
	if err != nil {
		return nil
	}
	domain := ServiceDomain(name)
	var out []Site
	for _, s := range reg.Sites {
		if s.Ignored || s.Paused {
			continue
		}
		if proj, pErr := LoadProjectConfig(s.Path); pErr == nil {
			matched := false
			for _, svc := range proj.Services {
				if svc.Name == name {
					out = append(out, s)
					matched = true
					break
				}
			}
			if matched {
				continue
			}
		}
		envFile, _ := EnvFileFor(s.Path)
		if data, err := os.ReadFile(filepath.Join(s.Path, envFile)); err == nil {
			if envfile.ReferencesContainer(string(data), name) {
				out = append(out, s)
				continue
			}
			// A site whose env was rewritten to the service's domain no longer
			// names the container at all, so without this it drops out of every
			// sweep that keeps it wired: the one that would put the container
			// name back when the domain goes away, and the provisioning that
			// keeps its database or bucket there.
			if domain != "" && strings.Contains(string(data), domain) {
				out = append(out, s)
			}
		}
	}
	return out
}

// CountSitesUsingService returns how many active (non-ignored, non-paused) site
// .env files reference lerd-{name}, i.e. are configured to use the service.
func CountSitesUsingService(name string) int {
	return len(SitesUsingService(name))
}

// ServicePublishedPort returns the published host port a service was pinned to
// (0 = preset/version default). It is non-zero when the user ran
// `lerd service port`, or when the port-ownership guard auto-shifted lerd's DB
// off the engine default because a host server owns it. Readers that surface a
// host-facing endpoint (a host-proxy app's .env, a connection URL) use it so the
// port reflects where lerd's container actually listens, not the default a
// coexisting host server may be sitting on.
func ServicePublishedPort(name string) int {
	cfg, err := LoadGlobal()
	if err != nil {
		return 0
	}
	if sc, ok := cfg.Services[name]; ok {
		return sc.PublishedPort
	}
	return 0
}

// ServiceConfigFor returns the global config entry for a service (its ports,
// image pin and enabled flag), or the zero ServiceConfig when the service isn't
// configured. Callers that need the effective host ports read HostPorts() off it
// so the preset default and any override resolve from the one registry.
func ServiceConfigFor(name string) ServiceConfig {
	cfg, err := LoadGlobal()
	if err != nil {
		return ServiceConfig{}
	}
	return cfg.Services[name]
}

// ServicePublishedPorts returns the per-container-port host overrides for a
// service's secondary mappings (nil when none). The primary override lives in
// ServicePublishedPort; these cover the extra ports a multi-port service exposes.
func ServicePublishedPorts(name string) map[int]int {
	cfg, err := LoadGlobal()
	if err != nil {
		return nil
	}
	if sc, ok := cfg.Services[name]; ok {
		return sc.PublishedPorts
	}
	return nil
}

// ServiceExtraPorts returns the extra published port mappings recorded for a
// service (set via `lerd service expose` or the Web UI ports modal), or nil when
// none. Applied at the quadlet choke point so every preset-backed service, not
// just the default-stack ones, honours the same extra-port overrides.
func ServiceExtraPorts(name string) []string {
	cfg, err := LoadGlobal()
	if err != nil {
		return nil
	}
	if sc, ok := cfg.Services[name]; ok {
		return sc.ExtraPorts
	}
	return nil
}

// ServiceDomain returns the hostname a service is served on, empty when it has
// none. Readers that hand a URL to a browser use it: the container name a
// service is otherwise reached at resolves nowhere outside the podman network.
func ServiceDomain(name string) string {
	cfg, err := LoadGlobal()
	if err != nil {
		return ""
	}
	return cfg.Services[name].Domain
}

// ServiceDomains returns every configured service domain keyed by service name.
// The hosts file and the vhost sweep both walk it, so neither has to know which
// services opted in.
func ServiceDomains() map[string]string {
	cfg, err := LoadGlobal()
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for name, sc := range cfg.Services {
		if sc.Domain != "" {
			out[name] = sc.Domain
		}
	}
	return out
}
