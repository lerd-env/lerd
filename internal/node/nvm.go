package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// nvmManager drives nvm-sh/nvm. Unlike fnm, nvm is not a binary: it is a bash
// function sourced from $NVM_DIR/nvm.sh (default ~/.nvm). Every invocation
// therefore runs through `bash -c` after sourcing the script, and generated
// shell fragments use bash rather than /bin/sh. lerd never installs nvm; it only
// drives one the user installed themselves.
type nvmManager struct{}

func (nvmManager) Name() string { return "nvm" }

// nvmDir resolves the nvm install directory: $NVM_DIR when set, else ~/.nvm.
func nvmDir() string {
	if d := os.Getenv("NVM_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nvm")
}

func (nvmManager) Available() bool {
	_, err := os.Stat(filepath.Join(nvmDir(), "nvm.sh"))
	return err == nil
}

// sourceScript is the shell prelude that loads nvm into the current bash
// process. Embedded verbatim in every generated fragment and in-process call.
func sourceScript() string {
	return fmt.Sprintf(`export NVM_DIR=%s; [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh"; `, shellQuote(nvmDir()))
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

func (m nvmManager) SetDefault(version string) error {
	if out, err := m.shellCmd("alias", "default", version).CombinedOutput(); err != nil {
		return fmt.Errorf("nvm alias default %s: %s", version, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m nvmManager) HasDefault() bool {
	out, err := m.shellCmd("version", "default").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != "N/A"
}

// nvmActivate returns the shell prelude that sources nvm, selects version, and
// puts that version's bin dir at the front of PATH. It aborts (exit 1) when no
// version becomes active rather than falling through to `exec "$@"`: a bare
// `exec node` would resolve `node` via PATH, and since lerd's own shim dir is on
// PATH, it would re-enter the very shim that invoked this and fork-bomb. nvm
// exports $NVM_BIN when a version is active, so a non-empty $NVM_BIN is the
// reliable signal that activation succeeded, and prepending it makes both the
// command and any child (npx, npm lifecycle scripts) resolve node to nvm's copy.
func nvmActivate(version string) string {
	return sourceScript() + fmt.Sprintf(`nvm use %s >/dev/null 2>&1; if [ -z "$NVM_BIN" ]; then echo 'lerd: no nvm Node available for %s (run: lerd node:install)' >&2; exit 1; fi; PATH="$NVM_BIN:$PATH"; export PATH; `, shellQuote(version), version)
}

func (nvmManager) Command(version, bin string, args []string) *exec.Cmd {
	if version == "" {
		version = "default"
	}
	full := append([]string{"-c", nvmActivate(version) + `exec "$@"`, "lerd-nvm", bin}, args...)
	return exec.Command("bash", full...)
}

func (nvmManager) ExecPrefix(version string) string {
	if version == "" {
		version = "default"
	}
	// A bash -c wrapper that sources nvm, activates the version (aborting if none
	// is available so it can't fork-bomb), then exec's the command supplied as
	// positional args ("$@"). "lerd-nvm" is $0.
	return fmt.Sprintf("bash -c %s lerd-nvm", shellQuote(nvmActivate(version)+`exec "$@"`))
}

func (nvmManager) ShimScript(lerdBin, bin string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
LERD="%s"
if [ -x "$LERD" ]; then
  exec "$LERD" %s "$@"
fi
export NVM_DIR=%s
[ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh"
VERSION=""
for f in .node-version .nvmrc; do
  [ -f "$f" ] && VERSION=$(tr -d '[:space:]' < "$f") && break
done
if [ -n "$VERSION" ]; then
  nvm install "$VERSION" >/dev/null 2>&1 || true
  nvm use "$VERSION" >/dev/null 2>&1
else
  nvm use default >/dev/null 2>&1
fi
if [ -z "$NVM_BIN" ]; then
  printf 'No Node.js version available via nvm. Run: lerd node:install 22\n' >&2
  exit 1
fi
PATH="$NVM_BIN:$PATH"
exec "$NVM_BIN/%s" "$@"
`, lerdBin, bin, shellQuote(nvmDir()), bin)
}

// nvmVersionRe matches a "vMAJOR.MINOR.PATCH" token on an installed-version line
// of `nvm ls --no-colors --no-alias` (e.g. "->     v24.16.0 *" or "  v18.20.0").
var nvmVersionRe = regexp.MustCompile(`\bv(\d+\.\d+\.\d+)\b`)

// parseNvmListFull extracts the full version strings (leading "v" stripped) from
// `nvm ls --no-colors --no-alias` output, in the order nvm reports them.
func parseNvmListFull(raw string) []string {
	var versions []string
	for _, line := range strings.Split(raw, "\n") {
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
