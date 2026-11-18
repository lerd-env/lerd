package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The forms docs/features/git-worktrees.md documents, none of which carry a
// path. Every one of them died on git's usage message before this.
func TestDeriveWorktreeAddArgs_fillsInTheDocumentedNewBranchForm(t *testing.T) {
	useWorktreeLocation(t, "")
	site := t.TempDir()
	want := mustCheckoutPath(t, site, "feat-x")

	got := mustDerive(t, site, []string{"-b", "feat-x"})

	if !reflect.DeepEqual(got, []string{"-b", "feat-x", want}) {
		t.Errorf("args = %v, want -b feat-x %s", got, want)
	}
}

// git's form is `-b <branch> <path> [<start-point>]`, so the path goes between
// the branch and the start point rather than on the end.
func TestDeriveWorktreeAddArgs_putsThePathBeforeAStartPoint(t *testing.T) {
	useWorktreeLocation(t, "")
	site := t.TempDir()
	want := mustCheckoutPath(t, site, "feat-x")

	got := mustDerive(t, site, []string{"--track", "-b", "feat-x", "origin/feat"})

	if !reflect.DeepEqual(got, []string{"--track", "-b", "feat-x", want, "origin/feat"}) {
		t.Errorf("args = %v, want the path before origin/feat", got)
	}
}

// An existing branch takes git's `<path> <branch>` order, the same shape the
// dashboard builds.
func TestDeriveWorktreeAddArgs_existingBranchGetsPathThenBranch(t *testing.T) {
	useWorktreeLocation(t, "")
	site := t.TempDir()
	want := mustCheckoutPath(t, site, "feature/auth")

	got := mustDerive(t, site, []string{"feature/auth"})

	if !reflect.DeepEqual(got, []string{want, "feature/auth"}) {
		t.Errorf("args = %v, want %s feature/auth", got, want)
	}
}

// A branch with a slash must not read as a path, or the fix would skip exactly
// the form it exists to repair.
func TestDeriveWorktreeAddArgs_aSlashInABranchIsNotAPath(t *testing.T) {
	if hasWorktreePathArg([]string{"feature/auth"}) {
		t.Error("feature/auth read as a path")
	}
	if !hasWorktreePathArg([]string{"-b", "x", "../demo-x"}) {
		t.Error("../demo-x should read as a path")
	}
}

// Anyone already typing a path keeps the behaviour they have.
func TestDeriveWorktreeAddArgs_leavesAnExplicitPathAlone(t *testing.T) {
	useWorktreeLocation(t, "")
	site := t.TempDir()
	in := []string{"-b", "feat-x", "../demo-feat-x"}

	if got := mustDerive(t, site, in); !reflect.DeepEqual(got, in) {
		t.Errorf("args = %v, want them untouched", got)
	}
}

// git's own shape puts the path first, and a bare folder name there has no path
// marker, so -b must not add a second path behind it.
func TestDeriveWorktreeAddArgs_leavesABareFolderBeforeTheBranchFlagAlone(t *testing.T) {
	useWorktreeLocation(t, "")
	site := t.TempDir()
	in := []string{"demo-feat-x", "-b", "feat-x"}

	if got := mustDerive(t, site, in); !reflect.DeepEqual(got, in) {
		t.Errorf("args = %v, want them untouched", got)
	}
}

// Two positionals with no -b could be a path and a commit-ish, or a branch and
// a commit-ish. Guessing is how a wrapper checks out the wrong thing.
func TestDeriveWorktreeAddArgs_passesThroughWhenAmbiguous(t *testing.T) {
	useWorktreeLocation(t, "")
	site := t.TempDir()
	in := []string{"--detach", "feat-x", "abc1234"}

	if got := mustDerive(t, site, in); !reflect.DeepEqual(got, in) {
		t.Errorf("args = %v, want them untouched", got)
	}
}

// The derived path is a child of the site, which is the layout the dashboard
// produces and the one .git/info/exclude is written for.
func TestDeriveWorktreeAddArgs_derivesAChildOfTheSite(t *testing.T) {
	useWorktreeLocation(t, "")
	site := t.TempDir()
	got := mustDerive(t, site, []string{"-b", "feat-x"})

	path := got[len(got)-1]
	if filepath.Dir(path) != site {
		t.Errorf("path %s is not a child of %s", path, site)
	}
	if !strings.HasSuffix(path, filepath.Base(site)+"-feat-x") {
		t.Errorf("path %s does not follow <base>-<branch>", path)
	}
}

func mustCheckoutPath(t *testing.T, site, branch string) string {
	t.Helper()
	p, err := WorktreeCheckoutPath(site, branch)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func mustDerive(t *testing.T, site string, args []string) []string {
	t.Helper()
	out, err := DeriveWorktreeAddArgs(site, args)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// useWorktreeLocation points lerd's config at a scratch home holding the given
// worktree_location, so a test never reads the real config.yaml.
func useWorktreeLocation(t *testing.T, location string, mounts ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local/share"))
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.WorktreeLocation = location
	cfg.Mounts = mounts
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestWorktreeCheckoutPath(t *testing.T) {
	useWorktreeLocation(t, "")
	parent := filepath.Join(t.TempDir(), "myapp")

	// Worktrees live under the parent: <parent>/<parentBase>-<slug>.
	got, err := WorktreeCheckoutPath(parent, "feature/auth")
	want := filepath.Join(parent, "myapp-feature-auth")
	if err != nil || got != want {
		t.Fatalf("got %q, %v want %q", got, err, want)
	}

	// When the default path already exists, it bumps a numeric suffix.
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	got, _ = WorktreeCheckoutPath(parent, "feature/auth")
	if got != want+"-2" {
		t.Fatalf("got %q want %q", got, want+"-2")
	}
}

func TestWorktreeCheckoutPath_siblingPutsItNextToTheSite(t *testing.T) {
	home := useWorktreeLocation(t, "sibling")
	sites := filepath.Join(home, "Sites")
	parent := filepath.Join(sites, "myapp")

	got, err := WorktreeCheckoutPath(parent, "feature/auth")
	want := filepath.Join(sites, "myapp-feature-auth")
	if err != nil || got != want {
		t.Fatalf("got %q, %v want %q", got, err, want)
	}

	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	got, _ = WorktreeCheckoutPath(parent, "feature/auth")
	if got != want+"-2" {
		t.Fatalf("got %q want %q", got, want+"-2")
	}
}

// Outside home the containers only see what a parked directory or mounts:
// covers, and lerd never mounts the folder beside a site by itself: that folder
// can be /usr/local, whose mount would hide the PHP image's own files.
func TestWorktreeCheckoutPath_siblingNeedsAFolderTheContainersSee(t *testing.T) {
	outside := t.TempDir()
	site := filepath.Join(outside, "srv", "shop")

	useWorktreeLocation(t, "sibling")
	_, err := WorktreeCheckoutPath(site, "feat")
	if err == nil || !strings.Contains(err.Error(), filepath.Dir(site)) || !strings.Contains(err.Error(), "mounts:") {
		t.Fatalf("want an error naming %s and mounts:, got %v", filepath.Dir(site), err)
	}

	useWorktreeLocation(t, "sibling", filepath.Join(outside, "srv"))
	got, err := WorktreeCheckoutPath(site, "feat")
	if want := filepath.Join(outside, "srv", "shop-feat"); err != nil || got != want {
		t.Fatalf("with the folder under mounts: got %q, %v want %q", got, err, want)
	}
}

func TestWorktreeCheckoutPath_refusesAnUnknownLocation(t *testing.T) {
	useWorktreeLocation(t, "elsewhere")

	_, err := WorktreeCheckoutPath(filepath.Join(t.TempDir(), "myapp"), "feat-x")
	if err == nil || !strings.Contains(err.Error(), `"elsewhere"`) {
		t.Fatalf("want an error naming the value, got %v", err)
	}
	if _, err := DeriveWorktreeAddArgs(t.TempDir(), []string{"-b", "feat-x"}); err == nil {
		t.Fatal("DeriveWorktreeAddArgs must surface the bad location")
	}
}

func TestExcludeLocalOverride_OnlyTouchesAGitCheckout(t *testing.T) {
	plain := t.TempDir()
	if err := ExcludeLocalOverride(plain); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(plain, ".git")); !os.IsNotExist(err) {
		t.Errorf("a plain folder got a .git: %v", err)
	}

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := ExcludeLocalOverride(repo); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if strings.Count(string(got), "/.lerd.local.yaml\n") != 1 {
		t.Errorf("exclude file = %q, want /.lerd.local.yaml once", got)
	}
}

func TestExcludeLocalOverride_ReachesAWorktreesSharedExclude(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "shop")
	wt := filepath.Join(filepath.Dir(repo), "shop-feature")
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run(repo, "init", "-q")
	run(repo, "commit", "-q", "--allow-empty", "-m", "init")
	run(repo, "worktree", "add", "-q", "-b", "feature", wt)

	if err := ExcludeLocalOverride(wt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".lerd.local.yaml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", wt, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), ".lerd.local.yaml") {
		t.Errorf("git status in the worktree still lists the local file:\n%s", out)
	}
}
