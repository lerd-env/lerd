package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// symlinkedHome lays out the ostree shape: <tmp>/home links to <tmp>/var-home,
// and the user's home is u under it. The temp dir is resolved first because
// macOS keeps it behind the /var symlink as well.
func symlinkedHome(t *testing.T) (tmp, realHome, linkHome string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realRoot := filepath.Join(tmp, "var-home")
	if err := os.MkdirAll(filepath.Join(realRoot, "u", "Projects", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, filepath.Join(tmp, "home")); err != nil {
		t.Fatal(err)
	}
	return tmp, filepath.Join(realRoot, "u"), filepath.Join(tmp, "home", "u")
}

func TestHomeSpellingsUnder(t *testing.T) {
	tmp, realHome, linkHome := symlinkedHome(t)
	linkRoot := filepath.Join(tmp, "home")

	for _, home := range []string{realHome, linkHome} {
		got := HomeSpellingsUnder(home, linkRoot)
		if !slices.Contains(got, realHome) || !slices.Contains(got, linkHome) {
			t.Errorf("HomeSpellingsUnder(%q) = %v, want both %q and %q", home, got, realHome, linkHome)
		}
	}

	plain := filepath.Join(tmp, "plain", "u")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := HomeSpellingsUnder(plain, filepath.Join(tmp, "nohome")); !slices.Equal(got, []string{plain}) {
		t.Errorf("HomeSpellingsUnder on a plain home = %v, want only %q", got, plain)
	}
}

// A path spelled through /home lies inside a home spelled through /var/home and
// the other way round, including a path that no longer exists, such as the
// folder of a worktree that has just been removed.
func TestPathWithin(t *testing.T) {
	tmp, realHome, linkHome := symlinkedHome(t)

	cases := []struct {
		p, root string
		want    bool
	}{
		{filepath.Join(linkHome, "Projects", "app"), realHome, true},
		{filepath.Join(realHome, "Projects", "app"), linkHome, true},
		{filepath.Join(linkHome, "Projects", "gone"), realHome, true},
		{filepath.Join(realHome, "Projects", "gone", "deep"), linkHome, true},
		{realHome, linkHome, true},
		{filepath.Join(tmp, "var-home", "u2"), realHome, false},
		{filepath.Join(tmp, "elsewhere"), linkHome, false},
		{"", realHome, false},
		{realHome, "", false},
	}
	for _, c := range cases {
		if got := PathWithin(c.p, c.root); got != c.want {
			t.Errorf("PathWithin(%q, %q) = %v, want %v", c.p, c.root, got, c.want)
		}
	}
}
