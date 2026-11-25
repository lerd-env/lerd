package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// Pausing the last site on a PHP version stops that version's FPM, while one
// another active site runs on and the default stay up.
func TestPauseSite_stopsAnFPMNothingElseUses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubResumePHP(t, nil, true)

	if err := os.MkdirAll(config.QuadletDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := &statefulLifecycle{active: map[string]bool{}}
	for _, u := range []string{"lerd-php83-fpm", "lerd-php84-fpm", "lerd-php85-fpm"} {
		if err := os.WriteFile(filepath.Join(config.QuadletDir(), u+".container"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		fake.active[u] = true
	}
	podman.UnitLifecycle = fake
	t.Cleanup(func() { podman.UnitLifecycle = nil })

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.PHP.DefaultVersion = "8.5"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]string{"old": "8.3", "live": "8.4"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".php-version"), []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := config.AddSite(config.Site{Name: name, Path: dir, Domains: []string{name + ".test"}, PHPVersion: v}); err != nil {
			t.Fatal(err)
		}
	}

	if err := PauseSite("old"); err != nil {
		t.Fatal(err)
	}
	for unit, want := range map[string]bool{"lerd-php83-fpm": false, "lerd-php84-fpm": true, "lerd-php85-fpm": true} {
		if fake.active[unit] != want {
			t.Errorf("%s active = %v, want %v", unit, fake.active[unit], want)
		}
	}
}
