package browsercapture

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestSetDebugbar_RewritesTheSiteVhostOnChangeOnly(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site := config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	var rewritten []string
	reloads := 0
	origRegen, origReload := regenerateSiteVhostFn, nginxReloadFn
	regenerateSiteVhostFn = func(s config.Site) error {
		if !s.Debugbar {
			t.Errorf("vhost rewritten from the old registry entry")
		}
		rewritten = append(rewritten, s.Name)
		return nil
	}
	nginxReloadFn = func() error { reloads++; return nil }
	t.Cleanup(func() { regenerateSiteVhostFn, nginxReloadFn = origRegen, origReload })

	if res, err := SetDebugbar(site, true); err != nil || res.NoChange {
		t.Fatalf("SetDebugbar = %+v, %v", res, err)
	}
	updated, _ := config.FindSite("shop")
	if res, _ := SetDebugbar(*updated, true); !res.NoChange {
		t.Fatalf("second SetDebugbar = %+v, want no change", res)
	}
	if len(rewritten) != 1 || reloads != 1 {
		t.Fatalf("rewritten %v with %d reloads, want [shop] once", rewritten, reloads)
	}
}

// A new capture route moves the bar too, so it rewrites the vhost with
// capture off as long as the bar is on.
func TestSaveSite_RouteChangeRewritesForTheBar(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site := config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), Debugbar: true}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	rewrites := 0
	origRegen, origReload := regenerateSiteVhostFn, nginxReloadFn
	regenerateSiteVhostFn = func(config.Site) error { rewrites++; return nil }
	nginxReloadFn = func() error { return nil }
	t.Cleanup(func() { regenerateSiteVhostFn, nginxReloadFn = origRegen, origReload })

	s := config.BrowserCaptureSettings{Enabled: true, Console: []string{}, Network: []string{}, Route: "/__dev/capture"}
	if err := SaveSite(site, s); err != nil {
		t.Fatal(err)
	}
	if rewrites != 1 {
		t.Fatalf("%d rewrites, want 1", rewrites)
	}
}
