package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestPHPVersionForDir_PrefersTheRegisteredSite pins the rule that the CLI runs on
// the same PHP as the site's FPM container. lerd link clamps to the framework's
// supported range, so a .php-version outside it would otherwise send composer,
// console and php:shell into a different container than the one serving the site.
func TestPHPVersionForDir_PrefersTheRegisteredSite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// The project asks for 8.1, but the framework clamp registered the site on 8.5.
	if err := os.WriteFile(filepath.Join(dir, ".php-version"), []byte("8.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: dir, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	got, err := phpVersionForDir(dir)
	if err != nil {
		t.Fatalf("phpVersionForDir: %v", err)
	}
	if got != "8.5" {
		t.Errorf("phpVersionForDir = %q, want %q (the version the site's FPM runs)", got, "8.5")
	}
}

// makeWorktree wires up the .git bookkeeping git itself writes for a worktree:
// a .git file in the checkout and a gitdir pointer under the parent's
// .git/worktrees, which is what ParentSiteForWorktreeDir reads.
func makeWorktree(t *testing.T, sitePath, wtPath, branch string) {
	t.Helper()
	book := filepath.Join(sitePath, ".git", "worktrees", branch)
	if err := os.MkdirAll(book, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(book, "gitdir"), []byte(filepath.Join(wtPath, ".git")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".git"), []byte("gitdir: "+book+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestPHPVersionForDir_NestedWorktreeOverride covers a worktree checked out inside
// its parent site's directory. The site path matches by prefix, so without the
// worktree check the parent's registry version wins and the CLI runs a different
// PHP than the worktree's own vhost points at.
func TestPHPVersionForDir_NestedWorktreeOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	site := filepath.Join(t.TempDir(), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")

	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}
	if err := config.SetWorktreePHPVersion(wt, "8.3"); err != nil {
		t.Fatal(err)
	}

	got, err := phpVersionForDir(wt)
	if err != nil {
		t.Fatalf("phpVersionForDir: %v", err)
	}
	if got != "8.3" {
		t.Errorf("phpVersionForDir = %q, want %q (the worktree's own pin)", got, "8.3")
	}
}

// TestPHPVersionForDir_WorktreeSubdirOverride resolves from a directory inside the
// worktree, since commands run from anywhere in the checkout.
func TestPHPVersionForDir_WorktreeSubdirOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	site := filepath.Join(t.TempDir(), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")

	sub := filepath.Join(wt, "app", "Http")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}
	if err := config.SetWorktreePHPVersion(wt, "8.3"); err != nil {
		t.Fatal(err)
	}

	got, err := phpVersionForDir(sub)
	if err != nil {
		t.Fatalf("phpVersionForDir: %v", err)
	}
	if got != "8.3" {
		t.Errorf("phpVersionForDir = %q, want %q (the worktree's own pin)", got, "8.3")
	}
}

// TestPHPVersionForDir_WorktreeInheritsParent keeps inherit-by-default working: a
// worktree with no override of its own runs the parent site's version.
func TestPHPVersionForDir_WorktreeInheritsParent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	site := filepath.Join(t.TempDir(), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")

	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	got, err := phpVersionForDir(wt)
	if err != nil {
		t.Fatalf("phpVersionForDir: %v", err)
	}
	if got != "8.5" {
		t.Errorf("phpVersionForDir = %q, want %q (inherited from the parent site)", got, "8.5")
	}
}

// TestPHPVersionForDir_FallsBackToDetection keeps the unlinked case working: a
// directory that is not a registered site still resolves from the project itself.
func TestPHPVersionForDir_FallsBackToDetection(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := filepath.Join(t.TempDir(), "loose")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".php-version"), []byte("8.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := phpVersionForDir(dir)
	if err != nil {
		t.Fatalf("phpVersionForDir: %v", err)
	}
	if got != "8.1" {
		t.Errorf("phpVersionForDir = %q, want %q", got, "8.1")
	}
}

// TestFPMContainerForDir_SiblingWorktreeOfCustomFPMSite covers a worktree checked
// out beside its project rather than inside it. It matches no registered site
// path, so without resolving the worktree's parent the exec falls back to the
// shared container and loses the per-site image the site is actually served from.
func TestFPMContainerForDir_SiblingWorktreeOfCustomFPMSite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	root := t.TempDir()
	site := filepath.Join(root, "app")
	wt := filepath.Join(root, "app-feature")
	makeWorktree(t, site, wt, "feature")

	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.4", Runtime: "fpm-custom"}); err != nil {
		t.Fatal(err)
	}

	got := fpmContainerForDir(wt, "8.4")
	if want := "lerd-cfpm-app"; got != want {
		t.Errorf("fpmContainerForDir = %q, want %q (the site's own image, which its vhost uses)", got, want)
	}
}

// TestFPMContainerForDir_SiblingWorktreeOfSharedFPMSite keeps the ordinary case
// on the shared per-version container.
func TestFPMContainerForDir_SiblingWorktreeOfSharedFPMSite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	root := t.TempDir()
	site := filepath.Join(root, "app")
	wt := filepath.Join(root, "app-feature")
	makeWorktree(t, site, wt, "feature")

	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}

	got := fpmContainerForDir(wt, "8.4")
	if want := "lerd-php84-fpm"; got != want {
		t.Errorf("fpmContainerForDir = %q, want %q", got, want)
	}
}
