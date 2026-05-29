package siteops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/podman"
)

// RegenerateSiteVhost regenerates the nginx vhost for a site after domain changes.
// If the primary domain changed, the old vhost file is removed. For secured sites
// the SSL vhost is generated and renamed to the main .conf path.
func RegenerateSiteVhost(site *config.Site, oldPrimary string) error {
	newPrimary := site.PrimaryDomain()

	if oldPrimary != newPrimary {
		_ = nginx.RemoveVhost(oldPrimary)
		if err := MoveCustomNginxConfig(oldPrimary, newPrimary); err != nil {
			fmt.Fprintf(os.Stderr, "lerd: migrating custom nginx override to %s: %v\n", newPrimary, err)
		}
	}

	if site.IsCustomContainer() {
		if site.Secured {
			if err := nginx.GenerateCustomSSLVhost(*site); err != nil {
				return fmt.Errorf("generating custom SSL vhost: %w", err)
			}
			sslConf := filepath.Join(config.NginxConfD(), newPrimary+"-ssl.conf")
			mainConf := filepath.Join(config.NginxConfD(), newPrimary+".conf")
			_ = os.Remove(mainConf)
			if err := os.Rename(sslConf, mainConf); err != nil {
				return fmt.Errorf("installing custom SSL vhost: %w", err)
			}
		} else {
			if err := nginx.GenerateCustomVhost(*site); err != nil {
				return fmt.Errorf("generating custom vhost: %w", err)
			}
		}
	} else if site.Secured {
		if err := nginx.GenerateSSLVhost(*site, site.PHPVersion); err != nil {
			return fmt.Errorf("generating SSL vhost: %w", err)
		}
		sslConf := filepath.Join(config.NginxConfD(), newPrimary+"-ssl.conf")
		mainConf := filepath.Join(config.NginxConfD(), newPrimary+".conf")
		_ = os.Remove(mainConf)
		if err := os.Rename(sslConf, mainConf); err != nil {
			return fmt.Errorf("installing SSL vhost: %w", err)
		}
	} else {
		if err := nginx.GenerateVhost(*site, site.PHPVersion); err != nil {
			return fmt.Errorf("generating vhost: %w", err)
		}
	}
	if podman.AfterUnitChange != nil {
		podman.AfterUnitChange("site:" + site.Name)
	}
	return nil
}

// MoveCustomNginxConfig follows a site's hand-authored nginx override across a
// primary-domain rename. The live snippet lives at custom.d/{domain}.conf and
// the generated vhost includes it by name, so without this the file is silently
// orphaned and the renamed site loses its custom config. Timestamped backups in
// custom.d.bkp/ are keyed the same way ({domain}.conf.bkp.*) and are moved too
// so the UI restore dropdown keeps working after a rename. Missing files are not
// an error; the caller renames domains far more often than it edits overrides.
func MoveCustomNginxConfig(oldPrimary, newPrimary string) error {
	if oldPrimary == newPrimary {
		return nil
	}
	live := config.NginxCustomD()
	// The live override is keyed solely by primary domain, so any file already
	// at the new name can only be a stale orphan from a prior rename (active
	// sites cannot share a primary); overwriting it with the current config is
	// the intended outcome, hence clobber=true.
	if err := moveFile(
		filepath.Join(live, oldPrimary+".conf"),
		filepath.Join(live, newPrimary+".conf"),
		true,
	); err != nil {
		return err
	}
	bkp := config.NginxCustomDBkp()
	entries, err := os.ReadDir(bkp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	oldPrefix := oldPrimary + ".conf.bkp."
	var firstErr error
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), oldPrefix) {
			continue
		}
		suffix := strings.TrimPrefix(e.Name(), oldPrefix)
		// Backups are never clobbered: a same-second timestamp collision across
		// the two domains would otherwise destroy recoverable history. Keep
		// going on error so one bad file can't strand the rest mid-migration.
		if err := moveFile(
			filepath.Join(bkp, e.Name()),
			filepath.Join(bkp, newPrimary+".conf.bkp."+suffix),
			false,
		); err != nil && firstErr == nil {
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
