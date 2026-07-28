package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/freeport"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/nginx"
	phpDet "github.com/geodro/lerd/internal/php"
)

// A host dev server normally advertises its own address, so everything it
// serves points at localhost on a port only this machine can reach. Behind a
// tunnel, on the LAN, or on a worktree domain, that address means nothing to
// the visitor and the page arrives unstyled. Giving the server a base prefix
// and proxying that one prefix from the site's vhost puts assets and the
// hot-reload socket on the site's own origin, where they follow whatever
// hostname the visitor actually used.

// devServerProjectConfig returns the first declared config filename that the
// project actually has, or empty when it has none of them.
func devServerProjectConfig(sitePath string, tool *config.DevServerTool) string {
	for _, name := range tool.ProjectConfig {
		if _, err := os.Stat(filepath.Join(sitePath, name)); err == nil {
			return name
		}
	}
	return ""
}

// pathIsIgnored reports whether the project's own git config already ignores
// rel. A project that would track the generated file is left alone: nobody
// should find lerd's plumbing staged in their commit. The index is deliberately
// consulted, so a path someone has actually committed counts as not ignored
// even when a rule would otherwise cover it.
func pathIsIgnored(sitePath, rel string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", rel)
	cmd.Dir = sitePath
	err := cmd.Run()
	if err == nil {
		return true
	}
	// Exit 1 is git's answer that the path is not ignored. Anything else means
	// it could not answer at all, which for a project outside git is not a
	// refusal: there is no commit for a generated file to end up in.
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false
	}
	return true
}

// writeDevServerWrapper generates the config that puts the dev server on the
// site's origin and returns its site-relative path. An empty path means the
// mechanism does not apply here and the caller must leave the command alone.
func writeDevServerWrapper(sitePath string, tool *config.DevServerTool, origin string, domains []string) (string, error) {
	projectConfig := devServerProjectConfig(sitePath, tool)
	if projectConfig == "" {
		return "", nil
	}
	if !pathIsIgnored(sitePath, tool.WrapperPath) {
		return "", nil
	}

	importPath, err := filepath.Rel(filepath.Dir(tool.WrapperPath), projectConfig)
	if err != nil {
		return "", err
	}
	importPath = filepath.ToSlash(importPath)
	if !strings.HasPrefix(importPath, ".") {
		importPath = "./" + importPath
	}
	hosts, err := json.Marshal(domains)
	if err != nil {
		return "", err
	}

	full := filepath.Join(sitePath, tool.WrapperPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return "", err
	}
	body := fmt.Sprintf(tool.Wrapper, importPath, tool.Base, origin, hosts)
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		return "", err
	}
	return tool.WrapperPath, nil
}

// devServerOrigin returns the URL the dev server should advertise and the
// hostnames it must accept, which is the site's own address rather than the
// tool's localhost guess.
func devServerOrigin(site *config.Site) (string, []string) {
	scheme := "http"
	if site.Secured {
		scheme = "https"
	}
	domains := site.Domains
	if len(domains) == 0 {
		domains = []string{site.PrimaryDomain()}
	}
	return scheme + "://" + site.PrimaryDomain(), domains
}

// devServerAddress returns the origin and accepted hostnames for whichever
// checkout is starting: the site itself, or one of its worktrees on its own
// subdomain.
func devServerAddress(site *config.Site, sitePath string) (string, []string, error) {
	if isSiteItself(site, sitePath) {
		origin, domains := devServerOrigin(site)
		return origin, domains, nil
	}
	wt, ok := worktreeForPath(site, sitePath)
	if !ok {
		return "", nil, fmt.Errorf("no worktree registered for %s", sitePath)
	}
	scheme := "http"
	if site.Secured {
		scheme = "https"
	}
	return scheme + "://" + wt.Domain, []string{wt.Domain}, nil
}

func isSiteItself(site *config.Site, sitePath string) bool {
	return config.CanonicalPath(sitePath) == config.CanonicalPath(site.Path)
}

// worktreeForPath resolves a checkout directory to the worktree git knows about.
func worktreeForPath(site *config.Site, sitePath string) (gitpkg.Worktree, bool) {
	wts, err := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return gitpkg.Worktree{}, false
	}
	for _, wt := range wts {
		if config.CanonicalPath(wt.Path) == config.CanonicalPath(sitePath) {
			return wt, true
		}
	}
	return gitpkg.Worktree{}, false
}

// regenSiteOrWorktreeVhost rewrites whichever vhost fronts this checkout so the
// dev server locations land on the domain that actually serves it.
func regenSiteOrWorktreeVhost(site *config.Site, sitePath string) {
	if isSiteItself(site, sitePath) {
		regenNginxVhost(site.Name, sitePath)
		return
	}
	wt, ok := worktreeForPath(site, sitePath)
	if !ok {
		return
	}
	phpVer := site.PHPVersion
	if detected, err := phpDet.DetectVersion(sitePath); err == nil && detected != "" {
		phpVer = detected
	}
	if err := nginx.GenerateWorktreeVhostFor(wt.Domain, sitePath, phpVer, site.PrimaryDomain(), site.Name, wt.Branch, site.Secured); err == nil {
		_ = nginx.Reload()
	}
}

// devServerEnsurePort pins the site's dev server to a stable host port and
// saves it, so the vhost generated later proxies somewhere the tool will
// actually be listening.
func devServerEnsurePort(siteName, sitePath string, defaultPort int) (int, error) {
	site, err := config.FindSite(siteName)
	if err != nil {
		return 0, err
	}
	key := ""
	current := site.DevServerPort
	if !isSiteItself(site, sitePath) {
		key = filepath.Base(sitePath)
		current = site.WorktreeDevPorts[key]
	}
	// A pinned port that something else has taken would make the tool refuse to
	// start rather than drift, so an unusable pin is re-picked here.
	if current != 0 && freeport.Bindable(current) {
		return current, nil
	}
	port := assignDevServerPort(siteName, key, defaultPort)
	if port == current {
		return port, nil
	}
	if key == "" {
		site.DevServerPort = port
	} else {
		if site.WorktreeDevPorts == nil {
			site.WorktreeDevPorts = map[string]int{}
		}
		site.WorktreeDevPorts[key] = port
	}
	if err := config.AddSite(*site); err != nil {
		return 0, fmt.Errorf("saving dev server port: %w", err)
	}
	return port, nil
}

// assignDevServerPort walks up from the tool's own default until it finds a
// port no other site has claimed and nothing on the machine is holding. The
// registry alone is not enough: a dev server started before ports were pinned,
// or anything else on the box, owns a port no site has claimed.
func assignDevServerPort(excludeSiteName, excludeWorktree string, defaultPort int) int {
	if defaultPort == 0 {
		defaultPort = 5173
	}
	used := map[int]bool{}
	if reg, err := config.LoadSites(); err == nil {
		for _, s := range reg.Sites {
			own := s.Name == excludeSiteName
			if s.DevServerPort != 0 && !(own && excludeWorktree == "") {
				used[s.DevServerPort] = true
			}
			for wt, p := range s.WorktreeDevPorts {
				if p == 0 || (own && wt == excludeWorktree) {
					continue
				}
				used[p] = true
			}
		}
	}
	port := freeport.FirstFree(defaultPort, func(p int) bool {
		return used[p] || !freeport.Bindable(p)
	})
	if port == 0 {
		return defaultPort
	}
	return port
}

// devServerSetup pins the port, writes the generated config and regenerates the
// site's vhost so nginx proxies the prefix. It returns the arguments to append
// to the worker command, empty when the project is not shaped for this.
func devServerSetup(siteName, sitePath string, tool *config.DevServerTool) (string, error) {
	site, err := config.FindSite(siteName)
	if err != nil {
		return "", err
	}
	// A worktree fronts its own domain and runs its own dev server, so it gets
	// its own origin and its own pinned port rather than the parent's.
	origin, domains, err := devServerAddress(site, sitePath)
	if err != nil {
		return "", err
	}
	wrapper, err := writeDevServerWrapper(sitePath, tool, origin, domains)
	if err != nil || wrapper == "" {
		return "", err
	}
	port, err := devServerEnsurePort(siteName, sitePath, tool.DefaultPort)
	if err != nil {
		return "", err
	}
	regenSiteOrWorktreeVhost(site, sitePath)
	return devServerArgs(tool, wrapper, port), nil
}

// devServerArgs renders the store's flag syntax for the generated config and
// the port nginx proxies to. Empty when there is no wrapper to point at.
func devServerArgs(tool *config.DevServerTool, wrapperPath string, port int) string {
	if wrapperPath == "" || tool.Args == "" {
		return ""
	}
	args := strings.ReplaceAll(tool.Args, "{config}", wrapperPath)
	return strings.ReplaceAll(args, "{port}", strconv.Itoa(port))
}
