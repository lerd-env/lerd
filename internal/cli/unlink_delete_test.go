package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func siteToDelete(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Path: dir, Domains: []string{"shop.test"}}); err != nil {
		t.Fatal(err)
	}
}

func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	return home
}

// Both refusals come before the unlink, so a refused delete leaves the site linked.
func TestUnlinkDeleteFilesRefusesBeforeUnlinking(t *testing.T) {
	home := isolatedHome(t)

	siteToDelete(t, home)
	err := UnlinkSiteWith("shop", UnlinkOptions{DeleteDir: true, Confirmed: true})
	if err == nil || !strings.Contains(err.Error(), "not deleting the directory") {
		t.Errorf("the home directory must be refused, got %v", err)
	}
	if _, err := config.FindSite("shop"); err != nil {
		t.Errorf("a refused delete must leave the site linked: %v", err)
	}
	if err := config.RemoveSite("shop"); err != nil {
		t.Fatal(err)
	}

	inside := filepath.Join(home, "Code", "shop")
	siteToDelete(t, inside)
	err = UnlinkSiteWith("shop", UnlinkOptions{DeleteDir: true})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("with no terminal to confirm, the delete must ask for --yes, got %v", err)
	}
	if _, err := config.FindSite("shop"); err != nil {
		t.Errorf("an unconfirmed delete must leave the site linked: %v", err)
	}
	if _, err := os.Stat(inside); err != nil {
		t.Errorf("an unconfirmed delete must leave the directory: %v", err)
	}
}
