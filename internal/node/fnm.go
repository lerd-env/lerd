package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// fnmManager drives Schniz/fnm, invoked as `fnm exec --using=<v> -- <cmd>`. lerd
// no longer installs it: the one it drives is left in its bin dir by an older
// lerd, or is the user's own. An empty path means lerd's.
type fnmManager struct{ path, dir string }

// newFnmManager resolves the fnm lerd should drive on this host.
func newFnmManager() fnmManager {
	home, _ := os.UserHomeDir()
	m := fnmManager{path: findFnm(home, exec.LookPath)}
	if m.path != "" && !m.lerdOwned() {
		m.dir = fnmDir()
	}
	return m
}

// fnmDir is the data dir the user's fnm was pointed at: the recorded one, then
// $FNM_DIR. Empty leaves fnm its default.
func fnmDir() string {
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
		if info, err := os.Stat(cfg.Node.FnmDir); err == nil && info.IsDir() {
			return cfg.Node.FnmDir
		}
	}
	return os.Getenv("FNM_DIR")
}

// RecordFnm saves in cfg where this shell finds the user's fnm and its versions.
// A process without that shell finds neither, so what is missing keeps its value.
// Reports whether anything changed; lerd's own fnm is not the user's to record.
func RecordFnm(cfg *config.GlobalConfig) bool {
	if isExecutableFile(lerdFnm()) {
		return false
	}
	before := cfg.Node
	if p, err := exec.LookPath("fnm"); err == nil {
		cfg.Node.FnmBin = p
	}
	if d := os.Getenv("FNM_DIR"); filepath.IsAbs(d) {
		cfg.Node.FnmDir = d
	}
	return cfg.Node.FnmBin != before.FnmBin || cfg.Node.FnmDir != before.FnmDir
}

// args puts fnm's own flags in front of a subcommand.
func (m fnmManager) args(sub ...string) []string {
	if m.dir == "" {
		return sub
	}
	return append([]string{"--fnm-dir", m.dir}, sub...)
}

// findFnm locates a usable fnm: lerd's own, the recorded one, PATH, then the places fnm's
// installer, cargo and the package managers use, which a daemon's minimal PATH
// would miss. Returns "" when there is none.
func findFnm(home string, lookPath func(string) (string, error)) string {
	if own := lerdFnm(); isExecutableFile(own) {
		return own
	}
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil && isExecutableFile(cfg.Node.FnmBin) {
		return cfg.Node.FnmBin
	}
	if p, err := lookPath("fnm"); err == nil {
		return p
	}
	dirs := miseDirs()
	if home != "" {
		dirs = append([]string{
			filepath.Join(home, ".local", "share", "fnm"),
			filepath.Join(home, ".fnm"),
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, ".local", "bin"),
		}, dirs...)
	}
	for _, dir := range dirs {
		if p := filepath.Join(dir, "fnm"); isExecutableFile(p) {
			return p
		}
	}
	return ""
}

func lerdFnm() string { return filepath.Join(config.BinDir(), "fnm") }

// FnmBinary is the fnm lerd drives on this host, empty when there is none.
func FnmBinary() string { return newFnmManager().path }

// UserFnm is the user's own fnm, empty when lerd's is there or neither is.
func UserFnm() string {
	if m := newFnmManager(); m.path != "" && !m.lerdOwned() {
		return m.path
	}
	return ""
}

func (m fnmManager) lerdOwned() bool { return m.bin() == lerdFnm() }

func (fnmManager) Name() string { return "fnm" }

func (m fnmManager) bin() string {
	if m.path == "" {
		return lerdFnm()
	}
	return m.path
}

func (m fnmManager) Available() bool {
	return isExecutableFile(m.bin())
}

// ListFull returns every installed full version (v stripped) fnm reports.
func (m fnmManager) ListFull() []string {
	out, err := exec.Command(m.bin(), m.args("list")...).Output()
	if err != nil {
		return nil
	}
	return parseFnmListFull(string(out))
}

func (m fnmManager) List() []string {
	return dedupeMajors(m.ListFull())
}

func (m fnmManager) Install(version string) error {
	if out, err := exec.Command(m.bin(), m.args("install", version)...).CombinedOutput(); err != nil {
		return fmt.Errorf("fnm install %s: %s", version, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m fnmManager) Uninstall(version string) error {
	return uninstallVersions(version, m.ListFull(), func(v string) error {
		if out, err := exec.Command(m.bin(), m.args("uninstall", v)...).CombinedOutput(); err != nil {
			return fmt.Errorf("fnm uninstall %s: %s", v, strings.TrimSpace(string(out)))
		}
		return nil
	})
}

func (m fnmManager) SetDefault(version string) error {
	if out, err := exec.Command(m.bin(), m.args("default", version)...).CombinedOutput(); err != nil {
		return fmt.Errorf("fnm default %s: %s", version, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m fnmManager) HasDefault() bool {
	return exec.Command(m.bin(), m.args("exec", "--using=default", "--", "true")...).Run() == nil
}

func (m fnmManager) Command(version, bin string, args []string) *exec.Cmd {
	cmdArgs := append([]string{"exec", "--using=" + execVersion(version), "--", bin}, args...)
	return exec.Command(m.bin(), m.args(cmdArgs...)...)
}

func (fnmManager) ApplyEnv(cmd *exec.Cmd, env []string) {
	if len(env) == 0 || cmd == nil {
		return
	}
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, env...)
}

// ExecPrefix quotes the version because the fragment it returns is spliced into
// a worker unit's `sh -c` body, where an unquoted value would be read as shell
// syntax rather than as an argument.
func (m fnmManager) ExecPrefix(version string) string {
	dir := ""
	if m.dir != "" {
		dir = "--fnm-dir " + shellQuote(m.dir) + " "
	}
	return fmt.Sprintf("'%s' %sexec --using=%s --", m.bin(), dir, shellQuote(execVersion(version)))
}

// withEnv appends `env K=V ...` to an exec prefix, skipping malformed entries.
func withEnv(prefix string, env []string) string {
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

// The result is interpolated into a /bin/sh wrapper, so values are quoted the
// same way nvmExports quotes its exports. An unquoted path with a space splits
// in two and env takes the tail as the command to run.
func (m fnmManager) ExecPrefixWithEnv(version string, env []string) string {
	return withEnv(m.ExecPrefix(version), env)
}

func (m fnmManager) ShimScript(lerdBin, bin string) string {
	return fmt.Sprintf(`#!/bin/sh
LERD="%s"
if [ -x "$LERD" ]; then
  exec "$LERD" %s "$@"
fi
FNM="%s"
VERSION=""
for f in .node-version .nvmrc; do
  [ -f "$f" ] && VERSION=$(tr -d '[:space:]' < "$f") && break
done
if [ -n "$VERSION" ]; then
  "$FNM" install "$VERSION" >/dev/null 2>&1 || true
  exec "$FNM" exec --using="$VERSION" -- %s "$@"
else
  if ! "$FNM" exec --using=default -- true >/dev/null 2>&1; then
    printf 'No Node.js version available via lerd. Run: lerd node:install 22\n' >&2
    exit 1
  fi
  exec "$FNM" exec --using=default -- %s "$@"
fi
`, lerdBin, bin, m.bin(), bin, bin)
}

// parseFnmListFull extracts the full version strings (leading "v" stripped) from
// `fnm list` output, preserving fnm's order. Each line looks like
// "* v20.0.0 default" or "  v18.0.0"; non-numeric-major entries (aliases like
// "lts/iron") are skipped.
func parseFnmListFull(raw string) []string {
	var versions []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		v := strings.TrimPrefix(fields[0], "v")
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

// parseFnmList extracts the unique numeric major versions from `fnm list`.
func parseFnmList(raw string) []string {
	return dedupeMajors(parseFnmListFull(raw))
}
