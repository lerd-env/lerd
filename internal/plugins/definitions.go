package plugins

import (
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// Every lerd process links this package, so registering here gives the
// watcher, dashboard, CLI and MCP the same view of plugin definitions.
func init() { config.PluginDefinitionDirs = definitionDirs }

// definitionDirs returns the folder of kind in each plugin that loads, so a
// broken or incompatible plugin adds no services or frameworks either.
func definitionDirs(kind string) []string {
	loaded, _ := Load()
	var out []string
	for _, p := range loaded {
		if p.ships(kind) {
			out = append(out, filepath.Join(p.Dir, kind))
		}
	}
	return out
}

func (p *Plugin) ships(kind string) bool {
	paths, _ := filepath.Glob(filepath.Join(p.Dir, kind, "*.yaml"))
	return len(paths) > 0
}

// installedDir is the folder of the installed copy of name, "" when none.
func installedDir(name string) string {
	dir := filepath.Join(Dir(), name)
	if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
		return ""
	}
	return dir
}
