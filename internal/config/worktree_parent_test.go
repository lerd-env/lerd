package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWorktreeMeta(t *testing.T, sitePath, branch, checkout string) {
	t.Helper()
	wtMeta := filepath.Join(sitePath, ".git", "worktrees", branch)
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestParentSiteForWorktreeDir_resolvesParent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sitePath := filepath.Join(tmp, "acme")
	if err := os.MkdirAll(filepath.Join(sitePath, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(t.TempDir(), "feat-a-checkout")
	if err := os.Mkdir(checkout, 0755); err != nil {
		t.Fatal(err)
	}
	writeWorktreeMeta(t, sitePath, "feat-a", checkout)

	if err := AddSite(Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}

	got, ok := ParentSiteForWorktreeDir(checkout)
	if !ok {
		t.Fatal("ParentSiteForWorktreeDir returned ok=false")
	}
	if got.Name != "acme" {
		t.Errorf("Site = %q, want %q", got.Name, "acme")
	}
	if got.PHPVersion != "8.4" {
		t.Errorf("PHPVersion = %q, want %q", got.PHPVersion, "8.4")
	}
}

func TestParentSiteForWorktreeDir_unrelatedDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if _, ok := ParentSiteForWorktreeDir(t.TempDir()); ok {
		t.Errorf("returned ok=true for unrelated dir")
	}
}

// On ostree hosts /home is a symlink to /var/home: git records the checkout
// under its real path while a shell's cwd spells it through the link.
func TestParentSiteForWorktreeDir_symlinkedSpelling(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	real := filepath.Join(tmp, "var-home")
	sitePath := filepath.Join(real, "acme")
	checkout := filepath.Join(sitePath, "acme-feat-a")
	if err := os.MkdirAll(checkout, 0755); err != nil {
		t.Fatal(err)
	}
	writeWorktreeMeta(t, sitePath, "feat-a", checkout)
	link := filepath.Join(tmp, "home")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	got, ok := ParentSiteForWorktreeDir(filepath.Join(link, "acme", "acme-feat-a"))
	if !ok || got.Name != "acme" {
		t.Fatalf("ParentSiteForWorktreeDir via the symlink = %v, %v; want acme", got, ok)
	}
}

func TestWorktreeMountDir(t *testing.T) {
	cases := []struct {
		name, checkout, want string
		ok                   bool
	}{
		{"inside the site is covered by the site's mount", "/srv/shop/shop-feat", "", false},
		{"beside the site mounts the folder holding it", "/srv/shop-feat", "/srv", true},
		{"anywhere else mounts its own parent", "/data/branches/shop-x", "/data/branches", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := WorktreeMountDir("/srv/shop", c.checkout)
			if got != c.want || ok != c.ok {
				t.Errorf("WorktreeMountDir = %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
		})
	}
}
