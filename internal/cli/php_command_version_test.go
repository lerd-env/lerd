package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Run inside a site, the version-scoped commands built for the newest PHP
// composer.json allowed instead of the one the site is served on (#2687).
func TestPhpCommandVersion(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubInstalledPHP(t, "8.3", "8.4", "8.6")

	composer := []byte(`{"require":{"php":"^8.4"}}`)
	project := func(name string) string {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "composer.json"), composer, 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	site := project("shop")
	if err := config.AddSite(config.Site{Name: "shop", Path: site, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(t.TempDir(), "shop-feature")
	makeWorktree(t, site, worktree, "feature")
	if err := config.SetWorktreePHPVersion(worktree, "8.3"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, dir, explicit, want string
	}{
		{"site root", site, "", "8.4"},
		{"site subdirectory", filepath.Join(site, "app"), "", "8.4"},
		{"worktree pin", worktree, "", "8.3"},
		{"unlinked project", project("loose"), "", "8.6"},
		{"explicit version wins", site, "8.6", "8.6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(tc.dir)
			var args []string
			if tc.explicit != "" {
				args = []string{tc.explicit}
			}
			if got, err := phpExtVersion(args); err != nil || got != tc.want {
				t.Errorf("phpExtVersion = %q, %v, want %q", got, err, tc.want)
			}
			if got, err := phpPkgVersion(tc.explicit); err != nil || got != tc.want {
				t.Errorf("phpPkgVersion = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}
