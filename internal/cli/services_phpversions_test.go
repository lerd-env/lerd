package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A legacy-framework site (a Laravel 6 project served by the borrowed Laravel 10
// definition, so VersionGuessed is true) must keep its real detected PHP in the
// active set. activePHPVersions drives which lerd-phpXX-fpm units coreUnits
// starts, so borrowing the def's 8.1 floor here would stop the wrong FPM unit
// and leave the site without a backend after `lerd start`.
func TestActivePHPVersions_GuessedFrameworkNotClamped(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Only a Laravel 10 definition exists, and it requires PHP 8.1+.
	store := config.StoreFrameworksDir()
	if err := os.MkdirAll(store, 0755); err != nil {
		t.Fatal(err)
	}
	body := "name: laravel\nlabel: Laravel\nversion: \"10\"\npublic_dir: public\n" +
		"php:\n  min: \"8.1\"\n  max: \"8.3\"\n" +
		"detect:\n  - composer: laravel/framework\n"
	if err := os.WriteFile(filepath.Join(store, "laravel@10.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"require":{"laravel/framework":"^6.0"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Pin the real version on disk so detection is deterministic regardless of
	// which PHP versions happen to be installed on the test host.
	if err := os.WriteFile(filepath.Join(dir, ".php-version"), []byte("7.4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name: "legacy", Domains: []string{"legacy.test"}, Path: dir,
		Framework: "laravel", PHPVersion: "7.4",
	}); err != nil {
		t.Fatal(err)
	}

	active := activePHPVersions()
	if !active["7.4"] {
		t.Errorf("activePHPVersions = %v, want it to include 7.4; a guessed framework's PHP range must not clamp the site up to the borrowed 8.1 floor", active)
	}
	if active["8.1"] {
		t.Errorf("activePHPVersions = %v, must not borrow the guessed Laravel 10 def's 8.1 floor", active)
	}
}

// A paused site is served by a landing page, so it holds no PHP version, while
// an active site's worktree pinned to another version does.
func TestActivePHPVersions_SkipsPausedCountsWorktrees(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	add := func(name, version string, paused bool) string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".php-version"), []byte(version+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := config.AddSite(config.Site{
			Name: name, Domains: []string{name + ".test"}, Path: dir, PHPVersion: version, Paused: paused,
		}); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	worktree := func(site, version string) {
		wt := t.TempDir()
		meta := filepath.Join(site, ".git", "worktrees", "feat")
		if err := os.MkdirAll(meta, 0o755); err != nil {
			t.Fatal(err)
		}
		for path, body := range map[string]string{
			filepath.Join(meta, "HEAD"):     "ref: refs/heads/feat\n",
			filepath.Join(meta, "gitdir"):   filepath.Join(wt, ".git") + "\n",
			filepath.Join(wt, ".git"):       "gitdir: " + meta + "\n",
			filepath.Join(wt, ".lerd.yaml"): "php_version: \"" + version + "\"\n",
		} {
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	worktree(add("live", "8.4", false), "8.2")
	worktree(add("resting", "8.3", true), "8.1")

	active := activePHPVersions()
	for v, want := range map[string]bool{"8.4": true, "8.2": true, "8.3": false, "8.1": false} {
		if active[v] != want {
			t.Errorf("active[%s] = %v, want %v (%v)", v, active[v], want, active)
		}
	}
}
