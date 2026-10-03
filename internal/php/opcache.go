package php

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// Seams so the routing can be tested without a container.
var (
	opcacheIsManual = podman.FPMSkipsRevalidation
	flushOPcacheFn  = podman.InvalidateOPcache
)

// FlushOPcacheForDir drops compiled files, as mode says, in the FPM container
// serving dir, on a host where FPM does not revalidate them by itself. Custom
// containers and FrankenPHP run no lerd FPM, and a folder outside every site
// has none to reach.
func FlushOPcacheForDir(dir, mode string) {
	if !opcacheIsManual() {
		return
	}
	site, _ := config.FindSiteByPath(SiteRootFor(dir))
	if _, parent, ok := WorktreeRootFor(dir); ok {
		site = parent
	}
	if site == nil || site.IsCustomContainer() || site.IsFrankenPHP() {
		return
	}
	version := site.PHPVersion
	if version == "" {
		var err error
		if version, err = VersionForDir(dir); err != nil {
			return
		}
	}
	_ = flushOPcacheFn(podman.FPMContainerName(*site, version), mode)
}
