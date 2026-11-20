package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// While lerd's own web server serves, nginx is gone: the TUI names the web
// server and offers no nginx command.
func TestTUI_namesNoNginxWhileLerdServes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, _ := config.LoadGlobal()
	cfg.WebServer = config.WebServerGo
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	for _, c := range coreProcesses() {
		if strings.Contains(c.name, "nginx") || strings.Contains(c.logs.Label, "nginx") || c.logs.ID == "lerd-nginx" {
			t.Errorf("core process %+v still names nginx", c)
		}
	}
	if !slices.ContainsFunc(coreProcesses(), func(c coreProcess) bool { return c.name == "web server" && c.logs.ID == "lerd-web" }) {
		t.Error("no web server row reading lerd-web's logs")
	}
	if slices.ContainsFunc(paletteFor(), func(c string) bool { return strings.HasPrefix(c, "nginx") }) {
		t.Error("the palette still offers nginx commands")
	}
	m := &Model{}
	for _, a := range m.quickActions() {
		if strings.Contains(strings.ToLower(a.label), "nginx") {
			t.Errorf("quick action %q", a.label)
		}
	}
}
