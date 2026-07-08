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
