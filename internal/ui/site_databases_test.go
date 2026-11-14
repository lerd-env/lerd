package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The site list tells plugins which databases a site points at, so a database
// browser can offer them without guessing from service names.
func TestBuildSites_CarriesTheSiteDatabases(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("db:\n  service: postgres\n  database: shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatal(err)
	}
	sites, err := buildSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || !sites[0].HasDatabase || len(sites[0].Databases) != 1 || sites[0].Databases[0] != (siteDatabase{Service: "postgres", Database: "shop"}) {
		t.Fatalf("sites = %+v, want shop with its postgres database", sites)
	}
}
