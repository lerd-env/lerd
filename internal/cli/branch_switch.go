package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/sitetpl"
)

// PlanStep is one thing a branch switch can do afterwards. Needed is the
// suggestion: Changed names the manifest that differs between the branches,
// Missing says the install was never done here.
type PlanStep struct {
	Label   string `json:"label"`
	Needed  bool   `json:"needed"`
	Changed string `json:"changed,omitempty"`
	Missing bool   `json:"missing,omitempty"`
}

// BranchPlan compares the main checkout with a branch before switching to it.
// A nil step is one the target branch or the definition has nothing for.
type BranchPlan struct {
	Ahead             int       `json:"ahead"`
	Behind            int       `json:"behind"`
	Files             int       `json:"files"`
	Composer          *PlanStep `json:"composer,omitempty"`
	JS                *PlanStep `json:"js,omitempty"`
	Migrate           *PlanStep `json:"migrate,omitempty"`
	MigrationsAdded   int       `json:"migrations_added"`
	MigrationsMissing int       `json:"migrations_missing"`
	// Conflicts are files with uncommitted work that the target branch also
	// changes; git refuses the switch until they are committed or stashed.
	Conflicts []string `json:"conflicts"`
	// DB is the site's database when its engine takes snapshots, else nil.
	DB *BranchDB `json:"db,omitempty"`
}

// BranchDB names the site's database and the newest snapshot taken while the
// target branch was checked out, which switching back can restore.
type BranchDB struct {
	Service  string               `json:"service"`
	Database string               `json:"database"`
	Restore  *serviceops.Snapshot `json:"restore,omitempty"`
}

// BranchSteps are what the user chose to run around the switch. Snapshot
// copies the database before anything moves; Restore names a snapshot to load
// once the branch is checked out, before migrations run on top of it. Create
// makes the branch new, starting at Base (the current commit when empty).
type BranchSteps struct {
	Composer, JS, Migrate, Snapshot, Create bool
	Restore, Base                           string
}

// branchDB snapshots and restores the site's database; a seam for tests.
type branchDB struct {
	snapshot func() (string, error)
	restore  func(name string) error
}

var (
	composerManifests = []string{"composer.lock", "composer.json"}
	jsManifests       = []string{"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb", "package.json"}
)

// PlanSiteBranch diffs the site's main checkout against branch.
func PlanSiteBranch(site *config.Site, branch string) (BranchPlan, error) {
	fw, _ := config.GetFrameworkForDir(site.Framework, site.Path)
	p, err := planBranch(fw, site.Path, branch)
	if err != nil {
		return p, err
	}
	if t, ok := snapshotTargetFor(site.Path); ok {
		snaps, _ := serviceops.ListSnapshots(t.Service, t.Database, false)
		p.DB = &BranchDB{Service: t.Service, Database: t.Database, Restore: newestSnapshotFor(snaps, localBranchName(site.Path, branch))}
	}
	return p, nil
}

// SwitchSiteBranch checks branch out in the site's main checkout, running the
// chosen steps around it and stopping at the first failure. It returns the name
// of the snapshot taken before switching, even on failure, so it can be restored.
func SwitchSiteBranch(site *config.Site, branch string, steps BranchSteps, out io.Writer) (string, error) {
	fw, _ := config.GetFrameworkForDir(site.Framework, site.Path)
	var db *branchDB
	if steps.Snapshot || steps.Restore != "" {
		t, ok := snapshotTargetFor(site.Path)
		if !ok {
			return "", fmt.Errorf("the site's database does not take snapshots")
		}
		db = siteBranchDB(site, t, out)
	}
	return switchBranch(fw, site.Path, branch, steps, db, out)
}

func siteBranchDB(site *config.Site, t serviceops.SnapshotTarget, out io.Writer) *branchDB {
	emit := func(e serviceops.PhaseEvent) {
		if e.Message != "" {
			fmt.Fprintln(out, e.Message)
		}
	}
	return &branchDB{
		snapshot: func() (string, error) {
			if err := ensureServiceRunning(t.Service); err != nil {
				return "", err
			}
			meta := serviceops.SnapshotMeta{Site: site.Name, GitBranch: snapshotGitBranch(site.Path)}
			s, err := serviceops.CreateSnapshot(t, branchSnapshotName(meta.GitBranch), meta, emit)
			if err != nil {
				return "", err
			}
			// One copy per branch: the new one replaces the last, once it is safely written.
			snaps, _ := serviceops.ListSnapshots(t.Service, t.Database, false)
			for _, old := range staleBranchSnapshots(snaps, meta.GitBranch, s.Name) {
				_ = serviceops.DeleteSnapshot(t.Service, t.Database, old, false)
			}
			return s.Name, nil
		},
		restore: func(name string) error {
			if err := ensureServiceRunning(t.Service); err != nil {
				return err
			}
			rep, err := serviceops.RestoreSnapshot(t, name, emit)
			if err == nil && rep.Errors > 0 {
				err = fmt.Errorf("restoring %s: %s", name, rep.Summary())
			}
			return err
		},
	}
}

// snapshotTargetFor resolves the site's database, when its engine takes snapshots.
func snapshotTargetFor(path string) (serviceops.SnapshotTarget, bool) {
	env, err := resolveDB(path, "", "")
	if err != nil {
		return serviceops.SnapshotTarget{}, false
	}
	t := snapshotTarget(env, false)
	return t, t.Database != "" && serviceops.SnapshotSupported(t.Service, false)
}

// branchSnapshotPrefix names the snapshots the switcher takes, which is how it
// tells its own copies from ones taken by hand.
const branchSnapshotPrefix = "before-switch"

// branchSnapshotName is the name a copy of branch is saved under; the store
// appends the time. Branch slashes would read as directories, so they go.
func branchSnapshotName(branch string) string {
	return branchSnapshotPrefix + "-" + gitpkg.SanitizeBranch(branch)
}

// staleBranchSnapshots lists the switcher's earlier copies of branch, all but keep.
func staleBranchSnapshots(snaps []serviceops.Snapshot, branch, keep string) []string {
	var stale []string
	for _, s := range snaps {
		if s.GitBranch == branch && s.Name != keep && strings.HasPrefix(s.Name, branchSnapshotPrefix+"-") {
			stale = append(stale, s.Name)
		}
	}
	return stale
}

// newestSnapshotFor picks the latest snapshot taken with branch checked out.
func newestSnapshotFor(snaps []serviceops.Snapshot, branch string) *serviceops.Snapshot {
	var best *serviceops.Snapshot
	for i := range snaps {
		if snaps[i].GitBranch == branch && (best == nil || snaps[i].Created.After(best.Created)) {
			best = &snaps[i]
		}
	}
	return best
}

// localBranchName is the branch a switch to ref ends up on: origin/x becomes x.
func localBranchName(path, ref string) string {
	if gitpkg.BranchExists(path, ref) {
		return ref
	}
	if _, after, ok := strings.Cut(ref, "/"); ok {
		return after
	}
	return ref
}

func planBranch(fw *config.Framework, path, branch string) (BranchPlan, error) {
	diff, err := gitpkg.DiffNameStatus(path, "HEAD", branch)
	if err != nil {
		return BranchPlan{}, fmt.Errorf("cannot compare with %s", branch)
	}
	p := BranchPlan{Files: len(diff), Conflicts: []string{}}
	for _, f := range gitpkg.DirtyFiles(path) {
		if _, ok := diff[f]; ok {
			p.Conflicts = append(p.Conflicts, f)
		}
	}
	p.Ahead, p.Behind = gitpkg.AheadBehind(path, "HEAD", branch)
	if gitpkg.FileAtRef(path, branch, "composer.json") {
		p.Composer = installStep("composer install", diff, composerManifests, filepath.Join(path, "vendor"))
	}
	if gitpkg.FileAtRef(path, branch, "package.json") {
		p.JS = installStep(gitpkg.JSInstallCommand(path), diff, jsManifests, filepath.Join(path, "node_modules"))
	}
	if dir := migrationsDirOf(fw); dir != "" {
		prefix := strings.TrimSuffix(dir, "/") + "/"
		for f, s := range diff {
			if strings.HasPrefix(f, prefix) {
				switch s {
				case 'A':
					p.MigrationsAdded++
				case 'D':
					p.MigrationsMissing++
				}
			}
		}
	}
	if c, _, ok := resolveMigrateCommand(fw, path); ok {
		p.Migrate = &PlanStep{Label: c.Command, Needed: p.MigrationsAdded > 0}
	}
	return p, nil
}

// installStep is due when a manifest differs or the install folder is absent.
func installStep(label string, diff map[string]byte, manifests []string, installDir string) *PlanStep {
	s := &PlanStep{Label: label}
	for _, m := range manifests {
		if _, ok := diff[m]; ok {
			s.Changed = m
			break
		}
	}
	if _, err := os.Stat(installDir); err != nil {
		s.Missing = true
	}
	s.Needed = s.Changed != "" || s.Missing
	return s
}

func switchBranch(fw *config.Framework, path, branch string, steps BranchSteps, db *branchDB, out io.Writer) (string, error) {
	snap := ""
	if steps.Snapshot && db != nil {
		name, err := db.snapshot()
		if err != nil {
			return "", fmt.Errorf("snapshot before switching: %w", err)
		}
		snap = name
		fmt.Fprintln(out, "Database saved as snapshot "+name)
	}
	var msg string
	var err error
	if steps.Create {
		msg, err = gitpkg.SwitchNew(path, branch, steps.Base)
	} else {
		msg, err = gitpkg.Switch(path, branch)
	}
	if err != nil {
		return snap, err
	}
	fmt.Fprintln(out, msg)
	if steps.Restore != "" && db != nil {
		if err := db.restore(steps.Restore); err != nil {
			return snap, err
		}
		fmt.Fprintln(out, "Database restored from snapshot "+steps.Restore)
	}
	if steps.Composer {
		if err := gitpkg.InstallComposer(path, out); err != nil {
			return snap, err
		}
	}
	if steps.JS {
		if err := gitpkg.InstallJS(path, out); err != nil {
			return snap, err
		}
	}
	if steps.Migrate {
		c, dir, ok := resolveMigrateCommand(fw, path)
		if !ok {
			return snap, fmt.Errorf("the framework definition declares no migrate command")
		}
		fmt.Fprintln(out, "$ "+c.Command)
		cmd := newCommandExec(dir, c.Command)
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil {
			return snap, fmt.Errorf("%s: %w", c.Command, err)
		}
	}
	return snap, nil
}

func migrationsDirOf(fw *config.Framework) string {
	if fw == nil || fw.Worktree == nil {
		return ""
	}
	return fw.Worktree.Migrations
}

// resolveMigrateCommand finds the command the definition names as its migrate
// step, expanded for path, with the directory it runs in.
func resolveMigrateCommand(fw *config.Framework, path string) (config.FrameworkCommand, string, bool) {
	if fw == nil || fw.Doctor == nil || fw.Doctor.MigrateCommand == "" {
		return config.FrameworkCommand{}, "", false
	}
	for _, c := range sitetpl.ExpandCommands(config.ResolveCommands(fw, nil, path), sitetpl.ForPath(path)) {
		if c.Name != fw.Doctor.MigrateCommand || c.Command == "" || c.ProjectOrigin {
			continue
		}
		dir := path
		if c.CWD != "" && c.CWD != "." {
			dir = filepath.Join(path, c.CWD)
		}
		return c, dir, true
	}
	return config.FrameworkCommand{}, "", false
}
