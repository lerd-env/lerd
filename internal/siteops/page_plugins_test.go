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

func TestSetOverviewCard_OnlyForAPluginWithACard(t *testing.T) {
	withTempEnv(t)
	rewritten, _ := stubPageVhosts(t)
	if err := plugins.Scaffold("hello", filepath.Join(plugins.Dir(), "hello")); err != nil {
		t.Fatal(err)
	}
	site := &config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}
	if err := SetOverviewCard(site, "hello", true); err != nil {
		t.Fatalf("SetOverviewCard: %v", err)
	}
	got, _ := config.FindSite("shop")
	if !slices.Equal(got.OverviewCards, []string{"hello"}) || len(*rewritten) != 0 {
		t.Errorf("OverviewCards = %v, rewritten %v: a card touches no vhost", got.OverviewCards, *rewritten)
	}
	if err := SetOverviewCard(site, "gone", true); err == nil {
		t.Error("switching on a plugin that is not installed must fail")
	}
	if err := SetOverviewCard(site, "gone", false); err != nil {
		t.Errorf("switching off must always work: %v", err)
	}
}

func TestSetSiteTab_OnUntilHiddenForTheSite(t *testing.T) {
	withTempEnv(t)
	if err := plugins.Scaffold("hello", filepath.Join(plugins.Dir(), "hello")); err != nil {
		t.Fatal(err)
	}
	site := &config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}
	if err := SetSiteTab(site, "hello", false); err != nil {
		t.Fatalf("SetSiteTab off: %v", err)
	}
	if got, _ := config.FindSite("shop"); !slices.Equal(got.HiddenTabs, []string{"hello"}) {
		t.Errorf("HiddenTabs = %v, want hello hidden", got.HiddenTabs)
	}
	if err := SetSiteTab(site, "hello", true); err != nil {
		t.Fatalf("SetSiteTab on: %v", err)
	}
	if got, _ := config.FindSite("shop"); got.HiddenTabs != nil {
		t.Errorf("HiddenTabs = %v, want none after switching back on", got.HiddenTabs)
	}
	if err := SetSiteTab(site, "gone", false); err == nil {
		t.Error("hiding the tab of a plugin that is not installed must fail")
	}
	if err := SetSiteTab(site, "gone", true); err != nil {
		t.Errorf("switching on must always work so a removed plugin's setting clears: %v", err)
	}
}
