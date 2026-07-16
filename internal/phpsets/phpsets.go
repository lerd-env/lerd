// Package phpsets answers what a PHP version's image actually carries, as
// opposed to what the user declared. The declared extension/package set applies
// to every version, but a version cannot always honour it (mongodb needs 8.1+,
// the 7.4/8.0 images are Alpine 3.16) and an image built before an entry was
// declared simply predates it. Every surface (CLI, web UI, TUI, MCP) reports
// from here so none of them can advertise what an image does not have.
package phpsets

import (
	"bufio"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// Seams so the report can be driven without podman in tests.
var (
	imageExistsFn = podman.FPMImageExists
	imageStaleFn  = podman.FPMImageStale
	imageIDFn     = podman.FPMImageID
	readModulesFn = podman.FPMPHPModules
)

// SetState is one declared set measured against a single version's image.
type SetState struct {
	// Declared is what the user asked for, everywhere.
	Declared []string `json:"declared"`
	// Has is what this image really carries.
	Has []string `json:"has"`
	// Cannot is what this image tried to load and could not. A rebuild will
	// not change these: they did not build on this version.
	Cannot []string `json:"cannot"`
}

// Report is the state of one PHP version's image.
type Report struct {
	Version string `json:"version"`
	// Built is false when lerd has never built an image for this version, in
	// which case nothing is claimed about its contents.
	Built bool `json:"built"`
	// NeedsRebuild means the image was built from an older declared set, so it
	// predates entries added since. A rebuild does fix this.
	NeedsRebuild bool     `json:"needs_rebuild"`
	Extensions   SetState `json:"extensions"`
	Packages     SetState `json:"packages"`
	// Modules is what `php -m` reports, filled in only by ModulesReport: it
	// starts a container, so the cheap per-version status never pays for it.
	Modules []string `json:"modules,omitempty"`
}

// VersionStatus measures the declared sets against one version's image. Cheap:
// it reads config and two image labels, and never starts a container.
func VersionStatus(cfg *config.GlobalConfig, version string) Report {
	r := Report{Version: version}
	r.Extensions.Declared = cfg.GetExtensions()
	r.Packages.Declared = cfg.GetPackages()

	if !imageExistsFn(version) {
		return r
	}
	r.Built = true
	if imageStaleFn(version) {
		// Its realised record describes an older set, so calling any entry
		// present or unbuildable from it would be a fresh lie.
		r.NeedsRebuild = true
		return r
	}
	r.Extensions.Has, r.Extensions.Cannot = split(cfg, version, r.Extensions.Declared)
	r.Packages.Has, r.Packages.Cannot = split(cfg, version, r.Packages.Declared)
	return r
}

// StatusAll reports every version given, in order.
func StatusAll(cfg *config.GlobalConfig, versions []string) []Report {
	out := make([]Report, 0, len(versions))
	for _, v := range versions {
		out = append(out, VersionStatus(cfg, v))
	}
	return out
}

// split partitions the declared set into what the image has and what it could
// not load, from the record written when it was built.
func split(cfg *config.GlobalConfig, version string, declared []string) (has, cannot []string) {
	if len(declared) == 0 {
		return nil, nil
	}
	cannot = cfg.MissingFromImage(version, declared)
	has = slices.DeleteFunc(slices.Clone(declared), func(e string) bool { return slices.Contains(cannot, e) })
	if len(has) == 0 {
		has = nil
	}
	return has, cannot
}

// ModulesReport is VersionStatus plus the image's own `php -m`, for surfaces
// that show a version in detail.
func ModulesReport(cfg *config.GlobalConfig, version string) (Report, error) {
	r := VersionStatus(cfg, version)
	if !r.Built {
		return r, nil
	}
	mods, err := Modules(version)
	if err != nil {
		return r, err
	}
	r.Modules = mods
	return r, nil
}

// modulesCache keys `php -m` output by the image it came from. The same image
// can never report different modules, and a rebuild changes the ID, so this is
// exact rather than a TTL guess.
var modulesCache = &moduleCache{byVersion: map[string]cachedModules{}}

type cachedModules struct {
	imageID string
	modules []string
}

type moduleCache struct {
	mu        sync.Mutex
	byVersion map[string]cachedModules
}

func (c *moduleCache) get(version, imageID string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	got, ok := c.byVersion[version]
	if !ok || got.imageID != imageID {
		return nil, false
	}
	return got.modules, true
}

func (c *moduleCache) put(version, imageID string, modules []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byVersion[version] = cachedModules{imageID: imageID, modules: modules}
}

func (c *moduleCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byVersion = map[string]cachedModules{}
}

// Modules returns the extensions a version's image actually loads, read from
// its own `php -m`. Empty (not an error) for a version with no image: absence
// of an image is not evidence about its contents.
func Modules(version string) ([]string, error) {
	imageID := imageIDFn(version)
	if imageID == "" {
		return nil, nil
	}
	if mods, ok := modulesCache.get(version, imageID); ok {
		return mods, nil
	}
	out, err := readModulesFn(version)
	if err != nil {
		return nil, err
	}
	mods := parseModules(out)
	modulesCache.put(version, imageID, mods)
	return mods, nil
}

// parseModules folds `php -m` into the names users install: the output prints
// display names, so "Zend OPcache" becomes opcache and the [PHP Modules] /
// [Zend Modules] headers are skipped. Sorted and deduped, since Zend extensions
// are listed twice.
func parseModules(out string) []string {
	seen := map[string]bool{}
	var mods []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		name := podman.CanonicalExtension(line)
		if seen[name] {
			continue
		}
		seen[name] = true
		mods = append(mods, name)
	}
	sort.Strings(mods)
	return mods
}
