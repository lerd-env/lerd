package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Disabling DNS with the default TLD renames sites test -> localhost. When the
// user declines the domain rewrite, the sites keep their .test domains, but
// HTTPS still tracks DNS: they must be unsecured so the torn-down resolver
// doesn't leave an HTTPS-only vhost nginx serves unreachably. Regression guard
// for the canonical-rename decline branch, which previously only logged a note.
func TestApplyDNSTLDMigration_DeclinedDisableUnsecuresSites(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	orig := confirmTLDRewrite
	t.Cleanup(func() { confirmTLDRewrite = orig })
	confirmTLDRewrite = func(string, bool) bool { return false } // decline the rewrite

	siteDir := filepath.Join(tmp, "alpha")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name:    "alpha",
		Path:    siteDir,
		Domains: []string{"alpha.test"},
		Secured: true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	if got := applyDNSTLDMigration("test", false); got != "localhost" {
		t.Fatalf("new TLD = %q, want localhost", got)
	}

	reg, err := config.LoadSites()
	if err != nil {
		t.Fatal(err)
	}
	site := reg.Sites[0]
	if site.Secured {
		t.Errorf("declined disable must unsecure the site; Secured still true (domain %s)", site.PrimaryDomain())
	}
	if site.PrimaryDomain() != "alpha.test" {
		t.Errorf("declined rewrite must keep the .test domain, got %s", site.PrimaryDomain())
	}
	if !site.SecuredBeforeDNSOff {
		t.Error("pre-disable HTTPS state should be recorded for a lossless re-enable")
	}
}

// A declined disable advances config's TLD (test -> localhost) while the site
// stays on .test. On re-enable, prevTLD is read back as localhost, so the
// sitesWithTLD("localhost") lookup finds nothing and, before the fix, HTTPS was
// never restored and SecuredBeforeDNSOff stayed set forever (#820). The re-enable
// must restore the .test site to HTTPS. Regression guard for the full round trip.
func TestApplyDNSTLDMigration_DeclinedDisableThenEnableRestoresHTTPS(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	orig := confirmTLDRewrite
	t.Cleanup(func() { confirmTLDRewrite = orig })
	confirmTLDRewrite = func(string, bool) bool { return false } // decline the rewrite

	siteDir := filepath.Join(tmp, "alpha")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name:    "alpha",
		Path:    siteDir,
		Domains: []string{"alpha.test"},
		Secured: true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	// dns:disable, declined -> config TLD advances to localhost, site drops to http.
	if got := applyDNSTLDMigration("test", false); got != "localhost" {
		t.Fatalf("disable new TLD = %q, want localhost", got)
	}

	// dns:enable reads back the advanced TLD (localhost) as prevTLD.
	if got := applyDNSTLDMigration("localhost", true); got != "test" {
		t.Fatalf("enable new TLD = %q, want test", got)
	}

	reg, err := config.LoadSites()
	if err != nil {
		t.Fatal(err)
	}
	site := reg.Sites[0]
	if !site.Secured {
		t.Errorf("re-enable must restore HTTPS for the declined-and-stayed .test site; Secured still false")
	}
	if site.SecuredBeforeDNSOff {
		t.Errorf("SecuredBeforeDNSOff must clear once HTTPS is restored")
	}
	if site.PrimaryDomain() != "alpha.test" {
		t.Errorf("domain must stay alpha.test, got %s", site.PrimaryDomain())
	}
}

// Accepting the rewrite still migrates domains and unsecures, so the decline fix
// doesn't alter the accepted path.
func TestApplyDNSTLDMigration_AcceptedDisableMigratesAndUnsecures(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	orig := confirmTLDRewrite
	t.Cleanup(func() { confirmTLDRewrite = orig })
	confirmTLDRewrite = func(string, bool) bool { return true }

	siteDir := filepath.Join(tmp, "beta")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name:    "beta",
		Path:    siteDir,
		Domains: []string{"beta.test"},
		Secured: true,
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	applyDNSTLDMigration("test", false)

	reg, _ := config.LoadSites()
	site := reg.Sites[0]
	if site.PrimaryDomain() != "beta.localhost" || site.Secured {
		t.Fatalf("accepted disable: domain=%s secured=%v, want beta.localhost/false", site.PrimaryDomain(), site.Secured)
	}
}
