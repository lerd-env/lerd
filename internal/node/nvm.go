package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/hostbin"
)

// nvmManager drives nvm-sh/nvm. Unlike fnm, nvm is not a binary: it is a bash
// function sourced from $NVM_DIR/nvm.sh (default ~/.nvm). Every invocation
// therefore runs through `bash -c` after sourcing the script, and generated
// shell fragments use bash rather than /bin/sh. lerd never installs nvm; it only
// drives one the user installed themselves.
type nvmManager struct{}

func (nvmManager) Name() string { return "nvm" }

// DiscoverNvmDir returns the live nvm location from $NVM_DIR or ~/.nvm,
// ignoring any persisted node.nvm_dir. Used when recording the path at
// install/switch time so daemons keep agreeing with the CLI.
func DiscoverNvmDir() string {
	if d := os.Getenv("NVM_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nvm")
}

// nvmDir resolves the nvm install directory: a persisted node.nvm_dir wins
// (so daemons without shell rc still find a custom install), then $NVM_DIR,
// then ~/.nvm.
func nvmDir() string {
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
		if d := cfg.NodeNvmDir(); d != "" {
			return d
		}
	}
	return DiscoverNvmDir()
}

func (nvmManager) Available() bool { return ScriptPresent() }

// brewNvmScripts lists where a Homebrew nvm keeps nvm.sh: <prefix>/opt/nvm,
// derived from the same prefixes everything else here resolves tools in. A var
// so tests can point it at a fixture.
var brewNvmScripts = func() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	var out []string
	for _, binDir := range hostbin.ExtraDirs() {
		out = append(out, filepath.Join(filepath.Dir(binDir), "opt", "nvm", "nvm.sh"))
	}
	return out
}

// nvmScript resolves the nvm.sh to source. $NVM_DIR/nvm.sh is the script
// install's layout and wins. Homebrew is the other common one: it keeps the
// script under its own prefix and has the user create an empty ~/.nvm for the
// versions, so NVM_DIR is a real nvm dir with no script in it. Empty when nvm
// is not installed at all.
func nvmScript() string {
	if p := filepath.Join(nvmDir(), "nvm.sh"); fileExists(p) {
		return p
	}
	for _, p := range brewNvmScripts() {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// ScriptPresent reports whether an nvm this host can drive is installed, in
// either layout. Install and the manager switch ask through here so a Homebrew
// nvm counts as one.
func ScriptPresent() bool { return nvmScript() != "" }

// scriptToSource is nvmScript with $NVM_DIR/nvm.sh as the fallback. A fragment
// generated on a host with no nvm still has to name a real path: the `[ -s ]`
// guard in front of it is a runtime check, so a worker unit written before nvm
// is installed starts working once it is, while an empty path would bake in a
// source line that can never fire.
func scriptToSource() string {
	if p := nvmScript(); p != "" {
		return p
	}
	return filepath.Join(nvmDir(), "nvm.sh")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// sourceScript is the shell prelude that loads nvm into the current bash
// process. Embedded verbatim in every generated fragment and in-process call.
// NVM_DIR and the script are named separately because Homebrew splits them.
func sourceScript() string {
	script := shellQuote(scriptToSource())
	return fmt.Sprintf(`export NVM_DIR=%s; [ -s %s ] && . %s; `,
		shellQuote(nvmDir()), script, script)
}

// shellCmd builds a bash command that sources nvm and runs `nvm <args...>`,
// passing args positionally so no quoting of the version is needed.
func (nvmManager) shellCmd(args ...string) *exec.Cmd {
	full := append([]string{"-c", sourceScript() + `nvm "$@"`, "lerd-nvm"}, args...)
	return exec.Command("bash", full...)
}

// listFull returns every installed full version (v stripped). --no-alias is
// essential: without it `nvm ls` also prints alias lines like
// "lts/iron -> v20.20.2 (-> N/A)" that reference versions which are not actually
// installed, which would otherwise be miscounted as present.
func (m nvmManager) listFull() []string {
	out, err := m.shellCmd("ls", "--no-colors", "--no-alias").Output()
	if err != nil {
		return nil
	}
	return parseNvmListFull(string(out))
}

func (m nvmManager) List() []string {
	return dedupeMajors(m.listFull())
}

func (m nvmManager) Install(version string) error {
	if out, err := m.shellCmd("install", version).CombinedOutput(); err != nil {
		return fmt.Errorf("nvm install %s: %s", version, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m nvmManager) Uninstall(version string) error {
	return uninstallVersions(version, m.listFull(), func(v string) error {
		if out, err := m.shellCmd("uninstall", v).CombinedOutput(); err != nil {
			return fmt.Errorf("nvm uninstall %s: %s", v, strings.TrimSpace(string(out)))
		}
		return nil
	})
}

// The nvm install belongs to the user, so lerd's default lives in its own
// config and never rewrites the user's `default` alias. The alias is still
// honoured read-only as a fallback when no config default is set.
func (m nvmManager) SetDefault(version string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	cfg.Node.DefaultVersion = version
	return config.SaveGlobal(cfg)
}

func (m nvmManager) HasDefault() bool {
	out, err := m.shellCmd("version", "default").Output()
	if err != nil {
		return false
	}
	return nvmDefaultUsable(string(out))
}

// nvmDefaultUsable reports whether `nvm version default` output names a real
// nvm-managed version. "system" means the alias points at the host node.
func nvmDefaultUsable(raw string) bool {
	v := strings.TrimSpace(raw)
	return v != "" && v != "N/A" && v != "system"
}

// nvmActivate returns the shell prelude that sources nvm, selects version, and
// puts that version's bin dir at the front of PATH. Both checks are required:
// nvm use can return 0 on the system branch (default alias "system" with a
// host node) while running nvm deactivate and unsetting NVM_BIN, and an empty
// NVM_BIN would make PATH=":$PATH" put the current directory on PATH. Honouring
// the exit status catches a missing pin; requiring a non-empty NVM_BIN catches
// the system fall-through.
func nvmActivate(version string) string {
	// The version is bound to a shell variable once and only ever referenced
	// through it. Interpolating it into the messages as well would put an
	// unquoted copy inside a single-quoted string, where one quote character in
	// the value is enough to end the string and start a command.
	return sourceScript() + fmt.Sprintf(
		`__lerd_nv=%s; `+
			`nvm use "$__lerd_nv" >/dev/null 2>&1 || { echo "lerd: no nvm Node available for $__lerd_nv (run: lerd node:install)" >&2; exit 1; }; `+
			`if [ -z "$NVM_BIN" ]; then echo "lerd: no nvm Node available for $__lerd_nv (run: lerd node:install)" >&2; exit 1; fi; `+
			`PATH="$NVM_BIN:$PATH"; export PATH; `,
		shellQuote(execVersion(version)))
}

// nvmExports builds `export KEY=VAL;` statements for ApplyEnv. Values are
// shell-quoted so spaces and quotes survive.
func nvmExports(env []string) string {
	var b strings.Builder
	for _, e := range env {
		key, val, ok := strings.Cut(e, "=")
		if !ok || key == "" {
			continue
		}
		b.WriteString("export ")
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(shellQuote(val))
		b.WriteString("; ")
	}
	return b.String()
}

func (nvmManager) Command(version, bin string, args []string) *exec.Cmd {
	full := append([]string{"-c", nvmActivate(version) + `exec "$@"`, "lerd-nvm", bin}, args...)
	return exec.Command("bash", full...)
}

func (nvmManager) ApplyEnv(cmd *exec.Cmd, env []string) {
	if len(env) == 0 || cmd == nil {
		return
	}
	exports := nvmExports(env)
	if exports == "" {
		return
	}
	// Args: bash -c '<activate>exec "$@"' lerd-nvm bin ...
	for i := 0; i+1 < len(cmd.Args); i++ {
		if cmd.Args[i] != "-c" {
			continue
		}
		script := cmd.Args[i+1]
		const marker = `exec "$@"`
		if idx := strings.LastIndex(script, marker); idx >= 0 {
			cmd.Args[i+1] = script[:idx] + exports + script[idx:]
		}
		return
	}
}

func (nvmManager) ExecPrefix(version string) string {
	// A bash -c wrapper that sources nvm, activates the version (aborting if
	// nvm use fails so it can't fork-bomb), then exec's the command supplied as
	// positional args ("$@"). "lerd-nvm" is $0.
	return fmt.Sprintf("bash -c %s lerd-nvm", shellQuote(nvmActivate(version)+`exec "$@"`))
}

// ShimScript satisfies Manager but is unused for nvm (WritesPathShims is false).
func (nvmManager) ShimScript(_, bin string) string {
	return fmt.Sprintf("#!/usr/bin/env bash\nprintf 'lerd: nvm does not install PATH shims; use: lerd %%s\\n' %s >&2\nexit 1\n", shellQuote(bin))
}

// nvmVersionRe matches a "vMAJOR.MINOR.PATCH" token on an installed-version line
// of `nvm ls --no-colors --no-alias` (e.g. "->     v24.16.0 *" or "  v18.20.0").
var nvmVersionRe = regexp.MustCompile(`\bv(\d+\.\d+\.\d+)\b`)

// nvmSystemField matches a standalone "system" token in nvm ls output so the
// system row (which may also carry a resolved version) is not treated as an
// installed nvm version.
var nvmSystemField = regexp.MustCompile(`(?:^|[\s>])system(?:\s|$|\*)`)

// parseNvmListFull extracts the full version strings (leading "v" stripped) from
// `nvm ls --no-colors --no-alias` output, in the order nvm reports them. Lines
// for the system node are skipped even when they embed a resolved version.
func parseNvmListFull(raw string) []string {
	var versions []string
	for _, line := range strings.Split(raw, "\n") {
		if nvmSystemField.MatchString(line) {
			continue
		}
		if m := nvmVersionRe.FindStringSubmatch(line); m != nil {
			versions = append(versions, m[1])
		}
	}
	return versions
}

// parseNvmList extracts the unique numeric major versions from nvm's list.
func parseNvmList(raw string) []string {
	return dedupeMajors(parseNvmListFull(raw))
}

// shellQuote wraps s in single quotes, escaping any embedded single quote via
// the standard '"'"' idiom, so it survives verbatim inside a shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
