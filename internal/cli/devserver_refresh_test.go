package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// devServerSite registers a site whose framework declares a host worker that
// starts vite, with the project shaped the way the mechanism requires, and
// returns the project directory.
func devServerSite(t *testing.T, secured bool) string {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_DATA_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "lerd", "frameworks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveFramework(&config.Framework{
		Name: "testfw",
		Workers: map[string]config.FrameworkWorker{
			"vite":  {Command: "npm run dev", Host: true},
			"queue": {Command: "php artisan queue:work"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	dir := gitRepo(t, "/node_modules\n")
	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "vite"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name:      "myapp",
		Path:      dir,
		Domains:   []string{"myapp.test"},
		Framework: "testfw",
		Secured:   secured,
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// addWorktree checks out a branch of the site as a git worktree beside it, with
// the same project shape (a worktree seeds node_modules from its parent), and
// returns its path.
func addWorktree(t *testing.T, sitePath, branch string) string {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = sitePath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("add", "-A")
	run("commit", "-qm", "initial")
	path := filepath.Join(filepath.Dir(sitePath), filepath.Base(sitePath)+"-"+branch)
	run("worktree", "add", "-q", "-b", branch, path)
	if err := os.MkdirAll(filepath.Join(path, "node_modules", "vite"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// Unsecuring a site leaves its dev server advertising https, so every asset URL
// on the page names a scheme nothing serves any more and the page arrives
// unstyled. The config has to be rewritten and the server put back on it.
func TestRefreshDevServersRestartsAfterTheSchemeChanges(t *testing.T) {
	dir := devServerSite(t, true)
	tool := config.DevServerToolInstalled(dir)
	if tool == nil {
		t.Fatal("vite not resolved from node_modules")
	}
	if _, err := writeDevServerWrapper(dir, tool, securedAddr("myapp.test")); err != nil {
		t.Fatal(err)
	}

	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}
	site.Secured = false
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}

	fake := &fakeUnitLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	RefreshDevServers(site)

	body, err := os.ReadFile(filepath.Join(dir, tool.WrapperPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"http://myapp.test"`) {
		t.Errorf("generated config still does not carry the site's plain origin:\n%s", body)
	}
	if fake.restartedUnit != "lerd-vite-myapp" {
		t.Errorf("restarted unit = %q, want lerd-vite-myapp", fake.restartedUnit)
	}
}

// Adding a domain has to reach the running server too: it answers only for the
// hosts it was started with and refuses the new one outright.
func TestRefreshDevServersFollowsAnAddedDomain(t *testing.T) {
	dir := devServerSite(t, false)
	tool := config.DevServerToolInstalled(dir)
	if tool == nil {
		t.Fatal("vite not resolved from node_modules")
	}
	plain := devServerAddrFor(false, "myapp.test", []string{"myapp.test"})
	if _, err := writeDevServerWrapper(dir, tool, plain); err != nil {
		t.Fatal(err)
	}

	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}
	site.Domains = append(site.Domains, "alias.test")
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}

	fake := &fakeUnitLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	RefreshDevServers(site)

	body, err := os.ReadFile(filepath.Join(dir, tool.WrapperPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `".alias.test"`) {
		t.Errorf("generated config does not answer for the new domain:\n%s", body)
	}
	if !strings.Contains(string(body), `"http://alias.test"`) {
		t.Errorf("generated config does not let a page on the new domain fetch assets:\n%s", body)
	}
	if fake.restartedUnit != "lerd-vite-myapp" {
		t.Errorf("restarted unit = %q, want lerd-vite-myapp", fake.restartedUnit)
	}
}

// A worktree fronts its own subdomain of the site, so it has to be realigned
// against that rather than against the parent's domain.
func TestRefreshDevServersFollowsAWorktreeSubdomain(t *testing.T) {
	dir := devServerSite(t, false)
	wt := addWorktree(t, dir, "feature")
	tool := config.DevServerToolInstalled(wt)
	if tool == nil {
		t.Fatal("vite not resolved from the worktree's node_modules")
	}
	stale := devServerAddrFor(false, "feature.old.test", []string{"feature.old.test"})
	if _, err := writeDevServerWrapper(wt, tool, stale); err != nil {
		t.Fatal(err)
	}

	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}
	site.WorktreeDevPorts = map[string]int{filepath.Base(wt): 5174}
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}

	fake := &fakeUnitLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	RefreshDevServers(site)

	body, err := os.ReadFile(filepath.Join(wt, tool.WrapperPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"http://feature.myapp.test"`) {
		t.Errorf("worktree config does not carry its own subdomain:\n%s", body)
	}
	if fake.restartedUnit != "lerd-vite-myapp-"+config.WorktreeUnitSlug(filepath.Base(wt)) {
		t.Errorf("restarted unit = %q, want the worktree's own unit", fake.restartedUnit)
	}
}

// Nothing moved means nothing is bounced: the refresh runs from every path that
// regenerates a vhost, including ones that leave the addresses exactly as they
// were.
func TestRefreshDevServersLeavesAnUnchangedSiteRunning(t *testing.T) {
	dir := devServerSite(t, true)
	tool := config.DevServerToolInstalled(dir)
	if tool == nil {
		t.Fatal("vite not resolved from node_modules")
	}
	if _, err := writeDevServerWrapper(dir, tool, securedAddr("myapp.test")); err != nil {
		t.Fatal(err)
	}
	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeUnitLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	RefreshDevServers(site)

	if fake.restartedUnit != "" {
		t.Errorf("restarted unit = %q, want no restart", fake.restartedUnit)
	}
}
