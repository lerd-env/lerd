package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestParseServiceRef(t *testing.T) {
	cases := []struct {
		ref     string
		name    string
		version string
	}{
		{"mysql", "mysql", ""},
		{"mysql@8.4", "mysql", "8.4"},
		{"redis@7", "redis", "7"},
	}
	for _, c := range cases {
		name, version := parseServiceRef(c.ref)
		if name != c.name || version != c.version {
			t.Errorf("parseServiceRef(%q) = (%q, %q), want (%q, %q)", c.ref, name, version, c.name, c.version)
		}
	}
}

func TestSteadWantDomains(t *testing.T) {
	got := steadWantDomains([]string{"Blog", "admin.blog"}, "test")
	want := []string{"blog.test", "admin.blog.test"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("steadWantDomains = %v, want %v", got, want)
	}
	if steadWantDomains(nil, "test") != nil {
		t.Error("steadWantDomains(nil) should be nil")
	}
}

func TestSteadStaleSites(t *testing.T) {
	sites := []config.Site{
		{Name: "kept", Path: "/srv/kept", Lerdstead: true},
		{Name: "stale", Path: "/srv/stale", Lerdstead: true},
		{Name: "manual", Path: "/srv/manual"},
		{Name: "ignored", Path: "/srv/ignored", Lerdstead: true, Ignored: true},
	}
	declared := map[string]bool{"/srv/kept": true}

	got := steadStaleSites(sites, declared)
	if len(got) != 1 || got[0].Name != "stale" {
		t.Errorf("steadStaleSites = %v, want only \"stale\"", got)
	}
}

func TestApplySteadSiteMarksProvenance(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir := filepath.Join(t.TempDir(), "blog")
	os.MkdirAll(dir, 0755)
	if err := config.AddSite(config.Site{Name: "blog", Domains: []string{"blog.test"}, Path: dir, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.GlobalConfig{}
	cfg.DNS.TLD = "test"
	// A bare entry (no overrides) must only claim the site for the file,
	// leaving domains, PHP, and HTTPS untouched.
	if err := applySteadSite(config.LerdsteadSite{Path: dir}, cfg); err != nil {
		t.Fatalf("applySteadSite: %v", err)
	}

	site, err := config.FindSite("blog")
	if err != nil {
		t.Fatal(err)
	}
	if !site.Lerdstead {
		t.Error("site was not marked as lerdstead-managed")
	}
	if site.PHPVersion != "8.4" || !reflect.DeepEqual(site.Domains, []string{"blog.test"}) || site.Secured {
		t.Errorf("bare entry changed the site: %+v", site)
	}
}

func TestSteadApplyDomainsSkipsConflictsWithoutChange(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	dir := t.TempDir()
	config.AddSite(config.Site{Name: "mysite", Domains: []string{"mysite.test"}, Path: dir})
	config.AddSite(config.Site{Name: "other", Domains: []string{"other.test"}, Path: t.TempDir()})

	cfg := &config.GlobalConfig{}
	cfg.DNS.TLD = "test"
	site, _ := config.FindSite("mysite")

	// "other" is owned elsewhere; what survives equals the current list, so
	// the converge must be a clean no-op (no vhost work, no registry write).
	entry := config.LerdsteadSite{Path: dir, Domains: []string{"mysite", "other"}}
	if err := steadApplyDomains(site, entry, cfg); err != nil {
		t.Fatalf("steadApplyDomains: %v", err)
	}
	got, _ := config.FindSite("mysite")
	if !reflect.DeepEqual(got.Domains, []string{"mysite.test"}) {
		t.Errorf("domains changed: %v", got.Domains)
	}
	other, _ := config.FindSite("other")
	if !reflect.DeepEqual(other.Domains, []string{"other.test"}) {
		t.Errorf("conflicting site was touched: %v", other.Domains)
	}
}

func TestSteadApplySecuredRespectsDNSAndAbsence(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	dir := t.TempDir()
	config.AddSite(config.Site{Name: "app", Domains: []string{"app.test"}, Path: dir})
	site, _ := config.FindSite("app")

	cfg := &config.GlobalConfig{}
	cfg.DNS.TLD = "test" // DNS.Enabled false → unmanaged

	// Absent key: leave the site alone.
	if err := steadApplySecured(site, config.LerdsteadSite{Path: dir}, cfg); err != nil {
		t.Fatalf("nil secured: %v", err)
	}
	// secured: true with lerd DNS disabled: warn and skip, not an error.
	yes := true
	if err := steadApplySecured(site, config.LerdsteadSite{Path: dir, Secured: &yes}, cfg); err != nil {
		t.Fatalf("secured with DNS off: %v", err)
	}
	if got, _ := config.FindSite("app"); got.Secured {
		t.Error("site must stay unsecured when lerd DNS is disabled")
	}
}

func TestSteadProjectServices(t *testing.T) {
	got := steadProjectServices([]string{"mysql", "rabbitmq@4"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// mysql is a default preset: referenced by bare name, no Preset field.
	if got[0].Name != "mysql" || got[0].Preset != "" {
		t.Errorf("default preset entry = %+v", got[0])
	}
	// rabbitmq is an add-on preset: Preset mirrors the name, version pinned.
	if got[1].Name != "rabbitmq" || got[1].Preset != "rabbitmq" || got[1].PresetVersion != "4" {
		t.Errorf("add-on preset entry = %+v", got[1])
	}
}
