package siteops

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
)

// stubPageVhosts records the sites whose vhosts were rewritten and counts reloads.
func stubPageVhosts(t *testing.T) (rewritten *[]string, reloads *int) {
	t.Helper()
	rewritten, reloads = new([]string), new(int)
	origRegen, origReload := pageVhostsFn, pageReloadFn
	pageVhostsFn = func(s config.Site) error { *rewritten = append(*rewritten, s.Name); return nil }
	pageReloadFn = func() error { *reloads++; return nil }
	t.Cleanup(func() { pageVhostsFn, pageReloadFn = origRegen, origReload })
	return rewritten, reloads
}

func TestSetPagePlugin_SwitchesAndRewritesTheVhost(t *testing.T) {
	withTempEnv(t)
	rewritten, reloads := stubPageVhosts(t)
	if err := plugins.Scaffold("hello", filepath.Join(plugins.Dir(), "hello")); err != nil {
		t.Fatal(err)
	}
	site := &config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}

	if err := SetPagePlugin(site, "hello", true); err != nil {
		t.Fatalf("SetPagePlugin: %v", err)
	}
	got, _ := config.FindSite("shop")
	if !slices.Equal(got.PagePlugins, []string{"hello"}) {
		t.Errorf("PagePlugins = %v", got.PagePlugins)
	}
	if !slices.Equal(*rewritten, []string{"shop"}) || *reloads != 1 {
		t.Errorf("rewritten %v, reloads %d: the pages must gain the script now", *rewritten, *reloads)
	}
}

// Switching on a plugin with no page script would add nothing but a dead
// setting, so it is refused; switching off always works, even for a plugin
// already removed, so its leftover setting can be cleared.
func TestSetPagePlugin_RefusesWhatCannotBeInjected(t *testing.T) {
	withTempEnv(t)
	stubPageVhosts(t)
	site := &config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), PagePlugins: []string{"gone"}}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}
	if err := SetPagePlugin(site, "gone", true); err == nil {
		t.Error("switching on a plugin that is not installed must fail")
	}
	if err := SetPagePlugin(site, "gone", false); err != nil {
		t.Errorf("switching off must always work: %v", err)
	}
}
