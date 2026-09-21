package node

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// miseManager drives jdx/mise. Unlike fnm, mise is a tool users commonly
// already have, so lerd drives the one it finds and only installs its own when
// there is none. The binary path is resolved once and carried on the value.
type miseManager struct{ bin string }

func (miseManager) Name() string { return "mise" }

// newMiseManager resolves the mise lerd should drive on this host.
func newMiseManager() miseManager {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return miseManager{bin: findMise(home, exec.LookPath)}
}

// findMise locates a usable mise: the user's own install first, then PATH, then
// the package-manager prefixes a daemon's minimal PATH would miss. Returns ""
// when there is none. lookPath is a seam so the probe is testable.
func findMise(home string, lookPath func(string) (string, error)) string {
	if home != "" {
		if p := miseInstallPath(home); isExecutableFile(p) {
			return p
		}
	}
	if p, err := lookPath("mise"); err == nil {
		return p
	}
	for _, dir := range misePrefixes() {
		p := filepath.Join(dir, "mise")
		if isExecutableFile(p) {
			return p
		}
	}
	return ""
}

// miseInstallPath is mise's own canonical location, which is where lerd puts it
// when the user has none. Deliberately not lerd's bin dir: a second mise there
// would be invisible to the user's shell and would drift from the one they run.
func miseInstallPath(home string) string {
	return filepath.Join(home, ".local", "bin", "mise")
}

// misePrefixes are the package-manager dirs a daemon's restricted PATH misses.
func misePrefixes() []string {
	if runtime.GOOS == "darwin" {
		return []string{"/opt/homebrew/bin", "/usr/local/bin"}
	}
	return []string{"/usr/local/bin", "/usr/bin"}
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func (m miseManager) Available() bool { return m.bin != "" }

// tool is the mise selector for a Node version: "node@22", or bare "node" when
// there is no usable pin, so mise resolves what its own config asks for.
func miseTool(version string) string {
	if v := SafeVersion(version); v != "" {
		return "node@" + v
	}
	return "node"
}

func (m miseManager) listFull() []string {
	out, err := exec.Command(m.bin, "ls", "node", "--json").Output()
	if err != nil {
		return nil
	}
	return parseMiseListFull(string(out))
}

func (m miseManager) List() []string { return dedupeMajors(m.listFull()) }

func (m miseManager) Install(version string) error {
	if out, err := exec.Command(m.bin, "install", miseTool(version)).CombinedOutput(); err != nil {
		return fmt.Errorf("mise install %s: %s", version, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m miseManager) Uninstall(version string) error {
	return uninstallVersions(version, m.listFull(), func(v string) error {
		if out, err := exec.Command(m.bin, "uninstall", "node@"+v).CombinedOutput(); err != nil {
			return fmt.Errorf("mise uninstall %s: %s", v, strings.TrimSpace(string(out)))
		}
		return nil
	})
}

// SetDefault writes the version to mise's global config, which is what a bare
// `node` resolves to outside a project that pins its own.
func (m miseManager) SetDefault(version string) error {
	if out, err := exec.Command(m.bin, "use", "--global", miseTool(version)).CombinedOutput(); err != nil {
		return fmt.Errorf("mise use --global %s: %s", version, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m miseManager) HasDefault() bool {
	return exec.Command(m.bin, "exec", "node", "--", "true").Run() == nil
}

func (m miseManager) Command(version, bin string, args []string) *exec.Cmd {
	cmdArgs := append([]string{"exec", miseTool(version), "--", bin}, args...)
	return exec.Command(m.bin, cmdArgs...)
}

func (miseManager) ApplyEnv(cmd *exec.Cmd, env []string) {
	if len(env) == 0 || cmd == nil {
		return
	}
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, env...)
}

// ExecPrefix quotes the binary path because the fragment is spliced into a
// worker unit's `sh -c` body, where a space would split it into two words.
func (m miseManager) ExecPrefix(version string) string {
	return fmt.Sprintf("'%s' exec %s --", m.bin, miseTool(version))
}

func (m miseManager) ExecPrefixWithEnv(version string, env []string) string {
	prefix := m.ExecPrefix(version)
	var assignments []string
	for _, e := range env {
		key, val, ok := strings.Cut(e, "=")
		if !ok || key == "" {
			continue
		}
		assignments = append(assignments, key+"="+shellQuote(val))
	}
	if len(assignments) == 0 {
		return prefix
	}
	return prefix + " env " + strings.Join(assignments, " ")
}

func (m miseManager) ShimScript(lerdBin, bin string) string {
	return fmt.Sprintf(`#!/bin/sh
LERD="%s"
if [ -x "$LERD" ]; then
  exec "$LERD" %s "$@"
fi
MISE="%s"
VERSION=""
for f in .node-version .nvmrc; do
  [ -f "$f" ] && VERSION=$(tr -d '[:space:]' < "$f") && break
done
if [ -n "$VERSION" ]; then
  "$MISE" install "node@$VERSION" >/dev/null 2>&1 || true
  exec "$MISE" exec "node@$VERSION" -- %s "$@"
else
  if ! "$MISE" exec node -- true >/dev/null 2>&1; then
    printf 'No Node.js version available via lerd. Run: lerd node:install 22\n' >&2
    exit 1
  fi
  exec "$MISE" exec node -- %s "$@"
fi
`, lerdBin, bin, m.bin, bin, bin)
}

// miseListEntry is the shape of one `mise ls --json` record; only the fields
// lerd reads are declared.
type miseListEntry struct {
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
}

// parseMiseListFull extracts the installed full versions from `mise ls node
// --json`, preserving mise's order. Versions mise knows but has not installed
// cannot be run, so they are skipped.
func parseMiseListFull(raw string) []string {
	var entries []miseListEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	var versions []string
	for _, e := range entries {
		if !e.Installed {
			continue
		}
		v := strings.TrimPrefix(e.Version, "v")
		if v == "" {
			continue
		}
		major := strings.SplitN(v, ".", 2)[0]
		if strings.Trim(major, "0123456789") != "" {
			continue
		}
		versions = append(versions, v)
	}
	return versions
}
