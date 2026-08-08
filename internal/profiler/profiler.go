// Package profiler turns the SPX profiler on or off globally. Arming flips a
// config flag and regenerates every PHP-FPM site's nginx vhost so the
// SPX_ENABLED cookie is injected into each request; disarming reverses it.
// No FPM restart is involved, only an nginx reload.
package profiler

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/nginx"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/siteops"
)

// SpxUIURL is the standalone SPX profiler web UI, served by the
// profiler.localhost nginx vhost. The dashboard embeds the same UI same-origin
// under /_spx/; this URL opens it directly (lerd profile open, MCP status).
const SpxUIURL = "http://profiler.localhost/?SPX_UI_URI=/"

// nginxReloadFn is the nginx reload hook, and servedStateFn the readiness probe.
// Both are swapped out in tests.
var (
	nginxReloadFn = nginx.Reload
	servedStateFn = nginx.ServedProfilerState
)

// How long SetProfiling waits for nginx to serve the toggle it was just given,
// and how often it asks. A reload settles in tens of milliseconds; the ceiling is
// only there so a stopped or wedged nginx cannot hold the caller.
var (
	servingTimeout = 5 * time.Second
	servingPoll    = 50 * time.Millisecond
)

// Result reports the outcome of a SetProfiling call.
type Result struct {
	Enabled  bool `json:"enabled"`
	NoChange bool `json:"no_change"`
}

// SetProfiling turns the SPX profiler on or off globally. When on, every
// PHP-FPM site's requests are profiled. The change regenerates each FPM
// site's vhost and reloads nginx.
func SetProfiling(on bool) (Result, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return Result{}, err
	}
	if cfg.IsProfilerEnabled() == on {
		return Result{Enabled: on, NoChange: true}, nil
	}
	cfg.Profiler.Enabled = on
	if err := config.SaveGlobal(cfg); err != nil {
		return Result{}, fmt.Errorf("saving config: %w", err)
	}
	if err := regenerateVhosts(); err != nil {
		return Result{}, err
	}
	// The state marker rides along on the profiler vhost, so it has to be written
	// before nginx is signalled or the probe below would confirm the old setting.
	if err := nginx.EnsureProfilerVhost(); err != nil {
		return Result{}, fmt.Errorf("writing profiler vhost: %w", err)
	}
	if err := nginxReloadFn(); err != nil {
		return Result{}, fmt.Errorf("reloading nginx: %w", err)
	}
	waitUntilServing(on)
	return Result{Enabled: on}, nil
}

// waitUntilServing blocks until nginx answers with the setting it was just
// given. A reload signals nginx rather than swapping its configuration in place:
// the old workers keep serving while the new ones start, so a request sent the
// instant the reload returns can still be handled with no profiler attached, and
// nothing profiles it. Gives up quietly at the deadline, since the setting was
// applied either way and an unconfirmable probe is not a reason to fail.
func waitUntilServing(on bool) {
	deadline := time.Now().Add(servingTimeout)
	for {
		if served, ok := servedStateFn(); ok && served == on {
			return
		}
		if !time.Now().Before(deadline) {
			return
		}
		time.Sleep(servingPoll)
	}
}

// ClearData deletes every captured SPX report from the profiler data
// directory. The directory itself is kept so the read-write bind mount into
// each FPM container stays valid; only its contents go. A missing or empty
// directory is not an error. Returns how many top-level entries were removed.
func ClearData() (int, error) {
	dir := config.SpxDataDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading spx data dir: %w", err)
	}
	removed := 0
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return removed, fmt.Errorf("removing %s: %w", e.Name(), err)
		}
		removed++
	}
	return removed, nil
}

// Profilable reports whether a site's requests can be profiled at all: SPX
// lives in the FPM image, so it takes a PHP site served by FPM. FrankenPHP
// serves PHP from its own image without the extension, and custom-container
// and host-proxy sites run no PHP of lerd's. The dashboard gates its
// profile-this-route action on the same rule the vhost rewrite uses.
func Profilable(s config.Site) bool {
	return ProfilableSite(s, phpPkg.SiteUsesPHP(s))
}

// ProfilableSite is Profilable for callers that already know whether the site
// is a PHP project, so the detection isn't repeated against the disk.
func ProfilableSite(s config.Site, usesPHP bool) bool {
	return servedByFPM(s) && usesPHP
}

// servedByFPM reports whether a site's requests go through an FPM vhost, the
// one place the SPX cookie can be injected.
func servedByFPM(s config.Site) bool {
	return !s.IsCustomContainer() && !s.IsFrankenPHP() && !s.IsHostProxy()
}

// regenerateVhosts rewrites the vhost of every active PHP-FPM site so the
// SPX_ENABLED injection reflects the current toggle. Paused, ignored,
// custom-container and FrankenPHP sites are skipped: they have no FPM vhost
// to profile, and regenerating a paused site would revive it.
func regenerateVhosts() error {
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}
	for i := range reg.Sites {
		s := reg.Sites[i]
		if s.Ignored || s.Paused || !servedByFPM(s) {
			continue
		}
		if err := siteops.RegenerateSiteVhost(&s, s.PrimaryDomain()); err != nil {
			return fmt.Errorf("regenerating vhost for %s: %w", s.Name, err)
		}
		// Worktree vhosts share the site template, so they need the toggle too.
		worktrees, err := gitpkg.DetectWorktrees(s.Path, s.PrimaryDomain())
		if err != nil {
			continue
		}
		for _, wt := range worktrees {
			php := config.WorktreePHPVersion(wt.Path, s.PHPVersion)
			_ = nginx.GenerateWorktreeVhostFor(wt.Domain, wt.Path, php, s.PrimaryDomain(), s.Name, wt.Branch, s.Secured)
		}
	}
	return nil
}
