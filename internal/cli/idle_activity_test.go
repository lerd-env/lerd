package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

func TestMatchWorktreeKey(t *testing.T) {
	wts := []gitpkg.Worktree{
		{Name: "wt", Branch: "feature-x", Path: "/home/u/Code/app-feature-x", Domain: "feature-x.app.test"},
		{Name: "wt", Branch: "bugfix/login", Path: "/home/u/Code/app-bugfix-login", Domain: "bugfix-login.app.test"},
	}

	cases := []struct {
		name string
		dir  string
		want string
	}{
		{"main checkout", "/home/u/Code/app", ""},
		{"worktree root", "/home/u/Code/app-feature-x", "app/" + config.WorktreeUnitSlug("app-feature-x")},
		{"nested in worktree", "/home/u/Code/app-feature-x/database/migrations", "app/" + config.WorktreeUnitSlug("app-feature-x")},
		{"second worktree", "/home/u/Code/app-bugfix-login/app", "app/" + config.WorktreeUnitSlug("app-bugfix-login")},
		{"sibling prefix not a match", "/home/u/Code/app-feature-x-extra", ""},
		{"trailing slash normalised", "/home/u/Code/app-feature-x/", "app/" + config.WorktreeUnitSlug("app-feature-x")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchWorktreeKey("app", filepath.Clean(tc.dir), wts); got != tc.want {
				t.Errorf("matchWorktreeKey(%q) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}

// A plugin runs its own executables, often through lerd's node or php shims in
// the site's folder, and that is not someone working on the site: it must not
// wake the site's workers and every service it sleeps on.
func TestRecordCwdActivity_ignoresAPluginsOwnRuns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	dir := filepath.Join(home, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Domains: []string{"app.test"}, Path: config.CanonicalPath(dir), PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LERD_PLUGIN_NAME", "")
	var pinged []string
	prev := siteActivityPing
	siteActivityPing = func(key string) { pinged = append(pinged, key) }
	t.Cleanup(func() { siteActivityPing = prev })

	recordCwdActivity(dir)
	if len(pinged) != 1 {
		t.Fatalf("a command in the site pinged %v, want the site once", pinged)
	}

	pinged = nil
	t.Setenv("LERD_PLUGIN_NAME", "db-browser")
	recordCwdActivity(dir)
	if len(pinged) != 0 {
		t.Errorf("a plugin's own run marked the site as used: %v", pinged)
	}
}

// A plugin running the site's own code, as the tinker plugin does through lerd
// php artisan tinker, needs the services the site's env names, so those still
// start for it; only the site's workers stay asleep.
func TestEnsureServicesForCwd_stillStartsServicesForAPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	dir := filepath.Join(home, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=lerd-mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Domains: []string{"app.test"}, Path: config.CanonicalPath(dir), PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}
	var started []string
	prev := ensureServiceRunningFn
	ensureServiceRunningFn = func(name string) error { started = append(started, name); return nil }
	t.Cleanup(func() { ensureServiceRunningFn = prev })

	t.Setenv("LERD_PLUGIN_NAME", "tinker")
	ensureServicesForCwd(dir)
	if len(started) != 1 || started[0] != "mysql" {
		t.Errorf("a plugin's run started %v, want mysql", started)
	}
}
