package podman

import "github.com/geodro/lerd/internal/config"

// WebServerUnitFor is the container and systemd unit of the named web server.
func WebServerUnitFor(kind string) string {
	if kind == config.WebServerGo {
		return "lerd-web"
	}
	return "lerd-nginx"
}

// WebServerUnit is the unit of the web server the config chooses; everything
// that starts, probes, binds or logs it uses this rather than a spelled-out
// name, so lerd's own server is never called nginx.
func WebServerUnit() string {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return WebServerUnitFor(config.WebServerNginx)
	}
	kind, _ := cfg.WebServerKind()
	return WebServerUnitFor(kind)
}
