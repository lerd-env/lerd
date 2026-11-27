package siteops

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/git"
)

// SiteDirReport is what deleting a site's directory takes with it, shown before
// anything is deleted. Refused, when set, says why the directory cannot be.
type SiteDirReport struct {
	Path             string   `json:"path"`
	InsideWorktrees  []string `json:"inside_worktrees,omitempty"`
	OutsideWorktrees []string `json:"outside_worktrees,omitempty"`
	Uncommitted      []string `json:"uncommitted,omitempty"`
	Unpushed         int      `json:"unpushed,omitempty"`
	Refused          string   `json:"refused,omitempty"`
}

// ReportSiteDir lists the worktrees inside and beside the directory, the
// checkouts with uncommitted work, and the commits on local branches no remote has.
func ReportSiteDir(path, domain string) SiteDirReport {
	r := SiteDirReport{Path: path}
	checkouts := []string{path}
	if git.IsMainRepo(path) {
		wts, _ := git.DetectWorktrees(path, domain)
		for _, wt := range wts {
			if config.PathWithin(wt.Path, path) {
				r.InsideWorktrees = append(r.InsideWorktrees, wt.Path)
			} else {
				r.OutsideWorktrees = append(r.OutsideWorktrees, wt.Path)
			}
			checkouts = append(checkouts, wt.Path)
		}
	}
	for _, dir := range checkouts {
		if hasOwnWork(dir, r.InsideWorktrees) {
			r.Uncommitted = append(r.Uncommitted, dir)
		}
	}
	if out, err := git.Output(path, "rev-list", "--count", "--branches", "--not", "--remotes"); err == nil {
		r.Unpushed, _ = strconv.Atoi(strings.TrimSpace(out))
	}
	if err := CheckSiteDir(path, nil); err != nil {
		r.Refused = err.Error()
	}
	return r
}

// hasOwnWork reports uncommitted work in a checkout, leaving out the worktrees
// nested in it, which the parent's status lists as untracked folders.
func hasOwnWork(dir string, nested []string) bool {
	for _, f := range git.DirtyFiles(dir) {
		owned := false
		for _, wt := range nested {
			owned = owned || (wt != dir && config.PathWithin(filepath.Join(dir, f), wt))
		}
		if !owned {
			return true
		}
	}
	return false
}

// CheckSiteDir refuses a directory lerd must never delete: anything not inside a
// folder its containers mount, one of those folders itself, or lerd's own files.
func CheckSiteDir(path string, parkedDirs []string) error {
	roots := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, home)
	}
	if cfg, err := config.LoadGlobal(); err == nil {
		parkedDirs = append(parkedDirs, cfg.ParkedDirectories...)
		roots = append(roots, cfg.Mounts...)
	}
	for _, d := range parkedDirs {
		roots = append(roots, expandDir(d))
	}
	return checkSiteDirDeletable(path, roots, []string{config.ConfigDir(), config.DataDir(), config.BinDir()})
}

func checkSiteDirDeletable(path string, roots, protected []string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s is not an absolute path", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s does not exist", path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink, remove it yourself", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	for _, p := range protected {
		if config.PathWithin(path, p) || config.PathWithin(p, path) {
			return fmt.Errorf("%s holds lerd's own files", path)
		}
	}
	inside := false
	for _, root := range roots {
		if config.PathWithin(root, path) {
			return fmt.Errorf("%s is a folder lerd mounts, not a project", path)
		}
		inside = inside || config.PathWithin(path, root)
	}
	if !inside {
		return fmt.Errorf("%s is outside the folders lerd mounts", path)
	}
	return nil
}

// RemoveSiteDir deletes an unlinked site's directory and drops what is left of
// its registry entry, which a parked site keeps as ignored after an unlink.
func RemoveSiteDir(name, path string, parkedDirs []string) error {
	if err := CheckSiteDir(path, parkedDirs); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("deleting %s: %w", path, err)
	}
	_ = config.RemoveSite(name)
	_ = config.RemoveSiteFromWorkspaces(name)
	return nil
}

func expandDir(dir string) string {
	dir = os.ExpandEnv(dir)
	if strings.HasPrefix(dir, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, dir[1:])
		}
	}
	return dir
}
