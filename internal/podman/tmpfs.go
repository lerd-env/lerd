package podman

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// TmpfsPathsFor returns the absolute paths the shared FPM container for this PHP
// version must hold in memory: the directories each opted-in site's framework
// declares, resolved against the site root.
func TmpfsPathsFor(version string) []string {
	native := false
	if cfg, err := config.LoadGlobal(); err == nil {
		native = cfg.PHPRuntimeMode() == config.PHPRuntimeNative
	}
	if !tmpfsSupported(runtime.GOOS, native) {
		return nil
	}
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	return tmpfsPaths(reg.Sites, version, declaredTmpfsPaths)
}

// tmpfsSupported reports whether a tmpfs can buy anything here. Linux shares a
// filesystem with PHP and the native runtime runs PHP on the host, so neither
// crosses a mount the cache could escape.
func tmpfsSupported(goos string, native bool) bool {
	return goos == "darwin" && !native
}

// tmpfsPaths resolves the declared paths of every site this shared container
// serves. A site on its own runtime is served elsewhere and has nothing here.
func tmpfsPaths(sites []config.Site, version string, declared func(config.Site) []string) []string {
	var out []string
	for _, site := range sites {
		s := site
		if s.PHPVersion != version || s.IsFrankenPHP() || s.IsCustomContainer() || s.IsCustomFPM() || s.IsProxyOnly() {
			continue
		}
		for _, p := range declared(s) {
			out = append(out, filepath.Join(s.Path, p))
		}
	}
	return out
}

// declaredTmpfsPaths returns the framework's tmpfs_paths for a site that has
// opted in. The paths come from the store and the opt-in from the project, since
// holding a cache in memory is a trade-off a project makes, not a framework property.
func declaredTmpfsPaths(site config.Site) []string {
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil || proj == nil || !proj.CacheInMemory {
		return nil
	}
	return config.TmpfsPathsForDir(site.Path)
}

// InjectTmpfs adds a Tmpfs= line for each path, after the %h:%h bind mount it
// shadows: podman applies mounts in order, so a tmpfs declared before the bind
// mount is buried by it and the cache stays on virtiofs.
func InjectTmpfs(content string, paths []string) string {
	if len(paths) == 0 {
		return content
	}
	var extra []string
	for _, p := range paths {
		if !bindMountable(p) {
			continue
		}
		line := fmt.Sprintf("Tmpfs=%s", p)
		if strings.Contains(content, line+"\n") {
			continue
		}
		extra = append(extra, line)
	}
	if len(extra) == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "Volume=") {
			out := make([]string, 0, len(lines)+len(extra))
			out = append(out, lines[:i+1]...)
			out = append(out, extra...)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return content
}
