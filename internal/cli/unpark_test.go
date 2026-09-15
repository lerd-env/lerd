package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Unpark used to drop each site's vhost and registry entry by hand, which left
// every site's workers running: a queue worker whose project is no longer a site
// restart-loops for as long as the machine is up, and unrelated commands later
// restart it. The shared teardown is what stops those workers, so what matters
// here is that unpark routes every site it removes through it, and nothing else.
func TestUnpark_stopsTheWorkersOfEverySiteItRemoves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	// Created before it is canonicalised, and canonicalised before it is used:
	// CanonicalPath resolves symlinks only for a path that exists, and on macOS
	// t.TempDir() hands back /var/folders/... which is a symlink to
	// /private/var/folders/.... Canonicalising a directory that is not there yet
	// falls back to Clean, leaving a prefix that matches no stored site, and
	// unpark then walks past every one of them.
	parked := filepath.Join(home, "Projects")
	if err := os.MkdirAll(parked, 0755); err != nil {
		t.Fatal(err)
	}
	parked = config.CanonicalPath(parked)
	// The real teardown reaches podman and nginx; only the routing is under test,
	// so both seams are stubbed and the worker hook is what we watch.
	var stopped []string
	prevTeardown, prevFinish := teardownSiteFn, finishSiteRemovalFn
	teardownSiteFn = func(s *config.Site, parked []string) {
		stopped = append(stopped, s.Name)
		_ = config.RemoveSite(s.Name)
	}
	finishSiteRemovalFn = func() error { return nil }
	t.Cleanup(func() { teardownSiteFn, finishSiteRemovalFn = prevTeardown, prevFinish })

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.ParkedDirectories = []string{parked}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(parked, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := config.AddSite(config.Site{
			Name: name, Domains: []string{name + ".test"}, Path: config.CanonicalPath(dir), PHPVersion: "8.5",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A site outside the parked directory must be left entirely alone.
	outside := filepath.Join(home, "elsewhere", "gamma")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name: "gamma", Domains: []string{"gamma.test"}, Path: config.CanonicalPath(outside), PHPVersion: "8.5",
	}); err != nil {
		t.Fatal(err)
	}

	if err := runUnpark(nil, []string{parked}); err != nil {
		t.Fatalf("runUnpark: %v", err)
	}

	if len(stopped) != 2 {
		t.Errorf("workers stopped for %v, want both parked sites", stopped)
	}
	for _, want := range []string{"alpha", "beta"} {
		found := false
		for _, got := range stopped {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("workers were not stopped for %s", want)
		}
	}
	for _, name := range stopped {
		if name == "gamma" {
			t.Error("a site outside the parked directory was torn down")
		}
	}

	reg, err := config.LoadSites()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range reg.Sites {
		if s.Name == "alpha" || s.Name == "beta" {
			t.Errorf("%s is still registered after unpark", s.Name)
		}
	}
	gammaStill := false
	for _, s := range reg.Sites {
		if s.Name == "gamma" {
			gammaStill = true
		}
	}
	if !gammaStill {
		t.Error("gamma was removed, but it lives outside the parked directory")
	}
}
