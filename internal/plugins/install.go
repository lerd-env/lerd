package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/git"
	"gopkg.in/yaml.v3"
)

// Install copies or clones src into a staging dir inside the plugins dir,
// validates it, then moves it under the name its manifest declares. Nothing is
// left behind when the plugin turns out to be invalid.
func Install(src string) (*Plugin, error) {
	stage, err := newStage()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, "plugin")

	if info, err := os.Stat(src); err == nil && info.IsDir() {
		if err := git.CopyTree(src, staged); err != nil {
			return nil, err
		}
	} else {
		clone := exec.Command("git", "clone", "--depth", "1", src, staged)
		// Fail on a private repo rather than wait on a password prompt no one
		// can answer when the dashboard asked for the install.
		clone.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := clone.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git clone %s: %s", src, strings.TrimSpace(string(out)))
		}
	}

	return place(stage, staged, "", false, nil)
}

// newStage makes a staging dir inside the plugins dir, so the final move into
// place is a rename on the same filesystem.
func newStage() (string, error) {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(Dir(), ".install-")
}

// place validates the plugin fetched into staged and moves it into the plugins
// dir under the name its manifest declares. want, when set, is the name the
// catalog listed it under; replace swaps out an installed copy, which is kept
// until the new one is in place; src records where a catalog install came from.
func place(stage, staged, want string, replace bool, src *Source) (*Plugin, error) {
	name, err := manifestName(staged)
	if err != nil {
		return nil, err
	}
	if want != "" && name != want {
		return nil, fmt.Errorf("the repository holds plugin %q, but it is listed as %q", name, want)
	}
	// The manifest name must match its directory, so validate under that name.
	named := filepath.Join(stage, name)
	if err := os.Rename(staged, named); err != nil {
		return nil, err
	}
	if _, err := ReadPlugin(named); err != nil {
		return nil, err
	}
	if err := checkJobs(named, name); err != nil {
		return nil, err
	}
	if clashes := config.PluginDefinitionClashes(named, installedDir(name)); len(clashes) > 0 {
		return nil, fmt.Errorf("plugin %s ships what lerd already has: %s", name, strings.Join(clashes, ", "))
	}
	if src != nil {
		if err := writeSource(named, src); err != nil {
			return nil, err
		}
	}
	dest := filepath.Join(Dir(), name)
	if _, err := os.Stat(dest); err == nil {
		if !replace {
			return nil, fmt.Errorf("plugin %s is already installed; remove it first", name)
		}
		if err := os.Rename(dest, filepath.Join(stage, "previous")); err != nil {
			return nil, err
		}
	}
	if err := os.Rename(named, dest); err != nil {
		return nil, err
	}
	ClearFailure(name)
	return ReadPlugin(dest)
}

// Remove deletes an installed plugin's folder, broken or not, and keeps the
// data it stored in DataDirFor.
func Remove(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	dir := filepath.Join(Dir(), name)
	if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
		return fmt.Errorf("plugin %s is not installed", name)
	}
	ClearFailure(name)
	return os.RemoveAll(dir)
}

func manifestName(dir string) (string, error) {
	var m struct {
		Name string `yaml:"name"`
	}
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return "", fmt.Errorf("not a lerd plugin: %w", err)
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("%s: %w", ManifestFile, err)
	}
	if !nameRe.MatchString(m.Name) {
		return "", fmt.Errorf("plugin manifest has an invalid name %q", m.Name)
	}
	return m.Name, nil
}
