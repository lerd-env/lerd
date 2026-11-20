package podman

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The unit carries the name of the server it runs, so nothing calls lerd's
// own web server nginx, and nginx's unit is a different one to stop.
func TestWebServerUnit_followsTheChosenServer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := WebServerUnit(); got != "lerd-nginx" {
		t.Errorf("default %q, want lerd-nginx", got)
	}
	cfg, _ := config.LoadGlobal()
	cfg.WebServer = config.WebServerGo
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	if got := WebServerUnit(); got != "lerd-web" {
		t.Errorf("go %q, want lerd-web", got)
	}
	if got := WebServerUnitFor(config.WebServerNginx); got != "lerd-nginx" {
		t.Errorf("nginx %q", got)
	}
}
