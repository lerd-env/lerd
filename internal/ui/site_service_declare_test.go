package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// linkTestSite is a registered site on a machine with the given default presets
// installed, with every lerd command stubbed and its arguments recorded.
func linkTestSite(t *testing.T, installed ...string) (*config.Site, *[]string) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	units := filepath.Join(cfg, "containers", "systemd")
	if err := os.MkdirAll(units, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range installed {
		if err := os.WriteFile(filepath.Join(units, "lerd-"+name+".container"), []byte("[Container]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	site, err := config.FindSite("shop")
	if err != nil {
		t.Fatal(err)
	}
	var ran []string
	prev := runLerdFn
	runLerdFn = func(_ string, args ...string) ([]byte, error) {
		ran = append(ran, strings.Join(args, " "))
		return nil, nil
	}
	t.Cleanup(func() { runLerdFn = prev })
	return site, &ran
}

func saveService(t *testing.T, svc *config.CustomService) {
	t.Helper()
	if svc.Image == "" && svc.External == nil {
		svc.Image = "example.test/" + svc.Name
	}
	if err := config.SaveCustomService(svc); err != nil {
		t.Fatal(err)
	}
}

func mariadbAlternate() *config.CustomService {
	return &config.CustomService{
		Name: "mariadb-12-3", Family: "mariadb", Preset: "mariadb", PresetVersion: "12.3",
		EnvVars: []string{"DB_CONNECTION=mariadb", "DB_HOST=lerd-mariadb-12-3"},
	}
}

// Declaring a service the env already reaches has to name the preset it came
// from, or a teammate's link finds no preset called mariadb-12-3.
func TestDeclareSiteServiceRecordsAVersionedAlternate(t *testing.T) {
	site, _ := linkTestSite(t)
	saveService(t, mariadbAlternate())
	if err := os.WriteFile(filepath.Join(site.Path, ".env"), []byte("DB_HOST=lerd-mariadb-12-3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := declareSiteService(site, "mariadb-12-3"); err != nil {
		t.Fatal(err)
	}
	proj, _ := config.LoadProjectConfig(site.Path)
	if len(proj.Services) != 1 || proj.Services[0].Preset != "mariadb" || proj.Services[0].PresetVersion != "12.3" {
		t.Errorf("services = %+v", proj.Services)
	}
}
