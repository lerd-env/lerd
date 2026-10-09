package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	snap, err := pullBranch(fixed(migratingFW), dir, BranchSteps{Snapshot: true, Migrate: true}, db, io.Discard)
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

	_, err := pullBranch(fixed(migratingFW), dir, BranchSteps{Migrate: true}, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "composer.lock") {
		t.Fatalf("want git's refusal naming the file, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err == nil {
		t.Error("migrate ran after a refused pull")
	}
}
