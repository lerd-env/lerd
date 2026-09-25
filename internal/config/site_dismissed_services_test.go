package config

import (
	"slices"
	"testing"
)

// A dismissed suggestion is this machine's choice about one site, kept in the
// registry once however often it is dismissed, and survives a reload.
func TestDismissSiteService(t *testing.T) {
	setDataDir(t)
	if err := AddSite(Site{Name: "shop", Domains: []string{"shop.test"}, Path: "/srv/shop"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := DismissSiteService("shop", "redis"); err != nil {
			t.Fatal(err)
		}
	}
	site, err := FindSite("shop")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(site.DismissedServices, []string{"redis"}) {
		t.Errorf("dismissed = %v, want [redis]", site.DismissedServices)
	}
	if err := DismissSiteService("nosuch", "redis"); err == nil {
		t.Error("dismissing on an unknown site should fail")
	}
}
