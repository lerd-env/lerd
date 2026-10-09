package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func putFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// switchFixture commits base files on main, then builds "other" from them
// with edit applied. vendor/ and node_modules/ exist so nothing reads as missing.
func switchFixture(t *testing.T, edit func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "main")
	putFile(t, filepath.Join(dir, ".gitignore"), "vendor/\nnode_modules/\n")
	putFile(t, filepath.Join(dir, "composer.json"), "{}")
	putFile(t, filepath.Join(dir, "composer.lock"), "v1")
	putFile(t, filepath.Join(dir, "package.json"), "{}")
	putFile(t, filepath.Join(dir, "package-lock.json"), "v1")
	writeMigrations(t, filepath.Join(dir, "db"), "a", "b")
	putFile(t, filepath.Join(dir, "vendor", "composer", "installed.json"), "{}")
	putFile(t, filepath.Join(dir, "node_modules", ".package-lock.json"), "{}")
	gitT(t, dir, "add", ".")
	gitT(t, dir, "commit", "-q", "-m", "base")
	gitT(t, dir, "checkout", "-q", "-b", "other")
	edit(dir)
	gitT(t, dir, "add", "-A", ".")
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "other")
	gitT(t, dir, "checkout", "-q", "main")
	return dir
}

var migratingFW = &config.Framework{
	Worktree: &config.FrameworkWorktree{Migrations: "db"},
	Doctor:   &config.FrameworkDoctor{MigrateCommand: "migrate"},
	Commands: []config.FrameworkCommand{{Name: "migrate", Command: "touch migrated"}},
}

func TestPlanBranch_nothingChanged(t *testing.T) {
	dir := switchFixture(t, func(string) {})

	p, err := planBranch(migratingFW, dir, "other")
	if err != nil {
		t.Fatal(err)
	}
	if p.Composer == nil || p.Composer.Needed || p.JS == nil || p.JS.Needed || p.Migrate == nil || p.Migrate.Needed {
		t.Fatalf("nothing should be due: %+v %+v %+v", p.Composer, p.JS, p.Migrate)
	}
	if p.Behind != 1 {
		t.Errorf("behind = %d, want 1", p.Behind)
	}
}

func TestPlanBranch_lockfilesAndMigrationsChanged(t *testing.T) {
	dir := switchFixture(t, func(dir string) {
		putFile(t, filepath.Join(dir, "composer.lock"), "v2")
		putFile(t, filepath.Join(dir, "package-lock.json"), "v2")
		os.Remove(filepath.Join(dir, "db", "b"))
		writeMigrations(t, filepath.Join(dir, "db"), "c", "d")
	})

	p, err := planBranch(migratingFW, dir, "other")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Composer.Needed || p.Composer.Changed != "composer.lock" {
		t.Errorf("composer: %+v", p.Composer)
	}
	if !p.JS.Needed || p.JS.Changed != "package-lock.json" || p.JS.Label != "npm ci" {
		t.Errorf("js: %+v", p.JS)
	}
	if !p.Migrate.Needed || p.MigrationsAdded != 2 || p.MigrationsMissing != 1 {
		t.Errorf("migrate: %+v added %d missing %d", p.Migrate, p.MigrationsAdded, p.MigrationsMissing)
	}
}

// No vendor/ means composer has never installed here, whatever the diff says.
func TestPlanBranch_missingInstallIsDue(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	os.RemoveAll(filepath.Join(dir, "vendor"))

	p, _ := planBranch(migratingFW, dir, "other")
	if !p.Composer.Needed || !p.Composer.Missing {
		t.Fatalf("composer: %+v", p.Composer)
	}
}

// A branch without a manifest, or a definition without a migrate command,
// has no such step to offer.
func TestPlanBranch_absentSteps(t *testing.T) {
	dir := switchFixture(t, func(dir string) {
		os.Remove(filepath.Join(dir, "package.json"))
		os.Remove(filepath.Join(dir, "package-lock.json"))
	})

	p, _ := planBranch(&config.Framework{}, dir, "other")
	if p.JS != nil || p.Migrate != nil || p.Composer == nil {
		t.Fatalf("got composer %+v js %+v migrate %+v", p.Composer, p.JS, p.Migrate)
	}
}

func TestSwitchBranch_runsOnlyTheChosenSteps(t *testing.T) {
	dir := switchFixture(t, func(string) {})

	if _, err := switchBranch(fixed(migratingFW), dir, "other", BranchSteps{Migrate: true}, nil, io.Discard); err != nil {
		t.Fatalf("switchBranch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err != nil {
		t.Error("the definition's migrate command did not run")
	}
}

func TestSwitchBranch_reportsGitRefusal(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	if _, err := switchBranch(fixed(migratingFW), dir, "nope", BranchSteps{Migrate: true}, nil, io.Discard); err == nil {
		t.Fatal("want an error for an unknown branch")
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err == nil {
		t.Error("migrate ran after a failed switch")
	}
}

// Uncommitted work on a file the target branch changes is what makes git
// refuse the switch; work elsewhere comes along without trouble.
func TestPlanBranch_reportsConflictingLocalChanges(t *testing.T) {
	dir := switchFixture(t, func(dir string) {
		putFile(t, filepath.Join(dir, "composer.lock"), "v2")
		putFile(t, filepath.Join(dir, "added.txt"), "theirs")
	})
	putFile(t, filepath.Join(dir, "composer.lock"), "mine")
	putFile(t, filepath.Join(dir, "added.txt"), "mine, untracked")
	putFile(t, filepath.Join(dir, "package.json"), `{"mine": true}`)

	p, err := planBranch(migratingFW, dir, "other")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range p.Conflicts {
		got[f] = true
	}
	if len(got) != 2 || !got["composer.lock"] || !got["added.txt"] {
		t.Fatalf("conflicts = %v, want composer.lock and added.txt", p.Conflicts)
	}
	// The prediction must match git: this switch is one git refuses.
	if _, err := switchBranch(fixed(migratingFW), dir, "other", BranchSteps{}, nil, io.Discard); err == nil {
		t.Error("git switched despite the predicted conflict")
	}
}

func TestNewestSnapshotFor(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 10, 1, h, 0, 0, 0, time.UTC) }
	snaps := []serviceops.Snapshot{
		{Name: "a", GitBranch: "dev", Created: at(1)},
		{Name: "b", GitBranch: "dev", Created: at(3)},
		{Name: "c", GitBranch: "main", Created: at(5)},
	}
	if s := newestSnapshotFor(snaps, "dev"); s == nil || s.Name != "b" {
		t.Fatalf("got %+v, want the newer dev snapshot", s)
	}
	if s := newestSnapshotFor(snaps, "feature"); s != nil {
		t.Fatalf("got %+v for a branch with no snapshot", s)
	}
}

// The database is copied before anything moves, and a snapshot of the target
// branch goes back in before migrations run on top of it.
func TestSwitchBranch_snapshotsFirstAndRestoresBeforeMigrating(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	var calls []string
	db := &branchDB{
		snapshot: func() (string, error) { calls = append(calls, "snapshot"); return "before-switch-1", nil },
		restore: func(name string) error {
			calls = append(calls, "restore "+name)
			if _, err := os.Stat(filepath.Join(dir, "migrated")); err == nil {
				t.Error("migrations ran before the restore")
			}
			return nil
		},
	}

	snap, err := switchBranch(fixed(migratingFW), dir, "other", BranchSteps{Snapshot: true, Restore: "dev-snap", Migrate: true}, db, io.Discard)
	if err != nil {
		t.Fatalf("switchBranch: %v", err)
	}
	if snap != "before-switch-1" {
		t.Errorf("snapshot name %q not reported back", snap)
	}
	if strings.Join(calls, ",") != "snapshot,restore dev-snap" {
		t.Errorf("calls = %v", calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err != nil {
		t.Error("migrate did not run")
	}
}

// A failing migration still reports the snapshot, so it can be put back.
func TestSwitchBranch_failedMigrationKeepsTheSnapshotName(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	failing := &config.Framework{
		Worktree: migratingFW.Worktree,
		Doctor:   migratingFW.Doctor,
		Commands: []config.FrameworkCommand{{Name: "migrate", Command: "exit 3"}},
	}
	db := &branchDB{snapshot: func() (string, error) { return "before-switch-2", nil }}

	snap, err := switchBranch(fixed(failing), dir, "other", BranchSteps{Snapshot: true, Migrate: true}, db, io.Discard)
	if err == nil || snap != "before-switch-2" {
		t.Fatalf("want the migrate error with the snapshot name, got %q, %v", snap, err)
	}
}

// Only the switcher's own copies of the same branch are replaced: a snapshot
// taken by hand, or another branch's copy, is never touched.
func TestStaleBranchSnapshots(t *testing.T) {
	snaps := []serviceops.Snapshot{
		{Name: "before-switch-1", GitBranch: "main"},
		{Name: "before-switch-2", GitBranch: "main"},
		{Name: "before-switch-3", GitBranch: "dev"},
		{Name: "pre-migration-1", GitBranch: "main"},
	}
	got := staleBranchSnapshots(snaps, "main", "before-switch-2")
	if strings.Join(got, ",") != "before-switch-1" {
		t.Fatalf("got %v, want only the older main copy", got)
	}
}

// Snapshot names hold no path separators, and carry the branch so two
// switches in the same second, from different branches, never collide.
func TestBranchSnapshotName(t *testing.T) {
	if got := branchSnapshotName("feature/payments"); got != "before-switch-feature-payments" {
		t.Fatalf("got %q", got)
	}
}

func TestSwitchBranch_createsANewBranchFromItsBase(t *testing.T) {
	dir := switchFixture(t, func(dir string) { putFile(t, filepath.Join(dir, "only-on-other"), "x") })

	if _, err := switchBranch(fixed(migratingFW), dir, "feature/new", BranchSteps{Create: true, Base: "other", Migrate: true}, nil, io.Discard); err != nil {
		t.Fatalf("switchBranch: %v", err)
	}
	out, _ := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output()
	if strings.TrimSpace(string(out)) != "feature/new" {
		t.Fatalf("on %q, want feature/new", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "only-on-other")); err != nil {
		t.Error("new branch did not start from its base")
	}
	if _, err := os.Stat(filepath.Join(dir, "migrated")); err != nil {
		t.Error("the chosen steps did not run after creating the branch")
	}
}

// A site linked below the repository root reads its manifests and migrations
// under its own folder; git reports paths from the root.
func TestPlanBranch_siteInARepoSubfolder(t *testing.T) {
	root := t.TempDir()
	site := filepath.Join(root, "apps", "shop")
	gitT(t, root, "init", "-q", "-b", "main")
	putFile(t, filepath.Join(site, "composer.json"), "{}")
	putFile(t, filepath.Join(site, "composer.lock"), "v1")
	writeMigrations(t, filepath.Join(site, "db"), "a")
	putFile(t, filepath.Join(site, "vendor", "composer", "installed.json"), "{}")
	putFile(t, filepath.Join(root, ".gitignore"), "vendor/\n")
	gitT(t, root, "add", ".")
	gitT(t, root, "commit", "-q", "-m", "base")
	gitT(t, root, "checkout", "-q", "-b", "other")
	putFile(t, filepath.Join(site, "composer.lock"), "v2")
	writeMigrations(t, filepath.Join(site, "db"), "b")
	gitT(t, root, "add", ".")
	gitT(t, root, "commit", "-q", "-m", "other")
	gitT(t, root, "checkout", "-q", "main")

	p, err := planBranch(migratingFW, site, "other")
	if err != nil {
		t.Fatal(err)
	}
	if p.Composer == nil || !p.Composer.Needed || p.Composer.Changed != "composer.lock" {
		t.Errorf("composer: %+v", p.Composer)
	}
	if p.MigrationsAdded != 1 || !p.Migrate.Needed {
		t.Errorf("migrations added %d, migrate %+v", p.MigrationsAdded, p.Migrate)
	}
}

// One switch per checkout: a second request while one runs is refused rather
// than letting its installs and migrations land on the other's branch.
func TestSwitchSiteBranch_refusesAConcurrentSwitch(t *testing.T) {
	site := &config.Site{Name: "acme", Path: t.TempDir()}
	release := CheckoutLock(site.Path)
	defer release()

	if _, err := SwitchSiteBranch(site, "dev", BranchSteps{}, io.Discard); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("want a refusal while another switch runs, got %v", err)
	}
}

// The target branch may point the site at another database; restoring the old
// one's snapshot into it would overwrite the wrong data.
func TestRestoreTarget(t *testing.T) {
	before := serviceops.SnapshotTarget{Service: "mysql", Database: "shop"}
	same := func() (serviceops.SnapshotTarget, bool) { return before, true }
	moved := func() (serviceops.SnapshotTarget, bool) {
		return serviceops.SnapshotTarget{Service: "mysql", Database: "shop_v2"}, true
	}
	if _, err := restoreTarget(before, same); err != nil {
		t.Errorf("same database refused: %v", err)
	}
	if _, err := restoreTarget(before, moved); err == nil || !strings.Contains(err.Error(), "shop_v2") {
		t.Errorf("want a refusal naming the new database, got %v", err)
	}
}

// The migrate command comes from the definition as it stands after checkout,
// since the target branch may be on another framework version.
func TestSwitchBranch_migratesWithTheDefinitionAfterCheckout(t *testing.T) {
	dir := switchFixture(t, func(string) {})
	resolved := ""
	fwAfter := func() *config.Framework {
		out, _ := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output()
		resolved = strings.TrimSpace(string(out))
		return migratingFW
	}

	if _, err := switchBranch(fwAfter, dir, "other", BranchSteps{Migrate: true}, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if resolved != "other" {
		t.Errorf("definition resolved on %q, want after checking out other", resolved)
	}
}

func fixed(fw *config.Framework) func() *config.Framework {
	return func() *config.Framework { return fw }
}
