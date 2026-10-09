package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
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
	p.Target, p.Branch, p.Head = strings.TrimSpace(target), strings.TrimSpace(branch), strings.TrimSpace(head)
	return p, err
}

// Reviewed is what a pull plan looked at: the branch and commit checked out,
// and the upstream commit it compared them with.
type Reviewed struct{ Branch, Head, Commit string }

// PullSite fast-forwards the checkout at dir to the commit its plan reviewed
// and runs the chosen steps around it, one switch or pull per checkout at a
// time. It returns the snapshot taken first, even on failure, so it can be
// restored.
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
	now, _ := gitpkg.Output(dir, "symbolic-ref", "--short", "HEAD")
	if now = strings.TrimSpace(now); now != r.Branch {
		return "", fmt.Errorf("the checkout is on %s now, not %s; review the pull again", now, r.Branch)
	}
	if head, _ := gitpkg.Output(dir, "rev-parse", "HEAD"); strings.TrimSpace(head) != r.Head {
		return "", fmt.Errorf("%s has moved since the review; review the pull again", r.Branch)
	}
	return runAround(fwAfter, dir, func() (string, error) { return gitpkg.FastForward(dir, r.Commit) }, steps, db, out)
}
