package linker

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A re-link is the only way to re-check a site's framework, which is what picks
// up a definition the store published after the site was linked. It used to
// rebuild the entry from the directory alone, so the consents and the pinned
// ports the registry held went with it.
func TestResolve_relinkKeepsWhatOnlyTheRegistryKnows(t *testing.T) {
	dir := projectDir(t, "myapp", "")
	existing := config.Site{
		Name:             "myapp",
		Domains:          []string{"myapp.test"},
		Path:             dir,
		Framework:        "laravel",
		PHPVersion:       "8.1",
		ApprovedCommands: []string{"php artisan vite:watch theme-vampire"},
		WorkerPorts:      map[string]int{"vite": 5180},
		DevServerPort:    5173,
		AppURL:           "https://myapp.test",
		LANPort:          8080,
		Pinned:           true,
	}
	if err := config.AddSite(existing); err != nil {
		t.Fatal(err)
	}

	plan, err := Resolve(dir, testConfig(), CLIPolicy("myapp", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Site

	if len(got.ApprovedCommands) != 1 || got.ApprovedCommands[0] != existing.ApprovedCommands[0] {
		t.Errorf("approved commands = %v, want them carried over", got.ApprovedCommands)
	}
	if got.WorkerPorts["vite"] != 5180 {
		t.Errorf("worker port = %d, want the pinned 5180", got.WorkerPorts["vite"])
	}
	if got.DevServerPort != 5173 {
		t.Errorf("dev server port = %d, want the pinned 5173", got.DevServerPort)
	}
	if got.AppURL != existing.AppURL {
		t.Errorf("app url = %q, want the per-machine override kept", got.AppURL)
	}
	if got.LANPort != 8080 || !got.Pinned {
		t.Errorf("lan port = %d, pinned = %v, want both kept", got.LANPort, got.Pinned)
	}
}

// The directory still decides what the directory knows, or a re-link could
// never move a site onto a framework definition that has just landed.
func TestResolve_relinkStillRederivesWhatTheDirectorySays(t *testing.T) {
	dir := projectDir(t, "myapp", "php_version: \"8.4\"\n")
	if err := config.AddSite(config.Site{
		Name:       "myapp",
		Domains:    []string{"myapp.test"},
		Path:       dir,
		Framework:  "laravel",
		PHPVersion: "8.1",
		PublicDir:  "web",
	}); err != nil {
		t.Fatal(err)
	}

	plan, err := Resolve(dir, testConfig(), CLIPolicy("myapp", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Site.PHPVersion != "8.4" {
		t.Errorf("php version = %q, want the 8.4 the project pins", plan.Site.PHPVersion)
	}
	if plan.Site.Framework == "laravel" {
		t.Error("framework was carried over from the registry, so a re-link could never re-detect it")
	}
}

// A first link has no entry to carry anything from.
func TestResolve_firstLinkIsUnaffected(t *testing.T) {
	dir := projectDir(t, "fresh", "")

	plan, err := Resolve(dir, testConfig(), CLIPolicy("", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Site.ApprovedCommands) != 0 || len(plan.Site.WorkerPorts) != 0 {
		t.Errorf("a fresh link invented state: %+v", plan.Site)
	}
}

// Unlinking a site inside a parked directory tombstones its entry so the
// watcher leaves it alone. Re-linking is what the docs promise will bring it
// back, and it used to carry the tombstone forward untouched.
func TestResolve_relinkClearsTheIgnoredTombstone(t *testing.T) {
	dir := projectDir(t, "portal", "")
	if err := config.AddSite(config.Site{
		Name:    "portal",
		Domains: []string{"portal.test"},
		Path:    dir,
		Ignored: true,
	}); err != nil {
		t.Fatal(err)
	}

	plan, err := Resolve(dir, testConfig(), CLIPolicy("portal", false, nil))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Site.Ignored {
		t.Error("the entry is still ignored, so the site stays hidden after a re-link")
	}
}
