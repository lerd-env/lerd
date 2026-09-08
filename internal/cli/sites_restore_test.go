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

	err := runSitesRestore("", false)
	if err == nil || !strings.Contains(err.Error(), "no site registry backup") {
		t.Fatalf("want a no-backup error, got %v", err)
	}
	if err := runSitesRestore("", true); err == nil {
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

	err := runSitesRestore("sites-20260101-000000.000.yaml", false)
	if err == nil || !strings.Contains(err.Error(), "no such backup") {
		t.Fatalf("want a no-such-backup error, got %v", err)
	}
}
