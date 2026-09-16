package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A registry that has never been rewritten has nothing to restore, and the
// command has to say so rather than report a successful restore of nothing.
func TestSitesRestoreWithoutBackups(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := runSitesRestore("", false, false)
	if err == nil || !strings.Contains(err.Error(), "no site registry backup") {
		t.Fatalf("want a no-backup error, got %v", err)
	}
	if err := runSitesRestore("", true, false); err == nil {
		t.Error("--list should report the same when there is nothing to list")
	}
}

func TestSitesRestoreRejectsUnknownBackup(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{{
		Name: "one", Domains: []string{"one.test"}, Path: "/srv/one",
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSites(&config.SiteRegistry{}); err != nil {
		t.Fatal(err)
	}

	err := runSitesRestore("sites-20260101-000000.000.yaml", false, false)
	if err == nil || !strings.Contains(err.Error(), "no such backup") {
		t.Fatalf("want a no-such-backup error, got %v", err)
	}
}

// The command rewrote live state from a file the user never saw. Without a tty
// there is nobody to confirm, so it has to refuse rather than proceed.
func TestSitesRestoreRefusesWithoutForceWhenNotInteractive(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{{
		Name: "demo", Domains: []string{"demo.test"}, Path: "/srv/demo", PHPVersion: "8.3",
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{{
		Name: "demo", Domains: []string{"demo.test"}, Path: "/srv/demo", PHPVersion: "8.5",
	}}}); err != nil {
		t.Fatal(err)
	}

	err := runSitesRestore("", false, false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("want a refusal pointing at --force, got %v", err)
	}
	// The registry must be untouched by a refused restore.
	reg, loadErr := config.LoadSites()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if reg.Sites[0].PHPVersion != "8.5" {
		t.Errorf("a refused restore changed the registry: PHP is %q, want 8.5", reg.Sites[0].PHPVersion)
	}
}
