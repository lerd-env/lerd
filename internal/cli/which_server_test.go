package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/webcustom"
)

// lerd which names the file that configures the server actually serving: the
// nginx vhost, or lerd's own web server's settings file.
func TestWhichServerRow_followsTheChosenServer(t *testing.T) {
	stubWebServerSwitch(t, false)
	site := config.Site{Name: "app", Domains: []string{"app.test"}}
	if label, _ := whichServerRow(site); label != "Nginx config" {
		t.Errorf("nginx label %q", label)
	}
	cfg, _ := config.LoadGlobal()
	cfg.WebServer = config.WebServerGo
	config.SaveGlobal(cfg)
	if label, path := whichServerRow(site); label != "Web server settings" || path != webcustom.SitePath("app.test") {
		t.Errorf("go row %q %q", label, path)
	}
}
