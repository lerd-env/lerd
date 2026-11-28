package nginx

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/hostbin"
	"github.com/geodro/lerd/internal/portforward"
)

// Seams for the host-mode legs a test cannot set up for real.
var (
	hostNginxBinary = func() (string, bool) { return hostbin.Look("nginx") }
	portsForwarded  = portforward.Installed
)

// HostModeBlocker returns why nginx stays in its container, or "" when it runs
// on the host. Every leg has to hold, so a missing one never half-moves the
// stack: a host nginx without the port forward answers nothing on 80 and 443,
// and one beside a containerised site cannot resolve that site's backend.
func HostModeBlocker() string {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.NginxRuntimeMode() != config.NginxRuntimeHost {
		return "nginx is not set to run on the host"
	}
	if _, ok := hostNginxBinary(); !ok {
		return "nginx is not installed on the host: brew install nginx"
	}
	if !portsForwarded() {
		return "ports 80 and 443 are not forwarded to loopback: run lerd install"
	}
	reg, err := config.LoadSites()
	if err != nil {
		return fmt.Sprintf("reading the sites: %v", err)
	}
	for _, s := range reg.Sites {
		if s.IsFrankenPHP() || s.IsCustomContainer() || s.IsCustomFPM() {
			return fmt.Sprintf("%s runs in its own container, which only container nginx can reach", s.Name)
		}
	}
	return ""
}

// OnHost reports whether nginx runs on the host rather than in lerd-nginx.
func OnHost() bool { return HostModeBlocker() == "" }
