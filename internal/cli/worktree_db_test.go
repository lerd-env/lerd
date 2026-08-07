package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// makeFakeWorktree creates a minimal main-repo + worktree fixture on disk and
// returns (sitePath, worktreeCheckoutPath).
func makeFakeWorktree(t *testing.T, branch string) (string, string) {
	t.Helper()
	siteDir := t.TempDir()
	gitDir := filepath.Join(siteDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	wtMeta := filepath.Join(gitDir, "worktrees", branch)
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(t.TempDir(), branch+"-checkout")
	if err := os.Mkdir(checkout, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return siteDir, checkout
}

// FindParentSiteForWorktree must locate the registered parent site whose
// .git/worktrees/<branch> contains the given checkout dir.
func TestFindParentSiteForWorktree_resolvesParent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sitePath, checkout := makeFakeWorktree(t, "feat-a")
	if err := config.AddSite(config.Site{
		Name:    "acme",
		Path:    sitePath,
		Domains: []string{"acme.test"},
	}); err != nil {
		t.Fatal(err)
	}

	site, branch, ok := FindParentSiteForWorktree(checkout)
	if !ok {
		t.Fatal("FindParentSiteForWorktree returned ok=false; want match")
	}
	if site.Name != "acme" {
		t.Errorf("site = %q, want %q", site.Name, "acme")
	}
	if branch != "feat-a" {
		t.Errorf("branch = %q, want %q", branch, "feat-a")
	}
}

// A directory that is not a worktree of any registered site returns ok=false.
func TestFindParentSiteForWorktree_unregisteredDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if _, _, ok := FindParentSiteForWorktree(t.TempDir()); ok {
		t.Errorf("FindParentSiteForWorktree returned ok=true for unrelated tempdir")
	}
}

// WorktreeDBName matches the projectDBName underscore convention.
func TestWorktreeDBName_underscoreSlug(t *testing.T) {
	cases := map[[2]string]string{
		{"acme", "feat-a"}:       "acme_feat_a",
		{"my_app", "feature/x"}:  "my_app_feature_x", // SiteSlug maps the slash to _ along with every other non-[a-z0-9_] char
		{"acme_app", "feat-x-y"}: "acme_app_feat_x_y",
		{"acme", "RELEASE-1"}:    "acme_release_1",
	}
	for in, want := range cases {
		if got := WorktreeDBName(in[0], in[1]); got != want {
			t.Errorf("WorktreeDBName(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
