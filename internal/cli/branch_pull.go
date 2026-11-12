package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/serviceops"
)

// PlanSitePull compares the checkout at dir, the site's or one of its
// worktrees, with what its upstream has, the same way a branch switch is planned.
func PlanSitePull(site *config.Site, dir string) (BranchPlan, error) {
	fw, _ := config.GetFrameworkForDir(site.Framework, dir)
	p, err := planPull(fw, dir)
	if err != nil {
		return p, err
	}
	if t, ok := snapshotTargetFor(dir); ok {
		p.DB = &BranchDB{Service: t.Service, Database: t.Database}
		// Only the main checkout's database is shared; a worktree's pull has none to offer.
		if dir == site.Path {
			p.SharedWorktrees = sharedWorktrees(site.Path, site.PrimaryDomain())
		}
	}
	return p, nil
}

// planPull fetches first, so the plan describes what the pull brings in.
func planPull(fw *config.Framework, dir string) (BranchPlan, error) {
	if st, err := gitpkg.ReadStatus(dir); err != nil || !st.Upstream {
		return BranchPlan{}, fmt.Errorf("this branch has no upstream to pull from")
	}
	if _, err := gitpkg.Fetch(dir); err != nil {
		return BranchPlan{}, err
	}
	target, err := gitpkg.Output(dir, "rev-parse", "@{upstream}")
	if err != nil {
		return BranchPlan{}, fmt.Errorf("cannot read the upstream commit")
	}
	branch, err := gitpkg.Output(dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return BranchPlan{}, fmt.Errorf("not on a branch")
	}
	head, err := gitpkg.Output(dir, "rev-parse", "HEAD")
	if err != nil {
		return BranchPlan{}, fmt.Errorf("cannot read the checked out commit")
	}
	p, err := planBranch(fw, dir, strings.TrimSpace(target))
	p.Branch, p.Head = strings.TrimSpace(branch), strings.TrimSpace(head)
	return p, err
}

// Reviewed is what a pull plan looked at: the branch and commit checked out,
// and the upstream commit it compared them with.
type Reviewed struct{ Branch, Head, Commit string }

// PullSite fast-forwards the checkout at dir to the reviewed commit and runs the
// chosen steps around it, one switch or pull per checkout at a time. It returns the
// snapshot taken first, so it can be restored if a step changing the database failed.
func PullSite(site *config.Site, dir string, r Reviewed, steps BranchSteps, out io.Writer) (string, error) {
	release := CheckoutLock(dir)
	if release == nil {
		return "", fmt.Errorf("a branch switch is running on this checkout")
	}
	defer release()
	fwAfter := func() *config.Framework {
		fw, _ := config.GetFrameworkForDir(site.Framework, dir)
		return fw
	}
	if steps.Isolate && dir != site.Path {
		return "", fmt.Errorf("only the main checkout's database is shared with worktrees")
	}
	var db *branchDB
	if steps.Snapshot || steps.Isolate {
		t, ok := snapshotTargetFor(dir)
		if !ok {
			return "", fmt.Errorf("the site's database does not take snapshots")
		}
		db = siteBranchDB(site, dir, t, out)
	}
	return pullBranch(fwAfter, dir, r, steps, db, out)
}

// pullBranch stops at the reviewed commit rather than pulling whatever the
// remote has now: the chosen steps only account for the commits reviewed. It
// refuses when the checkout has since moved to another branch.
func pullBranch(fwAfter func() *config.Framework, dir string, r Reviewed, steps BranchSteps, db *branchDB, out io.Writer) (string, error) {
	if r.Commit == "" || r.Branch == "" || r.Head == "" {
		return "", fmt.Errorf("no reviewed commit to pull to")
	}
	if err := checkedOut(dir, r); err != nil {
		return "", fmt.Errorf("%w since the review; review the pull again", err)
	}
	return runAround(fwAfter, dir, func() (string, error) { return gitpkg.FastForward(dir, r.Commit) }, steps, db, out)
}

// checkedOut says what changed when dir is no longer on r's branch and commit.
func checkedOut(dir string, r Reviewed) error {
	now, _ := gitpkg.Output(dir, "symbolic-ref", "--short", "HEAD")
	if now = strings.TrimSpace(now); now != r.Branch {
		return fmt.Errorf("the checkout is on %s now, not %s", now, r.Branch)
	}
	if head, _ := gitpkg.Output(dir, "rev-parse", "HEAD"); strings.TrimSpace(head) != r.Head {
		return fmt.Errorf("%s has moved", r.Branch)
	}
	return nil
}

// UndoPullSite takes the checkout at dir back to the commit a pull left, r.Head
// being where it landed and r.Commit where it started, then puts back the work
// the pull's stash kept and runs steps, restoring steps.Restore into snapDB.
func UndoPullSite(site *config.Site, dir string, r Reviewed, steps BranchSteps, snapDB serviceops.SnapshotTarget, keptSHA string, out io.Writer) (string, error) {
	release := CheckoutLock(dir)
	if release == nil {
		return "", fmt.Errorf("a branch switch is running on this checkout")
	}
	defer release()
	fwAfter := func() *config.Framework {
		fw, _ := config.GetFrameworkForDir(site.Framework, dir)
		return fw
	}
	var db *branchDB
	if steps.Restore != "" {
		// The snapshot was taken on the code before the pull, whose database
		// settings may differ from the pulled ones still checked out here.
		if snapDB.Service == "" || snapDB.Database == "" {
			return "", fmt.Errorf("no database named for the snapshot %s", steps.Restore)
		}
		db = siteBranchDB(site, dir, snapshotTarget(&dbEnv{service: snapDB.Service, database: snapDB.Database}, false), out)
	}
	return undoPull(fwAfter, dir, r, steps, keptSHA, db, out)
}

func undoPull(fwAfter func() *config.Framework, dir string, r Reviewed, steps BranchSteps, keptSHA string, db *branchDB, out io.Writer) (string, error) {
	if r.Commit == "" || r.Branch == "" || r.Head == "" {
		return "", fmt.Errorf("no pull to undo")
	}
	if err := checkedOut(dir, r); err != nil {
		return "", fmt.Errorf("%w since the pull, so it is not undone", err)
	}
	if steps.Restore == "" && steps.Migrate {
		fmt.Fprintln(out, "No snapshot was taken before the pull, so the database stays as it is")
	}
	// Kept work goes back before the installs: composer runs on the lockfile the
	// user had, which is the one in the stash.
	move := func() (string, error) {
		msg, err := gitpkg.WithStash(dir, "lerd: undoing the pull", func() (string, error) { return gitpkg.UndoFastForward(dir, r.Commit) })
		var parked *gitpkg.StashKept
		if (err != nil && !errors.As(err, &parked)) || keptSHA == "" {
			return msg, err
		}
		// Even when the undo's own stash was parked, the tree is clean again and
		// the pull's entry can still go back; the first one kept is reported.
		if popErr := gitpkg.PopStash(dir, keptSHA, stashLabel); err == nil {
			err = popErr
		} else if popErr != nil {
			fmt.Fprintln(out, popErr)
		}
		return msg, err
	}
	steps.Migrate, steps.Stash, steps.Snapshot, steps.Isolate = false, false, false, false
	return runAround(fwAfter, dir, move, steps, db, out)
}
