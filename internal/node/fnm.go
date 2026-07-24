package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// fnmManager drives Schniz/fnm, the single static binary lerd downloads to its
// bin dir at install time and invokes as `fnm exec --using=<v> -- <cmd>`.
type fnmManager struct{}

func (fnmManager) Name() string { return "fnm" }

// bin is the fnm binary lerd ships in its own bin dir.
func (fnmManager) bin() string { return filepath.Join(config.BinDir(), "fnm") }

func (m fnmManager) Available() bool {
	_, err := os.Stat(m.bin())
	return err == nil
}

// listFull returns every installed full version (v stripped) fnm reports.
func (m fnmManager) listFull() []string {
	out, err := exec.Command(m.bin(), "list").Output()
	if err != nil {
		return nil
	}
	return parseFnmListFull(string(out))
}

func (m fnmManager) List() []string {
	return dedupeMajors(m.listFull())
}

func (m fnmManager) Install(version string) error {
	if out, err := exec.Command(m.bin(), "install", version).CombinedOutput(); err != nil {
		return fmt.Errorf("fnm install %s: %s", version, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m fnmManager) Uninstall(version string) error {
	return uninstallVersions(version, m.listFull(), func(v string) error {
		if out, err := exec.Command(m.bin(), "uninstall", v).CombinedOutput(); err != nil {
			return fmt.Errorf("fnm uninstall %s: %s", v, strings.TrimSpace(string(out)))
		}
		return nil
	})
}

func (m fnmManager) SetDefault(version string) error {
	if out, err := exec.Command(m.bin(), "default", version).CombinedOutput(); err != nil {
		return fmt.Errorf("fnm default %s: %s", version, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m fnmManager) HasDefault() bool {
	return exec.Command(m.bin(), "exec", "--using=default", "--", "true").Run() == nil
}

func (m fnmManager) Command(version, bin string, args []string) *exec.Cmd {
	if version == "" {
		version = "default"
	}
	cmdArgs := append([]string{"exec", "--using=" + version, "--", bin}, args...)
	return exec.Command(m.bin(), cmdArgs...)
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

func (m fnmManager) ExecPrefix(version string) string {
	if version == "" {
		version = "default"
	}
	return fmt.Sprintf("'%s' exec --using=%s --", m.bin(), version)
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
