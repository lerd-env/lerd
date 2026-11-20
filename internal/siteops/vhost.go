package siteops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/webserver"
)

// RegenerateSiteVhost rewrites a site's vhost after its domains change, dropping
// the old primary's. Every domain change comes through here, so it is also where
// a running dev server is realigned with them.
func RegenerateSiteVhost(site *config.Site, oldPrimary string) error {
	newPrimary := site.PrimaryDomain()

	if oldPrimary != newPrimary {
		_ = webserver.RemoveSite(oldPrimary)
		if err := MoveCustomNginxConfig(oldPrimary, newPrimary); err != nil {
			fmt.Fprintf(os.Stderr, "lerd: migrating custom nginx override to %s: %v\n", newPrimary, err)
		}
	}

	if err := webserver.ApplySite(*site); err != nil {
		return fmt.Errorf("writing vhost: %w", err)
	}
	// A dev server serving under this vhost has the site's domains baked into the
	// config it was started with, so it follows the vhost that just moved.
	RefreshDevServers(site)
	if podman.AfterUnitChange != nil {
		podman.AfterUnitChange("site:" + site.Name)
	}
	return nil
}

// MoveCustomNginxConfig follows a site's hand-authored nginx overrides across a
// primary-domain rename. Each scope's main snippet lives at custom.d/{primary}
// plus the scope suffix and each worktree's at custom.d/{branch}.{primary}
// plus it; the generated vhosts include them by name, so without this they are
// orphaned and the renamed site (and its worktrees) silently lose their custom
// config. Timestamped backups in custom.d.bkp/ are keyed the same way and moved
// too so the UI restore dropdown keeps working. Missing files are not an error;
// renames far outnumber edits.
func MoveCustomNginxConfig(oldPrimary, newPrimary string) error {
	if oldPrimary == newPrimary {
		return nil
	}
	var firstErr error
	for _, scope := range nginxScopes {
		if err := moveCustomNginxScope(oldPrimary, newPrimary, scope); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// moveCustomNginxScope moves one scope's live override and backups. The suffix
// is the scope's filename tail (".conf" or ".location.conf"); the two never
// collide because no name ending in ".location.conf" also ends in ".conf"
// preceded by the primary domain.
func moveCustomNginxScope(oldPrimary, newPrimary string, scope NginxScope) error {
	suffix := scope.suffix()
	live := scope.dir()
	// The main override is keyed solely by primary domain, so any file already
	// at the new name can only be a stale orphan from a prior rename (active
	// sites cannot share a primary), hence clobber=true.
	if err := moveFile(
		filepath.Join(live, oldPrimary+suffix),
		filepath.Join(live, newPrimary+suffix),
		true,
	); err != nil {
		return err
	}
	var firstErr error
	// Worktree overrides ({branch}.{oldPrimary}{suffix}) must rewrite the primary
	// suffix while preserving the branch prefix, else the renamed worktree loses
	// its config and re-inherits the main one on the next daemon resync.
	wtSuffix := "." + oldPrimary + suffix
	if liveEntries, err := os.ReadDir(live); err == nil {
		for _, e := range liveEntries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, wtSuffix) || len(name) == len(wtSuffix) {
				continue
			}
			branch := strings.TrimSuffix(name, wtSuffix)
			// A separately-registered site whose primary is a subdomain of this
			// one (e.g. app.test + admin.app.test) matches the suffix but is not
			// our worktree; leave its override alone.
			if _, err := config.FindSiteByDomain(branch + "." + oldPrimary); err == nil {
				continue
			}
			if err := moveFile(
				filepath.Join(live, name),
				filepath.Join(live, branch+"."+newPrimary+suffix),
				true,
			); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	bkp := scope.bkpDir()
	entries, err := os.ReadDir(bkp)
	if err != nil {
		if os.IsNotExist(err) {
			return firstErr
		}
		return err
	}
	// Move both main ({oldPrimary}{suffix}.bkp.*) and worktree
	// ({branch}.{oldPrimary}{suffix}.bkp.*) backups, never clobbering so a
	// same-second collision can't destroy recoverable history.
	mainPrefix := oldPrimary + suffix + ".bkp."
	wtMarker := "." + oldPrimary + suffix + ".bkp."
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		var newName string
		switch {
		case strings.HasPrefix(name, mainPrefix):
			newName = newPrimary + suffix + ".bkp." + strings.TrimPrefix(name, mainPrefix)
		case strings.Contains(name, wtMarker):
			idx := strings.Index(name, wtMarker)
			if idx <= 0 {
				continue
			}
			if _, err := config.FindSiteByDomain(name[:idx] + "." + oldPrimary); err == nil {
				continue // sibling site's backup, not our worktree's
			}
			newName = name[:idx] + "." + newPrimary + suffix + ".bkp." + name[idx+len(wtMarker):]
		default:
			continue
		}
		if err := moveFile(filepath.Join(bkp, name), filepath.Join(bkp, newName), false); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// moveFile renames src to dst when src exists. A missing src is a no-op. When
// clobber is false an existing dst is left untouched (src stays put) so no data
// is destroyed; when true an existing dst is replaced.
func moveFile(src, dst string, clobber bool) error {
	if src == dst {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !clobber {
		if _, err := os.Stat(dst); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(src, dst)
}
