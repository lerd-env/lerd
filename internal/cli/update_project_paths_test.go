package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// On Silverblue and Bazzite /home is a symlink to /var/home, so a site can be
// registered under one spelling and parked under the other. It is still one
// project, and an update must refresh its skills once.
func TestGatherProjectPaths_symlinkedParkIsOneProject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	real := filepath.Join(root, "var", "home")
	site := filepath.Join(real, "Lerd", "demo")
	if err := os.MkdirAll(site, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "home")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "demo", Domains: []string{"demo.test"}, Path: site, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveGlobal(&config.GlobalConfig{ParkedDirectories: []string{filepath.Join(link, "Lerd")}}); err != nil {
		t.Fatal(err)
	}

	if got := gatherProjectPaths(); len(got) != 1 {
		t.Errorf("gatherProjectPaths = %v, want the one project once", got)
	}
}
