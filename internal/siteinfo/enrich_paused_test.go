package siteinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	phpPkg "github.com/geodro/lerd/internal/php"
)

// A paused site's PHP can be uninstalled. Re-detecting it then would persist the
// next version composer.json allows, and resuming would never bring its own back.
func TestEnrichVersions_PausedSiteKeepsItsPHP(t *testing.T) {
	stubPodman(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prev := phpPkg.InstalledForRuntimeFn
	phpPkg.InstalledForRuntimeFn = func() ([]string, error) { return []string{"8.4", "8.5"}, nil }
	t.Cleanup(func() { phpPkg.InstalledForRuntimeFn = prev })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^8.2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		paused bool
		want   string
	}{{true, "8.3"}, {false, "8.5"}} {
		s := config.Site{Name: "shop", Path: dir, PHPVersion: "8.3", Paused: c.paused}
		e := &EnrichedSite{Name: "shop", Path: dir, PHPVersion: "8.3", Paused: c.paused}
		e.enrichVersions(s, nil, false)
		if e.PHPVersion != c.want {
			t.Errorf("paused=%v: PHPVersion = %q, want %q", c.paused, e.PHPVersion, c.want)
		}
	}
}
