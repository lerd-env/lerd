package siteops

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckSiteDirDeletable(t *testing.T) {
	root := resolvedTempDir(t)
	project := filepath.Join(root, "Lerd", "shop")
	link := filepath.Join(root, "Lerd", "linked")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(project, link); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, ".local", "share", "lerd")
	roots := []string{root, filepath.Join(root, "Lerd")}
	protected := []string{data}

	if target, err := checkSiteDirDeletable(project, roots, nil, protected); err != nil || target != project {
		t.Errorf("a project inside a root must be deletable as itself, got %q %v", target, err)
	}
	for name, path := range map[string]string{
		"a root itself":                root,
		"a parked directory itself":    filepath.Join(root, "Lerd"),
		"a folder holding lerd's data": filepath.Join(root, ".local"),
		"lerd's data":                  filepath.Join(data, "sites"),
		"a folder no root covers":      t.TempDir(),
		"a symlink":                    link,
		"a missing folder":             filepath.Join(root, "Lerd", "gone"),
		"a relative path":              "Lerd/shop",
	} {
		if _, err := checkSiteDirDeletable(path, roots, nil, protected); err == nil {
			t.Errorf("%s (%s) must be refused", name, path)
		}
	}
}

// A folder above the project swapped for a symlink sends the stored path
// somewhere else, and only where it really leads may decide.
func TestCheckSiteDirRefusesAPathLeadingOutThroughASymlink(t *testing.T) {
	root, outside := resolvedTempDir(t), resolvedTempDir(t)
	project := filepath.Join(root, "Code", "shop")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "Code"), filepath.Join(outside, "Code")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "Code"), filepath.Join(root, "Code")); err != nil {
		t.Fatal(err)
	}
	if _, err := checkSiteDirDeletable(project, []string{root}, nil, nil); err == nil {
		t.Error("a path that resolves outside every root must be refused")
	}
}

// lerd mounts a linked project outside home by its own path, so that project is
// deletable even with no root above it, though never a root itself.
func TestCheckSiteDirAcceptsAMountedProjectOutsideTheRoots(t *testing.T) {
	root, elsewhere := resolvedTempDir(t), resolvedTempDir(t)
	project := filepath.Join(elsewhere, "shop")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := checkSiteDirDeletable(project, []string{root}, []string{project}, nil); err != nil {
		t.Errorf("a registered project lerd mounts must be deletable: %v", err)
	}
	if _, err := checkSiteDirDeletable(root, []string{root}, []string{root}, nil); err == nil {
		t.Error("a root registered as a project must still be refused")
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

// A repo with one commit no remote has, a worktree inside it and one beside it.
func repoWithWorktrees(t *testing.T) (project, inside, outside string) {
	t.Helper()
	root := resolvedTempDir(t)
	project = filepath.Join(root, "shop")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, project, "init", "-q", "-b", "main")
	gitIn(t, project, "config", "user.email", "t@example.com")
	gitIn(t, project, "config", "user.name", "t")
	gitIn(t, project, "config", "core.excludesFile", os.DevNull)
	if err := os.WriteFile(filepath.Join(project, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, project, "add", "README")
	gitIn(t, project, "commit", "-q", "-m", "init")
	inside = filepath.Join(project, "shop-feat")
	outside = filepath.Join(root, "shop-fix")
	gitIn(t, project, "worktree", "add", "-q", "-b", "feat", inside)
	gitIn(t, project, "worktree", "add", "-q", "-b", "fix", outside)
	return project, inside, outside
}

func TestReportSiteDirNamesWhatGoesWithIt(t *testing.T) {
	project, inside, outside := repoWithWorktrees(t)
	if err := os.WriteFile(filepath.Join(inside, "draft.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := ReportSiteDir(project, "shop.test")
	if !slices.Equal(r.InsideWorktrees, []string{inside}) {
		t.Errorf("inside worktrees = %v, want [%s]", r.InsideWorktrees, inside)
	}
	if !slices.Equal(r.OutsideWorktrees, []string{outside}) {
		t.Errorf("outside worktrees = %v, want [%s]", r.OutsideWorktrees, outside)
	}
	if !slices.Equal(r.Uncommitted, []string{inside}) {
		t.Errorf("uncommitted = %v, want only the worktree holding draft.txt", r.Uncommitted)
	}
	if r.Unpushed != 1 {
		t.Errorf("unpushed = %d, want the one commit no remote has", r.Unpushed)
	}
}

func TestReportSiteDirOutsideGitIsEmpty(t *testing.T) {
	r := ReportSiteDir(t.TempDir(), "plain.test")
	if len(r.InsideWorktrees)+len(r.OutsideWorktrees)+len(r.Uncommitted) != 0 || r.Unpushed != 0 {
		t.Errorf("a folder outside git has nothing to report, got %+v", r)
	}
}

// A parked site is only marked ignored on unlink; once its folder is gone there
// is nothing for the watcher to find, so the entry goes too.
func TestRemoveSiteDirDropsTheRegistryEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	parked := filepath.Join(home, "Lerd")
	project := filepath.Join(parked, "shop")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Path: project, Domains: []string{"shop.test"}}); err != nil {
		t.Fatal(err)
	}
	if err := config.IgnoreSite("shop"); err != nil {
		t.Fatal(err)
	}

	if err := RemoveSiteDir("shop", project, []string{parked}, nil); err != nil {
		t.Fatalf("RemoveSiteDir: %v", err)
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Errorf("the folder must be gone, stat err = %v", err)
	}
	if _, err := config.FindSite("shop"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("the ignored entry must be dropped, FindSite err = %v", err)
	}
}

// A commit made on a detached checkout belongs to no branch, and goes with the
// folder all the same.
func TestReportSiteDirCountsCommitsOnADetachedCheckout(t *testing.T) {
	project, _, _ := repoWithWorktrees(t)
	remote := resolvedTempDir(t)
	gitIn(t, remote, "init", "-q", "--bare")
	gitIn(t, project, "remote", "add", "origin", remote)
	gitIn(t, project, "push", "-q", "origin", "main", "feat", "fix")
	gitIn(t, project, "checkout", "-q", "--detach")
	gitIn(t, project, "commit", "-q", "--allow-empty", "-m", "detached")

	if r := ReportSiteDir(project, "shop.test"); r.Unpushed != 1 {
		t.Errorf("unpushed = %d, want the commit only the detached HEAD reaches", r.Unpushed)
	}
}

// A git check that fails must say so, or a clean report could hide lost work.
func TestReportSiteDirNamesTheChecksThatFailed(t *testing.T) {
	project, _, _ := repoWithWorktrees(t)
	if err := os.WriteFile(filepath.Join(project, ".git", "index"), []byte("not an index"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := ReportSiteDir(project, "shop.test")
	if !slices.Contains(r.Unchecked, project) {
		t.Errorf("unchecked = %v, want the project whose status could not be read", r.Unchecked)
	}
}

// A project holding a parked directory or a mount would take every project
// inside that folder with it, so it is refused like the folder itself.
func TestCheckSiteDirRefusesAFolderHoldingAMountRoot(t *testing.T) {
	root := resolvedTempDir(t)
	project := filepath.Join(root, "Code", "app")
	parked := filepath.Join(project, "projects")
	if err := os.MkdirAll(parked, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := checkSiteDirDeletable(project, []string{root, parked}, nil, nil); err == nil {
		t.Error("a project holding a parked directory must be refused")
	}
}

// A repository too damaged for git to recognise still holds work, so it is
// reported as unread rather than passed off as a plain folder.
func TestReportSiteDirNamesARepositoryGitCannotOpen(t *testing.T) {
	project, _, _ := repoWithWorktrees(t)
	if err := os.WriteFile(filepath.Join(project, ".git", "HEAD"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := ReportSiteDir(project, "shop.test"); !slices.Contains(r.Unchecked, project) {
		t.Errorf("unchecked = %v, want the repository git could not open", r.Unchecked)
	}
}
