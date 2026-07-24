package linker

import (
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/geodro/lerd/internal/store"
)

// Resolve decides everything about linking dir as a site without changing
// anything. It reads the directory, the project's .lerd.yaml and the site
// registry, and returns the plan Apply carries out. A plan whose Skip is set
// registers nothing.
func Resolve(dir string, cfg *config.GlobalConfig, p Policy) (*Plan, error) {
	plan := &Plan{Dir: dir}

	if parent, branch, ok := OwningWorktree(dir); ok {
		plan.Skip = SkipWorktree
		plan.WorktreeParent, plan.WorktreeBranch = parent, branch
		plan.SkipDetail = "the " + branch + " worktree of site " + parent.Name +
			"; worktrees inherit the parent's registration"
		return plan, nil
	}

	if p.SkipRegistered {
		if existing, err := config.FindSiteByPath(dir); err == nil && existing != nil {
			plan.Skip = SkipRegistered
			plan.SkipDetail = "already registered as " + existing.Name
			return plan, nil
		}
	}

	proj, _ := config.LoadProjectConfig(dir)
	plan.Project = proj

	rawName := filepath.Base(dir)
	if p.Name != "" {
		rawName = p.Name
	}
	baseName, _ := siteops.SiteNameAndDomain(rawName, cfg.DNS.TLD)
	name := FreeSiteName(baseName, dir)

	kept, removed := ResolveDomains(desiredDomains(proj, p.Name, name, cfg.DNS.TLD), baseName, dir, cfg.DNS.TLD)
	plan.DroppedDomains = removed

	secured := false
	if p.Certs {
		secured = siteops.ResolveSecured(relinkSecured(dir), proj, cfg)
	}

	// A project that runs in its own container, or a dev server on the host,
	// is served by proxy: no PHP version, no framework, no runtime to pick.
	if proj != nil && proj.Container != nil && proj.Container.Port > 0 {
		plan.Mode = ModeCustomContainer
		plan.Site = config.Site{
			Name:          name,
			Domains:       kept,
			Path:          dir,
			Secured:       secured,
			ContainerPort: proj.Container.Port,
			ContainerSSL:  proj.Container.SSL,
		}
		return plan, nil
	}
	if proj != nil && proj.Proxy != nil && proj.Proxy.Port > 0 {
		plan.Mode = ModeHostProxy
		plan.ProxyCommand = proj.Proxy.Command
		plan.Site = config.Site{
			Name:        name,
			Domains:     kept,
			Path:        dir,
			Secured:     secured,
			HostPort:    proj.Proxy.Port,
			HostSSL:     proj.Proxy.SSL,
			HostCommand: proj.Proxy.Command,
		}
		return plan, nil
	}

	framework, known := ResolveFramework(dir, p.Prompt != nil)
	publicDir := ""
	switch {
	case proj != nil && proj.PublicDir != "":
		publicDir = proj.PublicDir
	case !known:
		publicDir = config.DetectPublicDir(dir)
	}

	plan.FrameworkLabel = framework
	if framework == "" {
		plan.FrameworkLabel = "unknown (public: " + publicDir + ")"
	}

	versions := siteops.DetectSiteVersions(dir, framework, cfg.PHP.DefaultVersion, cfg.Node.DefaultVersion)
	phpVersion, nodeVersion := versions.PHP, versions.Node
	if proj != nil && proj.PHPVersion != "" {
		phpVersion = phpDet.ClampToRange(proj.PHPVersion, versions.PHPMin, versions.PHPMax)
	}
	// A version the framework's range moved us off is worth reporting, and when
	// a better one could be installed the caller may offer to build it.
	if versions.PHPMin != "" || versions.PHPMax != "" {
		if unclamped, _ := phpDet.DetectVersion(dir); unclamped != phpVersion {
			plan.PHPMin, plan.PHPMax = versions.PHPMin, versions.PHPMax
			if p.ImageBuild {
				plan.PHPSuggestion = versions.SuggestedPHP
			}
		}
	}

	site := config.Site{
		Name:        name,
		Domains:     kept,
		Path:        dir,
		PHPVersion:  phpVersion,
		NodeVersion: nodeVersion,
		Secured:     secured,
		Framework:   framework,
		PublicDir:   publicDir,
	}

	// A committed runtime rehydrates the site on a re-link rather than resetting
	// it to FPM. FrankenPHP publishes no image below PHP 8.2, and normalizing the
	// version up would silently run a different PHP than the site reports, so an
	// unsupported version falls back to FPM instead.
	if proj != nil && proj.Runtime != "" {
		site.Runtime = proj.Runtime
		site.RuntimeWorker = proj.RuntimeWorker
		if site.IsFrankenPHP() && !config.IsFrankenPHPVersion(site.PHPVersion) {
			site.Runtime = ""
			site.RuntimeWorker = false
			plan.FrankenPHPDeclined = true
		}
	}

	// A container section with no port on a PHP project means the site is served
	// by fastcgi from its own image. The Containerfile's FROM line fixes the PHP
	// version, so the ini mounts match the image the container really runs.
	if proj != nil && proj.Container != nil && proj.Container.Port == 0 {
		site.Runtime = "fpm-custom"
		if v := podman.CustomFPMBaseVersion(dir, proj.Container); v != "" {
			site.PHPVersion = v
		}
	}

	plan.Site = site
	switch {
	case site.IsCustomFPM():
		plan.Mode = ModeCustomFPM
	case site.IsFrankenPHP():
		plan.Mode = ModeFrankenPHP
	default:
		plan.Mode = ModeFPM
	}
	return plan, nil
}

// desiredDomains builds the domain list to attempt, before conflict filtering.
// A .lerd.yaml domains list wins over the generated one, and an explicitly
// requested name is forced to the front so it becomes the primary domain.
func desiredDomains(proj *config.ProjectConfig, requested, name, tld string) []string {
	var domains []string
	if proj != nil && len(proj.Domains) > 0 {
		for _, d := range proj.Domains {
			domains = append(domains, strings.ToLower(d)+"."+tld)
		}
	} else {
		domains = []string{name + "." + tld}
	}
	if requested == "" {
		return domains
	}

	explicit := strings.ToLower(requested) + "." + tld
	filtered := make([]string, 0, len(domains))
	for _, d := range domains {
		if d != explicit {
			filtered = append(filtered, d)
		}
	}
	return append([]string{explicit}, filtered...)
}

// relinkSecured reports whether any registration already at this path is
// secured, so a re-link carries HTTPS over. It is the read-only half of
// siteops.CleanupRelink, which Apply calls for the removal it also does.
func relinkSecured(path string) bool {
	reg, err := config.LoadSites()
	if err != nil {
		return false
	}
	for _, existing := range reg.Sites {
		if existing.Path == path && existing.Secured {
			return true
		}
	}
	return false
}

// ResolveFramework names the framework for dir. The store fallback asks the
// user which definition to install, so it is only reachable when the caller can
// put a question on screen.
func ResolveFramework(dir string, allowStoreFallback bool) (string, bool) {
	if name, ok := config.DetectFrameworkForDir(dir); ok {
		return name, true
	}
	if !allowStoreFallback {
		return "", false
	}
	return store.DetectFrameworkWithStore(dir)
}

// OwningWorktree returns the site dir is a git worktree of, so a checkout under
// a registered project is not registered again as a site of its own.
func OwningWorktree(dir string) (*config.Site, string, bool) {
	reg, err := config.LoadSites()
	if err != nil {
		return nil, "", false
	}
	for i := range reg.Sites {
		s := &reg.Sites[i]
		if s.Ignored || s.Path == dir {
			continue
		}
		wts, _ := gitpkg.DetectWorktrees(s.Path, s.PrimaryDomain())
		for _, wt := range wts {
			if wt.Path == dir {
				return s, wt.Branch, true
			}
		}
	}
	return nil, "", false
}
