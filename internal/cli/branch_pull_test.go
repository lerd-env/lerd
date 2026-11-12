package cli

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/serviceops"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

// pullSiteFixture returns a checkout tracking a bare remote that has one more
// commit, made by another clone with edit applied. The checkout has not fetched it.
func pullSiteFixture(t *testing.T, edit func(dir string)) string {
	t.Helper()
	remote, dir, other := t.TempDir(), t.TempDir(), t.TempDir()
	gitT(t, remote, "init", "-q", "--bare")
	gitT(t, dir, "init", "-q", "-b", "main")
	putFile(t, filepath.Join(dir, ".gitignore"), "vendor/\nnode_modules/\n")
	putFile(t, filepath.Join(dir, "composer.json"), "{}")
	putFile(t, filepath.Join(dir, "composer.lock"), "v1")
	writeMigrations(t, filepath.Join(dir, "db"), "a")
	putFile(t, filepath.Join(dir, "vendor", "composer", "installed.json"), "{}")
	gitT(t, dir, "add", ".")
	gitT(t, dir, "commit", "-q", "-m", "base")
	gitT(t, dir, "remote", "add", "origin", remote)
	gitT(t, dir, "push", "-q", "-u", "origin", "main")
	gitT(t, other, "clone", "-q", "-b", "main", remote, ".")
	edit(other)
	gitT(t, other, "add", "-A", ".")
	gitT(t, other, "commit", "-q", "--allow-empty", "-m", "theirs")
	gitT(t, other, "push", "-q")
	return dir
}

// The plan fetches first, so it describes what the pull will bring in.
func TestPlanPull_comparesWithTheFetchedUpstream(t *testing.T) {
	dir := pullSiteFixture(t, func(dir string) {
		putFile(t, filepath.Join(dir, "composer.lock"), "v2")
		writeMigrations(t, filepath.Join(dir, "db"), "b")
	})

	p, err := planPull(migratingFW, dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Behind != 1 || p.Ahead != 0 {
		t.Errorf("behind %d ahead %d, want 1 incoming commit", p.Behind, p.Ahead)
	}
	if p.Composer == nil || !p.Composer.Needed || p.Composer.Changed != "composer.lock" {
		t.Errorf("composer: %+v", p.Composer)
	}
	if p.Migrate == nil || !p.Migrate.Needed || p.MigrationsAdded != 1 {
		t.Errorf("migrate: %+v added %d", p.Migrate, p.MigrationsAdded)
	}
	if p.Target == "" || p.Branch != "main" || p.Head == "" {
		t.Errorf("the plan does not name what it reviewed: branch %q head %q target %q", p.Branch, p.Head, p.Target)
	}
}

// reviewedTarget plans the pull and returns what the plan reviewed.
func reviewedTarget(t *testing.T, dir string) Reviewed {
	t.Helper()
	p, err := planPull(migratingFW, dir)
	if err != nil {
		t.Fatal(err)
	}
	return Reviewed{Branch: p.Branch, Head: p.Head, Commit: p.Target}
}

// A commit made after the review changes what the pull would merge onto, so
// the dialog's plan no longer holds.
func TestPullBranch_refusesWhenTheCheckoutGainedACommit(t *testing.T) {
	dir := pullSiteFixture(t, func(string) {})
	reviewed := reviewedTarget(t, dir)
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "after review")

	if _, err := pullBranch(fixed(migratingFW), dir, reviewed, BranchSteps{}, nil, io.Discard); err == nil || !strings.Contains(err.Error(), "review the pull again") {
		t.Fatalf("want a refusal, got %v", err)
	}
}

// A switch between the review and the pull must not fast-forward the branch
// that is checked out now with the commit reviewed for another.
func TestPullBranch_refusesWhenTheCheckoutMovedToAnotherBranch(t *testing.T) {
	dir := pullSiteFixture(t, func(string) {})
	reviewed := reviewedTarget(t, dir)
	gitT(t, dir, "switch", "-q", "-c", "other")
	before := strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD"))

	_, err := pullBranch(fixed(migratingFW), dir, reviewed, BranchSteps{Migrate: true}, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "other") {
		t.Fatalf("want a refusal naming the branch now checked out, got %v", err)
	}
	if strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD")) != before {
		t.Error("the other branch was moved")
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err == nil {
		t.Error("migrated after the refusal")
	}
}

// Commits pushed after the review are left for the next pull, since the
// chosen steps never accounted for them.
func TestPullBranch_stopsAtTheReviewedCommit(t *testing.T) {
	dir := pullSiteFixture(t, func(string) {})
	reviewed := reviewedTarget(t, dir)
	other := t.TempDir()
	gitT(t, other, "clone", "-q", "-b", "main", strings.TrimSpace(gitOut(t, dir, "remote", "get-url", "origin")), ".")
	writeMigrations(t, filepath.Join(other, "db"), "late")
	gitT(t, other, "add", ".")
	gitT(t, other, "commit", "-q", "-m", "late")
	gitT(t, other, "push", "-q")

	if _, err := pullBranch(fixed(migratingFW), dir, reviewed, BranchSteps{}, nil, io.Discard); err != nil {
		t.Fatalf("pullBranch: %v", err)
	}
	if got := strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD")); got != reviewed.Commit {
		t.Fatalf("HEAD %s, want the reviewed %s", got, reviewed.Commit)
	}
}

func TestPullBranch_needsAReviewedCommit(t *testing.T) {
	dir := pullSiteFixture(t, func(string) {})
	if _, err := pullBranch(fixed(migratingFW), dir, Reviewed{Branch: "main"}, BranchSteps{}, nil, io.Discard); err == nil {
		t.Fatal("want a refusal without a reviewed commit")
	}
}

func TestPlanPull_noUpstream(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	if _, err := planPull(migratingFW, dir); err == nil {
		t.Fatal("want an error for a branch with no upstream")
	}
}

// The snapshot is taken before the code moves and migrations run after it.
func TestPullBranch_snapshotsThenFastForwardsThenMigrates(t *testing.T) {
	dir := pullSiteFixture(t, func(dir string) { writeMigrations(t, filepath.Join(dir, "db"), "b") })
	db := &branchDB{snapshot: func() (string, error) {
		if _, err := os.Stat(filepath.Join(dir, "db", "b")); err == nil {
			t.Error("snapshot taken after the pull")
		}
		return "before-switch-main", nil
	}}

	snap, err := pullBranch(fixed(migratingFW), dir, reviewedTarget(t, dir), BranchSteps{Snapshot: true, Migrate: true}, db, io.Discard)
	if err != nil {
		t.Fatalf("pullBranch: %v", err)
	}
	if snap != "before-switch-main" {
		t.Errorf("snapshot %q not reported back", snap)
	}
	if _, err := os.Stat(filepath.Join(dir, "db", "b")); err != nil {
		t.Error("the pull did not bring the new migration in")
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err != nil {
		t.Error("migrate did not run")
	}
}

// A refused pull leaves the steps after it unrun.
func TestPullBranch_refusedPullRunsNothingAfter(t *testing.T) {
	dir := pullSiteFixture(t, func(dir string) { putFile(t, filepath.Join(dir, "composer.lock"), "theirs") })
	putFile(t, filepath.Join(dir, "composer.lock"), "mine")

	_, err := pullBranch(fixed(migratingFW), dir, reviewedTarget(t, dir), BranchSteps{Migrate: true}, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "composer.lock") {
		t.Fatalf("want git's refusal naming the file, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err == nil {
		t.Error("migrate ran after a refused pull")
	}
}

// Worktrees on the parent's database are the ones a migration on main reaches;
// an isolated one has its own copy and is left out.
func TestSharedWorktrees(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	shared, isolated := filepath.Join(t.TempDir(), "shared"), filepath.Join(t.TempDir(), "own")
	gitT(t, dir, "worktree", "add", "-q", "-b", "shared", shared)
	gitT(t, dir, "worktree", "add", "-q", "-b", "own", isolated)
	if err := config.SetWorktreeDBIsolated(isolated, true); err != nil {
		t.Fatal(err)
	}

	got := sharedWorktrees(dir, "acme.test")
	if len(got) != 1 || got[0] != "shared" {
		t.Fatalf("shared worktrees = %v, want [shared]", got)
	}
}

// The worktrees get their copy of the database as it is now, before the code
// moves and before any migration runs on it.
func TestSwitchBranch_isolatesWorktreesBeforeTheCodeMoves(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	isolated := false
	db := &branchDB{isolate: func(io.Writer) error {
		if b := strings.TrimSpace(gitOut(t, dir, "symbolic-ref", "--short", "HEAD")); b != "main" {
			t.Errorf("isolated after the switch, on %s", b)
		}
		isolated = true
		return nil
	}}

	if _, err := switchBranch(fixed(migratingFW), dir, "other", BranchSteps{Isolate: true, Migrate: true}, db, io.Discard); err != nil {
		t.Fatalf("switchBranch: %v", err)
	}
	if !isolated {
		t.Error("worktrees were not isolated")
	}
}

// A failed isolation stops everything: the code stays put and nothing migrates.
func TestSwitchBranch_failedIsolationMovesNothing(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	db := &branchDB{isolate: func(io.Writer) error { return errors.New("clone failed") }}

	if _, err := switchBranch(fixed(migratingFW), dir, "other", BranchSteps{Isolate: true, Migrate: true}, db, io.Discard); err == nil {
		t.Fatal("want the isolation error")
	}
	if b := strings.TrimSpace(gitOut(t, dir, "symbolic-ref", "--short", "HEAD")); b != "main" {
		t.Errorf("switched to %s after a failed isolation", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err == nil {
		t.Error("migrated after a failed isolation")
	}
}

// A worktree's pull has no shared database to hand out copies of.
func TestPullSite_isolateOnlyFromTheMainCheckout(t *testing.T) {
	site := &config.Site{Name: "acme", Path: t.TempDir()}
	if _, err := PullSite(site, t.TempDir(), Reviewed{Branch: "main", Head: "abc", Commit: "def"}, BranchSteps{Isolate: true}, io.Discard); err == nil || !strings.Contains(err.Error(), "main checkout") {
		t.Fatalf("want a refusal for a worktree, got %v", err)
	}
}

// Local work that clashes on the way back out of the stash stays there, and
// the steps still run on the pulled code.
func TestPullBranch_stashConflictKeepsTheWorkAndRunsTheSteps(t *testing.T) {
	dir := pullSiteFixture(t, func(dir string) {
		putFile(t, filepath.Join(dir, "composer.lock"), "v2")
		writeMigrations(t, filepath.Join(dir, "db"), "b")
	})
	reviewed := reviewedTarget(t, dir)
	putFile(t, filepath.Join(dir, "composer.lock"), "local")

	_, err := pullBranch(fixed(migratingFW), dir, reviewed, BranchSteps{Stash: true, Migrate: true}, nil, io.Discard)
	var kept *gitpkg.StashKept
	if !errors.As(err, &kept) || kept.Err != nil || strings.Join(kept.Files, ",") != "composer.lock" {
		t.Fatalf("want only the kept stash reported, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err != nil {
		t.Error("migrate did not run after the stash was kept")
	}
}

// A step failing after the stash was kept reports both.
func TestPullBranch_stashKeptThenStepFails(t *testing.T) {
	dir := pullSiteFixture(t, func(dir string) {
		putFile(t, filepath.Join(dir, "composer.lock"), "v2")
	})
	reviewed := reviewedTarget(t, dir)
	putFile(t, filepath.Join(dir, "composer.lock"), "local")

	_, err := pullBranch(fixed(&config.Framework{}), dir, reviewed, BranchSteps{Stash: true, Migrate: true}, nil, io.Discard)
	var kept *gitpkg.StashKept
	if !errors.As(err, &kept) || kept.Err == nil || !strings.Contains(kept.Err.Error(), "migrate command") {
		t.Fatalf("want the kept stash with the migrate error, got %v", err)
	}
}

// Undo puts the code back where the pull found it, then the work the stash
// kept, and restores the database before anything else runs on it.
func TestUndoPull_putsTheCodeAndTheKeptWorkBack(t *testing.T) {
	dir := pullSiteFixture(t, func(dir string) {
		putFile(t, filepath.Join(dir, "composer.lock"), "v2")
	})
	reviewed := reviewedTarget(t, dir)
	putFile(t, filepath.Join(dir, "composer.lock"), "local")
	_, err := pullBranch(fixed(migratingFW), dir, reviewed, BranchSteps{Stash: true}, nil, io.Discard)
	var kept *gitpkg.StashKept
	if !errors.As(err, &kept) || kept.SHA == "" {
		t.Fatalf("want the stash kept with its commit, got %v", err)
	}

	var restored string
	db := &branchDB{restore: func(name string) error { restored = name; return nil }}
	landed := Reviewed{Branch: "main", Head: reviewed.Commit, Commit: reviewed.Head}
	if _, err := undoPull(fixed(migratingFW), dir, landed, BranchSteps{Restore: "branch-main-1"}, kept.SHA, db, io.Discard); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if head := strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD")); head != reviewed.Head {
		t.Errorf("HEAD %s, want %s", head, reviewed.Head)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "composer.lock")); string(b) != "local" {
		t.Errorf("composer.lock %q, want the local work back", b)
	}
	if list := strings.TrimSpace(gitOut(t, dir, "stash", "list")); list != "" {
		t.Errorf("stash left behind: %q", list)
	}
	if restored != "branch-main-1" {
		t.Errorf("restored %q", restored)
	}
}

// A commit made on top of the pull is not the undo's to throw away.
func TestUndoPull_refusesWhenTheBranchMovedOn(t *testing.T) {
	dir := pullSiteFixture(t, func(string) {})
	reviewed := reviewedTarget(t, dir)
	if _, err := pullBranch(fixed(migratingFW), dir, reviewed, BranchSteps{}, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "after the pull")

	landed := Reviewed{Branch: "main", Head: reviewed.Commit, Commit: reviewed.Head}
	if _, err := undoPull(fixed(migratingFW), dir, landed, BranchSteps{}, "", nil, io.Discard); err == nil || !strings.Contains(err.Error(), "moved since the pull") {
		t.Fatalf("want a refusal, got %v", err)
	}
}

// Work the user had on top of the pulled code comes along to the old commit.
func TestUndoPull_carriesUncommittedWork(t *testing.T) {
	dir := pullSiteFixture(t, func(dir string) {
		putFile(t, filepath.Join(dir, "composer.lock"), "v2")
	})
	reviewed := reviewedTarget(t, dir)
	if _, err := pullBranch(fixed(migratingFW), dir, reviewed, BranchSteps{}, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(dir, "composer.json"), `{"mine":1}`)

	landed := Reviewed{Branch: "main", Head: reviewed.Commit, Commit: reviewed.Head}
	if _, err := undoPull(fixed(migratingFW), dir, landed, BranchSteps{}, "", nil, io.Discard); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "composer.json")); string(b) != `{"mine":1}` {
		t.Error("uncommitted work lost")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "composer.lock")); string(b) != "v1" {
		t.Errorf("composer.lock %q, want v1", b)
	}
}

// The snapshot's database comes from the pull that took it; the pulled code's
// settings are never asked, and without one the code is not moved back at all.
func TestUndoPullSite_needsTheSnapshotsDatabase(t *testing.T) {
	dir := pullSiteFixture(t, func(string) {})
	reviewed := reviewedTarget(t, dir)
	if _, err := pullBranch(fixed(migratingFW), dir, reviewed, BranchSteps{}, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	landed := Reviewed{Branch: "main", Head: reviewed.Commit, Commit: reviewed.Head}
	site := &config.Site{Name: "shop", Path: dir}
	if _, err := UndoPullSite(site, dir, landed, BranchSteps{Restore: "branch-main-1"}, serviceops.SnapshotTarget{}, "", io.Discard); err == nil || !strings.Contains(err.Error(), "no database named") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if head := strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD")); head != reviewed.Commit {
		t.Errorf("the code moved back to %s", head)
	}
}
