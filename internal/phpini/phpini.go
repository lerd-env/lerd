// Package phpini centralizes the php.ini editor's scope handling so the CLI, the
// web UI, and the MCP server share one definition of what a scope is, which file
// it maps to, and which containers a change restarts. A scope is one of:
//
//	"shared"        the version-agnostic file applied to every PHP version
//	"<version>"     an installed PHP version's own file (e.g. "8.4")
//	"site:<name>"   a FrankenPHP site's own per-site file
//
// The shared file mounts below the per-version file in every container's conf.d,
// so a per-version (or per-site) key always overrides a shared one.
package phpini

import (
	"fmt"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/cfgedit"
	"github.com/geodro/lerd/internal/config"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

// SharedScope is the editor scope for the version-agnostic shared php.ini.
const SharedScope = "shared"

// UserTemplate seeds the editor when a per-version or per-site file does not
// exist yet. Matches the stub the podman EnsureUserIni writes so the editor
// shows the same guidance.
const UserTemplate = `; Lerd per-version PHP settings.
;
; Edit this file, then click Save to write it and restart FPM.
; Any key set here overrides the shared file (php:ini shared).
;
; memory_limit = 512M
; opcache.memory_consumption = 256
; realpath_cache_size = 4096k
; realpath_cache_ttl = 600
`

// SharedTemplate seeds the editor for the shared file. Applies to every version,
// with a per-version file winning on any conflicting key.
const SharedTemplate = `; Lerd shared PHP settings, applied to every PHP version.
;
; A per-version file (php:ini <version>) overrides any key set here. Unknown
; keys on a given version are ignored, not fatal, so a version-specific setting
; never breaks the others.
;
; memory_limit = 512M
; upload_max_filesize = 64M
; post_max_size = 64M
; max_execution_time = 60
`

// siteName decodes a "site:<name>" scope, returning the name and true; any other
// scope returns ("", false).
func siteName(scope string) (string, bool) {
	return strings.CutPrefix(scope, "site:")
}

// Valid reports whether a scope is a valid php.ini editor scope: the shared
// file, an installed PHP version, or "site:<name>" for a FrankenPHP site.
func Valid(scope string) bool {
	if scope == SharedScope {
		return true
	}
	if name, ok := siteName(scope); ok {
		s, err := config.FindSite(name)
		return err == nil && s != nil && s.IsFrankenPHP()
	}
	installed, _ := phpPkg.ListInstalled()
	return slices.Contains(installed, scope)
}

// ScopeFile returns the cfgedit.File a scope reads and writes: the shared file,
// a FrankenPHP site's own file, or the per-version file.
func ScopeFile(scope string) cfgedit.File {
	if scope == SharedScope {
		return cfgedit.File{
			Path:     config.SharedIniFile(),
			BkpDir:   config.SharedIniBkpDir(),
			BkpName:  "95-shared.ini",
			Template: SharedTemplate,
		}
	}
	if name, ok := siteName(scope); ok {
		return cfgedit.File{
			Path:     config.SitePHPUserIniFile(name),
			BkpDir:   config.SitePHPUserIniBkpDir(name),
			BkpName:  "98-user.ini",
			Template: UserTemplate,
		}
	}
	return cfgedit.File{
		Path:     config.PHPUserIniFile(scope),
		BkpDir:   config.PHPUserIniBkpDir(scope),
		BkpName:  "98-user.ini",
		Template: UserTemplate,
	}
}

// Ensure seeds the on-disk file backing a scope so the bind-mount source stays a
// regular file (podman would otherwise auto-create a directory at the conf.d
// path). Safe to call repeatedly; it never overwrites an existing file.
func Ensure(scope string) error {
	if scope == SharedScope {
		return podman.EnsureSharedIni()
	}
	if name, ok := siteName(scope); ok {
		return podman.EnsureSitePHPUserIni(name)
	}
	return podman.EnsureUserIni(scope)
}

// Restart applies a scope's ini change by restarting the containers that mount
// it. For the shared file that is every installed version's FPM plus its
// per-site containers; for a version, that one FPM plus its per-site containers;
// for a site, just that FrankenPHP container.
func Restart(scope string) error {
	if scope == SharedScope {
		return restartAllVersions()
	}
	if name, ok := siteName(scope); ok {
		return restartFrankenPHPSite(name)
	}
	return restartVersion(scope)
}

// RestartNoSeed restarts a scope's container(s) after a reset. The per-version
// file is not re-seeded (the shared FPM tolerates its absence: only that one
// version's container mounts it, and the reset means "no per-version override").
// The shared file IS re-seeded first, because every PHP container mounts it and
// a missing bind-mount source makes them all fail to start; re-seeding the
// commented template is functionally "no shared override" while keeping the
// mount valid. A FrankenPHP site is re-seeded for the same bind-mount reason.
func RestartNoSeed(scope string) error {
	if scope == SharedScope {
		_ = podman.EnsureSharedIni()
		var firstErr error
		for _, v := range installedVersions() {
			if err := restartFPMUnit(v); err != nil && firstErr == nil {
				firstErr = err
			}
			podman.RestartSiteContainersForVersion(v)
		}
		return firstErr
	}
	if name, ok := siteName(scope); ok {
		site, err := config.FindSite(name)
		if err != nil {
			return err
		}
		_ = podman.EnsureSitePHPUserIni(name)
		return podman.RestartUnit(podman.FrankenPHPContainerName(site.Name))
	}
	return restartFPMUnit(scope)
}

// restartAllVersions rewrites and restarts every installed version's FPM plus
// its per-site containers, so a shared-ini change reaches every PHP container.
func restartAllVersions() error {
	var firstErr error
	for _, v := range installedVersions() {
		if err := restartVersion(v); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func restartVersion(version string) error {
	if err := podman.WriteFPMQuadlet(version); err != nil {
		return fmt.Errorf("updating php quadlet: %w", err)
	}
	if err := restartFPMUnit(version); err != nil {
		return err
	}
	podman.RestartSiteContainersForVersion(version)
	return nil
}

func restartFrankenPHPSite(name string) error {
	site, err := config.FindSite(name)
	if err != nil {
		return err
	}
	entrypoint, env := site.FrankenPHPQuadletSpec()
	if err := podman.WriteFrankenPHPQuadlet(site.Name, site.Path, site.PHPVersion, entrypoint, env); err != nil {
		return fmt.Errorf("updating FrankenPHP quadlet: %w", err)
	}
	return podman.RestartUnit(podman.FrankenPHPContainerName(site.Name))
}

func restartFPMUnit(version string) error {
	short := strings.ReplaceAll(version, ".", "")
	return podman.RestartUnit("lerd-php" + short + "-fpm")
}

func installedVersions() []string {
	v, _ := phpPkg.ListInstalled()
	return v
}
