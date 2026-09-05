// Package logsource gives a single, filtered view over every log lerd can
// reach: framework application log files, PHP-FPM/nginx/dns/service container
// stdout, and worker/watcher/ui units (systemd journal on Linux, launchd log
// files on macOS). It wraps the existing readers (internal/applog, podman
// logs, journalctl) behind one Source registry + Read dispatcher so callers —
// the MCP `logs` tool, the `diag.logs` alias — never reimplement log access.
package logsource

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/applog"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

var phpVerRe = regexp.MustCompile(`^\d+\.\d+$`)

// Kind is the physical backend a Source is read from.
type Kind int

const (
	KindFile    Kind = iota // application log file on disk (real timestamps)
	KindPodman              // container stdout via `podman logs`
	KindJournal             // systemd journal (Linux) / launchd log file (macOS)
)

func (k Kind) String() string {
	switch k {
	case KindFile:
		return "file"
	case KindPodman:
		return "podman"
	case KindJournal:
		return "journal"
	}
	return "unknown"
}

// Scope distinguishes per-site sources from shared infrastructure.
type Scope string

const (
	ScopeSite   Scope = "site"
	ScopeGlobal Scope = "global"
)

// Source is one addressable log stream. Locator is a file path (KindFile), a
// container name (KindPodman), or a unit name (KindJournal).
type Source struct {
	Name    string
	Kind    Kind
	Locator string
	Scope   Scope
	Format  string // "monolog"|"raw" for KindFile, empty otherwise
	Label   string
}

// Sources enumerates every queryable source for the given site context plus the
// always-present global infrastructure sources. Either siteName or sitePath may
// be empty; when neither resolves to a registered site only globals are returned.
func Sources(siteName, sitePath string) ([]Source, error) {
	var out []Source
	if site := resolveSite(siteName, sitePath); site != nil {
		out = append(out, siteSources(site)...)
	}
	out = append(out, globalSources()...)
	return out, nil
}

// Resolve looks up a single source by name within the current context. It
// resolves the deterministic names (fpm, worker:*, globals) directly so a fetch
// doesn't pay for a full Sources() enumeration (framework resolution, log-file
// globbing, installed-PHP discovery); only app:* files and the unknown-name
// error path fall back to the full listing.
func Resolve(siteName, sitePath, name string) (Source, error) {
	site := resolveSite(siteName, sitePath)
	if site != nil {
		if s, ok := resolveSiteDirect(site, name); ok {
			return s, nil
		}
	}
	if s, ok := globalDirect(name); ok {
		return s, nil
	}

	srcs, err := Sources(siteName, sitePath)
	if err != nil {
		return Source{}, err
	}
	names := make([]string, 0, len(srcs))
	for _, s := range srcs {
		if s.Name == name {
			return s, nil
		}
		names = append(names, s.Name)
	}
	return Source{}, fmt.Errorf("unknown log source %q; valid: %s", name, strings.Join(names, ", "))
}

// resolveSiteDirect resolves the non-file site sources by name without touching
// the framework definition or globbing the log directory.
func resolveSiteDirect(site *config.Site, name string) (Source, bool) {
	if name == "fpm" {
		if c := FPMContainer(site); c != "" {
			return fpmSource(site, c), true
		}
		return Source{}, false
	}
	if worker, ok := strings.CutPrefix(name, "worker:"); ok {
		if proj, err := config.LoadProjectConfig(site.Path); err == nil && projectHasWorker(proj, worker) {
			return workerSource(site.Name, worker), true
		}
	}
	return Source{}, false
}

func projectHasWorker(proj *config.ProjectConfig, worker string) bool {
	for _, w := range proj.Workers {
		if w == worker {
			return true
		}
	}
	_, ok := proj.CustomWorkers[worker]
	return ok
}

func resolveSite(siteName, sitePath string) *config.Site {
	if siteName != "" {
		if s, err := config.FindSite(siteName); err == nil {
			return s
		}
	}
	if sitePath != "" {
		if s, err := config.FindSiteByPath(sitePath); err == nil {
			return s
		}
	}
	return nil
}

func siteSources(site *config.Site) []Source {
	var out []Source

	// 1. Application log files discovered from the framework definition.
	if fw, ok := config.GetFrameworkForDir(site.Framework, site.Path); ok && len(fw.Logs) > 0 {
		files, _ := applog.DiscoverLogFiles(site.Path, fw.Logs)
		for _, f := range files {
			path := applog.ResolveLogFilePath(site.Path, fw.Logs, f.Name)
			if path == "" {
				continue
			}
			out = append(out, Source{
				Name:    "app:" + f.Name,
				Kind:    KindFile,
				Locator: path,
				Scope:   ScopeSite,
				Format:  applog.FormatForFile(fw.Logs, f.Name),
				Label:   "app log " + f.Name + " (" + site.Name + ")",
			})
		}
	}

	// 2. The site's primary app container (shared FPM, or its own custom /
	//    FrankenPHP / custom-FPM container). Skipped for host-proxy sites that
	//    run no container.
	if c := FPMContainer(site); c != "" {
		out = append(out, fpmSource(site, c))
	}

	// 3. Worker units declared by the project.
	if proj, err := config.LoadProjectConfig(site.Path); err == nil {
		for _, w := range proj.Workers {
			out = append(out, workerSource(site.Name, w))
		}
		names := make([]string, 0, len(proj.CustomWorkers))
		for w := range proj.CustomWorkers {
			names = append(names, w)
		}
		sort.Strings(names)
		for _, w := range names {
			out = append(out, workerSource(site.Name, w))
		}
	}

	return out
}

// FPMContainer resolves the container that serves a site: its own custom /
// FrankenPHP / custom-FPM container when applicable, otherwise the shared
// lerd-php<version>-fpm. Returns "" for host-proxy sites, which run no
// container. Mirrors cli.resolveWorkerFPMUnit so logs target the same place
// workers exec into.
func FPMContainer(site *config.Site) string {
	switch {
	case site.IsHostProxy():
		return ""
	case site.IsCustomContainer():
		return podman.CustomContainerName(site.Name)
	case site.IsFrankenPHP():
		return podman.FrankenPHPContainerName(site.Name)
	case site.IsCustomFPM():
		return podman.CustomFPMContainerName(site.Name)
	}
	v := site.PHPVersion
	if detected, err := phpDet.DetectVersion(site.Path); err == nil && detected != "" {
		v = detected
	}
	cfg, err := config.LoadGlobal()
	return fpmTarget(site, v, err == nil && cfg.PHPRuntimeMode() == config.PHPRuntimeNative)
}

// fpmTarget names the unit a site's PHP log comes from. Under the native
// runtime the shared container is stopped on purpose, so reading it would show
// a log frozen at the moment of the switch rather than what is serving now.
func fpmTarget(site *config.Site, version string, native bool) string {
	if native && !site.IsFrankenPHP() && !site.IsCustomFPM() && !site.IsCustomContainer() && !site.IsHostProxy() {
		return nativephp.UnitLabel(version)
	}
	return "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
}

func fpmSource(site *config.Site, container string) Source {
	return Source{Name: "fpm", Kind: KindPodman, Locator: container, Scope: ScopeSite, Label: "PHP-FPM (" + site.Name + ")"}
}

func workerSource(siteName, worker string) Source {
	return Source{
		Name:    "worker:" + worker,
		Kind:    KindJournal,
		Locator: "lerd-" + worker + "-" + siteName,
		Scope:   ScopeSite,
		Label:   worker + " worker (" + siteName + ")",
	}
}

// staticGlobals are the fixed infrastructure sources present on every machine.
func staticGlobals() []Source {
	return []Source{
		{Name: "nginx", Kind: KindPodman, Locator: "lerd-nginx", Scope: ScopeGlobal, Label: "nginx"},
		{Name: "dns", Kind: KindPodman, Locator: "lerd-dns", Scope: ScopeGlobal, Label: "dnsmasq"},
		{Name: "watcher", Kind: KindJournal, Locator: "lerd-watcher", Scope: ScopeGlobal, Label: "file watcher"},
		{Name: "ui", Kind: KindJournal, Locator: "lerd-ui", Scope: ScopeGlobal, Label: "lerd UI server"},
	}
}

func serviceSource(name string) Source {
	return Source{Name: name, Kind: KindPodman, Locator: "lerd-" + name, Scope: ScopeGlobal, Label: name + " service"}
}

func phpSource(version string) Source {
	return Source{
		Name:    "php" + version,
		Kind:    KindPodman,
		Locator: "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm",
		Scope:   ScopeGlobal,
		Label:   "PHP-FPM " + version,
	}
}

func globalSources() []Source {
	out := staticGlobals()
	for _, svc := range config.DefaultPresetNames() {
		out = append(out, serviceSource(svc))
	}
	if versions, err := phpDet.ListInstalled(); err == nil {
		for _, v := range versions {
			out = append(out, phpSource(v))
		}
	}
	return out
}

// globalDirect resolves a global source by name without enumerating services or
// probing installed PHP versions.
func globalDirect(name string) (Source, bool) {
	for _, s := range staticGlobals() {
		if s.Name == name {
			return s, true
		}
	}
	if config.IsDefaultPreset(name) {
		return serviceSource(name), true
	}
	if v, ok := strings.CutPrefix(name, "php"); ok && phpVerRe.MatchString(v) {
		return phpSource(v), true
	}
	return Source{}, false
}
