package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Plugin definition kinds: the folders inside a plugin that hold service
// presets and frameworks, one <name>.yaml each.
const (
	PluginServices   = "services"
	PluginFrameworks = "frameworks"
)

// PluginDefinitionDirs returns the folder of the given kind in every installed
// plugin that loads. The plugins package sets it, since it imports config and
// not the other way round. Plugin definitions sit below lerd's own and the
// store's, so they only ever add names.
var PluginDefinitionDirs = func(kind string) []string { return nil }

// pluginDefinitionPath returns the first plugin file serving name, or "".
func pluginDefinitionPath(kind, name string) string {
	for _, dir := range PluginDefinitionDirs(kind) {
		path := filepath.Join(dir, name+".yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// pluginDefinitionNames lists the names every installed plugin ships of kind.
func pluginDefinitionNames(kind string) []string {
	var out []string
	for _, dir := range PluginDefinitionDirs(kind) {
		out = append(out, definitionNamesIn(dir)...)
	}
	return out
}

func definitionNamesIn(dir string) []string {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, strings.TrimSuffix(filepath.Base(p), ".yaml"))
	}
	return out
}

// loadPluginFramework returns the framework a plugin ships under name, or nil.
func loadPluginFramework(name string) *Framework {
	if path := pluginDefinitionPath(PluginFrameworks, name); path != "" {
		return loadFrameworkYAML(path)
	}
	return nil
}

// CheckPluginDefinitions validates the services and frameworks the plugin in
// dir ships, so a broken one refuses the plugin instead of vanishing later.
func CheckPluginDefinitions(dir string) error {
	for _, name := range definitionNamesIn(filepath.Join(dir, PluginServices)) {
		if !validServiceName.MatchString(name) {
			return fmt.Errorf("service %q must be lowercase letters, digits and dashes", name)
		}
		data, err := os.ReadFile(filepath.Join(dir, PluginServices, name+".yaml"))
		if err != nil {
			return err
		}
		if err := ValidatePresetYAML(data, name); err != nil {
			return err
		}
		if got := yamlName(data); got != name {
			return fmt.Errorf("service %s.yaml declares name %q", name, got)
		}
	}
	for _, name := range definitionNamesIn(filepath.Join(dir, PluginFrameworks)) {
		if !validServiceName.MatchString(name) {
			return fmt.Errorf("framework %q must be lowercase letters, digits and dashes", name)
		}
		data, err := os.ReadFile(filepath.Join(dir, PluginFrameworks, name+".yaml"))
		if err != nil {
			return err
		}
		var fw Framework
		if err := yaml.Unmarshal(data, &fw); err != nil {
			return fmt.Errorf("framework %s.yaml: %w", name, err)
		}
		if fw.Name != name {
			return fmt.Errorf("framework %s.yaml declares name %q", name, fw.Name)
		}
	}
	return nil
}

func yamlName(data []byte) string {
	var v struct {
		Name string `yaml:"name"`
	}
	_ = yaml.Unmarshal(data, &v)
	return v.Name
}

// PluginDefinitionClashes names each service and framework the plugin in dir
// ships that lerd already has from elsewhere. installed is the folder of this
// plugin's current copy when it is being updated, whose names are its own.
func PluginDefinitionClashes(dir, installed string) []string {
	var out []string
	for _, name := range definitionNamesIn(filepath.Join(dir, PluginServices)) {
		if presetServedOutside(name, installed) {
			out = append(out, "service "+name)
		}
	}
	for _, name := range definitionNamesIn(filepath.Join(dir, PluginFrameworks)) {
		if frameworkServedOutside(name, installed) {
			out = append(out, "framework "+name)
		}
	}
	return out
}

func presetServedOutside(name, installed string) bool {
	if _, ok := readOwnPresetBytes(name); ok {
		return true
	}
	return servedByOtherPlugin(PluginServices, name, installed)
}

func frameworkServedOutside(name, installed string) bool {
	if builtinFramework(name) != nil || cachedStoreEntryByName(name) != nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(FrameworksDir(), name+".yaml")); err == nil {
		return true
	}
	if storeFrameworkPath(name) != "" {
		return true
	}
	return servedByOtherPlugin(PluginFrameworks, name, installed)
}

func servedByOtherPlugin(kind, name, installed string) bool {
	own := ""
	if installed != "" {
		own = filepath.Join(installed, kind)
	}
	for _, dir := range PluginDefinitionDirs(kind) {
		if filepath.Clean(dir) == filepath.Clean(own) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name+".yaml")); err == nil {
			return true
		}
	}
	return false
}
