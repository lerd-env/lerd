package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	nodeDet "github.com/geodro/lerd/internal/node"
)

// userNPMPrefix returns the npm prefix the user configured themselves —
// npm_config_prefix / NPM_CONFIG_PREFIX in the environment, or a prefix= line
// in ~/.npmrc — so the npm shim can respect it instead of capturing globals
// into lerd's own data dir. Empty when the user has none.
func userNPMPrefix() string {
	for _, key := range []string{"npm_config_prefix", "NPM_CONFIG_PREFIX"} {
		if v := os.Getenv(key); v != "" {
			return expandHomePath(v)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".npmrc"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "prefix" {
			return expandHomePath(strings.Trim(strings.TrimSpace(v), `"'`))
		}
	}
	return ""
}

// expandHomePath resolves a leading ~ the way npm does for .npmrc values.
func expandHomePath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p[1:], "/"))
		}
	}
	return p
}

// npmGlobalPrefixEnv decides which npm prefix an npm/npx run gets: the user's
// own prefix when they configured one (their globals stay theirs), otherwise
// lerd's node-global dir. The returned entries are applied after version-
// manager activation (shimLeadingEnv strips an inherited npm_config_prefix so
// nvm activates cleanly). lerdOwned reports whether the prefix is lerd's,
// which drives the wrapper sync into lerd's bin dir.
func npmGlobalPrefixEnv() (env []string, lerdOwned bool) {
	if user := userNPMPrefix(); user != "" {
		return []string{"npm_config_prefix=" + user}, false
	}
	prefix := config.NodeGlobalDir()
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		return nil, false
	}
	return []string{"npm_config_prefix=" + prefix}, true
}

// nodeGlobalPackages lists the packages installed under an npm prefix as
// name@version specs (bare name when the version is unreadable), sorted, so
// uninstall and node:unmanage can tell the user exactly what npm captured
// into lerd's prefix. Nil when the prefix holds nothing.
func nodeGlobalPackages(prefix string) []string {
	root := filepath.Join(prefix, "lib", "node_modules")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var specs []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasPrefix(name, "@") {
			scoped, _ := os.ReadDir(filepath.Join(root, name))
			for _, s := range scoped {
				if s.IsDir() {
					specs = append(specs, packageSpec(root, name+"/"+s.Name()))
				}
			}
			continue
		}
		specs = append(specs, packageSpec(root, name))
	}
	sort.Strings(specs)
	return specs
}

// packageSpec returns "name@version" for an installed package dir, falling
// back to the bare name when package.json has no readable version.
func packageSpec(root, name string) string {
	data, err := os.ReadFile(filepath.Join(root, name, "package.json"))
	if err != nil {
		return name
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &pkg) != nil || pkg.Version == "" {
		return name
	}
	return name + "@" + pkg.Version
}

// removeNodeGlobalWrappers deletes every npm-global wrapper lerd synced into
// binDir. After node:unmanage the version manager behind them is gone, so a
// stale wrapper would shadow a freshly reinstalled user global on PATH.
func removeNodeGlobalWrappers(binDir string) {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(binDir, e.Name())
		if head, ok := readShimHead(full); ok && strings.Contains(head, nodeShimMarker) {
			os.Remove(full) //nolint:errcheck
		}
	}
}

// systemNPMPath returns the user's own npm binary — the first npm found in
// the system Node bin dirs, outside lerd's shims. Empty when there is none.
func systemNPMPath() string {
	for _, dir := range nodeDet.SystemNodeBinDirs() {
		npm := filepath.Join(dir, "npm")
		if info, err := os.Stat(npm); err == nil && !info.IsDir() {
			return npm
		}
	}
	return ""
}

// formatNodeGlobalsNote renders the captured-globals list for user-facing
// notes and prompts.
func formatNodeGlobalsNote(specs []string) string {
	return strings.Join(specs, ", ")
}

// reinstallNodeGlobals installs specs globally with the user's own npm,
// streaming its output. npm's own dir leads PATH so its node resolves ahead
// of lerd's shims, and any prefix pointing at lerd's dir is dropped so the
// packages land in the user's real prefix.
func reinstallNodeGlobals(npm string, specs []string) error {
	cmd := exec.Command(npm, append([]string{"install", "-g"}, specs...)...)
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		name, val, _ := strings.Cut(kv, "=")
		if (name == "npm_config_prefix" || name == "NPM_CONFIG_PREFIX") && val == config.NodeGlobalDir() {
			continue
		}
		if strings.EqualFold(name, "PATH") {
			kv = name + "=" + filepath.Dir(npm) + string(os.PathListSeparator) + val
		}
		env = append(env, kv)
	}
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
