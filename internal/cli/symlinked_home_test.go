package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// linkedProject lays out /home → /var/home in a temp dir with a project under
// it, and returns the project under its resolved and its linked spelling.
func linkedProject(t *testing.T) (realPath, linkPath string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realRoot := filepath.Join(tmp, "var-home")
	if err := os.MkdirAll(filepath.Join(realRoot, "u", "app", "app-feat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, filepath.Join(tmp, "home")); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(realRoot, "u", "app"), filepath.Join(tmp, "home", "u", "app")
}

// A site registered under /var/home is the same site when a shell in /home
// starts one of its workers: it gets the site's unit, not a worktree's, so the
// site does not end up with two queue workers.
func TestWorkerUnitNameFromTheOtherHomeSpelling(t *testing.T) {
	realPath, linkPath := linkedProject(t)
	registerSite(t, "app", realPath)

	if got := WorkerUnitName("app", linkPath, "queue"); got != "lerd-queue-app" {
		t.Errorf("WorkerUnitName from the linked spelling = %q, want lerd-queue-app", got)
	}
	if got := WorkerUnitName("app", filepath.Join(linkPath, "app-feat"), "queue"); got != "lerd-queue-app-app-feat" {
		t.Errorf("a real worktree still gets its own unit, got %q", got)
	}
}

// lerd env finds the APP_URL of the site it runs in whichever spelling of home
// the shell is in.
func TestSiteURLFromTheOtherHomeSpelling(t *testing.T) {
	realPath, linkPath := linkedProject(t)
	registerSite(t, "app", realPath)

	if got := siteURL(linkPath); got != "http://app.test" {
		t.Errorf("siteURL(linked spelling) = %q, want http://app.test", got)
	}
}

func TestQueueSiteNameFromTheOtherHomeSpelling(t *testing.T) {
	realPath, linkPath := linkedProject(t)
	registerSite(t, "shop", realPath)

	if got, err := queueSiteName(linkPath); err != nil || got != "shop" {
		t.Errorf("queueSiteName(linked spelling) = %q, %v, want shop", got, err)
	}
}

func TestMatchWorktreeKeyFromTheOtherHomeSpelling(t *testing.T) {
	realPath, linkPath := linkedProject(t)
	wts := []gitpkg.Worktree{{Path: filepath.Join(realPath, "app-feat")}}

	want := "app/" + config.WorktreeUnitSlug("app-feat")
	if got := matchWorktreeKey("app", filepath.Join(linkPath, "app-feat"), wts); got != want {
		t.Errorf("matchWorktreeKey(linked spelling) = %q, want %q", got, want)
	}
}
