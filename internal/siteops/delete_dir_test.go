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

func TestCheckSiteDirDeletable(t *testing.T) {
	root := t.TempDir()
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

	if err := checkSiteDirDeletable(project, roots, protected); err != nil {
		t.Errorf("a project inside a root must be deletable: %v", err)
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
		if err := checkSiteDirDeletable(path, roots, protected); err == nil {
			t.Errorf("%s (%s) must be refused", name, path)
		}
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
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
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

	if err := RemoveSiteDir("shop", project, []string{parked}); err != nil {
		t.Fatalf("RemoveSiteDir: %v", err)
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Errorf("the folder must be gone, stat err = %v", err)
	}
	if _, err := config.FindSite("shop"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("the ignored entry must be dropped, FindSite err = %v", err)
	}
}
