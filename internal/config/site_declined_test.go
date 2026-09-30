package config

import "testing"

// A service taken off a site stays declined across registry saves until it is
// added back, and lifting it leaves the other declines alone.
func TestSetSiteServiceDeclinedPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := AddSite(Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	for _, svc := range []string{"redis", "meilisearch", "redis"} {
		if err := SetSiteServiceDeclined("shop", svc, true); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := FindSite("shop")
	if len(s.DeclinedServices) != 2 || !s.DeclinesService("redis") || !s.DeclinesService("meilisearch") {
		t.Fatalf("declined = %v, want redis and meilisearch once each", s.DeclinedServices)
	}
	if err := SetSiteServiceDeclined("shop", "redis", false); err != nil {
		t.Fatal(err)
	}
	s, _ = FindSite("shop")
	if s.DeclinesService("redis") || !s.DeclinesService("meilisearch") {
		t.Errorf("declined = %v after lifting redis, want only meilisearch", s.DeclinedServices)
	}
	if err := SetSiteServiceDeclined("nosuch", "redis", false); err != nil {
		t.Errorf("lifting on an unregistered site: %v, want no-op", err)
	}
	if err := SetSiteServiceDeclined("nosuch", "redis", true); err == nil {
		t.Error("declining on an unregistered site succeeded")
	}
}
