package siteops

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/git"
)

// SiteDirReport is what deleting a site's directory takes with it, shown before
// anything is deleted. Unchecked lists where git could not be read, so the rest
// may be missing work; Refused, when set, says why the directory cannot go.
type SiteDirReport struct {
	Path             string   `json:"path"`
	InsideWorktrees  []string `json:"inside_worktrees,omitempty"`
	OutsideWorktrees []string `json:"outside_worktrees,omitempty"`
	Uncommitted      []string `json:"uncommitted,omitempty"`
	Unpushed         int      `json:"unpushed,omitempty"`
	Unchecked        []string `json:"unchecked,omitempty"`
	Refused          string   `json:"refused,omitempty"`
}

// ReportSiteDir lists the worktrees inside and beside the directory, the
// checkouts with uncommitted work, and the commits no remote has.
func ReportSiteDir(path, domain string) SiteDirReport {
	r := SiteDirReport{Path: path}
	if _, err := CheckSiteDir(path, nil, registeredSitePaths()); err != nil {
		r.Refused = err.Error()
	}
	unchecked := func(dir string) {
		if !slices.Contains(r.Unchecked, dir) {
			r.Unchecked = append(r.Unchecked, dir)
		}
	}
	if _, err := git.Output(path, "rev-parse", "--git-dir"); err != nil {
		// A .git git cannot open is a damaged repository, not a plain folder.
		if _, statErr := os.Lstat(filepath.Join(path, ".git")); statErr == nil {
			unchecked(path)
		}
		return r
	}
	checkouts := []string{path}
	if git.IsMainRepo(path) {
		wts, err := git.DetectWorktrees(path, domain)
		if err != nil {
			unchecked(path)
		}
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
		dirty, err := hasOwnWork(dir, r.InsideWorktrees)
		if err != nil {
			unchecked(dir)
		} else if dirty {
			r.Uncommitted = append(r.Uncommitted, dir)
		}
	}
	// --all takes in every local ref and each checkout's HEAD, so a detached
	// commit, a tag or a stash that only lives here is counted too.
	out, err := git.Output(path, "rev-list", "--count", "--all", "--not", "--remotes")
	if err != nil {
		unchecked(path)
	} else {
		r.Unpushed, _ = strconv.Atoi(strings.TrimSpace(out))
	}
	for _, dir := range checkouts {
		n, err := submoduleUnpushed(dir)
		if err != nil {
			unchecked(dir)
		}
		r.Unpushed += n
	}
	return r
}

// submoduleUnpushed counts the commits a checkout's submodules hold that none of
// their remotes has. Their history lives in the parent's .git, so it goes too.
func submoduleUnpushed(dir string) (int, error) {
	out, err := git.Output(dir, "submodule", "foreach", "--quiet", "--recursive", "git rev-list --count --all --not --remotes")
	if err != nil {
		return 0, err
	}
	total := 0
	for _, field := range strings.Fields(out) {
		n, err := strconv.Atoi(field)
		if err != nil {
			return total, fmt.Errorf("reading a submodule's commits: %q", field)
		}
		total += n
	}
	return total, nil
}

// hasOwnWork reports uncommitted work in a checkout, leaving out the worktrees
// nested in it, which the parent's status lists as untracked folders.
func hasOwnWork(dir string, nested []string) (bool, error) {
	files, err := git.DirtyFilesErr(dir)
	if err != nil {
		return false, err
	}
	for _, f := range files {
		owned := false
		for _, wt := range nested {
			owned = owned || (wt != dir && config.PathWithin(filepath.Join(dir, f), wt))
		}
		if !owned {
			return true, nil
		}
	}
	return false, nil
}

// registeredSitePaths is every linked site's directory, which lerd mounts by its
// own path when no other mount covers it.
func registeredSitePaths() []string {
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(reg.Sites))
	for _, s := range reg.Sites {
		paths = append(paths, s.Path)
	}
	return paths
}

// CheckSiteDir refuses a directory lerd must never delete: anything not inside a
// folder its containers mount, one of those folders itself, or lerd's own files.
// It returns where the path really leads, which is what may then be deleted.
func CheckSiteDir(path string, parkedDirs, sitePaths []string) (string, error) {
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
	return checkSiteDirDeletable(path, roots, sitePaths, []string{config.ConfigDir(), config.DataDir(), config.BinDir()})
}

// checkSiteDirDeletable compares where each path really leads, so a folder above
// the project swapped for a symlink cannot carry the delete somewhere else. A
// project counts as mounted by its own path only when no root covers it.
func checkSiteDirDeletable(path string, roots, projects, protected []string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s is not an absolute path", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("%s does not exist", path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s is a symlink, remove it yourself", path)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil || target == "/" {
		return "", fmt.Errorf("%s cannot be resolved to a project folder", path)
	}
	for _, p := range protected {
		p = resolved(p)
		if within(target, p) || within(p, target) {
			return "", fmt.Errorf("%s holds lerd's own files", path)
		}
	}
	inside := false
	for _, root := range roots {
		root = resolved(root)
		if within(root, target) {
			return "", fmt.Errorf("%s is or holds a folder lerd mounts, not a project", path)
		}
		inside = inside || within(target, root)
	}
	for _, project := range projects {
		covered := false
		for _, root := range roots {
			covered = covered || within(filepath.Clean(project), filepath.Clean(root))
		}
		inside = inside || (!covered && target == resolved(project))
	}
	if !inside {
		return "", fmt.Errorf("%s is outside the folders lerd mounts", path)
	}
	return target, nil
}

// RemoveSiteDir deletes an unlinked site's directory and drops what is left of
// its registry entry, which a parked site keeps as ignored after an unlink. The
// site paths are the registry's from before the unlink, which no longer lists it.
func RemoveSiteDir(name, path string, parkedDirs, sitePaths []string) error {
	target, err := CheckSiteDir(path, parkedDirs, sitePaths)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("deleting %s: %w", target, err)
	}
	_ = config.RemoveSite(name)
	_ = config.RemoveSiteFromWorkspaces(name)
	return nil
}

// resolved follows the symlinks in a path that exists, and cleans one that does not.
func resolved(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func within(p, root string) bool {
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")
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
