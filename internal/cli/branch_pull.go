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
	p, err := planBranch(fw, dir, strings.TrimSpace(target))
	p.Target = strings.TrimSpace(target)
	return p, err
}

// PullSite fast-forwards the checkout at dir to target, the commit its plan
// reviewed, and runs the chosen steps around it, one switch or pull per
// checkout at a time. It returns the snapshot taken first, even on failure, so
// it can be restored.
func PullSite(site *config.Site, dir, target string, steps BranchSteps, out io.Writer) (string, error) {
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
	if steps.Snapshot {
		t, ok := snapshotTargetFor(dir)
		if !ok {
			return "", fmt.Errorf("the site's database does not take snapshots")
		}
		db = siteBranchDB(site, dir, t, out)
	}
	return pullBranch(fwAfter, dir, target, steps, db, out)
}

// pullBranch stops at target rather than pulling whatever the remote has now:
// the chosen steps only account for the commits that were reviewed.
func pullBranch(fwAfter func() *config.Framework, dir, target string, steps BranchSteps, db *branchDB, out io.Writer) (string, error) {
	if target == "" {
		return "", fmt.Errorf("no reviewed commit to pull to")
	}
	return runAround(fwAfter, dir, func() (string, error) { return gitpkg.FastForward(dir, target) }, steps, db, out)
}
