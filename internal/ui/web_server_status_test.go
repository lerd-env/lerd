package ui

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The dashboard is told which server serves, so it can drop every nginx label
// and control while lerd's own runs; nginx's update flag stays nginx's alone.
func TestBuildStatus_namesTheWebServer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := buildStatus().WebServer; got != config.WebServerNginx {
		t.Errorf("default %q, want nginx", got)
	}
	cfg, _ := config.LoadGlobal()
	cfg.WebServer = config.WebServerGo
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	st := buildStatus()
	if st.WebServer != config.WebServerGo {
		t.Errorf("web_server %q, want go", st.WebServer)
	}
	if st.Nginx.UpdateAvailable {
		t.Error("an nginx update offered while lerd's own server serves")
	}
}
