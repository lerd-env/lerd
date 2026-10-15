package plugins

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/geodro/lerd/internal/config"
)

// Surfaces name which part of lerd ran a plugin file, passed as LERD_SURFACE so
// one script can serve several of them.
const (
	SurfaceCLI         = "cli"
	SurfaceSiteCommand = "site_command"
	SurfaceHook        = "hook"
	SurfaceMCP         = "mcp"
	SurfaceAPI         = "api"
)

// Dir is where plugins are installed, one directory each.
func Dir() string { return filepath.Join(config.DataDir(), "plugins") }

// DataDirFor is the plugin's own writable directory, kept outside the plugin
// so reinstalling or removing the code leaves what it stored alone.
func DataDirFor(name string) string { return filepath.Join(config.DataDir(), "plugin-data", name) }

// LoadError is a plugin directory that could not be loaded.
type LoadError struct {
	Dir string
	Err error
}

func (e LoadError) Error() string { return e.Err.Error() }

// Load reads every installed plugin. A broken one is reported and skipped so
// it never takes the rest of lerd down with it.
func Load() ([]*Plugin, []LoadError) {
	entries, err := os.ReadDir(Dir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []LoadError{{Dir: Dir(), Err: err}}
	}

	var plugins []*Plugin
	var errs []LoadError
	for _, e := range entries {
		dir := filepath.Join(Dir(), e.Name())
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
			continue
		}
		p, err := ReadPlugin(dir)
		if err != nil {
			errs = append(errs, LoadError{Dir: dir, Err: err})
			continue
		}
		plugins = append(plugins, p)
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins, errs
}

// Find returns the installed plugin called name.
func Find(name string) (*Plugin, error) {
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid plugin name %q", name)
	}
	return ReadPlugin(filepath.Join(Dir(), name))
}

// Command builds the process for one of the plugin's files. The plugin's
// context goes in as LERD_* variables on top of lerd's own environment; extra
// carries what the surface knows (site, event...). It creates the plugin's
// data dir so a script can write to it without checking first.
func (p *Plugin) Command(run, surface string, args []string, extra map[string]string) (*exec.Cmd, error) {
	if err := p.checkExecutable(run); err != nil {
		return nil, err
	}
	data := DataDirFor(p.Name)
	config.GuardRealWrite(data)
	if err := os.MkdirAll(data, 0o755); err != nil {
		return nil, err
	}
	token, err := Token(p.Name)
	if err != nil {
		return nil, err
	}
	self, _ := os.Executable()

	cmd := exec.Command(filepath.Join(p.Dir, run), args...)
	cmd.Env = append(os.Environ(),
		"LERD_PLUGIN_NAME="+p.Name,
		"LERD_PLUGIN_DIR="+p.Dir,
		"LERD_PLUGIN_DATA="+data,
		"LERD_SURFACE="+surface,
		"LERD_BIN="+self,
		"LERD_API_URL="+APIURL,
		"LERD_API_TOKEN="+token,
		"PATH="+searchPath(),
	)
	for k, v := range extra {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd, nil
}
