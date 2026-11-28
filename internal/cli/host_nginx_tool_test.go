package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The dashboard's update button names the tool, and nginx is not one lerd
// downloads, so the request has to reach its own update rather than be refused.
func TestUpdateOneTool_upgradesHomebrewNginx(t *testing.T) {
	orig := updateToolFn
	t.Cleanup(func() { updateToolFn = orig })
	var got string
	updateToolFn = func(_ *pinnedTools, name string) error { got = name; return nil }
	if err := UpdateOneTool("nginx"); err != nil {
		t.Fatalf("UpdateOneTool(nginx): %v", err)
	}
	if got != "nginx" {
		t.Errorf("updated %q, want nginx", got)
	}
}

func stubHostNginxInstall(t *testing.T, haveNginx, haveBrew bool, runtime string) *[]string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.Nginx.Runtime = runtime
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	var ran []string
	prevLook, prevRun := hostNginxLookFn, brewInstallFn
	t.Cleanup(func() { hostNginxLookFn, brewInstallFn = prevLook, prevRun })
	hostNginxLookFn = func(name string) (string, bool) {
		if name == "nginx" {
			return "/opt/homebrew/bin/nginx", haveNginx
		}
		return "/opt/homebrew/bin/brew", haveBrew
	}
	brewInstallFn = func(_, formula string, _ io.Writer) error { ran = append(ran, formula); return nil }
	return &ran
}

// Moving PHP to the host takes nginx with it, so the switch installs the
// Homebrew nginx it needs rather than leave the move half done.
func TestEnsureHostNginx_installsItWhenMissing(t *testing.T) {
	ran := stubHostNginxInstall(t, false, true, "")
	ensureHostNginx(&bytes.Buffer{})
	if strings.Join(*ran, ",") != "nginx" {
		t.Errorf("brew installs = %v, want nginx", *ran)
	}
}

func TestEnsureHostNginx_leavesItAloneWhenThereOrKeptInItsContainer(t *testing.T) {
	for _, c := range []struct {
		name        string
		nginx, brew bool
		runtime     string
	}{
		{"already installed", true, true, ""},
		{"kept in its container", false, true, config.NginxRuntimeContainer},
		{"no Homebrew", false, false, ""},
	} {
		ran := stubHostNginxInstall(t, c.nginx, c.brew, c.runtime)
		ensureHostNginx(&bytes.Buffer{})
		if len(*ran) != 0 {
			t.Errorf("%s: brew installed %v", c.name, *ran)
		}
	}
}
