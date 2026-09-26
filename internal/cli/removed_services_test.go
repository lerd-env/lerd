package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// The install pass restoring a site's inline service must leave it installed,
// not as a unit with no definition that runs while lerd calls it not installed.
func TestRestoreInlineService_savesTheDefinitionWithTheUnit(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	prev := podman.DaemonReloadFn
	podman.DaemonReloadFn = func() error { return nil }
	t.Cleanup(func() { podman.DaemonReloadFn = prev })

	inline := &config.CustomService{Image: "docker.io/phpmyadmin/phpmyadmin:latest", Ports: []string{"127.0.0.1:18081:80"}}
	if err := restoreInlineService("phpmyadmin", inline); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadCustomService("phpmyadmin"); err != nil {
		t.Fatalf("definition not saved: %v", err)
	}
	if !podman.QuadletInstalled("lerd-phpmyadmin") {
		t.Fatal("unit not written")
	}
}

func TestLinkApplyServices_skipsARemovedService(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_ = config.SetServiceRemoved("phpmyadmin", true)
	proj := &config.ProjectConfig{Services: []config.ProjectService{{
		Name:   "phpmyadmin",
		Custom: &config.CustomService{Image: "docker.io/phpmyadmin/phpmyadmin:latest"},
	}}}

	out := captureStdout(t, func() {
		if err := linkApplyServices(t.TempDir(), config.Site{Name: "drupal"}, proj); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "Skipped service phpmyadmin (removed") {
		t.Fatalf("output %q, want the skip explained", out)
	}
	if _, err := config.LoadCustomService("phpmyadmin"); err == nil {
		t.Fatal("link reinstalled a removed service")
	}
}
