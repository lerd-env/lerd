package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// The switch is install-wide, so a refusal should name every site standing in
// the way at once. Failing on the first one turns a single decision into a
// guessing game of repeated attempts.
func TestUnsupportedSitesNamesEveryBlocker(t *testing.T) {
	sites := []config.Site{
		{Name: "shop", PHPVersion: "8.4"},
		{Name: "legacy-cms", PHPVersion: "7.4"},
		{Name: "old-api", PHPVersion: "8.0"},
		{Name: "blog", PHPVersion: "8.2"},
		// Never served by the shared FPM container, so its version is irrelevant.
		{Name: "octane", PHPVersion: "7.4", Runtime: "frankenphp"},
	}
	msg := unsupportedSitesMessage(sites)
	if msg == "" {
		t.Fatal("expected a refusal naming the legacy sites")
	}
	for _, want := range []string{"legacy-cms", "7.4", "old-api", "8.0"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message should mention %q, got: %s", want, msg)
		}
	}
	for _, unwanted := range []string{"shop", "blog", "octane"} {
		if strings.Contains(msg, unwanted) {
			t.Errorf("message should not mention %q, got: %s", unwanted, msg)
		}
	}
}

func TestUnsupportedSitesSilentWhenAllSupported(t *testing.T) {
	if msg := unsupportedSitesMessage([]config.Site{{Name: "a", PHPVersion: "8.3"}}); msg != "" {
		t.Errorf("expected no refusal, got: %s", msg)
	}
}

// Native builds are published for Apple silicon only. An Intel Mac would
// otherwise be offered the switch and then told a binary is "not installed",
// which reads as something to go and fetch rather than a platform that has
// none.
func TestNativeAvailableOnBothMacArchitectures(t *testing.T) {
	if reason := nativeUnavailableOn("darwin", "amd64"); reason != "" {
		t.Errorf("Intel Macs have builds and must be allowed, got: %s", reason)
	}

	if reason := nativeUnavailableOn("darwin", "arm64"); reason != "" {
		t.Errorf("Apple silicon must be allowed, got: %s", reason)
	}
	if reason := nativeUnavailableOn("linux", "arm64"); reason == "" {
		t.Error("linux must still be refused whatever the arch")
	}
}

// A worktree pinning a PHP the native runtime cannot serve stops the switch too,
// named by its domain, rather than being left with a vhost nothing answers.
func TestUnsupportedSitesNamesAWorktreeBlocker(t *testing.T) {
	app, old := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(old, ".lerd.yaml"), []byte("php_version: \"7.4\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := switchWorktreesFor
	t.Cleanup(func() { switchWorktreesFor = prev })
	switchWorktreesFor = func(path, domain string) ([]gitpkg.Worktree, error) {
		if path != app {
			return nil, nil
		}
		return []gitpkg.Worktree{{Path: old, Domain: "legacy." + domain}}, nil
	}
	msg := unsupportedSitesMessage([]config.Site{{Name: "app", Domains: []string{"app.test"}, Path: app, PHPVersion: "8.4"}})
	if !strings.Contains(msg, "legacy.app.test (php 7.4)") || strings.Contains(msg, "app (php 8.4)") {
		t.Errorf("refusal should name only the worktree, got: %s", msg)
	}
}
