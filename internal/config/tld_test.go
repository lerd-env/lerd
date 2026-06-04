package config

import "testing"

func TestExtractTLD(t *testing.T) {
	cases := map[string]string{
		"zotero_pro.test":   "test",
		"alice.local":       "local",
		"feature.app.local": "local",
		"bob.test.":         "test",
		"MyApp.TEST":        "test",
		"singlelabel":       "singlelabel",
		"":                  "",
		"  spaced.test  ":   "test",
	}
	for in, want := range cases {
		if got := ExtractTLD(in); got != want {
			t.Errorf("ExtractTLD(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateTLD(t *testing.T) {
	cases := []struct {
		tld        string
		dnsEnabled bool
		wantErr    bool
		wantWarn   bool
	}{
		{"test", true, false, false},
		{"lab", true, false, false},
		{"internal", true, false, false},
		{"local", true, false, true},       // allowed, warned (mDNS)
		{"local", false, false, true},      // still warned regardless of mode
		{"com", true, false, true},         // public suffix → warn
		{"dev", true, false, true},         // public suffix → warn
		{"localhost", true, true, false},   // rejected with DNS on
		{"localhost", false, false, false}, // accepted as disabled sentinel
		{"", true, true, false},            // empty → reject
		{"with.dot", true, true, false},    // multi-label → reject
		{"-bad", true, true, false},        // leading hyphen → reject
		{"bad-", true, true, false},        // trailing hyphen → reject
		{"UP", true, false, false},         // uppercase normalises to "up", valid
		{".test", true, false, false},      // leading dot stripped
	}
	for _, c := range cases {
		warn, err := ValidateTLD(c.tld, c.dnsEnabled)
		if c.wantErr != (err != nil) {
			t.Errorf("ValidateTLD(%q, %v) err = %v, wantErr = %v", c.tld, c.dnsEnabled, err, c.wantErr)
		}
		if !c.wantErr && c.wantWarn != (warn != "") {
			t.Errorf("ValidateTLD(%q, %v) warn = %q, wantWarn = %v", c.tld, c.dnsEnabled, warn, c.wantWarn)
		}
	}
}

func TestActiveTLDs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir+"/config")
	t.Setenv("XDG_DATA_HOME", dir+"/data")
	invalidateGlobalCache()
	invalidateSitesCache()

	// Default config (no file) → DNS.TLD defaults to "test".
	if got := ActiveTLDs(); len(got) != 1 || got[0] != "test" {
		t.Fatalf("ActiveTLDs() with no sites = %v, want [test]", got)
	}

	// Register sites with mixed TLDs.
	reg := &SiteRegistry{Sites: []Site{
		{Name: "alice", Domains: []string{"alice.test", "alice.local"}},
		{Name: "bob", Domains: []string{"bob.test"}},
		{Name: "carol", Domains: []string{"carol.lab"}, Ignored: true}, // ignored → excluded
	}}
	if err := SaveSites(reg); err != nil {
		t.Fatalf("SaveSites: %v", err)
	}
	invalidateSitesCache()

	got := ActiveTLDs()
	want := map[string]bool{"test": true, "local": true}
	if len(got) != len(want) {
		t.Fatalf("ActiveTLDs() = %v, want keys %v", got, want)
	}
	for _, tld := range got {
		if !want[tld] {
			t.Errorf("ActiveTLDs() returned unexpected %q (ignored site leaked?)", tld)
		}
	}
}
