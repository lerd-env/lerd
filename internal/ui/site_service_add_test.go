package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The add action only takes up a service the dashboard actually offered, so a
// crafted request cannot write an arbitrary preset into a project's .lerd.yaml.
func TestAddSiteServiceRefusesUnsuggested(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	site := &config.Site{Name: "shop", Path: dir}

	if err := addSiteService(site, "solr"); err == nil {
		t.Fatal("an unsuggested service was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); !os.IsNotExist(err) {
		t.Error(".lerd.yaml was written for a refused service")
	}
}
