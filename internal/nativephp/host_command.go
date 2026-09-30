package nativephp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// HostPath is the PATH a command gets on the host under the native runtime:
// the project's composer binaries first, then lerd's shim dir, which is where
// php comes from.
func HostPath(dir string) string {
	return filepath.Join(dir, "vendor", "bin") + string(os.PathListSeparator) + config.PathWithBinDir()
}

// HostCommand builds the host process that stands in for a podman exec into
// the FPM container: argv runs in dir with HostPath, so a bare php is lerd's
// shim and runs the project's native build with its cli_ini.
func HostCommand(dir string, argv []string, extraEnv ...string) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	path := HostPath(dir)
	name := argv[0]
	if !strings.Contains(name, "/") {
		resolved, err := lookPathIn(name, path)
		if err != nil {
			return nil, err
		}
		name = resolved
	}
	c := exec.Command(name, argv[1:]...)
	// exec.Command resolves a bare name against lerd's own PATH rather than the
	// child's, so the lookup above uses the child's and this undoes exec's.
	c.Path = name
	c.Dir = dir
	c.Env = append(append(os.Environ(), "PATH="+path), extraEnv...)
	return c, nil
}

func lookPathIn(name, pathList string) (string, error) {
	for _, dir := range filepath.SplitList(pathList) {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found on the host PATH lerd uses (%s)", name, pathList)
}
