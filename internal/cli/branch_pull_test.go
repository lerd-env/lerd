package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	if p.Target == "" {
		t.Error("the plan does not name the commit it reviewed")
	}
}

// reviewedTarget plans the pull and returns the commit the plan reviewed.
func reviewedTarget(t *testing.T, dir string) string {
	t.Helper()
	p, err := planPull(migratingFW, dir)
	if err != nil {
		t.Fatal(err)
	}
	return p.Target
}

// Commits pushed after the review are left for the next pull, since the
// chosen steps never accounted for them.
func TestPullBranch_stopsAtTheReviewedCommit(t *testing.T) {
	dir := pullSiteFixture(t, func(string) {})
	target := reviewedTarget(t, dir)
	other := t.TempDir()
	gitT(t, other, "clone", "-q", "-b", "main", strings.TrimSpace(gitOut(t, dir, "remote", "get-url", "origin")), ".")
	writeMigrations(t, filepath.Join(other, "db"), "late")
	gitT(t, other, "add", ".")
	gitT(t, other, "commit", "-q", "-m", "late")
	gitT(t, other, "push", "-q")

	if _, err := pullBranch(fixed(migratingFW), dir, target, BranchSteps{}, nil, io.Discard); err != nil {
		t.Fatalf("pullBranch: %v", err)
	}
	if got := strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD")); got != target {
		t.Fatalf("HEAD %s, want the reviewed %s", got, target)
	}
}

func TestPullBranch_needsAReviewedCommit(t *testing.T) {
	dir := pullSiteFixture(t, func(string) {})
	if _, err := pullBranch(fixed(migratingFW), dir, "", BranchSteps{}, nil, io.Discard); err == nil {
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
