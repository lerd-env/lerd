package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// pinnedSite registers a site on version and drops a .lerd.yaml asking for a
// different one, the shape that made these call sites diverge: link clamped the
// site to the framework's range, the project's own pin sits outside it.
func pinnedSite(t *testing.T, name, registered, projectPin string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	yaml := "php_version: \"" + projectPin + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: name, Path: dir, PHPVersion: registered}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestPHPFPMContainer_FollowsTheSiteNotTheProjectPin covers `lerd logs` with no
// target. It names the container from the version, so resolving the project's
// pin instead of the site's sends the tail at a container the site is not
// served by, and at one that need not exist at all.
func TestPHPFPMContainer_FollowsTheSiteNotTheProjectPin(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := pinnedSite(t, "app", "8.5", "8.1")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	got, err := phpFPMContainer()
	if err != nil {
		t.Fatalf("phpFPMContainer: %v", err)
	}
	if want := "lerd-php85-fpm"; got != want {
		t.Errorf("phpFPMContainer = %q, want %q (the container serving the site)", got, want)
	}
}

// TestRegenSiteOrWorktreeVhost_FollowsTheParentSite covers the vhost a dev-server
// refresh rewrites for a worktree. A worktree inherits the parent's version
// unless its own .lerd.yaml overrides it, so a .php-version file the branch
// happens to carry must not move it: doing so points the worktree's upstream at
// an FPM container the parent site never runs.
func TestRegenSiteOrWorktreeVhost_FollowsTheParentSite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	root := t.TempDir()
	sitePath := filepath.Join(root, "app")
	wtPath := filepath.Join(root, "app-feature")
	makeWorktree(t, sitePath, wtPath, "feature")
	head := filepath.Join(sitePath, ".git", "worktrees", "feature", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".php-version"), []byte("8.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	site := config.Site{
		Name:       "app",
		Path:       sitePath,
		PHPVersion: "8.5",
		Domains:    []string{"app.test"},
	}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}

	regenSiteOrWorktreeVhost(&site, wtPath)

	conf := filepath.Join(tmp, "lerd", "nginx", "conf.d", "feature.app.test.conf")
	data, err := os.ReadFile(conf)
	if err != nil {
		t.Fatalf("reading the worktree vhost: %v", err)
	}
	if !strings.Contains(string(data), "php85") {
		t.Errorf("worktree vhost does not point at the parent's PHP 8.5 upstream:\n%s", data)
	}
}
