package nginx

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/hostbin"
	"github.com/geodro/lerd/internal/portforward"
)

// Seams for the host-mode legs a test cannot set up for real.
var (
	hostNginxBinary = func() (string, bool) { return hostbin.Look("nginx") }
	portsForwarded  = portforward.Installed
)

// HostModeBlocker returns why nginx stays in its container, or "" when it runs on
// the host. Every leg must hold: without the forward it answers nothing on 80 and
// 443, and beside a containerised site it cannot resolve that site's backend.
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
	// hostForm writes these paths into include and ssl lines unquoted.
	for _, dir := range []string{config.DataDir(), config.RunDir()} {
		if strings.ContainsAny(dir, " \t") {
			return fmt.Sprintf("%s has a space in its path, which the host's nginx config cannot carry", dir)
		}
	}
	reg, err := config.LoadSites()
	if err != nil {
		return fmt.Sprintf("reading the sites: %v", err)
	}
	for _, s := range reg.Sites {
		if needsContainerNginx(s) {
			return fmt.Sprintf("%s runs in its own container, which only container nginx can reach", s.Name)
		}
	}
	return ""
}

func needsContainerNginx(s config.Site) bool {
	return s.IsFrankenPHP() || s.IsCustomContainer() || s.IsCustomFPM()
}

// OnHost reports where nginx runs now, as Place last recorded it, not where it
// could: a registry change alone never moves it, so configs follow the nginx serving them.
func OnHost() bool { return config.NginxOnHost() }

// Place records nginx on the host when HostModeBlocker allows it and in its
// container otherwise, reporting whether that moved it. The caller rewrites the
// configs and the unit, then restarts nginx.
func Place() (bool, error) {
	want, was := HostModeBlocker() == "", OnHost()
	if want == was {
		return false, nil
	}
	return true, config.SetNginxOnHost(want)
}

// RefuseContainerSite refuses a site that only container nginx can reach while
// nginx runs on the host, rather than write a vhost the host's nginx rejects.
func RefuseContainerSite(s config.Site) error {
	if !OnHost() || !needsContainerNginx(s) {
		return nil
	}
	return fmt.Errorf("%s needs nginx in its container, and it runs on the host beside native PHP: run lerd php:runtime container first", s.Name)
}
