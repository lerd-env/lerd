package config

import "testing"

// The port is claimed once, kept across claims and stale AddSite copies, and
// never handed to two sites.
func TestClaimFrankenPHPHostPort(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := Site{Name: "a", Domains: []string{"a.test"}, Path: "/srv/a", Runtime: "frankenphp"}
	b := Site{Name: "b", Domains: []string{"b.test"}, Path: "/srv/b", Runtime: "frankenphp"}
	for _, s := range []Site{a, b} {
		if err := AddSite(s); err != nil {
			t.Fatal(err)
		}
	}
	first := func(taken map[int]bool) int {
		for p := 18000; ; p++ {
			if !taken[p] {
				return p
			}
		}
	}
	pa, err := ClaimFrankenPHPHostPort("a", first)
	if err != nil || pa != 18000 {
		t.Fatalf("claim a = %d, %v; want 18000", pa, err)
	}
	pb, err := ClaimFrankenPHPHostPort("b", first)
	if err != nil || pb != 18001 {
		t.Fatalf("claim b = %d, %v; want 18001, not a's port", pb, err)
	}
	if again, _ := ClaimFrankenPHPHostPort("a", func(map[int]bool) int { t.Fatal("picked again"); return 0 }); again != pa {
		t.Errorf("second claim of a = %d, want %d", again, pa)
	}
	// A caller holding a copy loaded before the claim must not wipe it.
	if err := AddSite(a); err != nil {
		t.Fatal(err)
	}
	if s, _ := FindSite("a"); s == nil || s.FrankenPHPHostPort != pa {
		t.Errorf("AddSite with a stale copy dropped the port: %+v", s)
	}
	if _, err := ClaimFrankenPHPHostPort("missing", first); err == nil {
		t.Error("claimed a port for a site that is not registered")
	}
}

// A port another site holds for a stopped dev server or worker is never claimed.
func TestClaimFrankenPHPHostPort_skipsPortsOtherSitesHold(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	proxy := Site{Name: "proxy", Domains: []string{"proxy.test"}, Path: "/srv/proxy", HostPort: 18000, WorktreeHostPorts: map[string]int{"wt": 18001}, WorkerPorts: map[string]int{"reverb": 18002}}
	franken := Site{Name: "franken", Domains: []string{"franken.test"}, Path: "/srv/franken", Runtime: "frankenphp"}
	for _, s := range []Site{proxy, franken} {
		if err := AddSite(s); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ClaimFrankenPHPHostPort("franken", func(taken map[int]bool) int {
		for p := 18000; ; p++ {
			if !taken[p] {
				return p
			}
		}
	})
	if err != nil || got != 18003 {
		t.Errorf("claim = %d, %v; want 18003, past the ports the proxy holds", got, err)
	}
}
