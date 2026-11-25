package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// Manager abstracts a Node version manager: mise, which lerd can install, and fnm
// and nvm, driven only where already present. Command runs it from Go; ExecPrefix
// and ShimScript are for generated scripts where lerd may be unreachable.
type Manager interface {
	// Name is the manager's identifier: "mise", "fnm" or "nvm".
	Name() string
	// Available reports whether the manager is usable on this host.
	Available() bool
	// List returns the installed Node major versions, deduped.
	List() []string
	// ListFull returns every installed full version, v stripped.
	ListFull() []string
	// Install installs the given version (a major like "20" or a full semver).
	Install(version string) error
	// Uninstall removes the given version.
	Uninstall(version string) error
	// SetDefault pins version as the manager's default.
	SetDefault(version string) error
	// HasDefault reports whether a usable default version is set.
	HasDefault() bool
	// Command builds a command that runs bin (with args) under version. An
	// empty version or "default" uses the manager's default. The caller sets
	// Dir/Env/streams on the returned command.
	Command(version, bin string, args []string) *exec.Cmd
	// ApplyEnv makes KEY=VAL pairs take effect for cmd after the manager has
	// activated the Node version. Call after setting cmd.Env. fnm appends to
	// cmd.Env; nvm embeds `export` statements after `nvm use` inside the bash
	// wrapper so vars like npm_config_prefix are not visible during activation
	// (nvm aborts when that variable is already set in the process environment).
	ApplyEnv(cmd *exec.Cmd, env []string)
	// ExecPrefix returns the shell tokens that precede a command so that
	// "<ExecPrefix(version)> <command> <args>" runs the command under version.
	// Used to build worker units and npm global wrappers.
	ExecPrefix(version string) string
	// ExecPrefixWithEnv is ExecPrefix with env vars set after Node activation,
	// so npm global wrappers see lerd's npm_config_prefix, not the manager's.
	ExecPrefixWithEnv(version string, env []string) string
	// ShimScript returns the full shell script for a node/npm/npx PATH shim
	// named bin. Only used when WritesPathShims is true (fnm). nvm returns a
	// stub that explains PATH shims are not installed for that manager.
	ShimScript(lerdBin, bin string) string
}

// Active returns the Node version manager lerd is configured to drive, honouring
// the node.manager config setting. See config.GlobalConfig.NodeManager for what
// an unset one means.
func Active() Manager {
	name := "mise"
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
		name = cfg.NodeManager()
	}
	return ManagerByName(name)
}

// Managed reports whether lerd is managing Node for this host. An explicit
// node.managed config preference wins; when the field is unset (configs from
// before it existed), presence of the node PATH shim is the historical signal.
// With nvm there is no node shim on PATH (nvm already owns node/npm/npx), so
// the persisted preference is what keeps managed mode visible to the UI/CLI.
func Managed() bool {
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
		if v, set := cfg.NodeManagedPref(); set {
			return v
		}
	}
	_, err := os.Stat(filepath.Join(config.BinDir(), "node"))
	return err == nil
}

// WritesPathShims reports whether lerd puts node/npm/npx wrappers in its bin dir.
// nvm and a user's own fnm are loaded by their shell, and lerd shims ahead of nvm
// make `nvm ls` / `nvm use` hang.
func WritesPathShims(m Manager) bool {
	if fnm, ok := m.(fnmManager); ok {
		return fnm.lerdOwned()
	}
	return m.Name() != "nvm"
}

// ManagerByName returns the manager for an explicit name, defaulting to mise for
// anything unrecognised (so a garbled config setting never breaks Node). Used by
// the node:manager switch to probe a target backend before selecting it.
func ManagerByName(name string) Manager {
	switch name {
	case "nvm":
		return nvmManager{}
	case "fnm":
		return newFnmManager()
	default:
		return newMiseManager()
	}
}

// Selectable lists the managers a switch may offer. lerd no longer
// installs fnm, so it is on the list only where one is already present.
func Selectable() []string {
	if ManagerByName("fnm").Available() {
		return []string{"mise", "fnm", "nvm"}
	}
	return []string{"mise", "nvm"}
}

// CheckSelectable refuses a manager lerd cannot switch to, naming why.
func CheckSelectable(name string) error {
	if slices.Contains(Selectable(), name) {
		return nil
	}
	if name == "fnm" {
		return fmt.Errorf("fnm is not installed and lerd no longer installs it, use mise or nvm")
	}
	return fmt.Errorf("unknown manager %q: use %s", name, strings.Join(Selectable(), " or "))
}

// execVersion is the selector a manager should activate: the caller's version
// when it is one, else the manager's default. A value that is not version
// shaped never reaches a command line, so a pin that slipped past detection
// falls back rather than being run.
func execVersion(version string) string {
	if v := SafeVersion(version); v != "" {
		return v
	}
	return "default"
}

// dedupeMajors reduces a list of full (or bare-major) version strings to unique
// numeric majors in first-seen order, skipping anything non-numeric. Shared by
// the fnm and nvm List implementations so every surface sees the same rules.
func dedupeMajors(versions []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range versions {
		major := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 2)[0]
		if major == "" || strings.Trim(major, "0123456789") != "" {
			continue
		}
		if !seen[major] {
			seen[major] = true
			out = append(out, major)
		}
	}
	return out
}

// uninstallVersions removes version via rm. A bare major ("20") removes every
// installed full version under it (matched against full); an exact version
// ("20.11.0") removes just itself. When a bare major matches nothing installed,
// it is passed through so the manager surfaces its own "not installed" error.
func uninstallVersions(version string, full []string, rm func(string) error) error {
	if strings.Contains(version, ".") {
		return rm(strings.TrimPrefix(version, "v"))
	}
	var lastErr error
	matched := false
	for _, v := range full {
		if strings.SplitN(v, ".", 2)[0] == version {
			matched = true
			if err := rm(v); err != nil {
				lastErr = err
			}
		}
	}
	if !matched {
		return rm(version)
	}
	return lastErr
}
